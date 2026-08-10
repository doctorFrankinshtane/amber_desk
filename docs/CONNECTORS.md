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

Storage families are optional capability interfaces. A provider may implement `TimelineConnector`, `TimelineDeleteConnector`, `MapConnector`, `RelationshipConnector`, `RelationshipAttachmentConnector`, `WorkspaceStateConnector`, or `CaseStoreConnector`. The generic HTTP layer selects a connected provider by the capabilities declared in metadata and falls back to memory when no provider is available.

`WorkspaceStateConnector` stores small opaque snapshots such as `active-case`. It keeps provider packages independent from the internal case model while allowing a workspace to survive process restarts. Providers must validate state keys, keep state local to their configured storage root, and write snapshots atomically.

`CaseStoreConnector` is the multi-dossier lifecycle contract. It lists case summaries, reads and writes opaque snapshots, stores the active-case pointer, and moves complete cases to provider-local trash. `TimelineDeleteConnector` is separate from `TimelineConnector` so existing timeline providers remain source compatible when deletion is unavailable.

Register the implementation in `main.go` with `connectors.NewRegistry`. The HTTP layer discovers it through the registry; connector-specific filesystem or network logic must not enter `internal/httpapi` or the browser case model.

## Metadata

Every connector must expose:

- A stable lowercase ASCII `id`
- A human-readable name and description
- Explicit capabilities such as `dossier.read`, `timeline.write`, and `map.read`
- Relationship capabilities `relationships.read` and `relationships.write` for clue cards, positions, and sourced threads
- Attachment capabilities `relationships.attachments.read`, `relationships.attachments.write`, and `relationships.attachments.delete` for card-local evidence files
- Workspace state capabilities `workspace.state.read` and `workspace.state.write` when the provider can restore the active workspace
- Case lifecycle capabilities `cases.read`, `cases.write`, and `cases.delete`
- `timeline.delete` only when event deletion uses safe provider-side semantics
- Whether required configuration is present
- A health state: `connected`, `unconfigured`, `offline`, or `error`

Adding capabilities must be backward compatible. Consumers must ignore unknown capabilities and response fields.

## Configuration

Read secrets and machine-specific paths from the environment or a future secret provider. Never place credentials in browser code, committed configuration, query strings, logs, or connector metadata.

Validate configuration at startup when possible. A missing optional connector configuration should leave the connector registered as `unconfigured`; it must not prevent Amber Desk from starting.

## Dossier Semantics

`DossierRef` identifies the current case without exposing the internal store. `DossierWrite.ExpectedModifiedAt` provides optimistic concurrency. Connectors that support external editing must return `connectors.ErrConflict` when the remote version changed after it was read.

The HTTP dossier response includes `caseId`, and clients must return it on `PUT`. The backend rejects a write when that value no longer matches the active case, preventing a stale editor opened on one investigation from overwriting another investigation's dossier.

Connector writes should be transactional when the external system allows it. The Obsidian implementation writes a temporary file in the destination directory, flushes it, and renames it over the Markdown document.

Attachment providers receive server-generated IDs and bounded content. They must validate node and attachment IDs again, keep original filenames as metadata rather than path components, enforce `MaxRelationshipAttachmentSize` and `MaxRelationshipAttachmentsPerNode`, and verify the SHA-256 digest when reading persisted content. Deletion should use recoverable local trash where the storage backend supports it.

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
GET /api/cases
PUT /api/cases/active
DELETE /api/cases/{id}
DELETE /api/events/{id}
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
