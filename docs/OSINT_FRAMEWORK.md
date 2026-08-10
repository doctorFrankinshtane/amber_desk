# OSINT Framework Integration Research

## Summary

`osint-framework.pages.dev` is a static D3 catalog viewer, not a stable public API. Amber Desk should not scrape the deployment or proxy the third-party tools listed in it.

The recommended integration is a versioned catalog provider that imports the official upstream dataset, validates it, and publishes an approved immutable snapshot through Amber Desk's own read-only catalog API.

## Sources

- Public viewer: <https://osint-framework.pages.dev/>
- Deployment dataset: <https://osint-framework.pages.dev/arf.json>
- Official repository: <https://github.com/lockfale/OSINT-Framework>
- Official dataset: <https://raw.githubusercontent.com/lockfale/osint-framework/master/public/arf.json>
- Upstream license: <https://raw.githubusercontent.com/lockfale/osint-framework/master/LICENSE>

The upstream repository is MIT licensed and carries copyright for Justin Nordine. Any distributed snapshot derived from it must retain the license and copyright notice.

## Observed Data Model

The catalog is a nested tree using `name`, `type`, `url`, and `children`. The current upstream dataset also includes optional metadata such as description, status, pricing, input/output expectations, OPSEC guidance, local installation, registration, Google dorks, editable URL templates, API availability, invitation requirements, and deprecation state.

The `pages.dev` copy can lag behind the repository and must not be treated as the source of truth.

## Proposed Provider Boundary

Introduce a separate `CatalogProvider` capability rather than extending the dossier connector:

```go
type CatalogProvider interface {
    Manifest() CatalogManifest
    FetchSnapshot(context.Context, SourceVersion) (RawSnapshot, error)
    Normalize(context.Context, RawSnapshot) (CatalogSnapshot, error)
    Validate(context.Context, CatalogSnapshot) ValidationReport
}
```

Snapshots should store the upstream commit SHA, ETag, content hash, import time, license, attribution, validation report, and approval state. The user-facing catalog reads only the latest approved snapshot and supports rollback.

## Import Safety

- Fetch a pinned `raw.githubusercontent.com/<sha>/public/arf.json` URL.
- Apply a strict response-size limit and JSON schema validation.
- Render upstream text as plain text.
- Permit `https:` links by default; warn on `http:`.
- Reject `javascript:`, `data:`, `file:`, and unknown schemes.
- Treat URL templates as templates and substitute values locally.
- Generate stable entry IDs from source path and URL.
- Produce a diff report before approving a new snapshot.
- Cache using SHA and ETag; rate-limit update checks.
- Keep third-party credentials and API access outside the catalog provider.

## Product Use

Catalog entries should launch external tools and record provenance in a case: tool name, URL, catalog source, snapshot SHA, timestamp, and analyst note. Amber Desk must not imply that catalog inclusion grants permission to automate or scrape the listed service.

## Attribution

Before shipping imported data, add a `NOTICE` entry similar to:

```text
Catalog data derived from OSINT Framework by Justin Nordine.
Source: https://github.com/lockfale/OSINT-Framework
Licensed under the MIT License.
```
