# Connector Development

Connectors isolate Amber Desk from external tools and storage systems. A connector owns configuration, health reporting, and translation between a capability-specific API and the external system.

## Contract

Import the stable contract from `amberdesk/pkg/connectors`. Built-in providers live under `internal/connectors/<id>`; external providers may live in their own Go module and are compiled into an Amber Desk distribution:

```go
type Connector interface {
    Metadata() Metadata
    Status(context.Context) Status
    ReadDossier(context.Context, DossierRef) (Dossier, error)
    WriteDossier(context.Context, DossierRef, DossierWrite) (Dossier, error)
}
```

Storage families are optional capability interfaces. A provider may implement `TimelineConnector`, `MapConnector`, both, or neither. The generic HTTP layer selects a connected provider by the capabilities declared in metadata and falls back to memory when no provider is available.

Register the implementation in `main.go` with `connectors.NewRegistry`. The HTTP layer discovers it through the registry; connector-specific filesystem or network logic must not enter `internal/httpapi` or the browser case model.

## Metadata

Every connector must expose:

- A stable lowercase ASCII `id`
- A human-readable name and description
- Explicit capabilities such as `dossier.read`, `timeline.write`, and `map.read`
- Relationship capabilities `relationships.read` and `relationships.write` for clue cards, positions, and sourced threads
- Whether required configuration is present
- A health state: `connected`, `unconfigured`, `offline`, or `error`

Adding capabilities must be backward compatible. Consumers must ignore unknown capabilities and response fields.

## Configuration

Read secrets and machine-specific paths from the environment or a future secret provider. Never place credentials in browser code, committed configuration, query strings, logs, or connector metadata.

Validate configuration at startup when possible. A missing optional connector configuration should leave the connector registered as `unconfigured`; it must not prevent Amber Desk from starting.

## Dossier Semantics

`DossierRef` identifies the current case without exposing the internal store. `DossierWrite.ExpectedModifiedAt` provides optimistic concurrency. Connectors that support external editing must return `connectors.ErrConflict` when the remote version changed after it was read.

Connector writes should be transactional when the external system allows it. The Obsidian implementation writes a temporary file in the destination directory, flushes it, and renames it over the Markdown document.

## Security Requirements

- Treat remote payloads and local vault files as untrusted input.
- Apply strict size limits before decoding or storing content.
- Prevent path traversal and symlink escape for filesystem connectors.
- Use request timeouts for network connectors.
- Validate outbound URL schemes and destinations.
- Return structured errors without leaking secrets or full local paths.
- Test unconfigured, unavailable, conflict, and malformed-input states.

## HTTP Surface

Generic dossier routes are exposed as:

```text
GET /api/integrations
GET /api/integrations/{id}/dossier
PUT /api/integrations/{id}/dossier
```

Timeline and map providers use the shared `/api/timeline` and `/api/map` route families. Browser code never imports an Obsidian-specific API.

Do not add a connector-specific route when an existing capability route can represent the operation. New capability families should receive a generic route and shared request/response types first.

## Pull Request Checklist

- Connector package has no unrelated dependencies.
- Metadata and capabilities are documented.
- Configuration has a safe unconfigured state.
- Secrets never reach the browser.
- Read, write, error, and conflict paths have tests.
- README and `.env.example` include new configuration.
- UI remains usable when the connector is absent or offline.
