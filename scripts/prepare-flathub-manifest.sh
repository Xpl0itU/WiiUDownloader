#!/usr/bin/env bash
# Emit the Flathub-ready manifest (git tag source + auto-update checker) to
# stdout. The local manifest uses a dir source; Flathub builds from a git tag.
#
# Usage:
#   ./scripts/prepare-flathub-manifest.sh v2.102 > /tmp/io.github.xpl0itu.wiiudownloader.json
#   ./scripts/prepare-flathub-manifest.sh          # uses latest git tag
set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:-}"
if [ -z "$TAG" ]; then
  TAG="$(git describe --tags --abbrev=0 2>/dev/null || echo v2.102)"
fi

python3 - "$TAG" <<'PY'
import json
import sys

tag = sys.argv[1]
manifest = json.load(open("packaging/flatpak/io.github.xpl0itu.wiiudownloader.json"))
manifest["modules"][0]["sources"] = [
    {
        "type": "git",
        "url": "https://github.com/Xpl0itU/WiiUDownloader.git",
        "tag": tag,
        # Lets Flathub's flatpak-external-data-checker open an update PR
        # automatically when a new vX.Y.Z tag is pushed.
        "x-checker-data": {"type": "git", "tag-pattern": r"^v([\d.]+)$"},
    }
]
json.dump(manifest, sys.stdout, indent=4)
print()
PY
