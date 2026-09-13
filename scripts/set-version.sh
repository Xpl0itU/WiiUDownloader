#!/usr/bin/env bash
# Set the release version, and the Flatpak release date.
#
#   scripts/set-version.sh 3.1              # date defaults to today
#   scripts/set-version.sh 3.1 2026-10-01
#
# The version is written to every declaration, because the app has no single
# source for it: the Windows version resource, the macOS Info.plist, the Flatpak
# metainfo and the VERSION default in both build scripts. The date goes to the
# Flatpak <release> only.
set -euo pipefail

usage() {
    echo "usage: $(basename "$0") <version> [date]" >&2
    exit 2
}

[ $# -ge 1 ] && [ $# -le 2 ] || usage

version=$1
date=${2:-$(date +%F)}

# The Windows resource needs an X.Y[.Z[.W]] core; a suffix like -Beta1 is kept in
# the display strings but cannot be part of the numeric tuple.
if ! [[ $version =~ ^[0-9]+(\.[0-9]+)*([-+][0-9A-Za-z.]+)?$ ]]; then
    echo "not a version: ${version}" >&2
    exit 2
fi
if ! [[ $date =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    echo "not a date (want YYYY-MM-DD): ${date}" >&2
    exit 2
fi

cd "$(cd "$(dirname "$0")/.." && pwd)"

python3 - "$version" "$date" <<'PY'
import json
import plistlib
import re
import sys
import xml.etree.ElementTree as ET

version, date = sys.argv[1], sys.argv[2]


def patch(path, pattern, replacement, count=1, last=False):
    text = open(path).read()
    if last:
        matches = list(re.finditer(pattern, text))
        if len(matches) < count:
            raise SystemExit(f"{path}: expected {count} match(es), found {len(matches)}")
        match = matches[-count]
        text = text[: match.start()] + match.expand(replacement) + text[match.end() :]
    else:
        text, done = re.subn(pattern, replacement, text, count=count)
        if done != count:
            raise SystemExit(f"{path}: expected {count} match(es), found {done}")
    open(path, "w").write(text)


# Windows version resource.
path = "cmd/WiiUDownloader/versioninfo.json"
with open(path) as f:
    info = json.load(f)
parts = [int(p) for p in version.split("-")[0].split("+")[0].split(".")]
parts += [0] * (4 - len(parts))
fixed = dict(zip(("Major", "Minor", "Patch", "Build"), parts[:4]))
info["FixedFileInfo"]["FileVersion"] = fixed
info["FixedFileInfo"]["ProductVersion"] = fixed
info["StringFileInfo"]["FileVersion"] = version
info["StringFileInfo"]["ProductVersion"] = version
with open(path, "w") as f:
    json.dump(info, f, indent=4)
    f.write("\n")

# macOS bundle: key-scoped so the copyright string and the two version strings
# cannot be confused with each other.
path = "data/Info.plist"
patch(path, r"(<key>CFBundleGetInfoString</key>\s*<string>)[^<]+(</string>)",
      rf"\g<1>{version}, Copyright 2022-2026 Xpl0itU\g<2>")
patch(path, r"(<key>CFBundleShortVersionString</key>\s*<string>)[^<]+(</string>)", rf"\g<1>{version}\g<2>")
patch(path, r"(<key>CFBundleVersion</key>\s*<string>)[^<]+(</string>)", rf"\g<1>{version}\g<2>")

# Flatpak release: version and date.
patch("packaging/flatpak/io.github.xpl0itu.wiiudownloader.metainfo.xml",
      r'(<release version=")[^"]*(" date=")[^"]*(")', rf"\g<1>{version}\g<2>{date}\g<3>")

# Build-script defaults; a tag or an exported VERSION still wins at build time.
# Only the last default is rewritten: build-flatpak.sh starts with an empty
# VERSION="${VERSION:-}" that has to stay empty so the CI tag can fill it.
for path in ("scripts/build-flatpak.sh", "packaging/appimage/make-appimage.sh"):
    patch(path, r'VERSION="\$\{VERSION:-[^}]*\}"', f'VERSION="${{VERSION:-{version}}}"', last=True)

# Re-read every file so a silent mismatch fails the script instead of the release.
with open("cmd/WiiUDownloader/versioninfo.json") as f:
    json.load(f)
with open("data/Info.plist", "rb") as f:
    plistlib.load(f)
ET.parse("packaging/flatpak/io.github.xpl0itu.wiiudownloader.metainfo.xml")
PY

echo "version ${version}, flatpak release date ${date}"
