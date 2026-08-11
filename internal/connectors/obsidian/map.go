package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"amberdesk/pkg/connectors"
)

type markerDocument struct {
	AmberDesk            documentHeader `yaml:"amber_desk"`
	connectors.MapMarker `yaml:",inline"`
}

type routeDocument struct {
	AmberDesk           documentHeader `yaml:"amber_desk"`
	connectors.MapRoute `yaml:",inline"`
}

func (c *Connector) ListMap(_ context.Context, ref connectors.DossierRef) (connectors.MapSnapshot, error) {
	markers, err := c.listMarkers(ref)
	if err != nil {
		return connectors.MapSnapshot{}, err
	}
	routes, err := c.listRoutes(ref)
	if err != nil {
		return connectors.MapSnapshot{}, err
	}
	return connectors.MapSnapshot{Markers: markers, Routes: routes, Backend: ID}, nil
}

func (c *Connector) CreateMapMarker(_ context.Context, ref connectors.DossierRef, marker connectors.MapMarker) (connectors.MapMarker, error) {
	now := time.Now().UTC()
	if marker.ID == "" {
		var err error
		marker.ID, err = documentID("MK")
		if err != nil {
			return connectors.MapMarker{}, err
		}
	}
	if marker.OccurredAt == "" {
		marker.OccurredAt = now.Format(time.RFC3339)
	}
	if err := validateMarker(marker); err != nil {
		return connectors.MapMarker{}, err
	}
	path, err := c.mapPath(ref, "Markers", marker.ID, true)
	if err != nil {
		return connectors.MapMarker{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return connectors.MapMarker{}, connectors.ErrConflict
	}
	if err := c.writeMarker(ref, marker); err != nil {
		return connectors.MapMarker{}, err
	}
	return marker, nil
}

func (c *Connector) UpdateMapMarker(_ context.Context, ref connectors.DossierRef, marker connectors.MapMarker) (connectors.MapMarker, error) {
	if err := validateMarker(marker); err != nil {
		return connectors.MapMarker{}, err
	}
	path, err := c.mapPath(ref, "Markers", marker.ID, false)
	if err != nil {
		return connectors.MapMarker{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return connectors.MapMarker{}, connectors.ErrEntityAbsent
	}
	if err := c.writeMarker(ref, marker); err != nil {
		return connectors.MapMarker{}, err
	}
	return marker, nil
}

func (c *Connector) DeleteMapMarker(ctx context.Context, ref connectors.DossierRef, markerID string) error {
	path, err := c.mapPath(ref, "Markers", markerID, false)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return connectors.ErrEntityAbsent
	} else if err != nil {
		return fmt.Errorf("delete map marker: %w", err)
	}
	snapshot, err := c.ListMap(ctx, ref)
	if err != nil {
		return err
	}
	for _, route := range snapshot.Routes {
		if route.FromMarkerID == markerID || route.ToMarkerID == markerID {
			if err := c.DeleteMapRoute(ctx, ref, route.ID); err != nil && !errors.Is(err, connectors.ErrEntityAbsent) {
				return err
			}
		}
	}
	return nil
}

func (c *Connector) CreateMapRoute(ctx context.Context, ref connectors.DossierRef, route connectors.MapRoute) (connectors.MapRoute, error) {
	if route.FromMarkerID == "" || route.ToMarkerID == "" || route.FromMarkerID == route.ToMarkerID {
		return connectors.MapRoute{}, errors.New("route requires two different markers")
	}
	snapshot, err := c.ListMap(ctx, ref)
	if err != nil {
		return connectors.MapRoute{}, err
	}
	foundFrom, foundTo := false, false
	fromLabel, toLabel := route.FromMarkerID, route.ToMarkerID
	for _, marker := range snapshot.Markers {
		if marker.ID == route.FromMarkerID {
			foundFrom, fromLabel = true, marker.Label
		}
		if marker.ID == route.ToMarkerID {
			foundTo, toLabel = true, marker.Label
		}
	}
	if !foundFrom || !foundTo {
		return connectors.MapRoute{}, errors.New("route marker does not exist")
	}
	if strings.TrimSpace(route.Label) == "" {
		route.Label = fromLabel + " -> " + toLabel
	}
	if route.ID == "" {
		route.ID, err = documentID("RT")
		if err != nil {
			return connectors.MapRoute{}, err
		}
	}
	path, err := c.mapPath(ref, "Routes", route.ID, true)
	if err != nil {
		return connectors.MapRoute{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return connectors.MapRoute{}, connectors.ErrConflict
	}
	document := routeDocument{AmberDesk: newDocumentHeader(documentKindMapRoute, ref.CaseID), MapRoute: route}
	data, err := marshalNote(document, "# "+route.Label+"\n\nMovement route from [["+route.FromMarkerID+"]] to [["+route.ToMarkerID+"]].")
	if err != nil {
		return connectors.MapRoute{}, err
	}
	if err := atomicWrite(path, data); err != nil {
		return connectors.MapRoute{}, err
	}
	return route, nil
}

func (c *Connector) DeleteMapRoute(_ context.Context, ref connectors.DossierRef, routeID string) error {
	path, err := c.mapPath(ref, "Routes", routeID, false)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return connectors.ErrEntityAbsent
	} else if err != nil {
		return fmt.Errorf("delete map route: %w", err)
	}
	return nil
}

func (c *Connector) listMarkers(ref connectors.DossierRef) ([]connectors.MapMarker, error) {
	documents, err := c.readMapDocuments(ref, "Markers")
	if err != nil {
		return nil, err
	}
	markers := make([]connectors.MapMarker, 0, len(documents))
	for _, data := range documents {
		var document markerDocument
		if err := unmarshalNote(data, &document); err != nil {
			return nil, err
		}
		if document.AmberDesk.valid(documentKindMapMarker, ref.CaseID) {
			markers = append(markers, document.MapMarker)
		}
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].OccurredAt < markers[j].OccurredAt })
	return markers, nil
}

func (c *Connector) listRoutes(ref connectors.DossierRef) ([]connectors.MapRoute, error) {
	documents, err := c.readMapDocuments(ref, "Routes")
	if err != nil {
		return nil, err
	}
	routes := make([]connectors.MapRoute, 0, len(documents))
	for _, data := range documents {
		var document routeDocument
		if err := unmarshalNote(data, &document); err != nil {
			return nil, err
		}
		if document.AmberDesk.valid(documentKindMapRoute, ref.CaseID) {
			routes = append(routes, document.MapRoute)
		}
	}
	return routes, nil
}

func (c *Connector) readMapDocuments(ref connectors.DossierRef, kind string) ([][]byte, error) {
	directory, err := c.caseDirectory(ref, false, "Map", kind)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return [][]byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read map directory: %w", err)
	}
	documents := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		documents = append(documents, data)
	}
	return documents, nil
}

func (c *Connector) writeMarker(ref connectors.DossierRef, marker connectors.MapMarker) error {
	path, err := c.mapPath(ref, "Markers", marker.ID, true)
	if err != nil {
		return err
	}
	document := markerDocument{AmberDesk: newDocumentHeader(documentKindMapMarker, ref.CaseID), MapMarker: marker}
	body := fmt.Sprintf("# %s\n\n%s\n\nCoordinates: `%.6f, %.6f`", marker.Label, marker.Description, marker.Latitude, marker.Longitude)
	data, err := marshalNote(document, body)
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (c *Connector) mapPath(ref connectors.DossierRef, kind, id string, create bool) (string, error) {
	directory, err := c.caseDirectory(ref, create, "Map", kind)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, sanitize(id)+".md")
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}

func validateMarker(marker connectors.MapMarker) error {
	if strings.TrimSpace(marker.ID) == "" || strings.TrimSpace(marker.Label) == "" {
		return errors.New("marker id and label are required")
	}
	if marker.Latitude < -90 || marker.Latitude > 90 || marker.Longitude < -180 || marker.Longitude > 180 {
		return errors.New("marker coordinates are outside valid bounds")
	}
	return nil
}
