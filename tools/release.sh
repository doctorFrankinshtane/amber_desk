#!/usr/bin/env bash
# Build release archives into dist/. Usage: tools/release.sh v0.1.0-alpha
set -euo pipefail

version=${1:?usage: tools/release.sh <version>}
cd "$(dirname "$0")/.."
rm -rf dist && mkdir dist

for target in windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os=${target%/*} arch=${target#*/}
  name=amberdesk-$version-$os-$arch
  bin=amberdesk
  [ "$os" = windows ] && bin=amberdesk.exe

  stage=dist/$name
  mkdir -p "$stage/tools/sherlock"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$stage/$bin" .
  cp LICENSE README.md .env.example "$stage/"
  cp tools/sherlock/setup.ps1 tools/sherlock/setup.sh tools/sherlock/requirements.lock "$stage/tools/sherlock/"

  (
    cd dist
    if [ "$os" = windows ] && command -v zip >/dev/null; then
      zip -qr "$name.zip" "$name"
    elif [ "$os" = windows ]; then
      powershell -NoProfile -Command "Compress-Archive -Path '$name' -DestinationPath '$name.zip'"
    else
      # Set modes explicitly: archives built on Windows would otherwise lose the executable bit.
      tar_opts=(--no-recursion --owner=0 --group=0 --numeric-owner)
      tar -cf "$name.tar" "${tar_opts[@]}" --mode=755 \
        "$name" "$name/tools" "$name/tools/sherlock" "$name/$bin" "$name/tools/sherlock/setup.sh"
      tar -rf "$name.tar" "${tar_opts[@]}" --mode=644 \
        "$name/LICENSE" "$name/README.md" "$name/.env.example" \
        "$name/tools/sherlock/setup.ps1" "$name/tools/sherlock/requirements.lock"
      gzip -9 "$name.tar"
    fi
    rm -rf "$name"
  )
done

(cd dist && sha256sum -- *.zip *.tar.gz > SHA256SUMS.txt)
ls -l dist
