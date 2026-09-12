#!/bin/sh
# Turns the binary built by get-dependencies.sh into an anylinux AppImage:
# quick-sharun bundles every library the app needs (GTK4, libadwaita, glib, the
# dynamic linker and libc) so the result runs on any distro, including musl ones.
set -eu

ARCH=$(uname -m)
VERSION="${VERSION:-2.105}"
export ARCH VERSION

export OUTPATH=./dist
export OUTNAME="WiiUDownloader-Linux-$ARCH.AppImage"
export ICON=./data/WiiUDownloader.png
export DESKTOP=./packaging/appimage/WiiUDownloader.desktop

# Ships the optional self-updater and the zsync info it checks against.
export ADD_HOOKS="self-updater.hook"
export UPINFO="gh-releases-zsync|Xpl0itU|WiiUDownloader|latest|WiiUDownloader-Linux-$ARCH.AppImage.zsync"

# Sets WM_CLASS so the launcher picks up the bundled icon.
export GTK_CLASS_FIX=1

quick-sharun /usr/bin/wiiudownloader
quick-sharun --make-appimage

# Actually run the AppImage; the container has xvfb from the anylinux setup.
quick-sharun --test ./dist/*.AppImage
