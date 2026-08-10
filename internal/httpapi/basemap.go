package httpapi

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type MapTileConfig struct {
	Directory string
	Extension string
	MinZoom   int
	MaxZoom   int
}

type basemapResponse struct {
	Mode       string `json:"mode"`
	Configured bool   `json:"configured"`
	Extension  string `json:"extension,omitempty"`
	MinZoom    int    `json:"minZoom"`
	MaxZoom    int    `json:"maxZoom"`
}

func (config MapTileConfig) normalized() MapTileConfig {
	config.Directory = strings.TrimSpace(config.Directory)
	config.Extension = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(config.Extension), "."))
	if config.Extension == "" {
		config.Extension = "png"
	}
	if config.Extension != "png" && config.Extension != "jpg" && config.Extension != "jpeg" && config.Extension != "webp" {
		config.Extension = "png"
	}
	if config.MinZoom < 0 || config.MinZoom > 24 {
		config.MinZoom = 0
	}
	if config.MaxZoom < config.MinZoom || config.MaxZoom > 24 {
		config.MaxZoom = 18
	}
	if config.Directory != "" {
		if absolute, err := filepath.Abs(config.Directory); err == nil {
			config.Directory = filepath.Clean(absolute)
		}
		if resolved, err := filepath.EvalSymlinks(config.Directory); err == nil {
			config.Directory = resolved
		}
	}
	return config
}

func (h *Handler) getBasemap(w http.ResponseWriter, _ *http.Request) {
	configured := h.mapTiles.Directory != ""
	if configured {
		info, err := os.Stat(h.mapTiles.Directory)
		configured = err == nil && info.IsDir()
	}
	mode := "natural_earth"
	if configured {
		mode = "local_xyz"
	}
	writeJSON(w, http.StatusOK, basemapResponse{Mode: mode, Configured: configured, Extension: h.mapTiles.Extension, MinZoom: h.mapTiles.MinZoom, MaxZoom: h.mapTiles.MaxZoom})
}

func (h *Handler) getMapTile(w http.ResponseWriter, r *http.Request) {
	if h.mapTiles.Directory == "" {
		http.NotFound(w, r)
		return
	}
	z, errZ := strconv.Atoi(r.PathValue("z"))
	x, errX := strconv.Atoi(r.PathValue("x"))
	y, errY := strconv.Atoi(r.PathValue("y"))
	if errZ != nil || errX != nil || errY != nil || z < h.mapTiles.MinZoom || z > h.mapTiles.MaxZoom || z > 24 {
		http.NotFound(w, r)
		return
	}
	limit := 1 << z
	if x < 0 || x >= limit || y < 0 || y >= limit {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(h.mapTiles.Directory, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+"."+h.mapTiles.Extension)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, "local map tile is unavailable")
		return
	}
	relative, err := filepath.Rel(h.mapTiles.Directory, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusForbidden, "local map tile escapes configured directory")
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		writeError(w, http.StatusBadRequest, "invalid local map tile")
		return
	}
	contentType := mime.TypeByExtension("." + h.mapTiles.Extension)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
