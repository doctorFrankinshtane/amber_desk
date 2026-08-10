package checklist

import (
	"strings"

	"amberdesk/pkg/connectors"
)

const Version = 1

func Default() connectors.ChecklistSnapshot {
	phase := func(id, key, title string, tasks ...connectors.ChecklistTask) connectors.ChecklistPhase {
		for i := range tasks {
			tasks[i].PhaseID = id
			tasks[i].Status = "pending"
		}
		return connectors.ChecklistPhase{ID: id, TitleKey: key, Title: title, Tasks: tasks}
	}
	task := func(id, key, title, action string) connectors.ChecklistTask {
		return connectors.ChecklistTask{ID: id, TitleKey: key, Title: title, Action: action}
	}
	return connectors.ChecklistSnapshot{Version: Version, Phases: []connectors.ChecklistPhase{
		phase("scope", "checklist.phase.scope", "Goal and boundaries",
			task("scope-question", "checklist.task.scopeQuestion", "Write the exact investigation question", ""),
			task("scope-subject", "checklist.task.scopeSubject", "Define the subject, time range, and geography", ""),
			task("scope-known", "checklist.task.scopeKnown", "Separate known facts from assumptions", ""),
			task("scope-ethics", "checklist.task.scopeEthics", "Record legal, ethical, and safety limits", ""),
			task("scope-stop", "checklist.task.scopeStop", "Define completion and stop conditions", "")),
		phase("seeds", "checklist.phase.seeds", "Seed data",
			task("seeds-identifiers", "checklist.task.seedIdentifiers", "Record every known identifier and its source", ""),
			task("seeds-variants", "checklist.task.seedVariants", "Add aliases and spelling variants", ""),
			task("seeds-photos", "checklist.task.seedPhotos", "Attach original profile images and files", "relationships"),
			task("seeds-entities", "checklist.task.seedEntities", "Add known people and organizations", "relationships"),
			task("seeds-hypotheses", "checklist.task.seedHypotheses", "Write initial hypotheses as unverified", "")),
		phase("plan", "checklist.phase.plan", "Search plan",
			task("plan-directions", "checklist.task.planDirections", "Choose the highest-value search directions", ""),
			task("plan-queries", "checklist.task.planQueries", "Prepare search queries and language variants", ""),
			task("plan-tools", "checklist.task.planTools", "Select appropriate tools and review limitations", "catalog"),
			task("plan-priority", "checklist.task.planPriority", "Prioritize volatile sources first", ""),
			task("plan-negative", "checklist.task.planNegative", "Record searches that produced no result", "timeline")),
		phase("collect", "checklist.phase.collect", "Collection and preservation",
			task("collect-original", "checklist.task.collectOriginal", "Save the original URL and distinguish reposts", "timeline"),
			task("collect-access", "checklist.task.collectAccess", "Record author and access time", "timeline"),
			task("collect-file", "checklist.task.collectFile", "Attach the original file or a faithful capture", "relationships"),
			task("collect-context", "checklist.task.collectContext", "Preserve surrounding text and context", "timeline"),
			task("collect-unaltered", "checklist.task.collectUnaltered", "Keep an unedited copy of media", "relationships"),
			task("collect-log", "checklist.task.collectLog", "Log the discovery in chronology", "timeline")),
		phase("verify", "checklist.phase.verify", "Verification",
			task("verify-source", "checklist.task.verifySource", "Assess the source identity and reliability", ""),
			task("verify-origin", "checklist.task.verifyOrigin", "Find the earliest available publication", "catalog"),
			task("verify-reverse", "checklist.task.verifyReverse", "Run reverse searches on images or frames", "catalog"),
			task("verify-metadata", "checklist.task.verifyMetadata", "Inspect metadata without treating it as proof", ""),
			task("verify-time", "checklist.task.verifyTime", "Verify date, time, and sequence", "timeline"),
			task("verify-place", "checklist.task.verifyPlace", "Verify location using independent landmarks", "map"),
			task("verify-corroborate", "checklist.task.verifyCorroborate", "Find independent corroboration", "")),
		phase("structure", "checklist.phase.structure", "Relationships, time, and geography",
			task("structure-nodes", "checklist.task.structureNodes", "Create cards for relevant entities and evidence", "relationships"),
			task("structure-edges", "checklist.task.structureEdges", "Connect confirmed and proposed relationships", "relationships"),
			task("structure-timeline", "checklist.task.structureTimeline", "Place key events in chronological order", "timeline"),
			task("structure-map", "checklist.task.structureMap", "Mark verified locations and movements", "map"),
			task("structure-conflicts", "checklist.task.structureConflicts", "Record contradictions instead of hiding them", "timeline")),
		phase("analyze", "checklist.phase.analyze", "Analysis and hypotheses",
			task("analyze-independent", "checklist.task.analyzeIndependent", "Separate independent sources from repetitions", ""),
			task("analyze-alternatives", "checklist.task.analyzeAlternatives", "Test at least one alternative explanation", ""),
			task("analyze-disconfirm", "checklist.task.analyzeDisconfirm", "Search for evidence that disproves the main theory", "catalog"),
			task("analyze-confidence", "checklist.task.analyzeConfidence", "Assign confidence to each conclusion", ""),
			task("analyze-gaps", "checklist.task.analyzeGaps", "List unknowns and evidence gaps", "")),
		phase("report", "checklist.phase.report", "Findings and review",
			task("report-findings", "checklist.task.reportFindings", "Write findings linked to supporting evidence", "timeline"),
			task("report-caveats", "checklist.task.reportCaveats", "State limitations and unresolved contradictions", ""),
			task("report-privacy", "checklist.task.reportPrivacy", "Remove unnecessary personal and sensitive data", ""),
			task("report-reproduce", "checklist.task.reportReproduce", "Check that another analyst can reproduce the path", ""),
			task("report-next", "checklist.task.reportNext", "Define next questions or close the investigation", "")),
	}}
}

func Merge(stored connectors.ChecklistSnapshot) connectors.ChecklistSnapshot {
	base := Default()
	if len(stored.Phases) == 0 {
		return base
	}
	existing := map[string]connectors.ChecklistTask{}
	custom := map[string][]connectors.ChecklistTask{}
	for _, phase := range stored.Phases {
		for _, item := range phase.Tasks {
			if item.Custom {
				custom[item.PhaseID] = append(custom[item.PhaseID], item)
			} else {
				existing[item.ID] = item
			}
		}
	}
	for pi := range base.Phases {
		for ti := range base.Phases[pi].Tasks {
			seed := base.Phases[pi].Tasks[ti]
			if item, ok := existing[seed.ID]; ok {
				item.TitleKey, item.Action, item.PhaseID, item.Custom = seed.TitleKey, seed.Action, seed.PhaseID, false
				if !item.Edited {
					item.Title = seed.Title
				}
				base.Phases[pi].Tasks[ti] = item
			}
		}
		base.Phases[pi].Tasks = append(base.Phases[pi].Tasks, custom[base.Phases[pi].ID]...)
	}
	base.RecommendedTaskID = stored.RecommendedTaskID
	return base
}

func Find(snapshot *connectors.ChecklistSnapshot, id string) (*connectors.ChecklistTask, bool) {
	for pi := range snapshot.Phases {
		for ti := range snapshot.Phases[pi].Tasks {
			if snapshot.Phases[pi].Tasks[ti].ID == id {
				return &snapshot.Phases[pi].Tasks[ti], true
			}
		}
	}
	return nil, false
}

func HasPhase(snapshot connectors.ChecklistSnapshot, id string) bool {
	for _, phase := range snapshot.Phases {
		if phase.ID == id {
			return true
		}
	}
	return false
}
func ValidStatus(value string) bool {
	return value == "pending" || value == "done" || value == "skipped"
}
func CleanText(value string, limit int) string {
	value = strings.TrimSpace(value)
	r := []rune(value)
	if len(r) > limit {
		value = string(r[:limit])
	}
	return value
}
