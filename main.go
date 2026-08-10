package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
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
	handler := httpapi.New(store, registry, webRoot)
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

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
