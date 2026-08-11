# Amber Desk User Guide

[Русская версия](USER_GUIDE_RU.md)

## 1. What Amber Desk stores

Amber Desk is a local web application. The browser talks to its Go backend on your computer.

With Obsidian configured, the backend stores cases as files in an existing local vault. Without a connected storage provider, supported workspaces use in-memory fallback storage. In-memory data is lost when the Amber Desk process stops.

Amber Desk is an early-stage investigation workspace, not a production evidence-management system. Keep independent backups and verify important evidence separately.

## 2. Start Amber Desk

Requirements:

- Go 1.25 or newer
- A current desktop browser
- Optional: an existing Obsidian vault
- Optional: Python and the bundled Sherlock setup script

Start without Obsidian:

```powershell
git clone https://github.com/doctorFrankinshtane/amber_desk.git
cd amber_desk
go run .
```

Open <http://localhost:8080>.

To use another port in PowerShell:

```powershell
$env:ADDR = "127.0.0.1:8081"
go run .
```

Environment variables apply to the current shell. Restart Amber Desk after changing them.

## 3. Connect Obsidian

Amber Desk uses the vault through the local filesystem. No Obsidian plugin is required.

1. Stop Amber Desk.
2. Set `OBSIDIAN_VAULT` to the absolute path of an existing vault.
3. Optionally set the relative dossier directory.
4. Start Amber Desk from the same shell.

```powershell
$env:OBSIDIAN_VAULT = "C:\Users\you\Documents\My Vault"
$env:OBSIDIAN_DOSSIER_DIR = "Amber Desk\Dossiers"
go run .
```

The default value of `OBSIDIAN_DOSSIER_DIR` is `Amber Desk/Dossiers`. It must be relative to the vault. In the header, **VAULT** shows the connector state:

- `CONNECTED`: the vault directory is available.
- `NOT CONFIGURED`: `OBSIDIAN_VAULT` was not set when the process started.
- `VAULT OFFLINE`: the configured path is unavailable.

Press **VAULT** to edit the current dossier Markdown. Use **RELOAD** to read changes made in Obsidian and **SAVE TO VAULT** to write the editor content. If the file changed after Amber Desk loaded it, saving is rejected as an external-change conflict. Reload before editing again.

### Default vault layout

```text
Amber Desk/
  Dossiers/
    <case-id>.md
  Cases/<case-id>/
    Checklist.md
    Timeline/<event-id>.md
    Map/Markers/<marker-id>.md
    Map/Routes/<route-id>.md
    Relations/Nodes/<node-id>.md
    Relations/Edges/<edge-id>.md
    Relations/Attachments/<node-id>/<attachment-id>/
      metadata.json
      content.bin
    .trash/
  .state/
    cases-index.json
    active-case-id.json
    cases/<case-id>.json
  .trash/<case-id>-<timestamp>/
```

The Markdown notes contain YAML frontmatter used by Amber Desk. Do not remove or rename its structured fields unless you understand the data format. Attachment bytes are stored as `content.bin`; the original filename and SHA-256 digest are in `metadata.json`.

## 4. Create the first dossier

1. Press **CREATE DOSSIER** in the left panel, or press the case block in the header and choose **CREATE NEW**.
2. In **CASE**, enter the required case name. Owner, objective, and tags are optional.
3. In **SUBJECT**, enter the required codename. Add the display name, risk, confidence, location, last-seen time, and aliases as needed.
4. In **CONNECTIONS**, add known identifiers and initial related entities.
5. Press **CREATE AND OPEN**.

When Obsidian is connected, creation writes the case snapshot, dossier, and initial relationship records into the vault. The checklist file is created when the checklist first loads. Check the backend labels in the interface: `OBSIDIAN` means persistent vault storage; `MEMORY` means session-only fallback.

## 5. Switch or delete dossiers

Press the case block in the header to open **DOSSIER INDEX**. Search by case name, ID, or subject, then select a dossier to make it active. Unsaved changes in the VAULT Markdown editor block switching until you save or discard them.

To delete the active dossier:

1. Press **DELETE** beside **SUBJECT DOSSIER**.
2. Type the exact case ID shown in the header.
3. Press **MOVE TO TRASH**.

Complete-case deletion requires a connected case-storage provider; the built-in provider is Obsidian. The dossier, case directory, and persisted snapshot are moved to `Amber Desk/.trash/<case-id>-<timestamp>/`. The next available dossier becomes active. Amber Desk has no restore button; restore files manually from a backup or the vault-local trash while the application is stopped.

## 6. Chronology and evidence

Open **TIMELINE**.

1. Press `+` in the timeline toolbar.
2. Enter title, type, observed time, summary, source, and confidence.
3. Press **ADD**.
4. Select an event to open **EVIDENCE INSPECTOR**.
5. Add analyst notes or change the event between `PENDING` and `VERIFIED`.

Search and the `ALL`, `ID`, `NET`, `MEDIA`, `SOC`, `FIN`, and `TOOL` filters narrow the visible records. Deleting an event requires confirmation. With Obsidian, its Markdown note moves to `Cases/<case-id>/.trash/Timeline/`.

The Markdown body is for reading; structured chronology fields are loaded from YAML frontmatter. Refresh the browser to reload timeline files edited directly in Obsidian.

## 7. Investigation checklist

The checklist is below the subject details in the left panel.

- Check a step to mark it done.
- Use `...` to edit its title, note, and status or pin it as the next step.
- Use `+` in the checklist header to add a custom step to a phase.
- Only custom steps can be deleted.
- Use the `>` action on a step to open its related Amber Desk workspace. It does not start an external search.

The progress count includes both done and skipped steps. With Obsidian, the complete state is stored in `Checklist.md`; otherwise it is held in memory.

## 8. Investigation map

Open **WORLD MAP**.

Add a location:

1. Choose **ADD MARKER**.
2. Click the map.
3. Enter a label and optional observation time and description.
4. Press **SAVE**.

Drag a marker to update its coordinates. To add movement, choose **CONNECT** and select two different markers. Select a marker or route to inspect or delete it. Deleting a marker also removes routes connected to it.

The bundled overview map, city labels, regions, urban areas, major roads, rivers, and lakes are local files. The map makes no runtime request to a public tile service.

### Local detailed tiles

For buildings, addresses, local streets, cafes, restaurants, and other POIs, provide a raster XYZ tile pack:

```text
D:\Maps\amber-xyz\
  0\0\0.png
  1\0\0.png
  1\0\1.png
```

```powershell
$env:MAP_TILE_DIR = "D:\Maps\amber-xyz"
$env:MAP_TILE_EXT = "png"
$env:MAP_TILE_MIN_ZOOM = "0"
$env:MAP_TILE_MAX_ZOOM = "18"
go run .
```

Supported extensions are `png`, `jpg`, `jpeg`, and `webp`. Names and POIs must already be rendered into the tiles. The browser requests visible tiles only from the local Amber Desk backend.

## 9. Relationship board, files, and photos

Open **RELATIONS**.

- **+ CARD** creates a subject, organization, account, location, infrastructure, evidence, or fact card.
- **THREAD** connects two cards. A thread stores its label, confidence, kind, source event IDs, and analyst note.
- Drag cards to arrange the board. **AUTO LAYOUT** recalculates positions.
- Select a card or thread to edit it in the right inspector.
- The primary subject card cannot be deleted.

For a saved card, use **+ ATTACH FILES** or **+ ADD PHOTO**. Each card accepts up to 20 files, each no larger than 10 MiB. A document-stack marker on the card shows that attachments exist. The first supported JPEG, PNG, WebP, or GIF becomes the cover; another image can be selected as the cover in the attachment list.

The `+` button on the dossier portrait uploads the primary subject photo. It uses the cover of the primary relationship card, so changing it in either location changes the same stored attachment.

Files are uploaded only to the local Amber Desk backend. With Obsidian connected, they are stored under the active case's `Relations/Attachments` directory. Removing an attachment or a card with attachments moves those files into the case-local `.trash` directory.

## 10. Sherlock username scans

Sherlock is optional. Install its pinned local runtime once:

```powershell
.\tools\sherlock\setup.ps1
```

On Linux or macOS:

```bash
./tools/sherlock/setup.sh
```

Amber Desk detects `.tools/sherlock` automatically. To use another compatible environment, set `SHERLOCK_PYTHON` to its absolute Python executable path and restart Amber Desk.

Run a scan:

1. Open **RELATIONS** and select the source card.
2. In the card inspector, press **SCAN** under **SHERLOCK**.
3. Enter a username and press **START SCAN**.
4. Read the external-request warning and confirm.
5. Use `CLAIMED`, `PROBLEMS`, or `ALL`, then filter by service name if needed.
6. Check only the candidate profiles you want to retain.
7. Press **ADD SELECTED**.

You can also enter `SHERLOCK <username>` in the bottom command line, but a source card must still be selected.

**Network warning:** the Sherlock process sends the username to many supported third-party websites. Those sites see requests from your network and may log the username, IP address, timing, and request metadata. Amber Desk does not send dossier notes or attachments to Sherlock targets. Results are candidates, not proof of identity; import is deliberately manual.

An import creates low-confidence account cards and `FOUND ON` threads, adds one pending chronology event, and attaches the JSON report to the source card. Tool runs are limited to localhost unless `ALLOW_REMOTE_TOOL_RUNS=1` is explicitly set.

## 11. OSINT source catalog

Open **OSINT TOOLS**. Categories, search, price filters, status filters, and source descriptions use the catalog bundled with Amber Desk. Browsing and filtering it is local.

- **IN TIMELINE** / **LOG TO TIMELINE** writes the selected catalog record into the active chronology. It does not open the source.
- **OPEN SOURCE** opens the third-party URL in a new browser tab with referrer suppression.

**Network warning:** after **OPEN SOURCE**, the destination and any resources it loads are outside Amber Desk. The site can receive your IP address, browser metadata, queries you enter, uploads, and its own cookies. Review the catalog's OPSEC note and the site's terms before sending sensitive data. A `LOCAL` badge describes an installable tool; it does not mean pressing **OPEN SOURCE** is offline.

## 12. Language and layout

Use the `EN` / `RU` selector in the header. The selection is stored in browser `localStorage` on this computer.

On desktop, use the chevrons beside **INVESTIGATION CHRONOLOGY** to hide or restore the left dossier panel and right evidence inspector independently. These preferences are also stored in browser `localStorage`. On narrow screens, use the `DOSSIER`, `TIMELINE`, and `EVIDENCE` tabs instead.

Amber Desk uses brief Terminal Sequence transitions only for newly created records, generated results, dialogs, and system messages. Existing data, filters, search, and the command palette update immediately. The interface follows the operating system or browser `prefers-reduced-motion` setting and replaces movement with a short opacity signal.

At startup, Terminal Decode progressively reports the interface core, local API, Obsidian vault, case index, and workspace state. The normal sequence lasts about 2.5 seconds and uses only same-origin requests to the local Amber Desk backend. These are runtime diagnostics, not a full `go test` run; full tests belong in development and CI so opening the interface cannot modify test storage or waste local resources. With `prefers-reduced-motion`, Amber Desk skips the intentional delay.

## 13. Privacy boundaries

Fully local during normal use:

- Browser-to-backend traffic on `localhost`
- Obsidian vault reads and writes
- Chronology, checklist, map, relationship board, attachments, and photos
- Bundled overview map and OSINT catalog browsing
- Locally configured XYZ tiles

Creates or may create third-party network traffic:

- A confirmed Sherlock scan
- **OPEN SOURCE** in the OSINT catalog
- Any site or tool you open separately from Amber Desk
- Installing dependencies with Go, Python, Git, or package managers

Amber Desk does not anonymize external traffic. Use a network setup appropriate to your investigation and applicable law. Do not expose the HTTP listener to untrusted networks; it has no user authentication.

## 14. Backup and recovery

For persistent work, connect Obsidian and back up the entire vault, not only `Dossiers/`. The `Cases/` and `.state/` directories are required for chronology, maps, relationships, attachments, checklist state, the case index, and the active-case pointer.

Recommended procedure:

1. Stop Amber Desk to avoid copying during a write.
2. Copy or snapshot the complete vault.
3. Keep at least one backup outside the working disk.
4. Test restoration into a separate vault path.

The vault-local `.trash` directories are recovery aids, not backups. Amber Desk does not provide automatic backup, retention, encryption, or a restore workflow.

## 15. Troubleshooting

### The page does not open

- Read the terminal output for the actual address.
- Default: <http://localhost:8080>.
- If the port is occupied, set another `ADDR`, restart, and use that port.
- Keep the terminal process running.

### VAULT says NOT CONFIGURED

- Set `OBSIDIAN_VAULT` in the same shell used to run `go run .`.
- Use an absolute path to an existing vault directory.
- Restart Amber Desk after setting the variable.

### VAULT says OFFLINE

- Check spelling, drive availability, and filesystem permissions.
- Confirm the path points to a directory, not a Markdown file.
- Check that the vault was not moved after Amber Desk started.

### A case is not present after restart

- Check the workspace backend labels. `MEMORY` data does not survive a restart.
- Verify `OBSIDIAN_VAULT` is configured and connected.
- Inspect `Amber Desk/.state/cases-index.json` and `.state/cases/` in the vault; restore from backup instead of editing these files while Amber Desk runs.

### Obsidian changes do not appear

- In the VAULT editor, press **RELOAD** for the dossier Markdown.
- Refresh the browser for chronology, checklist, map, or relationship files.
- Preserve Amber Desk YAML frontmatter and case IDs when editing generated notes.
- If parsing fails, restore the last valid file from backup.

### Saving the dossier reports EXTERNAL CHANGE

The file changed after Amber Desk loaded it. Press **RELOAD**, review the external version, reapply your edit, and save again.

### Detailed map is blank

- Confirm `MAP_TILE_DIR` is absolute and contains `{z}/{x}/{y}.<ext>` files.
- Confirm `MAP_TILE_EXT` and zoom limits match the pack.
- Restart Amber Desk after changing variables.
- A tile pack cannot display labels or POIs that were not rendered by its producer.

### Sherlock is unavailable

- Run the setup script and read its error output.
- If using `SHERLOCK_PYTHON`, point it to the Python executable, not the environment directory.
- Restart Amber Desk.
- Keep `ADDR` on `127.0.0.1` and leave `ALLOW_REMOTE_ACCESS=0`. Remote HTTP access must be an explicit deployment decision with separate network access controls.
- Run tools from `localhost`, or leave `ALLOW_REMOTE_TOOL_RUNS=0` for the default restriction.

### Sherlock shows problems or invalid profile URLs

Use the `PROBLEMS` filter to inspect affected services. A service definition or response may have changed. Do not import malformed or uncertain results; update the pinned integration only after reviewing the upstream change.

### An attachment is rejected

- Maximum: 20 files per card.
- Maximum size: 10 MiB per file.
- The filename must be a plain filename, at most 180 Unicode characters, without path separators or control characters.

### Before reporting a bug

Run:

```powershell
go test ./...
go vet ./...
```

Record the Amber Desk commit, operating system, browser, exact steps, visible error, and relevant backend log. Do not include real subject data, vault contents, credentials, or local paths in public reports.
