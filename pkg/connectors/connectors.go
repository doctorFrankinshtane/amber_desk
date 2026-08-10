// Package connectors exposes the stable extension contract for Amber Desk integrations.
// Provider-specific implementations should depend on this package rather than HTTP or case storage packages.
package connectors

import internal "amberdesk/internal/connectors"

var (
	ErrNotConfigured   = internal.ErrNotConfigured
	ErrConnectorAbsent = internal.ErrConnectorAbsent
	ErrConflict        = internal.ErrConflict
)

type Metadata = internal.Metadata
type Status = internal.Status
type Info = internal.Info
type DossierRef = internal.DossierRef
type Dossier = internal.Dossier
type DossierWrite = internal.DossierWrite
type Connector = internal.Connector
type Registry = internal.Registry

func NewRegistry(items ...Connector) *Registry {
	return internal.NewRegistry(items...)
}
