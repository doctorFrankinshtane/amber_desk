# Security Policy

Amber Desk is an early-stage investigation workspace and is not yet a production evidence-management system.

## Sensitive Data

Never commit or share real investigation data, Obsidian vault contents, credentials, API keys, exports, or local filesystem paths. Use synthetic fixtures in tests and screenshots.

The demo case bundled with the project is fictional. The in-memory case store resets at process restart, while configured connectors may write persistent data to external systems.

## Reports

Report vulnerabilities privately to the repository owner through GitHub's private vulnerability reporting when enabled. Include the affected version, reproduction steps, impact, and a minimal proof of concept without real subject data.

## Connector Review

External connectors expand the trust boundary. Review network destinations, permissions, secret handling, data retention, path validation, concurrency behavior, and third-party terms before enabling a connector with real investigations.
