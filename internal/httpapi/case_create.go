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
	current := h.store.Snapshot()
	if current.Subject.Codename != "UNASSIGNED" {
		writeError(w, http.StatusConflict, "an active dossier already exists")
		return
	}
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
	created := h.store.Replace(caseData)
	nodes, edges := seedRelationships(created)
	h.relationMu.Lock()
	h.nodes, h.edges = cloneRelationshipNodes(nodes), cloneRelationshipEdges(edges)
	h.relationMu.Unlock()
	h.mapMu.Lock()
	h.markers, h.routes = []connectors.MapMarker{}, []connectors.MapRoute{}
	h.mapMu.Unlock()

	syncStatus := h.syncCreatedCase(r.Context(), created, nodes, edges)
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

func (h *Handler) syncCreatedCase(ctx context.Context, caseData casefile.Case, nodes []connectors.RelationshipNode, edges []connectors.RelationshipEdge) caseSyncStatus {
	connector, ok := h.activeDossierConnector(ctx)
	if !ok {
		return caseSyncStatus{State: "memory", Backend: "memory"}
	}
	ref := connectors.DossierRef{CaseID: caseData.ID, CaseName: caseData.Name, SubjectName: caseData.Subject.Codename}
	dossier, err := connector.WriteDossier(ctx, ref, connectors.DossierWrite{Content: dossierMarkdown(caseData)})
	if err != nil {
		return caseSyncStatus{State: "sync_pending", Backend: connector.Metadata().ID, Message: err.Error()}
	}
	if stateConnector, ok := connector.(connectors.WorkspaceStateConnector); ok {
		state, marshalErr := json.MarshalIndent(caseData, "", "  ")
		if marshalErr != nil {
			return caseSyncStatus{State: "sync_pending", Backend: connector.Metadata().ID, Path: dossier.Path, Message: marshalErr.Error()}
		}
		if err := stateConnector.WriteWorkspaceState(ctx, "active-case", state); err != nil {
			return caseSyncStatus{State: "sync_pending", Backend: connector.Metadata().ID, Path: dossier.Path, Message: err.Error()}
		}
	}
	if graph, ok := connector.(connectors.RelationshipConnector); ok {
		for _, node := range nodes {
			if _, err := graph.CreateRelationshipNode(ctx, ref, node); err != nil {
				return caseSyncStatus{State: "sync_pending", Backend: connector.Metadata().ID, Path: dossier.Path, Message: err.Error()}
			}
		}
		for _, edge := range edges {
			if _, err := graph.CreateRelationshipEdge(ctx, ref, edge); err != nil {
				return caseSyncStatus{State: "sync_pending", Backend: connector.Metadata().ID, Path: dossier.Path, Message: err.Error()}
			}
		}
	}
	return caseSyncStatus{State: "synced", Backend: connector.Metadata().ID, Path: dossier.Path}
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
	if len(value) > max {
		value = value[:max]
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
