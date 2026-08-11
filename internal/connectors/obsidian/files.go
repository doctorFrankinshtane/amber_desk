package obsidian

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"amberdesk/pkg/connectors"
	"gopkg.in/yaml.v3"
)

func documentID(prefix string) (string, error) {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate document id: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}

type documentHeader struct {
	Version int    `yaml:"version"`
	Kind    string `yaml:"kind"`
	CaseID  string `yaml:"case_id"`
}

const (
	documentSchemaVersion        = 1
	documentKindTimelineEvent    = "timeline_event"
	documentKindMapMarker        = "map_marker"
	documentKindMapRoute         = "map_route"
	documentKindRelationshipNode = "relationship_node"
	documentKindRelationshipEdge = "relationship_edge"
	documentKindChecklist        = "investigation_checklist"
)

func newDocumentHeader(kind, caseID string) documentHeader {
	return documentHeader{Version: documentSchemaVersion, Kind: kind, CaseID: caseID}
}

func (header documentHeader) valid(kind, caseID string) bool {
	return header.Version == documentSchemaVersion && header.Kind == kind && header.CaseID == caseID
}

func (c *Connector) caseDirectory(ref connectors.DossierRef, create bool, parts ...string) (string, error) {
	if !c.configured {
		return "", connectors.ErrNotConfigured
	}
	if info, err := os.Stat(c.vaultPath); err != nil || !info.IsDir() {
		return "", errors.New("obsidian vault is unavailable")
	}
	casesRoot := c.casesRoot()
	root := filepath.Join(casesRoot, sanitize(ref.CaseID))
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		if legacy, found, findErr := findLegacyCaseEntry(casesRoot, ref.CaseID, true); findErr != nil {
			return "", findErr
		} else if found {
			root = legacy
		}
	}
	path := filepath.Join(append([]string{root}, parts...)...)
	if create {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", fmt.Errorf("create case directory: %w", err)
		}
	}
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}

func findLegacyCaseEntry(directory, caseID string, directoryOnly bool) (string, bool, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect legacy case paths: %w", err)
	}
	prefix := sanitize(caseID) + "-"
	for _, entry := range entries {
		if directoryOnly != entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		return filepath.Join(directory, entry.Name()), true, nil
	}
	return "", false, nil
}

func marshalNote(value any, body string) ([]byte, error) {
	frontmatter, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode note metadata: %w", err)
	}
	return []byte("---\n" + string(frontmatter) + "---\n\n" + strings.TrimSpace(body) + "\n"), nil
}

func unmarshalNote(data []byte, target any) error {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return errors.New("note is missing YAML frontmatter")
	}
	remainder := text[4:]
	end := strings.Index(remainder, "\n---\n")
	if end < 0 {
		return errors.New("note frontmatter is not terminated")
	}
	if err := yaml.Unmarshal([]byte(remainder[:end]), target); err != nil {
		return fmt.Errorf("decode note metadata: %w", err)
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".amberdesk-*.tmp")
	if err != nil {
		return fmt.Errorf("create note transaction: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure note transaction: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write note transaction: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync note transaction: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close note transaction: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit note transaction: %w", err)
	}
	return nil
}
