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
	MaxRelationshipAttachmentSize     = internal.MaxRelationshipAttachmentSize
	MaxRelationshipAttachmentsPerNode = internal.MaxRelationshipAttachmentsPerNode
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
type Connector = internal.Connector
type TimelineConnector = internal.TimelineConnector
type TimelineDeleteConnector = internal.TimelineDeleteConnector
type MapConnector = internal.MapConnector
type RelationshipConnector = internal.RelationshipConnector
type RelationshipAttachmentConnector = internal.RelationshipAttachmentConnector
type WorkspaceStateConnector = internal.WorkspaceStateConnector
type CaseStoreConnector = internal.CaseStoreConnector
type Registry = internal.Registry

func NewRegistry(items ...Connector) *Registry {
	return internal.NewRegistry(items...)
}
