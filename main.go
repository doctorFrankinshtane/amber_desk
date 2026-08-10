package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"amberdesk/internal/casefile"
	"amberdesk/internal/connectors/obsidian"
	"amberdesk/internal/httpapi"
	"amberdesk/pkg/connectors"
)

//go:embed web/*
var webFiles embed.FS

func main() {
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}

	store := casefile.NewStore(casefile.DemoCase())
	obsidianConnector, err := obsidian.New(obsidian.Config{
		VaultPath:  os.Getenv("OBSIDIAN_VAULT"),
		DossierDir: envOr("OBSIDIAN_DOSSIER_DIR", "Amber Desk/Dossiers"),
	})
	if err != nil {
		log.Fatalf("configure obsidian connector: %v", err)
	}
	registry := connectors.NewRegistry(obsidianConnector)
	handler := httpapi.NewWithConfig(store, registry, webRoot, httpapi.Config{MapTiles: httpapi.MapTileConfig{
		Directory: os.Getenv("MAP_TILE_DIR"),
		Extension: envOr("MAP_TILE_EXT", "png"),
		MinZoom:   envInt("MAP_TILE_MIN_ZOOM", 0),
		MaxZoom:   envInt("MAP_TILE_MAX_ZOOM", 18),
	}})
	addr := envOr("ADDR", ":8080")

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	fmt.Printf("Amber Desk listening on http://localhost%s\n", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
