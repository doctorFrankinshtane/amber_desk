package obsidian_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amberdesk/internal/connectors/obsidian"
	"amberdesk/pkg/connectors"
)

func TestCaseStoreMigratesLegacyStateAndRestoresIndex(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"id":"CASE-1234ABCD","name":"Legacy case","status":"active","updatedAt":"2026-08-10T10:00:00Z","subject":{"codename":"ALPHA"},"events":[],"tags":[]}`)
	if err := connector.WriteWorkspaceState(context.Background(), "active-case", legacy); err != nil {
		t.Fatal(err)
	}
	items, err := connector.ListCases(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "CASE-1234ABCD" || !items[0].Active {
		t.Fatalf("migrated cases: %v %+v", err, items)
	}
	stored, err := connector.ReadCase(context.Background(), items[0].ID)
	if err != nil || !json.Valid(stored) {
		t.Fatalf("migrated snapshot: %v %s", err, stored)
	}
	if _, err := os.Stat(filepath.Join(vault, "Amber Desk", ".state", "active-case.legacy.json")); err != nil {
		t.Fatalf("legacy state was not retired: %v", err)
	}
	reopened, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	activeID, err := reopened.ActiveCaseID(context.Background())
	if err != nil || activeID != "CASE-1234ABCD" {
		t.Fatalf("restored active case: %q %v", activeID, err)
	}
}

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

	path := filepath.Join(vault, "Amber Desk", "Cases", "NS-04", "Timeline", "EV-1.md")
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

func TestRelationshipsPersistAsMarkdownAndCascade(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ref := connectors.DossierRef{CaseID: "CASE-001", CaseName: "ORION"}
	primary, err := connector.CreateRelationshipNode(ctx, ref, connectors.RelationshipNode{ID: "NODE-1", Type: "object", Title: "ORION", Primary: true, X: .5, Y: .5, SourceIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	related, err := connector.CreateRelationshipNode(ctx, ref, connectors.RelationshipNode{ID: "NODE-2", Type: "organization", Title: "VECTOR LLC", X: .2, Y: .2, SourceIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.CreateRelationshipEdge(ctx, ref, connectors.RelationshipEdge{ID: "REL-1", SourceID: primary.ID, TargetID: related.ID, Label: "CONTROLS", Confidence: 82, Kind: "critical", SourceIDs: []string{"EV-14"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := connector.ListRelationships(ctx, ref)
	if err != nil || len(snapshot.Nodes) != 2 || len(snapshot.Edges) != 1 {
		t.Fatalf("relationship snapshot: %v %+v", err, snapshot)
	}
	path := filepath.Join(vault, "Amber Desk", "Cases", "CASE-001", "Relations", "Edges", "REL-1.md")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "kind: relationship_edge") || !strings.Contains(string(data), "CONTROLS") {
		t.Fatalf("relationship markdown: %v %q", err, data)
	}
	if err := connector.DeleteRelationshipNode(ctx, ref, related.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = connector.ListRelationships(ctx, ref)
	if err != nil || len(snapshot.Nodes) != 1 || len(snapshot.Edges) != 0 {
		t.Fatalf("cascade relationship delete: %v %+v", err, snapshot)
	}
}
