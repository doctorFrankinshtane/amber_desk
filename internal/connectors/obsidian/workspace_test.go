package obsidian_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amberdesk/internal/connectors/obsidian"
	"amberdesk/pkg/connectors"
)

func TestTimelinePersistsAsMarkdownNotes(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR"}
	seed := connectors.TimelineEvent{ID: "EV-1", OccurredAt: "2026-08-10T10:00:00Z", Title: "Initial sighting", Type: "identity", Status: "pending", Confidence: 80}

	events, err := connector.BootstrapTimeline(context.Background(), ref, []connectors.TimelineEvent{seed})
	if err != nil || len(events) != 1 {
		t.Fatalf("bootstrap timeline: %v %+v", err, events)
	}
	updated, err := connector.SetTimelineStatus(context.Background(), ref, seed.ID, "verified")
	if err != nil || updated.Status != "verified" {
		t.Fatalf("set status: %v %+v", err, updated)
	}
	updated, err = connector.AddTimelineNote(context.Background(), ref, seed.ID, connectors.TimelineNote{Text: "Confirmed", CreatedAt: "2026-08-10T11:00:00Z"})
	if err != nil || len(updated.Notes) != 1 {
		t.Fatalf("add note: %v %+v", err, updated)
	}

	path := filepath.Join(vault, "Amber Desk", "Cases", "NS-04-NORTHSTAR", "Timeline", "EV-1.md")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "kind: timeline_event") || !strings.Contains(string(data), "Confirmed") {
		t.Fatalf("timeline markdown = %v %q", err, data)
	}
}

func TestMapPersistsMarkersAndRoutes(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ref := connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR"}
	first, err := connector.CreateMapMarker(ctx, ref, connectors.MapMarker{Label: "Yekaterinburg", Latitude: 56.8389, Longitude: 60.6057})
	if err != nil {
		t.Fatal(err)
	}
	second, err := connector.CreateMapMarker(ctx, ref, connectors.MapMarker{Label: "Warsaw", Latitude: 52.2297, Longitude: 21.0122})
	if err != nil {
		t.Fatal(err)
	}
	route, err := connector.CreateMapRoute(ctx, ref, connectors.MapRoute{FromMarkerID: first.ID, ToMarkerID: second.ID, Label: "Observed movement"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := connector.ListMap(ctx, ref)
	if err != nil || len(snapshot.Markers) != 2 || len(snapshot.Routes) != 1 || snapshot.Backend != "obsidian" {
		t.Fatalf("map snapshot: %v %+v", err, snapshot)
	}
	if err := connector.DeleteMapMarker(ctx, ref, first.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = connector.ListMap(ctx, ref)
	if err != nil || len(snapshot.Markers) != 1 || len(snapshot.Routes) != 0 {
		t.Fatalf("map after cascading delete: %v %+v (route %s)", err, snapshot, route.ID)
	}
}
