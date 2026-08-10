// Package connectors exposes the stable extension contract for Amber Desk integrations.
// Provider-specific implementations should depend on this package rather than HTTP or case storage packages.
package connectors

import internal "amberdesk/internal/connectors"

var (
	ErrNotConfigured   = internal.ErrNotConfigured
	ErrConnectorAbsent = internal.ErrConnectorAbsent
	ErrConflict        = internal.ErrConflict
	ErrEntityAbsent    = internal.ErrEntityAbsent
)

type Metadata = internal.Metadata
type Status = internal.Status
type Info = internal.Info
type DossierRef = internal.DossierRef
type Dossier = internal.Dossier
type DossierWrite = internal.DossierWrite
type TimelineNote = internal.TimelineNote
type TimelineEvent = internal.TimelineEvent
type TimelineSnapshot = internal.TimelineSnapshot
type MapMarker = internal.MapMarker
type MapRoute = internal.MapRoute
type MapSnapshot = internal.MapSnapshot
type RelationshipNode = internal.RelationshipNode
type RelationshipEdge = internal.RelationshipEdge
type RelationshipSnapshot = internal.RelationshipSnapshot
type Connector = internal.Connector
type TimelineConnector = internal.TimelineConnector
type MapConnector = internal.MapConnector
type RelationshipConnector = internal.RelationshipConnector
type Registry = internal.Registry

func NewRegistry(items ...Connector) *Registry {
	return internal.NewRegistry(items...)
}
