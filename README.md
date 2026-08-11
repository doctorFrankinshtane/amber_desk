<p align="center">
  <img src="docs/assets/amber-desk-logo.png" width="180" alt="Amber Desk pixel globe logo">
</p>

<h1 align="center">amber_desk</h1>

<p align="center">
  Local-first OSINT investigation workspace with dossiers, chronology, maps, relationship graphs, and extensible connectors.
</p>

<p align="center">
  <img alt="Development status" src="https://img.shields.io/badge/status-active%20development-F2A23A?style=flat-square">
  <img alt="Go 1.22 or newer" src="https://img.shields.io/badge/Go-1.22%2B-F2A23A?style=flat-square&logo=go&logoColor=090604">
  <img alt="Leaflet 1.9.4" src="https://img.shields.io/badge/Leaflet-1.9.4-F2A23A?style=flat-square&logo=leaflet&logoColor=090604">
  <img alt="Vanilla JavaScript ES2020 or newer" src="https://img.shields.io/badge/JavaScript-ES2020%2B-F2A23A?style=flat-square&logo=javascript&logoColor=090604">
</p>

<p align="center">
  <a href="docs/USER_GUIDE.md">User guide</a> ·
  <a href="docs/USER_GUIDE_RU.md">Руководство</a> ·
  <a href="CONTRIBUTING.md">Contributing</a> ·
  <a href="SECURITY.md">Security</a>
</p>

![Amber Desk dossier workspace](docs/assets/amber-desk.png)

## Overview

Amber Desk keeps investigation data on your machine and presents it in a dense amber CRT interface. The Go backend embeds the browser client and exposes connector capabilities without coupling case data to a specific integration.

- Obsidian-backed dossiers, chronology, checklist, maps, and relationship graphs
- Multi-dossier workflow with local trash and conflict protection
- Detective-style relationship board with photos and file attachments
- Offline world overview and support for local XYZ map packs
- Bundled OSINT source catalog with provenance logging
- Optional native Sherlock username scans with manual result selection
- English and Russian interface

> **Development status:** active pre-release. Connector APIs and persisted schemas may evolve before the first stable release. Do not use real investigation data without backups.

## Stack

| Layer | Version |
| --- | --- |
| Backend | Go `1.22+` |
| Frontend | HTML5, CSS3, Vanilla JavaScript `ES2020+` |
| Maps | Leaflet `1.9.4` |
| Markdown metadata | `gopkg.in/yaml.v3` `3.0.1` |
| Optional username search | Sherlock `0.16.0` |
| Primary local storage | Obsidian-compatible Markdown, no plugin required |

## Quick Start

```powershell
git clone https://github.com/doctorFrankinshtane/amber_desk.git
cd amber_desk
go run .
```

Open <http://localhost:8080>.

To connect an Obsidian vault:

```powershell
$env:OBSIDIAN_VAULT = "C:\Users\you\Documents\My Vault"
go run .
```

Amber Desk creates its own directories inside the selected vault. Existing unrelated notes are not scanned or modified.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | Local HTTP listen address |
| `OBSIDIAN_VAULT` | unset | Absolute path to an Obsidian vault |
| `OBSIDIAN_DOSSIER_DIR` | `Amber Desk/Dossiers` | Dossier directory inside the vault |
| `MAP_TILE_DIR` | unset | Absolute path to a local raster XYZ pack |
| `SHERLOCK_PYTHON` | project-local runtime | Supported Sherlock Python executable |
| `ALLOW_REMOTE_TOOL_RUNS` | `0` | Allow process tools beyond localhost |

See [.env.example](.env.example) and the [English](docs/USER_GUIDE.md) or [Russian](docs/USER_GUIDE_RU.md) guide for complete setup and privacy boundaries.

## Development

```powershell
go test ./...
go vet ./...
go build .
```

Connector contracts are documented in [docs/CONNECTORS.md](docs/CONNECTORS.md). Third-party versions and licenses are listed in [docs/THIRD_PARTY.md](docs/THIRD_PARTY.md).
