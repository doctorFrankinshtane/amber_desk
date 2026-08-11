# Third-Party Notices

Amber Desk vendors runtime assets so the investigation workspace and map remain usable without third-party network requests.

## Leaflet

- Version: 1.9.4
- Project: https://leafletjs.com/
- License: BSD 2-Clause
- Vendored files: `web/vendor/leaflet/`

The complete Leaflet license text is included at `web/vendor/leaflet/LICENSE`.

## Cytoscape.js

- Version: 3.34.0
- Project: https://js.cytoscape.org/
- License: MIT
- Vendored files: `web/vendor/cytoscape/`

The complete Cytoscape.js license text is included at `web/vendor/cytoscape/LICENSE`.

## Natural Earth

- Datasets: Natural Earth 1:50m countries, states/provinces, populated places, urban areas, lakes and rivers; Natural Earth 1:10m roads, filtered to major routes and simplified for local rendering
- Project: https://www.naturalearthdata.com/
- Source distribution: https://github.com/nvkelso/natural-earth-vector
- License: public domain
- Vendored derivatives: `web/data/world.geojson`, `web/data/cities.geojson`, and the remaining GeoJSON files in `web/data/`, including zoom-partitioned road layers

Natural Earth requests attribution but does not require it. Amber Desk includes this notice to preserve provenance.

## OSINT Framework

- Snapshot version: `a744e613d7ded0aaa854896feb2a1069de34d2f8`
- Project: https://github.com/lockfale/osint-framework
- License: MIT
- Vendored dataset: `web/data/osint-framework.json`

Catalog data is derived from OSINT Framework by Justin Nordine. The complete upstream license and copyright notice is included at `docs/licenses/OSINT-FRAMEWORK-LICENSE`.

## Sherlock

- Version: 0.16.0
- Project: https://github.com/sherlock-project/sherlock
- License: MIT
- Distribution: optional project-local Python environment created by `tools/sherlock/setup.ps1` or `setup.sh`

Sherlock is not vendored into Amber Desk. Its package and dependencies are installed only when an operator runs the setup script. A scan contacts supported third-party sites with the submitted username; Amber Desk requires explicit confirmation for each run.

## Development tooling

- Playwright `1.62.1` runs the CI browser smoke test and is not part of the Amber Desk runtime.
