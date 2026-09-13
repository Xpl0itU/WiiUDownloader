#!/bin/sh
# Runs inside the pkgforge Arch container (see .github/workflows/linux.yml).
# The binary is built here on purpose: quick-sharun bundles the libraries this
# container ships, so the binary has to be linked against them.
set -eu

echo "Installing build dependencies..."
echo "---------------------------------------------------------------"
pacman -Syu --noconfirm --needed \
	base-devel \
	curl \
	git \
	go \
	pkgconf \
	gtk4 \
	libadwaita \
	librsvg \
	gdk-pixbuf2 \
	glib2 \
	adwaita-icon-theme \
	hicolor-icon-theme \
	gobject-introspection \
	zsync

echo "Installing debloated packages..."
echo "---------------------------------------------------------------"
get-debloated-pkgs --add-common --prefer-nano

echo "Building WiiUDownloader..."
echo "---------------------------------------------------------------"
cd "$(dirname "$0")/../.."

curl --insecure -Lo db.go -H 'User-Agent: NUSspliBuilder/2.1' 'https://napi.v10lator.de/db?t=go'
if grep -q 'var titleEntry =' db.go; then
	if grep -q 'type TitleEntry struct' db.go; then
		sed -i '/type TitleEntry struct/,/}/d' db.go
	fi
	sed -i 's/var titleEntry =/func init() { TitleDatabase =/' db.go
	echo '}' >> db.go
fi

go build -C cmd/WiiUDownloader -o /usr/bin/wiiudownloader .
