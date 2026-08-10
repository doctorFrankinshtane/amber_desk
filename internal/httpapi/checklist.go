package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	guide "amberdesk/internal/checklist"
	"amberdesk/pkg/connectors"
)

type checklistTaskUpdate struct {
	CaseID      string  `json:"caseId"`
	Title       *string `json:"title"`
	Note        *string `json:"note"`
	Status      *string `json:"status"`
	Recommended *bool   `json:"recommended"`
}

func (h *Handler) getChecklist(w http.ResponseWriter, r *http.Request) {
	h.checklistMu.Lock()
	defer h.checklistMu.Unlock()
	snapshot, err := h.loadChecklist(r.Context())
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) createChecklistTask(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID  string `json:"caseId"`
		PhaseID string `json:"phaseId"`
		Title   string `json:"title"`
		Note    string `json:"note"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.validChecklistCase(input.CaseID) {
		writeError(w, http.StatusConflict, "active case changed; reload the checklist")
		return
	}
	input.Title, input.Note = guide.CleanText(input.Title, 200), guide.CleanText(input.Note, 1000)
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "task title is required")
		return
	}
	h.checklistMu.Lock()
	defer h.checklistMu.Unlock()
	snapshot, err := h.loadChecklist(r.Context())
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if !guide.HasPhase(snapshot, input.PhaseID) {
		writeError(w, http.StatusBadRequest, "valid phaseId is required")
		return
	}
	task := connectors.ChecklistTask{ID: newID("TASK"), PhaseID: input.PhaseID, Title: input.Title, Edited: true, Note: input.Note, Status: "pending", Custom: true, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	for i := range snapshot.Phases {
		if snapshot.Phases[i].ID == input.PhaseID {
			snapshot.Phases[i].Tasks = append(snapshot.Phases[i].Tasks, task)
			break
		}
	}
	snapshot, err = h.saveChecklist(r.Context(), snapshot)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

func (h *Handler) updateChecklistTask(w http.ResponseWriter, r *http.Request) {
	var input checklistTaskUpdate
	if decodeJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.validChecklistCase(input.CaseID) {
		writeError(w, http.StatusConflict, "active case changed; reload the checklist")
		return
	}
	h.checklistMu.Lock()
	defer h.checklistMu.Unlock()
	snapshot, err := h.loadChecklist(r.Context())
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	task, found := guide.Find(&snapshot, r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
		return
	}
	if input.Title != nil {
		value := guide.CleanText(*input.Title, 200)
		if value == "" {
			writeError(w, http.StatusBadRequest, "task title is required")
			return
		}
		task.Title, task.Edited = value, true
	}
	if input.Note != nil {
		task.Note = guide.CleanText(*input.Note, 1000)
	}
	if input.Status != nil {
		if !guide.ValidStatus(*input.Status) {
			writeError(w, http.StatusBadRequest, "status must be pending, done, or skipped")
			return
		}
		task.Status = *input.Status
	}
	if input.Recommended != nil {
		if *input.Recommended {
			snapshot.RecommendedTaskID = task.ID
		} else if snapshot.RecommendedTaskID == task.ID {
			snapshot.RecommendedTaskID = ""
		}
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	snapshot, err = h.saveChecklist(r.Context(), snapshot)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) deleteChecklistTask(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID string `json:"caseId"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.validChecklistCase(input.CaseID) {
		writeError(w, http.StatusConflict, "active case changed; reload the checklist")
		return
	}
	h.checklistMu.Lock()
	defer h.checklistMu.Unlock()
	snapshot, err := h.loadChecklist(r.Context())
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	id, removed := r.PathValue("id"), false
	for pi := range snapshot.Phases {
		for ti := range snapshot.Phases[pi].Tasks {
			item := snapshot.Phases[pi].Tasks[ti]
			if item.ID != id {
				continue
			}
			if !item.Custom {
				writeError(w, http.StatusConflict, "built-in checklist tasks cannot be deleted")
				return
			}
			snapshot.Phases[pi].Tasks = append(snapshot.Phases[pi].Tasks[:ti], snapshot.Phases[pi].Tasks[ti+1:]...)
			removed = true
			break
		}
	}
	if !removed {
		writeError(w, http.StatusNotFound, connectors.ErrEntityAbsent.Error())
		return
	}
	if snapshot.RecommendedTaskID == id {
		snapshot.RecommendedTaskID = ""
	}
	if _, err := h.saveChecklist(r.Context(), snapshot); err != nil {
		writeConnectorError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) resetChecklist(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID         string `json:"caseId"`
		PreserveCustom bool   `json:"preserveCustom"`
	}
	if decodeJSON(r, &input) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.validChecklistCase(input.CaseID) {
		writeError(w, http.StatusConflict, "active case changed; reload the checklist")
		return
	}
	h.checklistMu.Lock()
	defer h.checklistMu.Unlock()
	next := guide.Default()
	if input.PreserveCustom {
		current, err := h.loadChecklist(r.Context())
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		for _, phase := range current.Phases {
			for _, item := range phase.Tasks {
				if !item.Custom {
					continue
				}
				for pi := range next.Phases {
					if next.Phases[pi].ID == item.PhaseID {
						next.Phases[pi].Tasks = append(next.Phases[pi].Tasks, item)
					}
				}
			}
		}
	}
	next, err := h.saveChecklist(r.Context(), next)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, next)
}

func (h *Handler) loadChecklist(ctx context.Context) (connectors.ChecklistSnapshot, error) {
	ref := h.dossierRef()
	if connector, ok := h.activeChecklistConnector(ctx); ok {
		snapshot, err := connector.ReadChecklist(ctx, ref)
		if errors.Is(err, connectors.ErrEntityAbsent) {
			snapshot = guide.Default()
		} else if err != nil {
			return connectors.ChecklistSnapshot{}, err
		}
		snapshot = guide.Merge(snapshot)
		snapshot.Backend = connector.Metadata().ID
		if errors.Is(err, connectors.ErrEntityAbsent) {
			return connector.WriteChecklist(ctx, ref, snapshot)
		}
		return snapshot, nil
	}
	snapshot, ok := h.checklists[ref.CaseID]
	if !ok {
		snapshot = guide.Default()
		h.checklists[ref.CaseID] = snapshot
	}
	snapshot = guide.Merge(snapshot)
	snapshot.Backend = "memory"
	return snapshot, nil
}

func (h *Handler) saveChecklist(ctx context.Context, snapshot connectors.ChecklistSnapshot) (connectors.ChecklistSnapshot, error) {
	snapshot = guide.Merge(snapshot)
	if connector, ok := h.activeChecklistConnector(ctx); ok {
		return connector.WriteChecklist(ctx, h.dossierRef(), snapshot)
	}
	snapshot.Backend = "memory"
	h.checklists[h.dossierRef().CaseID] = snapshot
	return snapshot, nil
}

func (h *Handler) activeChecklistConnector(ctx context.Context) (connectors.ChecklistConnector, bool) {
	for _, info := range h.connectors.List(ctx) {
		if info.Status.State != "connected" || !hasCapability(info.Metadata.Capabilities, "checklist.read") {
			continue
		}
		connector, err := h.connectors.Get(info.Metadata.ID)
		if err == nil {
			value, ok := connector.(connectors.ChecklistConnector)
			return value, ok
		}
	}
	return nil, false
}

func (h *Handler) validChecklistCase(caseID string) bool {
	return caseID != "" && caseID == h.dossierRef().CaseID
}
