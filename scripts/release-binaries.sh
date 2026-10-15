#!/usr/bin/env bash
# Builds the release binaries into dist/, as the Release workflow attaches them
# to a GitHub release: the CLI for Linux, macOS and Windows (amd64 and arm64),
# the Linux agent (amd64 and arm64), and SHA256SUMS.
#
#   scripts/release-binaries.sh 0.5.0          (or v0.5.0; `make release-binaries VERSION=0.5.0`)
#   SRC=path/to/checkout scripts/release-binaries.sh 0.4.0
#
# SRC is the source tree to build (default: this repository); the Release
# workflow uses it to build an older tag with this script. OUT is the output
# directory (default: dist/ here), emptied first.
#
# Assets, each archive holding the binary, LICENSE and NOTICE at its root (the
# agent's also install.sh, config.example.yaml and fleetml-agent.service):
#   fleetml-linux-amd64.tar.gz   fleetml-linux-arm64.tar.gz
#   fleetml-darwin-amd64.tar.gz  fleetml-darwin-arm64.tar.gz
#   fleetml-windows-amd64.zip    fleetml-windows-arm64.zip    (fleetml.exe)
#   fleetml-agent-linux-amd64.tar.gz  fleetml-agent-linux-arm64.tar.gz
#   SHA256SUMS
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=${1:?usage: scripts/release-binaries.sh <version, e.g. 0.5.0>}
VERSION=${VERSION#v}
SRC=$(cd "${SRC:-.}" && pwd)
OUT=${OUT:-dist}
mkdir -p "$OUT" && OUT=$(cd "$OUT" && pwd)
rm -rf "${OUT:?}"/*

LDFLAGS="-s -w -X github.com/fleetml/fleetml/internal/version.Version=$VERSION"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Archive entries owned by 0:0 without user or group names (not those of
# whoever builds), and without extended attributes (macOS tar adds Apple ones
# that GNU tar warns about). GNU tar and the BSD tar of macOS spell it apart.
if tar --version 2>/dev/null | grep -q 'GNU tar'; then
  tar_opts=(--owner=0 --group=0 --numeric-owner --no-xattrs)
else
  tar_opts=(--uid 0 --gid 0 --numeric-owner --no-xattrs)
fi

# release <binary name> <package> <os> <arch>
release() {
  local name=$1 pkg=$2 os=$3 arch=$4 exe=""
  [ "$os" = windows ] && exe=.exe
  local dir="$WORK/$name-$os-$arch"
  mkdir -p "$dir"
  (cd "$SRC" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$LDFLAGS" -o "$dir/$name$exe" "$pkg")
  local files=("$name$exe" LICENSE NOTICE)
  cp "$SRC/LICENSE" "$SRC/NOTICE" "$dir/"
  # The agent comes with its installer, example config and systemd unit.
  if [ "$name" = fleetml-agent ]; then
    cp "$SRC/deploy/agent/install.sh" "$SRC/deploy/agent/config.example.yaml" "$SRC/deploy/agent/fleetml-agent.service" "$dir/"
    files+=(install.sh config.example.yaml fleetml-agent.service)
  fi
  if [ "$os" = windows ]; then
    (cd "$dir" && zip -q -X "$OUT/$name-$os-$arch.zip" "${files[@]}")
  else
    COPYFILE_DISABLE=1 tar "${tar_opts[@]}" -C "$dir" -czf "$OUT/$name-$os-$arch.tar.gz" "${files[@]}"
  fi
  echo "$name-$os-$arch"
}

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  release fleetml ./cmd/fleetml "${target%/*}" "${target#*/}"
done
for target in linux/amd64 linux/arm64; do
  release fleetml-agent ./cmd/fleetml-agent "${target%/*}" "${target#*/}"
done

# sha256sum on Linux (and in WSL), shasum on macOS: the same output format.
if command -v sha256sum >/dev/null; then sum=(sha256sum); else sum=(shasum -a 256); fi
(cd "$OUT" && "${sum[@]}" ./*.tar.gz ./*.zip | sed 's| \./| |' > SHA256SUMS)
echo "built $(find "$OUT" -type f | wc -l | tr -d ' ') files for version $VERSION in $OUT"
