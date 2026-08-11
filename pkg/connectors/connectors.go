// Package connectors exposes the stable extension contract for Amber Desk integrations.
// Provider-specific implementations should depend on this package rather than HTTP or case storage packages.
package connectors

import internal "amberdesk/internal/connectors"

var (
	ErrNotConfigured   = internal.ErrNotConfigured
	ErrConnectorAbsent = internal.ErrConnectorAbsent
	ErrConflict        = internal.ErrConflict
	ErrEntityAbsent    = internal.ErrEntityAbsent
	ErrInvalidFilename = internal.ErrInvalidFilename
	ErrAttachmentLimit = internal.ErrAttachmentLimit
	ErrAttachmentLarge = internal.ErrAttachmentLarge
)

const (
	BackendMemory                           = internal.BackendMemory
	StateConnected                          = internal.StateConnected
	StateUnconfigured                       = internal.StateUnconfigured
	StateOffline                            = internal.StateOffline
	StateError                              = internal.StateError
	CapabilityDossierRead                   = internal.CapabilityDossierRead
	CapabilityDossierWrite                  = internal.CapabilityDossierWrite
	CapabilityTimelineRead                  = internal.CapabilityTimelineRead
	CapabilityTimelineWrite                 = internal.CapabilityTimelineWrite
	CapabilityTimelineDelete                = internal.CapabilityTimelineDelete
	CapabilityMapRead                       = internal.CapabilityMapRead
	CapabilityMapWrite                      = internal.CapabilityMapWrite
	CapabilityRelationshipsRead             = internal.CapabilityRelationshipsRead
	CapabilityRelationshipsWrite            = internal.CapabilityRelationshipsWrite
	CapabilityRelationshipAttachmentsRead   = internal.CapabilityRelationshipAttachmentsRead
	CapabilityRelationshipAttachmentsWrite  = internal.CapabilityRelationshipAttachmentsWrite
	CapabilityRelationshipAttachmentsDelete = internal.CapabilityRelationshipAttachmentsDelete
	CapabilityChecklistRead                 = internal.CapabilityChecklistRead
	CapabilityChecklistWrite                = internal.CapabilityChecklistWrite
	CapabilityWorkspaceStateRead            = internal.CapabilityWorkspaceStateRead
	CapabilityWorkspaceStateWrite           = internal.CapabilityWorkspaceStateWrite
	CapabilityCasesRead                     = internal.CapabilityCasesRead
	CapabilityCasesWrite                    = internal.CapabilityCasesWrite
	CapabilityCasesDelete                   = internal.CapabilityCasesDelete
	MaxRelationshipAttachmentSize           = internal.MaxRelationshipAttachmentSize
	MaxRelationshipAttachmentsPerNode       = internal.MaxRelationshipAttachmentsPerNode
	MaxRelationshipAttachmentFilenameRunes  = internal.MaxRelationshipAttachmentFilenameRunes
	MaxTimelineTitleRunes                   = internal.MaxTimelineTitleRunes
	MaxTimelineSummaryRunes                 = internal.MaxTimelineSummaryRunes
	MaxTimelineSourceRunes                  = internal.MaxTimelineSourceRunes
	MaxTimelineNoteRunes                    = internal.MaxTimelineNoteRunes
)

type Metadata = internal.Metadata
type Status = internal.Status
type Info = internal.Info
type DossierRef = internal.DossierRef
type Dossier = internal.Dossier
type DossierWrite = internal.DossierWrite
type CaseSummary = internal.CaseSummary
type TimelineNote = internal.TimelineNote
type TimelineEvent = internal.TimelineEvent
type TimelineSnapshot = internal.TimelineSnapshot
type MapMarker = internal.MapMarker
type MapRoute = internal.MapRoute
type MapSnapshot = internal.MapSnapshot
type RelationshipNode = internal.RelationshipNode
type RelationshipEdge = internal.RelationshipEdge
type RelationshipSnapshot = internal.RelationshipSnapshot
type RelationshipAttachment = internal.RelationshipAttachment
type ChecklistTask = internal.ChecklistTask
type ChecklistPhase = internal.ChecklistPhase
type ChecklistSnapshot = internal.ChecklistSnapshot
type Connector = internal.Connector
type TimelineConnector = internal.TimelineConnector
type TimelineDeleteConnector = internal.TimelineDeleteConnector
type MapConnector = internal.MapConnector
type RelationshipConnector = internal.RelationshipConnector
type RelationshipAttachmentConnector = internal.RelationshipAttachmentConnector
type ChecklistConnector = internal.ChecklistConnector
type WorkspaceStateConnector = internal.WorkspaceStateConnector
type CaseStoreConnector = internal.CaseStoreConnector
type Registry = internal.Registry

func NewRegistry(items ...Connector) *Registry {
	return internal.NewRegistry(items...)
}

func HasCapability(items []string, expected string) bool {
	return internal.HasCapability(items, expected)
}

func RelationshipImageMediaTypes() []string {
	return internal.RelationshipImageMediaTypes()
}

func IsRelationshipImageMediaType(mediaType string) bool {
	return internal.IsRelationshipImageMediaType(mediaType)
}

func NormalizeTimelineEvent(event TimelineEvent) (TimelineEvent, error) {
	return internal.NormalizeTimelineEvent(event)
}

func NormalizeTimelineNote(note TimelineNote) (TimelineNote, error) {
	return internal.NormalizeTimelineNote(note)
}
