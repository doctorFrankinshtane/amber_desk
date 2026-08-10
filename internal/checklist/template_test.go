package checklist_test

import (
	"testing"

	"amberdesk/internal/checklist"
	"amberdesk/pkg/connectors"
)

func TestMergeAddsTemplateTasksAndPreservesStateAndCustomTasks(t *testing.T) {
	stored := connectors.ChecklistSnapshot{Version: 0, RecommendedTaskID: "CUSTOM-1", Phases: []connectors.ChecklistPhase{{ID: "scope", Tasks: []connectors.ChecklistTask{
		{ID: "scope-question", PhaseID: "scope", Title: "My investigation question", Edited: true, Status: "done"},
		{ID: "CUSTOM-1", PhaseID: "scope", Title: "Check the local archive", Status: "pending", Custom: true},
	}}}}
	merged := checklist.Merge(stored)
	if merged.Version != checklist.Version || len(merged.Phases) != 8 || merged.RecommendedTaskID != "CUSTOM-1" {
		t.Fatalf("merged snapshot: %+v", merged)
	}
	first := merged.Phases[0].Tasks[0]
	if first.Status != "done" || first.Title != "My investigation question" || !first.Edited {
		t.Fatalf("stored task state was lost: %+v", first)
	}
	custom := merged.Phases[0].Tasks[len(merged.Phases[0].Tasks)-1]
	if custom.ID != "CUSTOM-1" || !custom.Custom {
		t.Fatalf("custom task was lost: %+v", custom)
	}
	if len(merged.Phases[0].Tasks) <= len(stored.Phases[0].Tasks) {
		t.Fatal("missing built-in tasks were not restored")
	}
}
