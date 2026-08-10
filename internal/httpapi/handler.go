package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"amberdesk/internal/casefile"
)

type Handler struct {
	store *casefile.Store
	web   http.Handler
}

func New(store *casefile.Store, webFiles fs.FS) http.Handler {
	h := &Handler{store: store, web: http.FileServer(http.FS(webFiles))}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/case", h.getCase)
	mux.HandleFunc("PATCH /api/events/{id}/status", h.setStatus)
	mux.HandleFunc("POST /api/events/{id}/notes", h.addNote)
	mux.Handle("/", h.web)
	return securityHeaders(mux)
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
	note := casefile.Note{Text: strings.TrimSpace(input.Text), CreatedAt: time.Now().Format("15:04")}
	event, err := h.store.AddNote(r.PathValue("id"), note)
	if errors.Is(err, casefile.ErrEventNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, event)
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}
