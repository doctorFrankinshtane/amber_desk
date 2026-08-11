package obsidian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"amberdesk/pkg/connectors"
)

var caseIDPattern = regexp.MustCompile(`^CASE-[A-Z0-9]{8}$`)

const caseIndexSchemaVersion = 1

type caseIndex struct {
	Version int                      `json:"version"`
	Cases   []connectors.CaseSummary `json:"cases"`
}

type activeCasePointer struct {
	CaseID string `json:"caseId"`
}

func (c *Connector) ListCases(_ context.Context) ([]connectors.CaseSummary, error) {
	c.caseMu.Lock()
	defer c.caseMu.Unlock()

	index, err := c.loadCaseIndex()
	if err != nil {
		return nil, err
	}
	active, err := c.activeCaseID()
	if err != nil && !errors.Is(err, connectors.ErrEntityAbsent) {
		return nil, err
	}
	result := append([]connectors.CaseSummary{}, index.Cases...)
	for i := range result {
		result[i].Active = result[i].ID == active
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Active != result[j].Active {
			return result[i].Active
		}
		return result[i].UpdatedAt > result[j].UpdatedAt
	})
	return result, nil
}

func (c *Connector) ReadCase(_ context.Context, caseID string) ([]byte, error) {
	c.caseMu.Lock()
	defer c.caseMu.Unlock()
	return c.readCase(caseID)
}

func (c *Connector) readCase(caseID string) ([]byte, error) {
	path, err := c.caseSnapshotPath(caseID, false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, connectors.ErrEntityAbsent
	}
	if err != nil {
		return nil, fmt.Errorf("read case snapshot: %w", err)
	}
	return data, nil
}

func (c *Connector) WriteCase(_ context.Context, summary connectors.CaseSummary, data []byte) error {
	c.caseMu.Lock()
	defer c.caseMu.Unlock()

	if !caseIDPattern.MatchString(summary.ID) || summary.Name == "" || summary.Subject == "" {
		return errors.New("invalid case summary")
	}
	path, err := c.caseSnapshotPath(summary.ID, true)
	if err != nil {
		return err
	}
	index, err := c.loadCaseIndex()
	if err != nil {
		return err
	}
	previous, previousErr := os.ReadFile(path)
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return fmt.Errorf("read previous case snapshot: %w", previousErr)
	}
	if err := atomicWrite(path, data); err != nil {
		return fmt.Errorf("write case snapshot: %w", err)
	}
	summary.Active = false
	found := false
	for i := range index.Cases {
		if index.Cases[i].ID == summary.ID {
			index.Cases[i] = summary
			found = true
			break
		}
	}
	if !found {
		index.Cases = append(index.Cases, summary)
	}
	if err := c.writeCaseIndex(index); err != nil {
		if previousErr == nil {
			_ = atomicWrite(path, previous)
		} else {
			_ = os.Remove(path)
		}
		return err
	}
	return nil
}

func (c *Connector) SetActiveCase(_ context.Context, caseID string) error {
	c.caseMu.Lock()
	defer c.caseMu.Unlock()
	return c.setActiveCase(caseID)
}

func (c *Connector) setActiveCase(caseID string) error {
	if caseID != "" {
		index, err := c.loadCaseIndex()
		if err != nil {
			return err
		}
		found := false
		for _, item := range index.Cases {
			if item.ID == caseID {
				found = true
				break
			}
		}
		if !found {
			return connectors.ErrEntityAbsent
		}
	}
	root, err := c.caseStateRoot(true)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(activeCasePointer{CaseID: caseID})
	return atomicWrite(filepath.Join(root, "active-case-id.json"), data)
}

func (c *Connector) ActiveCaseID(_ context.Context) (string, error) {
	c.caseMu.Lock()
	defer c.caseMu.Unlock()
	return c.activeCaseID()
}

func (c *Connector) TrashCase(_ context.Context, caseID string) (string, error) {
	c.caseMu.Lock()
	defer c.caseMu.Unlock()

	if !caseIDPattern.MatchString(caseID) {
		return "", connectors.ErrEntityAbsent
	}
	index, err := c.loadCaseIndex()
	if err != nil {
		return "", err
	}
	position := -1
	for i, item := range index.Cases {
		if item.ID == caseID {
			position = i
			break
		}
	}
	if position < 0 {
		return "", connectors.ErrEntityAbsent
	}
	originalIndex := caseIndex{Version: index.Version, Cases: append([]connectors.CaseSummary{}, index.Cases...)}
	remaining := append([]connectors.CaseSummary{}, index.Cases[:position]...)
	remaining = append(remaining, index.Cases[position+1:]...)
	sort.SliceStable(remaining, func(i, j int) bool { return remaining[i].UpdatedAt > remaining[j].UpdatedAt })
	next := ""
	if len(remaining) > 0 {
		next = remaining[0].ID
		if _, err := c.readCase(next); err != nil {
			return "", fmt.Errorf("validate next case snapshot: %w", err)
		}
	}

	trashRoot := filepath.Join(c.workspaceRoot(), ".trash", caseID+"-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	for _, root := range []string{c.workspaceRoot(), c.casesRoot(), c.dossierRoot(), c.stateRoot()} {
		if err := ensureInside(c.vaultPath, root); err != nil {
			return "", err
		}
	}
	type move struct{ source, target string }
	moves := []move{}
	appendMatches := func(directory, destination string) error {
		entries, readErr := os.ReadDir(directory)
		if errors.Is(readErr, os.ErrNotExist) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			name := entry.Name()
			base := strings.TrimSuffix(name, filepath.Ext(name))
			if base == caseID || strings.HasPrefix(base, caseID+"-") {
				moves = append(moves, move{filepath.Join(directory, name), filepath.Join(trashRoot, destination, name)})
			}
		}
		return nil
	}
	if err := appendMatches(c.casesRoot(), "Cases"); err != nil {
		return "", fmt.Errorf("inspect case files: %w", err)
	}
	if err := appendMatches(c.dossierRoot(), "Dossiers"); err != nil {
		return "", fmt.Errorf("inspect dossier files: %w", err)
	}
	snapshot, err := c.caseSnapshotPath(caseID, false)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(snapshot); err == nil {
		moves = append(moves, move{snapshot, filepath.Join(trashRoot, ".state", "cases", filepath.Base(snapshot))})
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect case snapshot: %w", err)
	}

	completed := []move{}
	rollback := func() {
		for i := len(completed) - 1; i >= 0; i-- {
			_ = os.MkdirAll(filepath.Dir(completed[i].source), 0o755)
			_ = os.Rename(completed[i].target, completed[i].source)
		}
		_ = os.RemoveAll(trashRoot)
	}
	for _, item := range moves {
		if err := os.MkdirAll(filepath.Dir(item.target), 0o755); err != nil {
			rollback()
			return "", fmt.Errorf("prepare case trash: %w", err)
		}
		if err := os.Rename(item.source, item.target); err != nil {
			rollback()
			return "", fmt.Errorf("trash case files: %w", err)
		}
		completed = append(completed, item)
	}

	index.Cases = remaining
	if err := c.writeCaseIndex(index); err != nil {
		rollback()
		return "", err
	}
	if err := c.setActiveCase(next); err != nil {
		rollback()
		_ = c.writeCaseIndex(originalIndex)
		_ = c.setActiveCase(caseID)
		return "", err
	}
	return next, nil
}

func (c *Connector) loadCaseIndex() (caseIndex, error) {
	root, err := c.caseStateRoot(false)
	if err != nil {
		return caseIndex{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "cases-index.json"))
	if errors.Is(err, os.ErrNotExist) {
		return c.migrateLegacyCaseState()
	}
	if err != nil {
		return caseIndex{}, fmt.Errorf("read case index: %w", err)
	}
	var index caseIndex
	if err := json.Unmarshal(data, &index); err != nil || index.Version != caseIndexSchemaVersion {
		return caseIndex{}, errors.New("case index is invalid")
	}
	if index.Cases == nil {
		index.Cases = []connectors.CaseSummary{}
	}
	return index, nil
}

func (c *Connector) migrateLegacyCaseState() (caseIndex, error) {
	index := caseIndex{Version: caseIndexSchemaVersion, Cases: []connectors.CaseSummary{}}
	root, err := c.caseStateRoot(true)
	if err != nil {
		return index, err
	}
	legacyPath := filepath.Join(root, "active-case.json")
	legacy, err := os.ReadFile(legacyPath)
	migrated := false
	if err == nil {
		var value struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			UpdatedAt string `json:"updatedAt"`
			Subject   struct {
				Codename string `json:"codename"`
			} `json:"subject"`
		}
		if json.Unmarshal(legacy, &value) == nil && caseIDPattern.MatchString(value.ID) && value.Subject.Codename != "UNASSIGNED" {
			summary := connectors.CaseSummary{ID: value.ID, Name: value.Name, Subject: value.Subject.Codename, Status: value.Status, UpdatedAt: value.UpdatedAt}
			path, pathErr := c.caseSnapshotPath(value.ID, true)
			if pathErr != nil {
				return index, pathErr
			}
			if err := atomicWrite(path, legacy); err != nil {
				return index, err
			}
			index.Cases = append(index.Cases, summary)
			migrated = true
			pointer, _ := json.Marshal(activeCasePointer{CaseID: value.ID})
			if err := atomicWrite(filepath.Join(root, "active-case-id.json"), pointer); err != nil {
				return index, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return index, fmt.Errorf("read legacy case state: %w", err)
	}
	if err := c.writeCaseIndex(index); err != nil {
		return index, err
	}
	if migrated {
		if err := os.Rename(legacyPath, filepath.Join(root, "active-case.legacy.json")); err != nil {
			return index, fmt.Errorf("retire legacy case state: %w", err)
		}
	}
	return index, nil
}

func (c *Connector) writeCaseIndex(index caseIndex) error {
	root, err := c.caseStateRoot(true)
	if err != nil {
		return err
	}
	index.Version = 1
	if index.Cases == nil {
		index.Cases = []connectors.CaseSummary{}
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode case index: %w", err)
	}
	if err := atomicWrite(filepath.Join(root, "cases-index.json"), data); err != nil {
		return fmt.Errorf("write case index: %w", err)
	}
	return nil
}

func (c *Connector) activeCaseID() (string, error) {
	root, err := c.caseStateRoot(false)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, "active-case-id.json"))
	if errors.Is(err, os.ErrNotExist) {
		if _, migrateErr := c.loadCaseIndex(); migrateErr != nil {
			return "", migrateErr
		}
		data, err = os.ReadFile(filepath.Join(root, "active-case-id.json"))
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", connectors.ErrEntityAbsent
	}
	if err != nil {
		return "", fmt.Errorf("read active case pointer: %w", err)
	}
	var pointer activeCasePointer
	if json.Unmarshal(data, &pointer) != nil || (pointer.CaseID != "" && !caseIDPattern.MatchString(pointer.CaseID)) {
		return "", errors.New("active case pointer is invalid")
	}
	if pointer.CaseID == "" {
		return "", connectors.ErrEntityAbsent
	}
	return pointer.CaseID, nil
}

func (c *Connector) caseSnapshotPath(caseID string, create bool) (string, error) {
	if !caseIDPattern.MatchString(caseID) {
		return "", connectors.ErrEntityAbsent
	}
	root, err := c.caseStateRoot(create)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(root, "cases")
	if create {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return "", fmt.Errorf("create case state directory: %w", err)
		}
	}
	path := filepath.Join(directory, caseID+".json")
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}

func (c *Connector) caseStateRoot(create bool) (string, error) {
	if !c.configured {
		return "", connectors.ErrNotConfigured
	}
	if info, err := os.Stat(c.vaultPath); err != nil || !info.IsDir() {
		return "", errors.New("obsidian vault is unavailable")
	}
	root := c.stateRoot()
	if create {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", fmt.Errorf("create case state root: %w", err)
		}
	}
	if err := ensureInside(c.vaultPath, root); err != nil {
		return "", err
	}
	return root, nil
}
