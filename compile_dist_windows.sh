#!/bin/bash
set -e

# Script to compile and package WiiUDownloader for Windows.
# Intended to be run in an MSYS2 environment (UCRT64).

echo "Starting build and package process..."

# Get GOPATH and handle Windows paths
GOPATH=$(go env GOPATH)
if command -v cygpath &> /dev/null; then
    GOPATH=$(cygpath -u "$GOPATH")
fi
export PATH=$PATH:$GOPATH/bin

# Set MSYSTEM_PREFIX if not set (default to /ucrt64 as in CI)
if [ -z "$MSYSTEM_PREFIX" ]; then
    export MSYSTEM_PREFIX="/ucrt64"
fi

echo "Installing tools..."
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest

echo "Generating assets..."
python3 grabTitles.py
if command -v magick &> /dev/null; then
    magick data/WiiUDownloader.png -define icon:auto-resize=256,128,64,48,32,16 cmd/WiiUDownloader/WiiUDownloader.ico
else
    echo "Warning: ImageMagick not found. Skipping icon generation."
fi

go generate
cd ../..

echo "Building WiiUDownloader..."
cd cmd/WiiUDownloader
go generate
go build -ldflags="-H=windowsgui" -o ../../main.exe .
cd ../..

echo "Preparing distribution directory..."
rm -rf dist
mkdir -p dist/lib
mkdir -p dist/share/icons
mkdir -p dist/share/glib-2.0
mkdir -p dist/share/glib-2.0/schemas/

echo "Copying dependencies..."
if ! command -v ntldd &> /dev/null; then
    echo "Error: ntldd not found. Ensure you are in MSYS2 environment with mingw-w64-ucrt-x86_64-ntldd installed."
    exit 1
fi

# Copy DLLs for main.exe
for ff in $(ntldd -R main.exe  | tr '\\' '/' | grep -io "$(cygpath -m ${MSYSTEM_PREFIX}).\+\.dll" | sort -u); do
    cp "$(cygpath -u "$ff")" dist/
done

# Copy gdk-pixbuf
cp -r "${MSYSTEM_PREFIX}/lib/gdk-pixbuf-2.0" ./dist/lib/gdk-pixbuf-2.0
for loader in dist/lib/gdk-pixbuf-2.0/2.10.0/loaders/*.dll; do
    for dep in $(ntldd -R "$(cygpath -m "$loader")" | tr '\\' '/' | grep -io "$(cygpath -m ${MSYSTEM_PREFIX}).\+\.dll" | sort -u); do
        cp "$(cygpath -u "$dep")" dist/ || true
    done
done

# Copy icons and schemas
cp -r "${MSYSTEM_PREFIX}/share/icons/"* ./dist/share/icons/
cp "${MSYSTEM_PREFIX}/share/glib-2.0/schemas/"* dist/share/glib-2.0/schemas/

echo "Compiling schemas and updating loaders..."
glib-compile-schemas.exe dist/share/glib-2.0/schemas/
gdk-pixbuf-query-loaders > dist/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache

echo "Copying executables..."
fi
cp main.exe dist/WiiUDownloader.exe

echo "Creating zip archive..."
cd dist
if [ -f "libx265-215.dll" ]; then
    rm "libx265-215.dll"
fi
zip -9 -r ../WiiUDownloader-Windows.zip .
cd ..

echo "Build and package complete: WiiUDownloader-Windows.zip"
