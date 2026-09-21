#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
VENV="$ROOT/.tools/sherlock"
PYTHON=${PYTHON:-python3}

"$PYTHON" -m venv "$VENV"
"$VENV/bin/python" -m pip install --disable-pip-version-check --require-hashes -r "$ROOT/tools/sherlock/requirements.lock"
"$VENV/bin/python" -c "import importlib.metadata; print('Sherlock ' + importlib.metadata.version('sherlock-project') + ' installed locally')"
