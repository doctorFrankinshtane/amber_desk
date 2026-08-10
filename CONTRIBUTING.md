# Contributing

Amber Desk is designed as a small core with explicit extension boundaries.

## Development Flow

1. Open an issue describing the workflow and capability being added.
2. Keep connector logic in its own package.
3. Add tests for success, validation, offline, and conflict behavior.
4. Run `go test ./...`, `go vet ./...`, and `node --check web/app.js`.
5. Update public configuration and connector documentation.

Use focused commits and avoid committing generated binaries, local vaults, screenshots from real investigations, credentials, or subject data.

## Frontend

All user-facing interface strings belong in `web/i18n.js`. English is the fallback language. New UI must remain keyboard accessible, fit the mobile tab layout, and expose explicit loading, empty, offline, and conflict states.

## Backend

Prefer the Go standard library. Shared contracts belong in small packages; provider-specific behavior stays behind interfaces. Avoid changing existing JSON fields or capability names in incompatible ways.

## Reporting Security Issues

Do not open public issues containing credentials, personal data, vault paths, or exploitable details. Follow [SECURITY.md](SECURITY.md).

