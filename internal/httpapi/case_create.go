package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"amberdesk/internal/casefile"
	"amberdesk/pkg/connectors"
)

type caseSyncStatus struct {
	State   string `json:"state"`
	Backend string `json:"backend"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
}

type createCaseResponse struct {
	Case          casefile.Case                   `json:"case"`
	Relationships connectors.RelationshipSnapshot `json:"relationships"`
	Sync          caseSyncStatus                  `json:"sync"`
}

func (h *Handler) createCase(w http.ResponseWriter, r *http.Request) {
	h.caseMu.Lock()
	defer h.caseMu.Unlock()
	var input casefile.Case
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid dossier")
		return
	}
	caseData, err := normalizeCase(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	caseData.ID = strings.ToUpper(newID("CASE")[:13])
	previous := h.store.Snapshot()
	nodes, edges := seedRelationships(caseData)
	syncStatus, syncErr := h.syncCreatedCase(r.Context(), caseData, nodes, edges, previous.ID)
	if syncErr != nil {
		writeError(w, http.StatusBadGateway, syncErr.Error())
		return
	}
	created := h.store.Replace(caseData)
	h.relationMu.Lock()
	h.nodes, h.edges = cloneRelationshipNodes(nodes), cloneRelationshipEdges(edges)
	h.relationMu.Unlock()
	h.mapMu.Lock()
	h.markers, h.routes = []connectors.MapMarker{}, []connectors.MapRoute{}
	h.mapMu.Unlock()

	writeJSON(w, http.StatusCreated, createCaseResponse{Case: created, Relationships: connectors.RelationshipSnapshot{Nodes: nodes, Edges: edges, Backend: syncStatus.Backend}, Sync: syncStatus})
}

func normalizeCase(input casefile.Case) (casefile.Case, error) {
	input.Name = limited(input.Name, 120)
	input.Owner = limited(input.Owner, 100)
	input.Objective = limited(input.Objective, 4000)
	input.Subject.Codename = limited(input.Subject.Codename, 100)
	input.Subject.DisplayName = limited(input.Subject.DisplayName, 160)
	input.Subject.Location = limited(input.Subject.Location, 240)
	input.Subject.LastSeen = limited(input.Subject.LastSeen, 80)
	if input.Name == "" || input.Subject.Codename == "" {
		return casefile.Case{}, fmt.Errorf("case name and subject codename are required")
	}
	if input.Owner == "" {
		input.Owner = "LOCAL ANALYST"
	}
	if input.Subject.DisplayName == "" {
		input.Subject.DisplayName = "--"
	}
	if input.Subject.Location == "" {
		input.Subject.Location = "--"
	}
	if input.Subject.LastSeen == "" {
		input.Subject.LastSeen = "--"
	}
	if input.Subject.Risk == "" {
		input.Subject.Risk = "low"
	}
	if input.Subject.Risk != "low" && input.Subject.Risk != "medium" && input.Subject.Risk != "high" {
		return casefile.Case{}, fmt.Errorf("risk must be low, medium, or high")
	}
	if input.Subject.Confidence < 0 || input.Subject.Confidence > 100 {
		return casefile.Case{}, fmt.Errorf("confidence must be from 0 to 100")
	}
	input.Tags = cleanStrings(input.Tags, 20, 60)
	input.Subject.Aliases = cleanStrings(input.Subject.Aliases, 30, 120)
	if len(input.Subject.Identifiers) > 40 || len(input.Subject.Relations) > 30 {
		return casefile.Case{}, fmt.Errorf("too many identifiers or initial relations")
	}
	for i := range input.Subject.Identifiers {
		input.Subject.Identifiers[i].Type = limited(input.Subject.Identifiers[i].Type, 40)
		input.Subject.Identifiers[i].Value = limited(input.Subject.Identifiers[i].Value, 240)
		if input.Subject.Identifiers[i].Type == "" || input.Subject.Identifiers[i].Value == "" {
			return casefile.Case{}, fmt.Errorf("identifier type and value are required")
		}
	}
	for i := range input.Subject.Relations {
		input.Subject.Relations[i].Name = limited(input.Subject.Relations[i].Name, 160)
		input.Subject.Relations[i].Type = limited(input.Subject.Relations[i].Type, 40)
		input.Subject.Relations[i].Risk = limited(input.Subject.Relations[i].Risk, 20)
		if input.Subject.Relations[i].Name == "" {
			return casefile.Case{}, fmt.Errorf("related subject name is required")
		}
		if input.Subject.Relations[i].Type == "" {
			input.Subject.Relations[i].Type = "subject"
		}
		if input.Subject.Relations[i].Risk == "" {
			input.Subject.Relations[i].Risk = "low"
		}
	}
	input.Status = "active"
	input.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	input.SourceCount = 0
	input.Events = []casefile.Event{}
	return input, nil
}

func (h *Handler) syncCreatedCase(ctx context.Context, caseData casefile.Case, nodes []connectors.RelationshipNode, edges []connectors.RelationshipEdge, previousCaseID string) (caseSyncStatus, error) {
	connector, ok := h.activeDossierConnector(ctx)
	if !ok {
		return caseSyncStatus{State: "memory", Backend: "memory"}, nil
	}
	backend := connector.Metadata().ID
	ref := connectors.DossierRef{CaseID: caseData.ID, CaseName: caseData.Name, SubjectName: caseData.Subject.Codename}
	state, err := json.MarshalIndent(caseData, "", "  ")
	if err != nil {
		return caseSyncStatus{}, fmt.Errorf("encode case state: %w", err)
	}
	caseStore, durableCases := connector.(connectors.CaseStoreConnector)
	casePersisted := false
	rollback := func(syncErr error) (caseSyncStatus, error) {
		if !casePersisted {
			return caseSyncStatus{}, fmt.Errorf("synchronize new case: %w", syncErr)
		}
		_, rollbackErr := caseStore.TrashCase(ctx, caseData.ID)
		if rollbackErr == nil && previousCaseID != "" && previousCaseID != casefile.BlankCase().ID {
			rollbackErr = caseStore.SetActiveCase(ctx, previousCaseID)
		}
		if rollbackErr != nil {
			return caseSyncStatus{}, fmt.Errorf("synchronize new case: %v; rollback: %w", syncErr, rollbackErr)
		}
		return caseSyncStatus{}, fmt.Errorf("synchronize new case: %w", syncErr)
	}
	if durableCases {
		summary := connectors.CaseSummary{ID: caseData.ID, Name: caseData.Name, Subject: caseData.Subject.Codename, Status: caseData.Status, UpdatedAt: caseData.UpdatedAt}
		if err := caseStore.WriteCase(ctx, summary, state); err != nil {
			return caseSyncStatus{}, fmt.Errorf("persist case state: %w", err)
		}
		casePersisted = true
	}
	dossier, err := connector.WriteDossier(ctx, ref, connectors.DossierWrite{Content: dossierMarkdown(caseData)})
	if err != nil {
		return rollback(err)
	}
	if graph, ok := connector.(connectors.RelationshipConnector); ok {
		for _, node := range nodes {
			if _, err := graph.CreateRelationshipNode(ctx, ref, node); err != nil {
				return rollback(err)
			}
		}
		for _, edge := range edges {
			if _, err := graph.CreateRelationshipEdge(ctx, ref, edge); err != nil {
				return rollback(err)
			}
		}
	}
	if durableCases {
		if err := caseStore.SetActiveCase(ctx, caseData.ID); err != nil {
			return rollback(err)
		}
	} else if stateConnector, ok := connector.(connectors.WorkspaceStateConnector); ok {
		if err := stateConnector.WriteWorkspaceState(ctx, "active-case", state); err != nil {
			return caseSyncStatus{}, fmt.Errorf("persist active case: %w", err)
		}
	}
	return caseSyncStatus{State: "synced", Backend: backend, Path: dossier.Path}, nil
}

func (h *Handler) activeDossierConnector(ctx context.Context) (connectors.Connector, bool) {
	for _, info := range h.connectors.List(ctx) {
		if info.Status.State != "connected" || !hasCapability(info.Metadata.Capabilities, "dossier.write") {
			continue
		}
		connector, err := h.connectors.Get(info.Metadata.ID)
		return connector, err == nil
	}
	return nil, false
}

func dossierMarkdown(value casefile.Case) string {
	var body strings.Builder
	body.WriteString("---\namber_desk:\n  version: 1\n  case_id: ")
	body.WriteString(strconv.Quote(value.ID))
	body.WriteString("\n  connector: obsidian\ntags:\n")
	for _, tag := range append([]string{"amber-desk", "dossier"}, value.Tags...) {
		body.WriteString("  - " + strconv.Quote(tag) + "\n")
	}
	body.WriteString("---\n\n# " + value.Name + "\n\n## Subject\n\n**" + value.Subject.Codename + "**")
	if value.Subject.DisplayName != "--" {
		body.WriteString(" / " + value.Subject.DisplayName)
	}
	body.WriteString("\n\n## Executive summary\n\n" + value.Objective + "\n\n## Known identifiers\n\n")
	for _, identifier := range value.Subject.Identifiers {
		body.WriteString("- **" + identifier.Type + ":** " + identifier.Value + "\n")
	}
	body.WriteString("\n## Timeline\n\n\n## Evidence assessment\n\n\n## Open questions\n\n")
	return body.String()
}

func limited(value string, max int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	runes := []rune(value)
	if len(runes) > max {
		value = string(runes[:max])
	}
	return value
}

func cleanStrings(values []string, maxItems, maxLength int) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if cleaned := limited(value, maxLength); cleaned != "" && len(result) < maxItems {
			result = append(result, cleaned)
		}
	}
	return result
}
