$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$venv = Join-Path $root ".tools\sherlock"
$python = if ($env:PYTHON) { $env:PYTHON } else { "python" }

& $python -m venv $venv
& (Join-Path $venv "Scripts\python.exe") -m pip install --disable-pip-version-check --require-hashes -r (Join-Path $PSScriptRoot "requirements.lock")
& (Join-Path $venv "Scripts\python.exe") -c "import importlib.metadata; print('Sherlock ' + importlib.metadata.version('sherlock-project') + ' installed locally')"
