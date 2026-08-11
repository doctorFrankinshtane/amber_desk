package obsidian_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"amberdesk/internal/connectors/obsidian"
	"amberdesk/pkg/connectors"
)

func TestReadWriteDossier(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault, DossierDir: "Dossiers"})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR", SubjectName: "SUBJECT_021"}

	draft, err := connector.ReadDossier(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Exists || !strings.Contains(draft.Content, "# NORTHSTAR") {
		t.Fatalf("unexpected draft: %+v", draft)
	}

	saved, err := connector.WriteDossier(context.Background(), ref, connectors.DossierWrite{Content: draft.Content + "\nVerified content.\n"})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Exists || saved.Path != "Dossiers/NS-04.md" {
		t.Fatalf("unexpected saved document: %+v", saved)
	}
	data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(saved.Path)))
	if err != nil || !strings.Contains(string(data), "Verified content") {
		t.Fatalf("saved file mismatch: %v %q", err, data)
	}
	overwritten, err := connector.WriteDossier(context.Background(), ref, connectors.DossierWrite{Content: "second version", ExpectedModifiedAt: saved.ModifiedAt})
	if err != nil {
		t.Fatalf("overwrite dossier: %v", err)
	}
	if overwritten.Content != "second version" {
		t.Fatalf("unexpected overwritten content: %q", overwritten.Content)
	}
}

func TestConcurrentDossierWritesEnforceExpectedVersion(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault, DossierDir: "Dossiers"})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR"}
	saved, err := connector.WriteDossier(context.Background(), ref, connectors.DossierWrite{Content: "initial"})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, content := range []string{"analyst one", "analyst two"} {
		workers.Add(1)
		go func(content string) {
			defer workers.Done()
			<-start
			_, writeErr := connector.WriteDossier(context.Background(), ref, connectors.DossierWrite{Content: content, ExpectedModifiedAt: saved.ModifiedAt})
			results <- writeErr
		}(content)
	}
	close(start)
	workers.Wait()
	close(results)

	succeeded, conflicted := 0, 0
	for result := range results {
		switch {
		case result == nil:
			succeeded++
		case errors.Is(result, connectors.ErrConflict):
			conflicted++
		default:
			t.Fatalf("unexpected write error: %v", result)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("writes succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func TestDetectsExternalEditConflict(t *testing.T) {
	vault := t.TempDir()
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault, DossierDir: "Dossiers"})
	if err != nil {
		t.Fatal(err)
	}
	ref := connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR"}
	saved, err := connector.WriteDossier(context.Background(), ref, connectors.DossierWrite{Content: "version one"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vault, filepath.FromSlash(saved.Path))
	if err := os.WriteFile(path, []byte("external edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := info.ModTime().Add(time.Second)
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
	_, err = connector.WriteDossier(context.Background(), ref, connectors.DossierWrite{Content: "stale overwrite", ExpectedModifiedAt: saved.ModifiedAt})
	if !errors.Is(err, connectors.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
}

func TestRejectsEscapingDirectory(t *testing.T) {
	_, err := obsidian.New(obsidian.Config{VaultPath: t.TempDir(), DossierDir: "../outside"})
	if err == nil {
		t.Fatal("expected unsafe directory error")
	}
}

func TestRejectsSymlinkEscape(t *testing.T) {
	vault := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(vault, "Dossiers")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	connector, err := obsidian.New(obsidian.Config{VaultPath: vault, DossierDir: "Dossiers"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.ReadDossier(context.Background(), connectors.DossierRef{CaseID: "NS-04", CaseName: "NORTHSTAR"})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v, want symlink escape rejection", err)
	}
}

func TestUnconfiguredStatus(t *testing.T) {
	connector, err := obsidian.New(obsidian.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if status := connector.Status(context.Background()); status.State != "unconfigured" {
		t.Fatalf("status = %q", status.State)
	}
}
