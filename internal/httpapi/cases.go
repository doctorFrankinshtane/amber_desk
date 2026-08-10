package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"amberdesk/internal/casefile"
	"amberdesk/pkg/connectors"
)

func (h *Handler) listCases(w http.ResponseWriter, r *http.Request) {
	store, ok := h.activeCaseStore(r.Context())
	if !ok {
		current := h.store.Snapshot()
		items := []connectors.CaseSummary{}
		if current.Subject.Codename != "UNASSIGNED" {
			items = append(items, caseSummary(current, true))
		}
		writeJSON(w, http.StatusOK, map[string]any{"cases": items, "backend": "memory"})
		return
	}
	items, err := store.ListCases(r.Context())
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": items, "backend": store.Metadata().ID})
}

func (h *Handler) activateCase(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID         string `json:"caseId"`
		ExpectedCaseID string `json:"expectedCaseId"`
	}
	if decodeJSON(r, &input) != nil || !strings.HasPrefix(input.CaseID, "CASE-") {
		writeError(w, http.StatusBadRequest, "valid caseId is required")
		return
	}
	h.caseMu.Lock()
	defer h.caseMu.Unlock()
	current := h.store.Snapshot()
	if input.ExpectedCaseID != current.ID {
		writeError(w, http.StatusConflict, "active case changed; reload the case list")
		return
	}
	store, ok := h.activeCaseStore(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "case storage is unavailable")
		return
	}
	data, err := store.ReadCase(r.Context(), input.CaseID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	var selected casefile.Case
	if json.Unmarshal(data, &selected) != nil || selected.ID != input.CaseID {
		writeError(w, http.StatusBadGateway, "stored case snapshot is invalid")
		return
	}
	if err := store.SetActiveCase(r.Context(), selected.ID); err != nil {
		writeConnectorError(w, err)
		return
	}
	h.replaceActiveCase(selected)
	writeJSON(w, http.StatusOK, selected)
}

func (h *Handler) deleteCase(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	var input struct {
		ConfirmCaseID  string `json:"confirmCaseId"`
		ExpectedCaseID string `json:"expectedCaseId"`
	}
	if decodeJSON(r, &input) != nil || input.ConfirmCaseID != caseID {
		writeError(w, http.StatusBadRequest, "exact case ID confirmation is required")
		return
	}
	h.caseMu.Lock()
	defer h.caseMu.Unlock()
	current := h.store.Snapshot()
	if current.ID != caseID || input.ExpectedCaseID != current.ID {
		writeError(w, http.StatusConflict, "active case changed; reload before deleting")
		return
	}
	store, ok := h.activeCaseStore(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "case storage is unavailable")
		return
	}
	nextID, err := store.TrashCase(r.Context(), caseID)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	next := casefile.BlankCase()
	if nextID != "" {
		data, readErr := store.ReadCase(r.Context(), nextID)
		if readErr != nil || json.Unmarshal(data, &next) != nil {
			writeError(w, http.StatusBadGateway, "next case snapshot is unavailable")
			return
		}
	}
	h.replaceActiveCase(next)
	items, _ := store.ListCases(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"case": next, "cases": items, "backend": store.Metadata().ID})
}

func (h *Handler) replaceActiveCase(selected casefile.Case) {
	h.store.Replace(selected)
	h.relationMu.Lock()
	h.nodes, h.edges = []connectors.RelationshipNode{}, []connectors.RelationshipEdge{}
	h.relationMu.Unlock()
	h.mapMu.Lock()
	h.markers, h.routes = []connectors.MapMarker{}, []connectors.MapRoute{}
	h.mapMu.Unlock()
}

func (h *Handler) activeCaseStore(ctx context.Context) (connectors.CaseStoreConnector, bool) {
	for _, info := range h.connectors.List(ctx) {
		if info.Status.State != "connected" || !hasCapability(info.Metadata.Capabilities, "cases.read") {
			continue
		}
		connector, err := h.connectors.Get(info.Metadata.ID)
		if err != nil {
			continue
		}
		store, ok := connector.(connectors.CaseStoreConnector)
		return store, ok
	}
	return nil, false
}

func caseSummary(value casefile.Case, active bool) connectors.CaseSummary {
	return connectors.CaseSummary{ID: value.ID, Name: value.Name, Subject: value.Subject.Codename, Status: value.Status, UpdatedAt: value.UpdatedAt, Active: active}
}
