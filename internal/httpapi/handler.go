package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"amberdesk/internal/casefile"
	"amberdesk/internal/connectors"
	"amberdesk/pkg/catalog"
)

type Handler struct {
	store      *casefile.Store
	connectors *connectors.Registry
	web        http.Handler
	mapMu      sync.Mutex
	markers    []connectors.MapMarker
	routes     []connectors.MapRoute
	relationMu sync.Mutex
	caseMu     sync.Mutex
	nodes      []connectors.RelationshipNode
	edges      []connectors.RelationshipEdge
	mapTiles   MapTileConfig
	catalog    catalog.Provider
}

func New(store *casefile.Store, registry *connectors.Registry, webFiles fs.FS) http.Handler {
	return NewWithConfig(store, registry, webFiles, Config{})
}

type Config struct {
	MapTiles MapTileConfig
	Catalog  catalog.Provider
}

func NewWithConfig(store *casefile.Store, registry *connectors.Registry, webFiles fs.FS, config Config) http.Handler {
	h := &Handler{store: store, connectors: registry, web: http.FileServer(http.FS(webFiles)), mapTiles: config.MapTiles.normalized(), catalog: config.Catalog}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/case", h.getCase)
	mux.HandleFunc("POST /api/case", h.createCase)
	mux.HandleFunc("GET /api/cases", h.listCases)
	mux.HandleFunc("PUT /api/cases/active", h.activateCase)
	mux.HandleFunc("DELETE /api/cases/{id}", h.deleteCase)
	mux.HandleFunc("GET /api/timeline", h.listTimeline)
	mux.HandleFunc("POST /api/timeline/bootstrap", h.bootstrapTimeline)
	mux.HandleFunc("POST /api/timeline/events", h.createTimelineEvent)
	mux.HandleFunc("PATCH /api/events/{id}/status", h.setStatus)
	mux.HandleFunc("POST /api/events/{id}/notes", h.addNote)
	mux.HandleFunc("DELETE /api/events/{id}", h.deleteEvent)
	mux.HandleFunc("GET /api/map", h.listMap)
	mux.HandleFunc("POST /api/map/markers", h.createMapMarker)
	mux.HandleFunc("PUT /api/map/markers/{id}", h.updateMapMarker)
	mux.HandleFunc("DELETE /api/map/markers/{id}", h.deleteMapMarker)
	mux.HandleFunc("POST /api/map/routes", h.createMapRoute)
	mux.HandleFunc("DELETE /api/map/routes/{id}", h.deleteMapRoute)
	mux.HandleFunc("GET /api/map/basemap", h.getBasemap)
	mux.HandleFunc("GET /api/map/tiles/{z}/{x}/{y}", h.getMapTile)
	mux.HandleFunc("GET /api/catalog", h.getCatalog)
	mux.HandleFunc("GET /api/relationships", h.listRelationships)
	mux.HandleFunc("POST /api/relationships/nodes", h.createRelationshipNode)
	mux.HandleFunc("PUT /api/relationships/nodes/{id}", h.updateRelationshipNode)
	mux.HandleFunc("DELETE /api/relationships/nodes/{id}", h.deleteRelationshipNode)
	mux.HandleFunc("POST /api/relationships/edges", h.createRelationshipEdge)
	mux.HandleFunc("PUT /api/relationships/edges/{id}", h.updateRelationshipEdge)
	mux.HandleFunc("DELETE /api/relationships/edges/{id}", h.deleteRelationshipEdge)
	mux.HandleFunc("GET /api/integrations", h.listIntegrations)
	mux.HandleFunc("GET /api/integrations/{id}/dossier", h.readDossier)
	mux.HandleFunc("PUT /api/integrations/{id}/dossier", h.writeDossier)
	mux.Handle("/", h.web)
	return securityHeaders(mux)
}

func (h *Handler) getCatalog(w http.ResponseWriter, r *http.Request) {
	if h.catalog == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog provider is not configured")
		return
	}
	snapshot, err := h.catalog.Snapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "catalog provider is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) listIntegrations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.connectors.List(r.Context()))
}

func (h *Handler) readDossier(w http.ResponseWriter, r *http.Request) {
	connector, ok := h.connectorForRequest(w, r)
	if !ok {
		return
	}
	ref := h.dossierRef()
	dossier, err := connector.ReadDossier(r.Context(), ref)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	dossier.CaseID = ref.CaseID
	writeJSON(w, http.StatusOK, dossier)
}

func (h *Handler) writeDossier(w http.ResponseWriter, r *http.Request) {
	connector, ok := h.connectorForRequest(w, r)
	if !ok {
		return
	}
	var input struct {
		CaseID             string `json:"caseId"`
		Content            string `json:"content"`
		ExpectedModifiedAt string `json:"expectedModifiedAt"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(input.Content) > 1<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "dossier exceeds 1 MiB")
		return
	}
	ref := h.dossierRef()
	if input.CaseID == "" || input.CaseID != ref.CaseID {
		writeError(w, http.StatusConflict, "active case changed; reload the dossier before saving")
		return
	}
	dossier, err := connector.WriteDossier(r.Context(), ref, connectors.DossierWrite{Content: input.Content, ExpectedModifiedAt: input.ExpectedModifiedAt})
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	dossier.CaseID = ref.CaseID
	writeJSON(w, http.StatusOK, dossier)
}

func (h *Handler) connectorForRequest(w http.ResponseWriter, r *http.Request) (connectors.Connector, bool) {
	connector, err := h.connectors.Get(r.PathValue("id"))
	if errors.Is(err, connectors.ErrConnectorAbsent) {
		writeError(w, http.StatusNotFound, err.Error())
		return nil, false
	}
	return connector, true
}

func (h *Handler) dossierRef() connectors.DossierRef {
	caseData := h.store.Snapshot()
	return connectors.DossierRef{CaseID: caseData.ID, CaseName: caseData.Name, SubjectName: caseData.Subject.Codename}
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) getCase(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.store.Snapshot())
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.Status != "pending" && input.Status != "verified" {
		writeError(w, http.StatusBadRequest, "status must be pending or verified")
		return
	}
	if connector, ok := h.activeTimelineConnector(r.Context()); ok {
		event, err := connector.SetTimelineStatus(r.Context(), h.dossierRef(), r.PathValue("id"), input.Status)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, event)
		return
	}
	event, err := h.store.SetStatus(r.PathValue("id"), input.Status)
	if errors.Is(err, casefile.ErrEventNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *Handler) addNote(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &input); err != nil || strings.TrimSpace(input.Text) == "" {
		writeError(w, http.StatusBadRequest, "note text is required")
		return
	}
	noteText := strings.TrimSpace(input.Text)
	if connector, ok := h.activeTimelineConnector(r.Context()); ok {
		note := connectors.TimelineNote{Text: noteText, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
		event, err := connector.AddTimelineNote(r.Context(), h.dossierRef(), r.PathValue("id"), note)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, event)
		return
	}
	note := casefile.Note{Text: noteText, CreatedAt: time.Now().Format("15:04")}
	event, err := h.store.AddNote(r.PathValue("id"), note)
	if errors.Is(err, casefile.ErrEventNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CaseID string `json:"caseId"`
	}
	if decodeJSON(r, &input) != nil || input.CaseID == "" {
		writeError(w, http.StatusBadRequest, "caseId is required")
		return
	}
	if input.CaseID != h.store.Snapshot().ID {
		writeError(w, http.StatusConflict, "active case changed; reload before deleting the event")
		return
	}
	eventID := r.PathValue("id")
	if connector, ok := h.activeTimelineConnector(r.Context()); ok {
		deleter, supported := connector.(connectors.TimelineDeleteConnector)
		if !supported {
			writeError(w, http.StatusNotImplemented, "timeline connector does not support deletion")
			return
		}
		if err := deleter.DeleteTimelineEvent(r.Context(), h.dossierRef(), eventID); err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deletedId": eventID})
		return
	}
	if err := h.store.DeleteEvent(eventID); errors.Is(err, casefile.ErrEventNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deletedId": eventID})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeConnectorError(w http.ResponseWriter, err error) {
	if errors.Is(err, connectors.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if errors.Is(err, connectors.ErrConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, connectors.ErrEntityAbsent) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}
