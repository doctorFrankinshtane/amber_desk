```text
 ________  _____ ______   ________  _______   ________  ________  _______   ________  ___  __
|\   __  \|\   _ \  _   \|\   __  \|\  ___ \ |\   __  \|\   ___ \|\  ___ \ |\   ____\|\  \|\  \
\ \  \|\  \ \  \\\__\ \  \ \  \|\ /\ \   __/|\ \  \|\  \ \  \_|\ \ \   __/|\ \  \___|\ \  \/  /|_
 \ \   __  \ \  \\|__| \  \ \   __  \ \  \_|/_\ \   _  _\ \  \ \\ \ \  \_|/_\ \_____  \ \   ___  \
  \ \  \ \  \ \  \    \ \  \ \  \|\  \ \  \_|\ \ \  \\  \\ \  \_\\ \ \  \_|\ \|____|\  \ \  \\ \  \
   \ \__\ \__\ \__\    \ \__\ \_______\ \_______\ \__\\ _\\ \_______\ \_______\____\_\  \ \__\\ \__\
    \|__|\|__|\|__|     \|__|\|_______|\|_______|\|__|\|__|\|_______|\|_______|\_________\|__| \|__|
                                                                              \|_________|
```

# Amber Desk

Extensible browser workspace for OSINT investigations. Amber Desk combines a dense dossier timeline, evidence review, command workflow, and backend connectors in a pixel-inspired amber CRT interface.

![Amber Desk dossier workspace](docs/assets/amber-desk.png)

## Features

- Dossier, chronology, and evidence workspaces
- English and Russian interface localization
- Search and evidence filters
- Verification status and analyst notes
- Keyboard command palette
- Responsive desktop and mobile layouts
- Go backend with embedded frontend assets
- Connector registry for external tools
- Bidirectional Obsidian Markdown dossier synchronization
- Conflict protection when an Obsidian file changes externally

## Quick Start

Requirements: Go 1.22 or newer.

```powershell
git clone https://github.com/doctorFrankinshtane/amber_desk.git
cd amber_desk
go run .
```

Open <http://localhost:8080>.

## Obsidian

Amber Desk accesses an existing Obsidian vault through the local filesystem. It does not require an Obsidian plugin.

```powershell
$env:OBSIDIAN_VAULT = "C:\Users\you\Documents\My Vault"
$env:OBSIDIAN_DOSSIER_DIR = "Amber Desk\Dossiers"
go run .
```

Open **Vault** in the application header to create or edit the current case dossier. The generated file contains YAML frontmatter and standard Markdown. Changes made in Obsidian can be reloaded; stale saves return a conflict instead of overwriting external edits.

![Obsidian dossier editor](docs/assets/obsidian-connector.png)

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | HTTP listen address |
| `OBSIDIAN_VAULT` | unset | Absolute path to an existing Obsidian vault |
| `OBSIDIAN_DOSSIER_DIR` | `Amber Desk/Dossiers` | Relative dossier directory inside the vault |

See [.env.example](.env.example) for a local template.

## API

- `GET /api/health`
- `GET /api/case`
- `PATCH /api/events/{id}/status`
- `POST /api/events/{id}/notes`
- `GET /api/integrations`
- `GET /api/integrations/{id}/dossier`
- `PUT /api/integrations/{id}/dossier`

The investigation store is currently in memory and resets when the server restarts. Connector-backed documents persist in their external system.

## Extensions

Integrations implement a small Go connector interface and register with the backend registry. The frontend discovers connector metadata and health through the API rather than importing integration-specific code into the case model.

Read [Connector Development](docs/CONNECTORS.md) before adding an integration. Contribution conventions are in [CONTRIBUTING.md](CONTRIBUTING.md).

## Development

```powershell
go test ./...
go vet ./...
go build .
```

Do not commit vault contents, API keys, investigation exports, or real subject data. See [SECURITY.md](SECURITY.md).
