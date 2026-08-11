package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"amberdesk/internal/casefile"
	catalogprovider "amberdesk/internal/catalog"
	"amberdesk/internal/connectors/obsidian"
	"amberdesk/internal/httpapi"
	"amberdesk/internal/sherlock"
	"amberdesk/pkg/catalog"
	"amberdesk/pkg/connectors"
)

//go:embed web/*
var webFiles embed.FS

func main() {
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}

	catalogData, err := fs.ReadFile(webFiles, "web/data/osint-framework.json")
	if err != nil {
		log.Fatalf("load OSINT catalog: %v", err)
	}
	catalogProvider, err := catalogprovider.NewStatic(bytes.NewReader(catalogData), catalog.Metadata{
		Name: "OSINT Framework", Source: "https://github.com/lockfale/osint-framework", Version: "a744e613d7ded0aaa854896feb2a1069de34d2f8", ImportedAt: "2026-08-10", License: "MIT",
	})
	if err != nil {
		log.Fatalf("validate OSINT catalog: %v", err)
	}
	obsidianConnector, err := obsidian.New(obsidian.Config{
		VaultPath:  os.Getenv("OBSIDIAN_VAULT"),
		DossierDir: envOr("OBSIDIAN_DOSSIER_DIR", "Amber Desk/Dossiers"),
	})
	if err != nil {
		log.Fatalf("configure obsidian connector: %v", err)
	}
	initialCase := casefile.BlankCase()
	if caseStore, ok := any(obsidianConnector).(connectors.CaseStoreConnector); ok {
		if activeID, activeErr := caseStore.ActiveCaseID(context.Background()); activeErr == nil {
			data, stateErr := caseStore.ReadCase(context.Background(), activeID)
			if stateErr == nil {
				if decodeErr := json.Unmarshal(data, &initialCase); decodeErr != nil {
					log.Printf("ignore invalid persisted case state: %v", decodeErr)
					initialCase = casefile.BlankCase()
				}
			} else {
				log.Printf("load persisted case state: %v", stateErr)
			}
		} else if !errors.Is(activeErr, connectors.ErrEntityAbsent) && !errors.Is(activeErr, connectors.ErrNotConfigured) {
			log.Printf("load active case: %v", activeErr)
		}
	} else if stateConnector, ok := any(obsidianConnector).(connectors.WorkspaceStateConnector); ok {
		if data, stateErr := stateConnector.ReadWorkspaceState(context.Background(), "active-case"); stateErr == nil {
			if decodeErr := json.Unmarshal(data, &initialCase); decodeErr != nil {
				log.Printf("ignore invalid persisted case state: %v", decodeErr)
				initialCase = casefile.BlankCase()
			}
		} else if !errors.Is(stateErr, connectors.ErrEntityAbsent) && !errors.Is(stateErr, connectors.ErrNotConfigured) {
			log.Printf("load persisted case state: %v", stateErr)
		}
	}
	store := casefile.NewStore(initialCase)
	registry := connectors.NewRegistry(obsidianConnector)
	toolContext, stopTools := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopTools()
	handler := httpapi.NewWithConfig(store, registry, webRoot, httpapi.Config{Catalog: catalogProvider, SherlockRunner: sherlock.NewCLIRunner(os.Getenv("SHERLOCK_PYTHON")), AllowRemoteToolRuns: os.Getenv("ALLOW_REMOTE_TOOL_RUNS") == "1", AllowRemoteAccess: os.Getenv("ALLOW_REMOTE_ACCESS") == "1", ToolContext: toolContext, MapTiles: httpapi.MapTileConfig{
		Directory: os.Getenv("MAP_TILE_DIR"),
		Extension: envOr("MAP_TILE_EXT", "png"),
		MinZoom:   envInt("MAP_TILE_MIN_ZOOM", 0),
		MaxZoom:   envInt("MAP_TILE_MAX_ZOOM", 18),
	}})
	addr := envOr("ADDR", "127.0.0.1:8080")

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		<-toolContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
	}()

	fmt.Printf("Amber Desk listening on %s\n", displayURL(addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func displayURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
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
