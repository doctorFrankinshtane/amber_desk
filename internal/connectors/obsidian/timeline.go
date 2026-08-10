package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"amberdesk/pkg/connectors"
)

type timelineDocument struct {
	AmberDesk                documentHeader `yaml:"amber_desk"`
	connectors.TimelineEvent `yaml:",inline"`
}

func (c *Connector) ListTimeline(_ context.Context, ref connectors.DossierRef) ([]connectors.TimelineEvent, error) {
	directory, err := c.caseDirectory(ref, false, "Timeline")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []connectors.TimelineEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read timeline directory: %w", err)
	}
	events := make([]connectors.TimelineEvent, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read timeline note: %w", err)
		}
		var document timelineDocument
		if err := unmarshalNote(data, &document); err != nil {
			return nil, fmt.Errorf("parse timeline note %s: %w", entry.Name(), err)
		}
		if document.AmberDesk.Kind != "timeline_event" || document.AmberDesk.CaseID != ref.CaseID {
			continue
		}
		events = append(events, document.TimelineEvent)
	}
	sort.SliceStable(events, func(i, j int) bool {
		left, right := events[i], events[j]
		leftTime, leftErr := time.Parse(time.RFC3339, left.OccurredAt)
		rightTime, rightErr := time.Parse(time.RFC3339, right.OccurredAt)
		if leftErr == nil && rightErr == nil && !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		if left.OccurredAt != right.OccurredAt {
			return left.OccurredAt > right.OccurredAt
		}
		return left.ID > right.ID
	})
	return events, nil
}

func (c *Connector) BootstrapTimeline(ctx context.Context, ref connectors.DossierRef, events []connectors.TimelineEvent) ([]connectors.TimelineEvent, error) {
	for _, event := range events {
		path, err := c.timelinePath(ref, event.ID, true)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect timeline note: %w", err)
		}
		if err := c.writeTimelineEvent(ref, event); err != nil {
			return nil, err
		}
	}
	return c.ListTimeline(ctx, ref)
}

func (c *Connector) CreateTimelineEvent(_ context.Context, ref connectors.DossierRef, event connectors.TimelineEvent) (connectors.TimelineEvent, error) {
	now := time.Now().UTC()
	if event.ID == "" {
		var err error
		event.ID, err = documentID("EV")
		if err != nil {
			return connectors.TimelineEvent{}, err
		}
	}
	if event.OccurredAt == "" {
		event.OccurredAt = now.Format(time.RFC3339)
	}
	if event.Date == "" {
		event.Date = strings.ToUpper(now.Format("02 Jan"))
	}
	if event.Time == "" {
		event.Time = now.Format("15:04")
	}
	if event.Status == "" {
		event.Status = "pending"
	}
	if event.Confidence < 0 || event.Confidence > 100 || strings.TrimSpace(event.Title) == "" {
		return connectors.TimelineEvent{}, errors.New("timeline title and confidence from 0 to 100 are required")
	}
	path, err := c.timelinePath(ref, event.ID, true)
	if err != nil {
		return connectors.TimelineEvent{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return connectors.TimelineEvent{}, connectors.ErrConflict
	}
	if err := c.writeTimelineEvent(ref, event); err != nil {
		return connectors.TimelineEvent{}, err
	}
	return event, nil
}

func (c *Connector) SetTimelineStatus(_ context.Context, ref connectors.DossierRef, eventID, status string) (connectors.TimelineEvent, error) {
	if status != "pending" && status != "verified" {
		return connectors.TimelineEvent{}, errors.New("status must be pending or verified")
	}
	event, err := c.readTimelineEvent(ref, eventID)
	if err != nil {
		return connectors.TimelineEvent{}, err
	}
	event.Status = status
	if err := c.writeTimelineEvent(ref, event); err != nil {
		return connectors.TimelineEvent{}, err
	}
	return event, nil
}

func (c *Connector) AddTimelineNote(_ context.Context, ref connectors.DossierRef, eventID string, note connectors.TimelineNote) (connectors.TimelineEvent, error) {
	note.Text = strings.TrimSpace(note.Text)
	if note.Text == "" {
		return connectors.TimelineEvent{}, errors.New("note text is required")
	}
	event, err := c.readTimelineEvent(ref, eventID)
	if err != nil {
		return connectors.TimelineEvent{}, err
	}
	event.Notes = append(event.Notes, note)
	if err := c.writeTimelineEvent(ref, event); err != nil {
		return connectors.TimelineEvent{}, err
	}
	return event, nil
}

func (c *Connector) readTimelineEvent(ref connectors.DossierRef, eventID string) (connectors.TimelineEvent, error) {
	path, err := c.timelinePath(ref, eventID, false)
	if err != nil {
		return connectors.TimelineEvent{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return connectors.TimelineEvent{}, connectors.ErrEntityAbsent
	}
	if err != nil {
		return connectors.TimelineEvent{}, fmt.Errorf("read timeline event: %w", err)
	}
	var document timelineDocument
	if err := unmarshalNote(data, &document); err != nil {
		return connectors.TimelineEvent{}, err
	}
	return document.TimelineEvent, nil
}

func (c *Connector) writeTimelineEvent(ref connectors.DossierRef, event connectors.TimelineEvent) error {
	path, err := c.timelinePath(ref, event.ID, true)
	if err != nil {
		return err
	}
	document := timelineDocument{AmberDesk: documentHeader{Version: 1, Kind: "timeline_event", CaseID: ref.CaseID}, TimelineEvent: event}
	body := "# " + event.Title + "\n\n" + event.Summary + "\n\n## Amber Desk\n\nStructured fields are stored in YAML frontmatter."
	data, err := marshalNote(document, body)
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func (c *Connector) timelinePath(ref connectors.DossierRef, eventID string, create bool) (string, error) {
	directory, err := c.caseDirectory(ref, create, "Timeline")
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, sanitize(eventID)+".md")
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}
