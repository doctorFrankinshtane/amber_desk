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

- Dossier, Obsidian-backed chronology, evidence, and world map workspaces
- English and Russian interface localization
- Search and evidence filters
- Verification status and analyst notes
- Keyboard command palette
- Responsive desktop and mobile layouts
- Go backend with embedded frontend assets
- Connector registry for external tools
- Bidirectional Obsidian Markdown dossier synchronization
- Interactive local world map with draggable markers and movement routes
- Offline local XYZ map packs for street, building, address, and POI detail
- Timeline, marker, and route notes persisted as readable Markdown
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

When the vault is connected, the chronology and map use Obsidian as their canonical backend. Amber Desk stores one note per entity:

```text
Amber Desk/
  Dossiers/
  Cases/<case-id>-<case-name>/
    Timeline/<event-id>.md
    Map/Markers/<marker-id>.md
    Map/Routes/<route-id>.md
```

Each note has versioned YAML frontmatter for Amber Desk and a readable Markdown body for editing and linking inside Obsidian. An empty vault offers **Import Demo** once; after that, changes in either application are picked up by the browser automatically or on reload.

![Obsidian dossier editor](docs/assets/obsidian-connector.png)

## Investigation Map

Open **World Map** above the chronology. Use **Add Marker** and click the map to record a location. Markers can be selected, edited, or dragged. Use **Connect**, then select two markers to create a movement route.

The map makes no runtime requests to external map services. The bundled Natural Earth dataset provides a worldwide overview with cities, urban areas, regions, major roads, rivers, and lakes. For street-level buildings, addresses, cafes, restaurants, and other POIs, configure a locally stored raster XYZ pack:

```text
D:\Maps\amber-xyz\
  0\0\0.png
  1\0\0.png
  1\0\1.png
  ...
```

Set `MAP_TILE_DIR` to the pack root. Labels and POIs must be rendered into the local tiles by the pack producer. Amber Desk serves only the tiles needed for the visible viewport through its own Go backend; the browser communicates exclusively with `localhost`.

![Amber Desk world map](docs/assets/amber-map.png)

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | HTTP listen address |
| `OBSIDIAN_VAULT` | unset | Absolute path to an existing Obsidian vault |
| `OBSIDIAN_DOSSIER_DIR` | `Amber Desk/Dossiers` | Relative dossier directory inside the vault |
| `MAP_TILE_DIR` | unset | Absolute path to a local raster XYZ map pack |
| `MAP_TILE_EXT` | `png` | Tile file extension: `png`, `jpg`, `jpeg`, or `webp` |
| `MAP_TILE_MIN_ZOOM` | `0` | Lowest zoom available in the local pack |
| `MAP_TILE_MAX_ZOOM` | `18` | Highest zoom available in the local pack |

See [.env.example](.env.example) for a local template.

## API

- `GET /api/health`
- `GET /api/case`
- `GET /api/timeline`
- `POST /api/timeline/bootstrap`
- `POST /api/timeline/events`
- `PATCH /api/events/{id}/status`
- `POST /api/events/{id}/notes`
- `GET /api/map`
- `POST /api/map/markers`
- `PUT /api/map/markers/{id}`
- `DELETE /api/map/markers/{id}`
- `POST /api/map/routes`
- `DELETE /api/map/routes/{id}`
- `GET /api/integrations`
- `GET /api/integrations/{id}/dossier`
- `PUT /api/integrations/{id}/dossier`

The demo case remains in memory. When a connected provider advertises `timeline.*` or `map.*`, those capability routes persist through that provider; otherwise they use a session-only memory fallback.

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

Third-party asset and library notices are documented in [Third-Party Notices](docs/THIRD_PARTY.md).
