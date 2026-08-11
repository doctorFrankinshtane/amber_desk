package connectors

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	BackendMemory = "memory"

	StateConnected    = "connected"
	StateUnconfigured = "unconfigured"
	StateOffline      = "offline"
	StateError        = "error"

	CapabilityDossierRead                   = "dossier.read"
	CapabilityDossierWrite                  = "dossier.write"
	CapabilityTimelineRead                  = "timeline.read"
	CapabilityTimelineWrite                 = "timeline.write"
	CapabilityTimelineDelete                = "timeline.delete"
	CapabilityMapRead                       = "map.read"
	CapabilityMapWrite                      = "map.write"
	CapabilityRelationshipsRead             = "relationships.read"
	CapabilityRelationshipsWrite            = "relationships.write"
	CapabilityRelationshipAttachmentsRead   = "relationships.attachments.read"
	CapabilityRelationshipAttachmentsWrite  = "relationships.attachments.write"
	CapabilityRelationshipAttachmentsDelete = "relationships.attachments.delete"
	CapabilityChecklistRead                 = "checklist.read"
	CapabilityChecklistWrite                = "checklist.write"
	CapabilityWorkspaceStateRead            = "workspace.state.read"
	CapabilityWorkspaceStateWrite           = "workspace.state.write"
	CapabilityCasesRead                     = "cases.read"
	CapabilityCasesWrite                    = "cases.write"
	CapabilityCasesDelete                   = "cases.delete"

	MaxRelationshipAttachmentSize          int64 = 10 << 20
	MaxRelationshipAttachmentsPerNode            = 20
	MaxRelationshipAttachmentFilenameRunes       = 180

	MaxTimelineTitleRunes       = 140
	MaxTimelineSummaryRunes     = 2000
	MaxTimelineSourceRunes      = 120
	MaxTimelineNoteRunes        = 280
	MaxTimelineTypeRunes        = 40
	MaxTimelineSourceURLRunes   = 4096
	MaxTimelineFingerprintRunes = 240
	MaxTimelineIndicators       = 40
	MaxTimelineIndicatorRunes   = 240
	MaxTimelineNotes            = 100
)

var relationshipImageMediaTypes = [...]string{
	"image/gif",
	"image/jpeg",
	"image/png",
	"image/webp",
}

func HasCapability(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func RelationshipImageMediaTypes() []string {
	result := make([]string, len(relationshipImageMediaTypes))
	copy(result, relationshipImageMediaTypes[:])
	return result
}

func IsRelationshipImageMediaType(mediaType string) bool {
	for _, supported := range relationshipImageMediaTypes {
		if mediaType == supported {
			return true
		}
	}
	return false
}

func NormalizeTimelineEvent(event TimelineEvent) (TimelineEvent, error) {
	event.Indicators = append([]string(nil), event.Indicators...)
	event.Notes = append([]TimelineNote(nil), event.Notes...)
	event.Title = strings.TrimSpace(event.Title)
	event.Summary = strings.TrimSpace(event.Summary)
	event.Source = strings.TrimSpace(event.Source)
	event.Type = strings.TrimSpace(event.Type)
	event.SourceURL = strings.TrimSpace(event.SourceURL)
	event.Fingerprint = strings.TrimSpace(event.Fingerprint)

	if event.Title == "" {
		return TimelineEvent{}, errors.New("timeline event title is required")
	}
	if event.Confidence < 0 || event.Confidence > 100 {
		return TimelineEvent{}, errors.New("timeline confidence must be between 0 and 100")
	}
	if event.Status != "" && event.Status != "pending" && event.Status != "verified" {
		return TimelineEvent{}, errors.New("timeline status must be pending or verified")
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{"title", event.Title, MaxTimelineTitleRunes},
		{"summary", event.Summary, MaxTimelineSummaryRunes},
		{"source", event.Source, MaxTimelineSourceRunes},
		{"type", event.Type, MaxTimelineTypeRunes},
		{"source URL", event.SourceURL, MaxTimelineSourceURLRunes},
		{"fingerprint", event.Fingerprint, MaxTimelineFingerprintRunes},
	} {
		if utf8.RuneCountInString(field.value) > field.limit {
			return TimelineEvent{}, fmt.Errorf("timeline %s exceeds %d characters", field.name, field.limit)
		}
	}
	if len(event.Indicators) > MaxTimelineIndicators {
		return TimelineEvent{}, fmt.Errorf("timeline indicators exceed %d items", MaxTimelineIndicators)
	}
	for index := range event.Indicators {
		event.Indicators[index] = strings.TrimSpace(event.Indicators[index])
		if utf8.RuneCountInString(event.Indicators[index]) > MaxTimelineIndicatorRunes {
			return TimelineEvent{}, fmt.Errorf("timeline indicator exceeds %d characters", MaxTimelineIndicatorRunes)
		}
	}
	if len(event.Notes) > MaxTimelineNotes {
		return TimelineEvent{}, fmt.Errorf("timeline notes exceed %d items", MaxTimelineNotes)
	}
	for index := range event.Notes {
		note, err := NormalizeTimelineNote(event.Notes[index])
		if err != nil {
			return TimelineEvent{}, err
		}
		event.Notes[index] = note
	}
	return event, nil
}

func NormalizeTimelineNote(note TimelineNote) (TimelineNote, error) {
	note.Text = strings.TrimSpace(note.Text)
	if note.Text == "" {
		return TimelineNote{}, errors.New("timeline note is required")
	}
	if utf8.RuneCountInString(note.Text) > MaxTimelineNoteRunes {
		return TimelineNote{}, fmt.Errorf("timeline note exceeds %d characters", MaxTimelineNoteRunes)
	}
	return note, nil
}
