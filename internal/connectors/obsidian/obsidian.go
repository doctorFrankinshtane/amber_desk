package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"amberdesk/pkg/connectors"
)

const ID = "obsidian"

var unsafeFilename = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type Config struct {
	VaultPath  string
	DossierDir string
}

type Connector struct {
	vaultPath    string
	dossierDir   string
	configured   bool
	attachmentMu sync.RWMutex
}

func New(config Config) (*Connector, error) {
	directory := strings.TrimSpace(config.DossierDir)
	if directory == "" {
		directory = filepath.Join("Amber Desk", "Dossiers")
	}
	cleanDirectory := filepath.Clean(directory)
	if filepath.IsAbs(cleanDirectory) || cleanDirectory == ".." || strings.HasPrefix(cleanDirectory, ".."+string(filepath.Separator)) {
		return nil, errors.New("obsidian dossier directory must be relative to the vault")
	}

	vault := strings.TrimSpace(config.VaultPath)
	if vault != "" {
		absolute, err := filepath.Abs(vault)
		if err != nil {
			return nil, fmt.Errorf("resolve obsidian vault: %w", err)
		}
		vault = filepath.Clean(absolute)
	}

	return &Connector{vaultPath: vault, dossierDir: cleanDirectory, configured: vault != ""}, nil
}

func (c *Connector) Metadata() connectors.Metadata {
	return connectors.Metadata{
		ID: ID, Name: "Obsidian", Description: "Markdown dossier synchronization with a local Obsidian vault",
		Capabilities: []string{"dossier.read", "dossier.write", "timeline.read", "timeline.write", "timeline.delete", "map.read", "map.write", "relationships.read", "relationships.write", "relationships.attachments.read", "relationships.attachments.write", "relationships.attachments.delete", "checklist.read", "checklist.write", "workspace.state.read", "workspace.state.write", "cases.read", "cases.write", "cases.delete"}, Configured: c.configured,
	}
}

func (c *Connector) Status(_ context.Context) connectors.Status {
	if !c.configured {
		return connectors.Status{State: "unconfigured", Message: "OBSIDIAN_VAULT is not set"}
	}
	info, err := os.Stat(c.vaultPath)
	if err != nil {
		return connectors.Status{State: "offline", Message: "vault path is unavailable"}
	}
	if !info.IsDir() {
		return connectors.Status{State: "error", Message: "vault path is not a directory"}
	}
	return connectors.Status{State: "connected", Message: "vault is available"}
}

func (c *Connector) ReadDossier(_ context.Context, ref connectors.DossierRef) (connectors.Dossier, error) {
	path, relative, err := c.dossierPath(ref, false)
	if err != nil {
		return connectors.Dossier{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return connectors.Dossier{Content: defaultDossier(ref), Path: relative, Exists: false}, nil
	}
	if err != nil {
		return connectors.Dossier{}, fmt.Errorf("read obsidian dossier: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return connectors.Dossier{}, fmt.Errorf("stat obsidian dossier: %w", err)
	}
	return connectors.Dossier{Content: string(data), Path: relative, Exists: true, ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano)}, nil
}

func (c *Connector) WriteDossier(_ context.Context, ref connectors.DossierRef, write connectors.DossierWrite) (connectors.Dossier, error) {
	path, relative, err := c.dossierPath(ref, true)
	if err != nil {
		return connectors.Dossier{}, err
	}
	directory := filepath.Dir(path)
	if write.ExpectedModifiedAt != "" {
		info, statErr := os.Stat(path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return connectors.Dossier{}, fmt.Errorf("stat dossier version: %w", statErr)
		}
		if statErr == nil && info.ModTime().UTC().Format(time.RFC3339Nano) != write.ExpectedModifiedAt {
			return connectors.Dossier{}, connectors.ErrConflict
		}
	}
	temporary, err := os.CreateTemp(directory, ".amberdesk-*.tmp")
	if err != nil {
		return connectors.Dossier{}, fmt.Errorf("create dossier transaction: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return connectors.Dossier{}, fmt.Errorf("secure dossier transaction: %w", err)
	}
	if _, err := temporary.WriteString(write.Content); err != nil {
		temporary.Close()
		return connectors.Dossier{}, fmt.Errorf("write dossier transaction: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return connectors.Dossier{}, fmt.Errorf("sync dossier transaction: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return connectors.Dossier{}, fmt.Errorf("close dossier transaction: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return connectors.Dossier{}, fmt.Errorf("commit dossier transaction: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return connectors.Dossier{}, fmt.Errorf("stat saved dossier: %w", err)
	}
	return connectors.Dossier{Content: write.Content, Path: relative, Exists: true, ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano)}, nil
}

func (c *Connector) dossierPath(ref connectors.DossierRef, createDirectory bool) (string, string, error) {
	if !c.configured {
		return "", "", connectors.ErrNotConfigured
	}
	vaultInfo, err := os.Stat(c.vaultPath)
	if err != nil || !vaultInfo.IsDir() {
		return "", "", errors.New("obsidian vault is unavailable")
	}
	directory := filepath.Join(c.vaultPath, c.dossierDir)
	if createDirectory {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return "", "", fmt.Errorf("create dossier directory: %w", err)
		}
	}
	if err := ensureInside(c.vaultPath, directory); err != nil {
		return "", "", err
	}
	path := filepath.Join(directory, sanitize(ref.CaseID)+".md")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if legacy, found, findErr := findLegacyCaseEntry(directory, ref.CaseID, false); findErr != nil {
			return "", "", findErr
		} else if found && strings.EqualFold(filepath.Ext(legacy), ".md") {
			path = legacy
		}
	}
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(c.vaultPath, path)
	if err != nil {
		return "", "", fmt.Errorf("resolve dossier path: %w", err)
	}
	return path, filepath.ToSlash(relative), nil
}

func ensureInside(base, target string) error {
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("dossier path escapes the configured vault")
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return errors.New("cannot resolve configured vault")
	}
	existing := target
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("cannot inspect dossier path")
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return errors.New("cannot resolve dossier path")
		}
		existing = parent
	}
	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return errors.New("cannot resolve dossier path")
	}
	resolvedRelative, err := filepath.Rel(resolvedBase, resolvedExisting)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return errors.New("dossier path escapes the configured vault through a symlink")
	}
	return nil
}

func sanitize(value string) string {
	value = strings.Trim(unsafeFilename.ReplaceAllString(value, "-"), "-")
	if value == "" {
		return "untitled"
	}
	return value
}

func defaultDossier(ref connectors.DossierRef) string {
	return fmt.Sprintf(`---
amber_desk:
  version: 1
  case_id: %s
  connector: obsidian
tags:
  - amber-desk
  - dossier
---

# %s

## Subject

**%s**

## Executive summary


## Known identifiers


## Timeline


## Evidence assessment


## Open questions

`, strconv.Quote(ref.CaseID), ref.CaseName, ref.SubjectName)
}
