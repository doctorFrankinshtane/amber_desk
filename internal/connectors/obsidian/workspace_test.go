package obsidian_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestTimelineRejectsOversizedText(t *testing.T) {
	connector, err := obsidian.New(obsidian.Config{VaultPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "CASE-LIMITS01", CaseName: "LIMITS"}
	_, err = connector.CreateTimelineEvent(context.Background(), ref, connectors.TimelineEvent{Title: strings.Repeat("я", connectors.MaxTimelineTitleRunes+1), Confidence: 50})
	if err == nil || !strings.Contains(err.Error(), "title exceeds") {
		t.Fatalf("oversized title error = %v", err)
	}
}

func TestTimelineIgnoresUnsupportedDocumentSchema(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "CASE-SCHEMA01", CaseName: "SCHEMA"}
	created, err := connector.CreateTimelineEvent(context.Background(), ref, connectors.TimelineEvent{ID: "EV-SCHEMA", Title: "Versioned note", Confidence: 50})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vault, "Amber Desk", "Cases", ref.CaseID, "Timeline", created.ID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "version: 1", "version: 2", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	events, err := connector.ListTimeline(context.Background(), ref)
	if err != nil || len(events) != 0 {
		t.Fatalf("unsupported schema must be ignored: %v %+v", err, events)
	}
}

func TestCustomDossierDirectoryUsesOneWorkspaceRoot(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault, DossierDir: filepath.Join("Investigations", "Case Notes")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const caseID = "CASE-ABCD1234"
	ref := connectors.DossierRef{CaseID: caseID, CaseName: "CUSTOM PATH", SubjectName: "SUBJECT"}
	if _, err := connector.WriteDossier(ctx, ref, connectors.DossierWrite{Content: "# Custom dossier\n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := connector.CreateTimelineEvent(ctx, ref, connectors.TimelineEvent{ID: "EV-CUSTOM", Title: "Custom root event", Confidence: 50}); err != nil {
		t.Fatal(err)
	}
	summary := connectors.CaseSummary{ID: caseID, Name: ref.CaseName, Subject: ref.SubjectName, Status: "active", UpdatedAt: "2026-08-11T10:00:00Z"}
	if err := connector.WriteCase(ctx, summary, []byte(`{"id":"CASE-ABCD1234"}`)); err != nil {
		t.Fatal(err)
	}
	if err := connector.SetActiveCase(ctx, caseID); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		filepath.Join(vault, "Investigations", "Case Notes", caseID+".md"),
		filepath.Join(vault, "Investigations", "Cases", caseID, "Timeline", "EV-CUSTOM.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected custom workspace file %s: %v", path, err)
		}
	}
	if _, err := connector.TrashCase(ctx, caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vault, "Investigations", "Case Notes", caseID+".md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dossier was not moved to trash: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(vault, "Investigations", ".trash", caseID+"-*", "Dossiers", caseID+".md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("trashed custom dossier: %v %v", err, matches)
	}
}

func TestConcurrentTimelineNotesDoNotOverwriteEachOther(t *testing.T) {
	connector, err := obsidian.New(obsidian.Config{VaultPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR"}
	seed := connectors.TimelineEvent{ID: "EV-CONCURRENT", OccurredAt: "2026-08-10T10:00:00Z", Title: "Shared event", Status: "pending", Confidence: 80}
	if _, err := connector.CreateTimelineEvent(context.Background(), ref, seed); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for _, text := range []string{"First independent source", "Second independent source"} {
		workers.Add(1)
		go func(text string) {
			defer workers.Done()
			<-start
			_, noteErr := connector.AddTimelineNote(context.Background(), ref, seed.ID, connectors.TimelineNote{Text: text, CreatedAt: "2026-08-10T11:00:00Z"})
			errors <- noteErr
		}(text)
	}
	close(start)
	workers.Wait()
	close(errors)
	for noteErr := range errors {
		if noteErr != nil {
			t.Fatal(noteErr)
		}
	}

	events, err := connector.ListTimeline(context.Background(), ref)
	if err != nil || len(events) != 1 || len(events[0].Notes) != 2 {
		t.Fatalf("concurrent notes: %v %+v", err, events)
	}
}

func TestChecklistPersistsAsReadableMarkdown(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "CASE-CHECK", CaseName: "CHECKLIST"}
	snapshot := connectors.ChecklistSnapshot{Version: 1, RecommendedTaskID: "TASK-1", Phases: []connectors.ChecklistPhase{{ID: "scope", Title: "Goal and boundaries", TitleKey: "checklist.phase.scope", Tasks: []connectors.ChecklistTask{{ID: "TASK-1", PhaseID: "scope", Title: "Define the question", Status: "done"}}}}}
	written, err := connector.WriteChecklist(context.Background(), ref, snapshot)
	if err != nil || written.Backend != "obsidian" {
		t.Fatalf("write checklist: %v %+v", err, written)
	}
	reopened, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.ReadChecklist(context.Background(), ref)
	if err != nil || restored.RecommendedTaskID != "TASK-1" || restored.Phases[0].Tasks[0].Status != "done" {
		t.Fatalf("read checklist: %v %+v", err, restored)
	}
	data, err := os.ReadFile(filepath.Join(vault, "Amber Desk", "Cases", "CASE-CHECK", "Checklist.md"))
	if err != nil || !strings.Contains(string(data), "kind: investigation_checklist") || !strings.Contains(string(data), "- [x] Define the question") {
		t.Fatalf("checklist markdown: %v %q", err, data)
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

func TestRelationshipAttachmentRoundTripAndTrash(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ref := connectors.DossierRef{CaseID: "CASE-ATTACH", CaseName: "ATTACHMENTS"}
	if _, err := connector.CreateRelationshipNode(ctx, ref, connectors.RelationshipNode{ID: "NODE-1", Type: "evidence", Title: "Receipt", X: .5, Y: .5}); err != nil {
		t.Fatal(err)
	}
	content := []byte("local evidence bytes\n")
	stored, err := connector.StoreRelationshipAttachment(ctx, ref, connectors.RelationshipAttachment{ID: "ATT-1", NodeID: "NODE-1", Filename: "receipt.txt", MediaType: "text/plain"}, content)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if stored.Size != int64(len(content)) || stored.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("attachment metadata: %+v", stored)
	}
	items, err := connector.ListRelationshipAttachments(ctx, ref, "NODE-1")
	if err != nil || len(items) != 1 || items[0].Filename != "receipt.txt" {
		t.Fatalf("attachment list: %v %+v", err, items)
	}
	metadata, restored, err := connector.ReadRelationshipAttachment(ctx, ref, "NODE-1", "ATT-1")
	if err != nil || metadata.SHA256 != stored.SHA256 || string(restored) != string(content) {
		t.Fatalf("attachment read: %v %+v %q", err, metadata, restored)
	}
	snapshot, err := connector.ListRelationships(ctx, ref)
	if err != nil || snapshot.Nodes[0].AttachmentCount != 1 {
		t.Fatalf("attachment count: %v %+v", err, snapshot)
	}
	coverNode := snapshot.Nodes[0]
	coverNode.CoverAttachmentID = stored.ID
	if _, err := connector.UpdateRelationshipNode(ctx, ref, coverNode); err != nil {
		t.Fatal(err)
	}
	reopened, err := obsidian.New(obsidian.Config{VaultPath: vault})
	if err != nil {
		t.Fatal(err)
	}
	reopenedSnapshot, err := reopened.ListRelationships(ctx, ref)
	if err != nil || reopenedSnapshot.Nodes[0].CoverAttachmentID != stored.ID {
		t.Fatalf("cover round trip: %v %+v", err, reopenedSnapshot)
	}
	if _, err := connector.StoreRelationshipAttachment(ctx, ref, connectors.RelationshipAttachment{ID: "ATT-2", NodeID: "NODE-1", Filename: "../escape.txt"}, content); !errors.Is(err, connectors.ErrInvalidFilename) {
		t.Fatalf("traversal filename = %v", err)
	}
	if err := connector.DeleteRelationshipAttachment(ctx, ref, "NODE-1", "ATT-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connector.ReadRelationshipAttachment(ctx, ref, "NODE-1", "ATT-1"); !errors.Is(err, connectors.ErrEntityAbsent) {
		t.Fatalf("deleted attachment read = %v", err)
	}
	trash, _ := filepath.Glob(filepath.Join(vault, "Amber Desk", "Cases", "CASE-ATTACH", ".trash", "Attachments", "NODE-1", "*-ATT-1"))
	if len(trash) != 1 {
		t.Fatalf("attachment trash = %v", trash)
	}
}
