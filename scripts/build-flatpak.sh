#!/usr/bin/env bash
# Build WiiUDownloader as a Flatpak bundle, reusing the same prebuilt
# `WiiUDownloader` binary the AppImage build produces.
#
# Requires Docker (Flatpak can't run natively on macOS; works on Linux too).
# The flatpak bundle points at the freedesktop 25.08 runtime for GTK3 and just
# installs the prebuilt binary — it does NOT recompile, so the binary (and the
# title database it embeds) is identical to the AppImage release.
#
# Usage:
#   ./scripts/build-flatpak.sh                       # arch = host arch
#     # If ./WiiUDownloader doesn't exist, it is compiled first (AppImage-style:
#     # curl db.go + go build in the Dockerfile.linux container), then bundled.
#     # If it does exist (e.g. CI after the AppImage build step), it's reused.
#   VERSION=2.102 ./scripts/build-flatpak.sh
#   ./scripts/build-flatpak.sh x86_64                # cross-arch (needs qemu)
set -euo pipefail

cd "$(dirname "$0")/.."

ARCH="${1:-$(uname -m)}"
case "$ARCH" in
  arm64|aarch64) QARCH=aarch64 ;;
  x86_64|amd64)  QARCH=x86_64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

# Version from env, else the tag being built (CI), else 2.102.
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

# 1) Build the binary if it isn't already present. Same recipe as the AppImage
#    build in .github/workflows/linux.yml (curl the title db + go build).
if [ ! -f WiiUDownloader ]; then
  echo "Building WiiUDownloader binary ($QARCH)..."
  DOCKER_ARCH=$(case "$QARCH" in x86_64) echo x86_64 ;; *) echo aarch64 ;; esac)
  docker build . --file Dockerfile.linux --tag wiiu-builder --build-arg ARCH="$DOCKER_ARCH"
  docker run --rm -v "$PWD":/project wiiu-builder bash -lc \
    "curl --insecure -Lo db.go -H 'User-Agent: NUSspliBuilder/2.1' 'https://napi.v10lator.de/db?t=go' && if grep -q 'var titleEntry =' db.go; then if grep -q 'type TitleEntry struct' db.go; then sed -i '/type TitleEntry struct/,/}/d' db.go; fi && sed -i 's/var titleEntry =/func init() { TitleDatabase =/' db.go && echo '}' >> db.go; fi && go build -C cmd/WiiUDownloader -o ../../WiiUDownloader ."
fi
# 2) Assemble the flatpak from the prebuilt binary. Runtime/sdk install into
#    the SYSTEM flatpak dir (named volume -> downloads survive container exit).
#    flatpak-builder must be invoked with --user here (with these volumes it
#    resolves the SDK that way). --disable-rofiles-fuse: Docker has no /dev/fuse.
#    State dir lives inside $OUT because flatpak-builder refuses a state dir on
#    a different filesystem than the build dir.
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
flatpak-builder --user --arch="$QARCH" --repo="$OUT/repo" --force-clean \
  --disable-rofiles-fuse --state-dir="$OUT/.state" \
  "$OUT/build-dir" packaging/flatpak/io.github.xpl0itu.wiiudownloader.json
'

docker run --rm -v "$PWD":/build -w /build \
  -v wiiu-flatpak-data:/root/.local/share/flatpak \
  "$IMAGE" \
  flatpak build-bundle --arch="$QARCH" \
    "$OUT/repo" "$OUT/WiiUDownloader-Linux-$QARCH.flatpak" "$APP_ID"

echo "OK: $OUT/WiiUDownloader-Linux-$QARCH.flatpak"