package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"amberdesk/pkg/connectors"
)

type checklistDocument struct {
	AmberDesk         documentHeader              `yaml:"amber_desk"`
	Version           int                         `yaml:"checklist_version"`
	RecommendedTaskID string                      `yaml:"recommended_task_id,omitempty"`
	Phases            []connectors.ChecklistPhase `yaml:"phases"`
}

func (c *Connector) ReadChecklist(_ context.Context, ref connectors.DossierRef) (connectors.ChecklistSnapshot, error) {
	path, err := c.checklistPath(ref, false)
	if err != nil {
		return connectors.ChecklistSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return connectors.ChecklistSnapshot{}, connectors.ErrEntityAbsent
	}
	if err != nil {
		return connectors.ChecklistSnapshot{}, fmt.Errorf("read checklist: %w", err)
	}
	var document checklistDocument
	if err := unmarshalNote(data, &document); err != nil {
		return connectors.ChecklistSnapshot{}, fmt.Errorf("parse checklist: %w", err)
	}
	if !document.AmberDesk.valid(documentKindChecklist, ref.CaseID) {
		return connectors.ChecklistSnapshot{}, connectors.ErrEntityAbsent
	}
	return connectors.ChecklistSnapshot{Version: document.Version, RecommendedTaskID: document.RecommendedTaskID, Phases: document.Phases, Backend: ID}, nil
}

func (c *Connector) WriteChecklist(_ context.Context, ref connectors.DossierRef, snapshot connectors.ChecklistSnapshot) (connectors.ChecklistSnapshot, error) {
	path, err := c.checklistPath(ref, true)
	if err != nil {
		return connectors.ChecklistSnapshot{}, err
	}
	document := checklistDocument{AmberDesk: newDocumentHeader(documentKindChecklist, ref.CaseID), Version: snapshot.Version, RecommendedTaskID: snapshot.RecommendedTaskID, Phases: snapshot.Phases}
	data, err := marshalNote(document, checklistBody(snapshot))
	if err != nil {
		return connectors.ChecklistSnapshot{}, err
	}
	if err := atomicWrite(path, data); err != nil {
		return connectors.ChecklistSnapshot{}, err
	}
	snapshot.Backend = ID
	return snapshot, nil
}

func (c *Connector) checklistPath(ref connectors.DossierRef, create bool) (string, error) {
	directory, err := c.caseDirectory(ref, create)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, "Checklist.md")
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}

func checklistBody(snapshot connectors.ChecklistSnapshot) string {
	var body strings.Builder
	body.WriteString("# Investigation Checklist\n")
	for _, phase := range snapshot.Phases {
		body.WriteString("\n## " + phase.Title + "\n\n")
		for _, task := range phase.Tasks {
			mark := " "
			if task.Status == "done" {
				mark = "x"
			} else if task.Status == "skipped" {
				mark = "~"
			}
			body.WriteString("- [" + mark + "] " + task.Title)
			if task.Note != "" {
				body.WriteString(" — " + strings.ReplaceAll(task.Note, "\n", " "))
			}
			body.WriteString("\n")
		}
	}
	body.WriteString("\n## Amber Desk\n\nStructured checklist metadata is stored in YAML frontmatter.\n")
	return body.String()
}
