package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"amberdesk/internal/casefile"
	"amberdesk/pkg/connectors"
)

func (h *Handler) listTimeline(w http.ResponseWriter, r *http.Request) {
	if connector, ok := h.activeTimelineConnector(r.Context()); ok {
		events, err := connector.ListTimeline(r.Context(), h.dossierRef())
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, connectors.TimelineSnapshot{Events: events, Backend: connector.Metadata().ID})
		return
	}
	snapshot := h.store.Snapshot()
	writeJSON(w, http.StatusOK, connectors.TimelineSnapshot{Events: caseEventsToTimeline(snapshot.Events), Backend: "memory"})
}

func (h *Handler) bootstrapTimeline(w http.ResponseWriter, r *http.Request) {
	connector, ok := h.activeTimelineConnector(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "no connected timeline connector")
		return
	}
	snapshot := h.store.Snapshot()
	events, err := connector.BootstrapTimeline(r.Context(), h.dossierRef(), caseEventsToTimeline(snapshot.Events))
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, connectors.TimelineSnapshot{Events: events, Backend: connector.Metadata().ID})
}

func (h *Handler) createTimelineEvent(w http.ResponseWriter, r *http.Request) {
	var event connectors.TimelineEvent
	if err := decodeJSON(r, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid timeline event")
		return
	}
	if connector, ok := h.activeTimelineConnector(r.Context()); ok {
		created, err := connector.CreateTimelineEvent(r.Context(), h.dossierRef(), event)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	if strings.TrimSpace(event.Title) == "" || event.Confidence < 0 || event.Confidence > 100 {
		writeError(w, http.StatusBadRequest, "timeline title and confidence from 0 to 100 are required")
		return
	}
	now := time.Now().UTC()
	if event.ID == "" {
		event.ID = newID("EV")
	}
	if event.OccurredAt == "" {
		event.OccurredAt = now.Format(time.RFC3339)
	}
	if event.Date == "" {
		event.Date = strings.ToUpper(now.Format("02 Jan"))
	}
	if event.Time == "" {
		event.Time = now.Format("15:04")
	}
	if event.Status == "" {
		event.Status = "pending"
	}
	created, err := h.store.AddEvent(timelineToCaseEvent(event))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, caseEventToTimeline(created, 0))
}

func (h *Handler) listMap(w http.ResponseWriter, r *http.Request) {
	if connector, ok := h.activeMapConnector(r.Context()); ok {
		snapshot, err := connector.ListMap(r.Context(), h.dossierRef())
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	h.mapMu.Lock()
	defer h.mapMu.Unlock()
	writeJSON(w, http.StatusOK, connectors.MapSnapshot{Markers: append([]connectors.MapMarker(nil), h.markers...), Routes: append([]connectors.MapRoute(nil), h.routes...), Backend: "memory"})
}

func (h *Handler) createMapMarker(w http.ResponseWriter, r *http.Request) {
	var marker connectors.MapMarker
	if err := decodeJSON(r, &marker); err != nil || !validMarker(marker, false) {
		writeError(w, http.StatusBadRequest, "valid marker label and coordinates are required")
		return
	}
	if connector, ok := h.activeMapConnector(r.Context()); ok {
		created, err := connector.CreateMapMarker(r.Context(), h.dossierRef(), marker)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	marker.ID = newID("MK")
	if marker.OccurredAt == "" {
		marker.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	h.mapMu.Lock()
	h.markers = append(h.markers, marker)
	h.mapMu.Unlock()
	writeJSON(w, http.StatusCreated, marker)
}

func (h *Handler) updateMapMarker(w http.ResponseWriter, r *http.Request) {
	var marker connectors.MapMarker
	if err := decodeJSON(r, &marker); err != nil {
		writeError(w, http.StatusBadRequest, "invalid marker")
		return
	}
	marker.ID = r.PathValue("id")
	if !validMarker(marker, true) {
		writeError(w, http.StatusBadRequest, "valid marker label and coordinates are required")
		return
	}
	if connector, ok := h.activeMapConnector(r.Context()); ok {
		updated, err := connector.UpdateMapMarker(r.Context(), h.dossierRef(), marker)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	h.mapMu.Lock()
	defer h.mapMu.Unlock()
	for i := range h.markers {
		if h.markers[i].ID == marker.ID {
			h.markers[i] = marker
			writeJSON(w, http.StatusOK, marker)
			return
		}
	}
	writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
}

func (h *Handler) deleteMapMarker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if connector, ok := h.activeMapConnector(r.Context()); ok {
		if err := connector.DeleteMapMarker(r.Context(), h.dossierRef(), id); err != nil {
			writeConnectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.mapMu.Lock()
	defer h.mapMu.Unlock()
	removed := false
	for i := len(h.markers) - 1; i >= 0; i-- {
		if h.markers[i].ID == id {
			h.markers = append(h.markers[:i], h.markers[i+1:]...)
			removed = true
		}
	}
	for i := len(h.routes) - 1; i >= 0; i-- {
		if h.routes[i].FromMarkerID == id || h.routes[i].ToMarkerID == id {
			h.routes = append(h.routes[:i], h.routes[i+1:]...)
		}
	}
	if !removed {
		writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createMapRoute(w http.ResponseWriter, r *http.Request) {
	var route connectors.MapRoute
	if err := decodeJSON(r, &route); err != nil || route.FromMarkerID == "" || route.ToMarkerID == "" || route.FromMarkerID == route.ToMarkerID {
		writeError(w, http.StatusBadRequest, "route requires two different markers")
		return
	}
	if connector, ok := h.activeMapConnector(r.Context()); ok {
		created, err := connector.CreateMapRoute(r.Context(), h.dossierRef(), route)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	h.mapMu.Lock()
	defer h.mapMu.Unlock()
	foundFrom, foundTo := false, false
	for _, marker := range h.markers {
		foundFrom = foundFrom || marker.ID == route.FromMarkerID
		foundTo = foundTo || marker.ID == route.ToMarkerID
	}
	if !foundFrom || !foundTo {
		writeError(w, http.StatusBadRequest, "route marker does not exist")
		return
	}
	route.ID = newID("RT")
	h.routes = append(h.routes, route)
	writeJSON(w, http.StatusCreated, route)
}

func (h *Handler) deleteMapRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if connector, ok := h.activeMapConnector(r.Context()); ok {
		if err := connector.DeleteMapRoute(r.Context(), h.dossierRef(), id); err != nil {
			writeConnectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.mapMu.Lock()
	defer h.mapMu.Unlock()
	for i := range h.routes {
		if h.routes[i].ID == id {
			h.routes = append(h.routes[:i], h.routes[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
}

func (h *Handler) activeTimelineConnector(ctx context.Context) (connectors.TimelineConnector, bool) {
	for _, info := range h.connectors.List(ctx) {
		if info.Status.State != "connected" || !hasCapability(info.Metadata.Capabilities, "timeline.read") {
			continue
		}
		connector, err := h.connectors.Get(info.Metadata.ID)
		if err == nil {
			timeline, ok := connector.(connectors.TimelineConnector)
			return timeline, ok
		}
	}
	return nil, false
}

func (h *Handler) activeMapConnector(ctx context.Context) (connectors.MapConnector, bool) {
	for _, info := range h.connectors.List(ctx) {
		if info.Status.State != "connected" || !hasCapability(info.Metadata.Capabilities, "map.read") {
			continue
		}
		connector, err := h.connectors.Get(info.Metadata.ID)
		if err == nil {
			geo, ok := connector.(connectors.MapConnector)
			return geo, ok
		}
	}
	return nil, false
}

func hasCapability(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func caseEventsToTimeline(events []casefile.Event) []connectors.TimelineEvent {
	result := make([]connectors.TimelineEvent, len(events))
	for i, event := range events {
		result[i] = caseEventToTimeline(event, i)
	}
	return result
}

func caseEventToTimeline(event casefile.Event, index int) connectors.TimelineEvent {
	notes := make([]connectors.TimelineNote, len(event.Notes))
	for i, note := range event.Notes {
		notes[i] = connectors.TimelineNote{Text: note.Text, CreatedAt: note.CreatedAt}
	}
	occurredAt := event.OccurredAt
	if occurredAt == "" {
		occurredAt = legacyOccurredAt(event, index)
	}
	return connectors.TimelineEvent{ID: event.ID, OccurredAt: occurredAt, Time: event.Time, Date: event.Date, Type: event.Type, Title: event.Title, Summary: event.Summary, Source: event.Source, SourceURL: event.SourceURL, Confidence: event.Confidence, Status: event.Status, Fingerprint: event.Fingerprint, Indicators: append([]string(nil), event.Indicators...), Notes: notes}
}

func legacyOccurredAt(event casefile.Event, index int) string {
	parsed, err := time.Parse("02 Jan 15:04", strings.TrimSpace(event.Date+" "+event.Time))
	if err == nil {
		return time.Date(1970, parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.UTC).Format(time.RFC3339)
	}
	return time.Unix(int64(index), 0).UTC().Format(time.RFC3339)
}

func timelineToCaseEvent(event connectors.TimelineEvent) casefile.Event {
	notes := make([]casefile.Note, len(event.Notes))
	for i, note := range event.Notes {
		notes[i] = casefile.Note{Text: note.Text, CreatedAt: note.CreatedAt}
	}
	return casefile.Event{ID: event.ID, OccurredAt: event.OccurredAt, Time: event.Time, Date: event.Date, Type: event.Type, Title: event.Title, Summary: event.Summary, Source: event.Source, SourceURL: event.SourceURL, Confidence: event.Confidence, Status: event.Status, Fingerprint: event.Fingerprint, Indicators: append([]string(nil), event.Indicators...), Notes: notes}
}

func validMarker(marker connectors.MapMarker, requireID bool) bool {
	return (!requireID || marker.ID != "") && strings.TrimSpace(marker.Label) != "" && marker.Latitude >= -90 && marker.Latitude <= 90 && marker.Longitude >= -180 && marker.Longitude <= 180
}

func newID(prefix string) string {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err == nil {
		return prefix + "-" + hex.EncodeToString(random)
	}
	return prefix + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}
