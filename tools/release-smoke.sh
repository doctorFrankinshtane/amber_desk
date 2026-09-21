#!/usr/bin/env bash
# Smoke-test a release archive the way a new user meets it: extract into an
# empty directory, run the binary, then install Sherlock with the bundled script.
# Usage: tools/release-smoke.sh dist/amberdesk-<version>-<os>-<arch>.{zip,tar.gz}
set -euo pipefail

archive=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
name=$(basename "$archive" | sed -E 's/\.(zip|tar\.gz)$//')
work=$(mktemp -d)
cd "$work"

case $archive in
  *.zip) powershell -NoProfile -Command "Expand-Archive -LiteralPath '$(cygpath -w "$archive")' -DestinationPath '.'" ;;
  *) tar xzf "$archive" ;;
esac

bin=$work/$name/amberdesk
[ -f "$bin.exe" ] && bin=$bin.exe
[ -x "$bin" ] || { echo "binary is not executable: $bin" >&2; exit 1; }

base=http://127.0.0.1:18080
ADDR=127.0.0.1:18080 "$bin" &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT
for _ in $(seq 30); do curl -fs "$base/api/health" >/dev/null && break; sleep 1; done
curl -fsS "$base/api/health"
curl -fsS "$base/" | grep -q '<html' || { echo "web client not served" >&2; exit 1; }
curl -fsS "$base/api/catalog" | grep -q '"categories"' || { echo "catalog missing" >&2; exit 1; }

if [ -f "$name/tools/sherlock/setup.sh" ] && [ "${bin##*.}" != exe ]; then
  "$name/tools/sherlock/setup.sh"
else
  powershell -NoProfile -ExecutionPolicy Bypass -File "$name/tools/sherlock/setup.ps1"
fi
status=$(curl -fsS "$base/api/tools/sherlock/status")
echo "$status"
echo "$status" | grep -q '"ready":true' || { echo "Sherlock not detected next to binary" >&2; exit 1; }
echo "release smoke passed: $name"
