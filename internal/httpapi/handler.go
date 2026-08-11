package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"amberdesk/internal/casefile"
	"amberdesk/internal/connectors"
	"amberdesk/internal/sherlock"
	"amberdesk/pkg/catalog"
)

type Handler struct {
	store            *casefile.Store
	connectors       *connectors.Registry
	web              http.Handler
	mapMu            sync.Mutex
	markers          []connectors.MapMarker
	routes           []connectors.MapRoute
	relationMu       sync.Mutex
	coverMu          sync.Mutex
	caseMu           sync.Mutex
	nodes            []connectors.RelationshipNode
	edges            []connectors.RelationshipEdge
	attachmentMu     sync.Mutex
	attachments      map[string]map[string]memoryAttachment
	checklistMu      sync.Mutex
	checklists       map[string]connectors.ChecklistSnapshot
	mapTiles         MapTileConfig
	catalog          catalog.Provider
	sherlock         *sherlock.Manager
	allowRemoteTools bool
	allowRemote      bool
}

func New(store *casefile.Store, registry *connectors.Registry, webFiles fs.FS) http.Handler {
	return NewWithConfig(store, registry, webFiles, Config{})
}

type Config struct {
	MapTiles            MapTileConfig
	Catalog             catalog.Provider
	SherlockRunner      sherlock.Runner
	AllowRemoteToolRuns bool
	AllowRemoteAccess   bool
	ToolContext         context.Context
}

func NewWithConfig(store *casefile.Store, registry *connectors.Registry, webFiles fs.FS, config Config) http.Handler {
	toolContext := config.ToolContext
	if toolContext == nil {
		toolContext = context.Background()
	}
	h := &Handler{store: store, connectors: registry, web: staticHandler(webFiles), mapTiles: config.MapTiles.normalized(), catalog: config.Catalog, attachments: make(map[string]map[string]memoryAttachment), checklists: make(map[string]connectors.ChecklistSnapshot), sherlock: sherlock.NewManager(toolContext, config.SherlockRunner), allowRemoteTools: config.AllowRemoteToolRuns, allowRemote: config.AllowRemoteAccess}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/config", h.getRuntimeConfig)
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
	mux.HandleFunc("PUT /api/relationships/nodes/{id}/cover", h.setRelationshipCover)
	mux.HandleFunc("GET /api/relationships/nodes/{id}/attachments", h.listRelationshipAttachments)
	mux.HandleFunc("POST /api/relationships/nodes/{id}/attachments", h.createRelationshipAttachment)
	mux.HandleFunc("GET /api/relationships/nodes/{id}/attachments/{attachmentId}", h.downloadRelationshipAttachment)
	mux.HandleFunc("DELETE /api/relationships/nodes/{id}/attachments/{attachmentId}", h.deleteRelationshipAttachment)
	mux.HandleFunc("POST /api/relationships/edges", h.createRelationshipEdge)
	mux.HandleFunc("PUT /api/relationships/edges/{id}", h.updateRelationshipEdge)
	mux.HandleFunc("DELETE /api/relationships/edges/{id}", h.deleteRelationshipEdge)
	mux.HandleFunc("GET /api/checklist", h.getChecklist)
	mux.HandleFunc("POST /api/checklist/tasks", h.createChecklistTask)
	mux.HandleFunc("PUT /api/checklist/tasks/{id}", h.updateChecklistTask)
	mux.HandleFunc("DELETE /api/checklist/tasks/{id}", h.deleteChecklistTask)
	mux.HandleFunc("POST /api/checklist/reset", h.resetChecklist)
	mux.HandleFunc("GET /api/tools/sherlock/status", h.sherlockStatus)
	mux.HandleFunc("POST /api/tools/sherlock/scans", h.startSherlockScan)
	mux.HandleFunc("GET /api/tools/sherlock/scans/{id}", h.getSherlockScan)
	mux.HandleFunc("GET /api/tools/sherlock/scans/{id}/events", h.streamSherlockScan)
	mux.HandleFunc("DELETE /api/tools/sherlock/scans/{id}", h.cancelSherlockScan)
	mux.HandleFunc("POST /api/tools/sherlock/scans/{id}/import", h.importSherlockScan)
	mux.HandleFunc("GET /api/integrations", h.listIntegrations)
	mux.HandleFunc("GET /api/integrations/{id}/dossier", h.readDossier)
	mux.HandleFunc("PUT /api/integrations/{id}/dossier", h.writeDossier)
	mux.Handle("/", h.web)
	return securityHeaders(h.requestGuard(mux))
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
	if err := decodeJSONLimit(r, &input, maxDossierJSONBodySize); err != nil {
		if errors.Is(err, errRequestBodyTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "dossier request exceeds 6 MiB")
			return
		}
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
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid note")
		return
	}
	normalized, err := connectors.NormalizeTimelineNote(connectors.TimelineNote{Text: input.Text})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	noteText := normalized.Text
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

const (
	maxJSONBodySize        = 64 << 10
	maxDossierJSONBodySize = 6 << 20
)

var errRequestBodyTooLarge = errors.New("request body is too large")

func decodeJSON(r *http.Request, target any) error {
	return decodeJSONLimit(r, target, maxJSONBodySize)
}

func decodeJSONLimit(r *http.Request, target any, maximum int64) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, maximum+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maximum {
		return errRequestBodyTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
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
	if errors.Is(err, connectors.ErrInvalidFilename) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, connectors.ErrAttachmentLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment exceeds "+formatByteLimit(connectors.MaxRelationshipAttachmentSize))
		return
	}
	if errors.Is(err, connectors.ErrAttachmentLimit) {
		writeError(w, http.StatusConflict, "attachment limit reached (maximum "+strconv.Itoa(connectors.MaxRelationshipAttachmentsPerNode)+" per card)")
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

func formatByteLimit(value int64) string {
	if value%(1<<20) == 0 {
		return strconv.FormatInt(value/(1<<20), 10) + " MiB"
	}
	return strconv.FormatInt(value, 10) + " bytes"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) requestGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.allowRemote && !isLoopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, "Amber Desk is limited to localhost")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
				writeError(w, http.StatusForbidden, "cross-site API requests are not allowed")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && !sameOriginHost(origin, r.Host) {
				writeError(w, http.StatusForbidden, "foreign origin is not allowed")
				return
			}
			if isMutation(r.Method) {
				if !validMutationMediaType(r) {
					writeError(w, http.StatusUnsupportedMediaType, "application/json is required")
					return
				}
				if !isAttachmentUpload(r) {
					maximum := int64(maxJSONBodySize)
					if strings.HasPrefix(r.URL.Path, "/api/integrations/") && strings.HasSuffix(r.URL.Path, "/dossier") {
						maximum = maxDossierJSONBodySize
					}
					if r.ContentLength > maximum {
						writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(value string) bool {
	host := value
	if parsed, _, err := net.SplitHostPort(value); err == nil {
		host = parsed
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameOriginHost(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, requestHost)
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func validMutationMediaType(r *http.Request) bool {
	if r.Method == http.MethodDelete && r.ContentLength == 0 {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	if isAttachmentUpload(r) {
		return mediaType == "multipart/form-data"
	}
	return mediaType == "application/json"
}

func isAttachmentUpload(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/relationships/nodes/") && strings.HasSuffix(r.URL.Path, "/attachments")
}

func staticHandler(files fs.FS) http.Handler {
	etags := make(map[string]string)
	_ = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		etag := `"` + hex.EncodeToString(sum[:16]) + `"`
		etags["/"+path.Clean(name)] = etag
		if name == "index.html" {
			etags["/"] = etag
		}
		return nil
	})
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if etag := etags[r.URL.Path]; etag != "" {
			w.Header().Set("Cache-Control", "private, no-cache")
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		server.ServeHTTP(w, r)
	})
}
