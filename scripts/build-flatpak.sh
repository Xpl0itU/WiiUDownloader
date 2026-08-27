#!/usr/bin/env bash
# Build WiiUDownloader as a Flatpak bundle and package it as a tar.gz.
#
# Requires Docker (Flatpak can't run natively on macOS). Works on Linux too.
# Uses the official Flathub CI image, so the result matches what Flathub builds.
#
# Usage:
#   ./scripts/build-flatpak.sh              # arch = host arch, version 2.102
#   VERSION=2.102 ./scripts/build-flatpak.sh
#   ./scripts/build-flatpak.sh x86_64       # cross-arch (needs qemu on macOS arm)
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="${1:-$(uname -m)}"
case "$ARCH" in
  arm64|aarch64) QARCH=aarch64 ;;
  x86_64|amd64)  QARCH=x86_64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

# Version from env, else the tag being built (CI), else the next release.
VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
  case "${GITHUB_REF:-}" in
    refs/tags/v*) VERSION="${GITHUB_REF#refs/tags/v}" ;;
  esac
fi
VERSION="${VERSION:-2.102}"
APP_ID=io.github.xpl0itu.wiiudownloader
IMAGE=ghcr.io/flathub-infra/flatpak-github-actions:gnome-50
OUT=dist/flatpak
mkdir -p "$OUT"

# Runtimes/sdk/extensions install into the SYSTEM flatpak dir (named volume,
# so downloads survive container exit and rebuilds are fast). flatpak-builder
# must be invoked with --user here (with these volumes it resolves the SDK
# that way); plain --system builds fail to find the SDK in this image.
# --disable-rofiles-fuse is required: Docker has no /dev/fuse. State dir
# lives inside $OUT because flatpak-builder refuses a state dir on a
# different filesystem than the build dir (the Docker bind mount is a
# different fs than /).
docker run --rm --privileged -e QARCH="$QARCH" -e OUT="$OUT" \
  -v "$PWD":/build -w /build \
  -v wiiu-flatpak-system:/var/lib/flatpak \
  -v wiiu-flatpak-data:/root/.local/share/flatpak \
  "$IMAGE" bash -lc '
set -e
mkdir -p /var/lib/dbus
[ -f /var/lib/dbus/machine-id ] || dbus-uuidgen > /var/lib/dbus/machine-id
flatpak install -y flathub org.freedesktop.Platform//25.08
flatpak install -y flathub org.freedesktop.Sdk//25.08
flatpak install -y flathub org.freedesktop.Sdk.Extension.golang//25.08
flatpak-builder --user --arch="$QARCH" --repo="$OUT/repo" --force-clean \
  --disable-rofiles-fuse --state-dir="$OUT/.state" \
  "$OUT/build-dir" packaging/flatpak/io.github.xpl0itu.wiiudownloader.json
'

docker run --rm -v "$PWD":/build -w /build \
  -v wiiu-flatpak-data:/root/.local/share/flatpak \
  "$IMAGE" \
  flatpak build-bundle --arch="$QARCH" \
    "$OUT/repo" "$OUT/wiiudownloader-$VERSION-$QARCH.flatpak" "$APP_ID"

tar -C "$OUT" -czf "$OUT/wiiudownloader-$VERSION-$QARCH.tar.gz" \
  "wiiudownloader-$VERSION-$QARCH.flatpak"

echo "OK: $OUT/wiiudownloader-$VERSION-$QARCH.tar.gz"
