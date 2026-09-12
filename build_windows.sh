#!/bin/bash
# Build WiiUDownloader for Windows inside MSYS2 (UCRT64).
# Mirrors .github/workflows/windows.yml so local builds and CI stay in sync.
#
#   ./build_windows.sh         build main.exe
#   ./build_windows.sh --run   build, then launch the executable
#   ./build_windows.sh --dist  build, then assemble dist/ and WiiUDownloader-Windows.zip
set -e

MODE="${1:-}"

GOPATH=$(go env GOPATH)
if command -v cygpath &> /dev/null; then
    GOPATH=$(cygpath -u "$GOPATH")
fi
export PATH=$PATH:$GOPATH/bin

if [ -z "$MSYSTEM_PREFIX" ]; then
    export MSYSTEM_PREFIX="/ucrt64"
fi

echo "Installing tools..."
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest

if [ ! -f db.go ]; then
    echo "Fetching title database..."
    curl --insecure -Lo db.go -H "User-Agent: NUSspliBuilder/2.1" "https://napi.v10lator.de/db?t=go"
fi

echo "Generating icon..."
if command -v magick &> /dev/null; then
    magick data/WiiUDownloader.png -define icon:auto-resize=256,128,64,48,32,16 cmd/WiiUDownloader/WiiUDownloader.ico
else
    echo "Warning: ImageMagick (magick) not found, icon generation skipped."
fi

echo "Building WiiUDownloader..."
(cd cmd/WiiUDownloader && go generate && GOARCH=amd64 go build -ldflags="-H=windowsgui" -o ../../main.exe .)

if [ "$MODE" = "--run" ]; then
    exec ./main.exe
fi

if [ "$MODE" != "--dist" ]; then
    echo "Build complete: main.exe"
    exit 0
fi

echo "Preparing distribution directory..."
rm -rf dist
mkdir -p dist/lib dist/share/icons dist/share/glib-2.0/schemas

echo "Copying dependencies..."
${MSYSTEM_PREFIX}/bin/ntldd -R main.exe | tr '\\' '/' | grep -io "$(cygpath -m ${MSYSTEM_PREFIX}).\+\.dll" | sort -u | cygpath -f - -u | xargs -I {} cp "{}" dist/

cp -r "${MSYSTEM_PREFIX}/lib/gdk-pixbuf-2.0" ./dist/lib/gdk-pixbuf-2.0

find dist/lib/gdk-pixbuf-2.0/2.10.0/loaders/ -name "*.dll" -print0 | xargs -0 -I {} ${MSYSTEM_PREFIX}/bin/ntldd -R "{}" | tr '\\' '/' | grep -io "$(cygpath -m ${MSYSTEM_PREFIX}).\+\.dll" | sort -u | cygpath -f - -u | xargs -I {} cp -n "{}" dist/ || true

cp -r "${MSYSTEM_PREFIX}/share/icons/"* ./dist/share/icons/
cp -r "${MSYSTEM_PREFIX}/share/glib-2.0/schemas/"* dist/share/glib-2.0/schemas/

echo "Compiling schemas and updating loaders..."
glib-compile-schemas.exe dist/share/glib-2.0/schemas/
gdk-pixbuf-query-loaders > dist/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache
cp main.exe dist/WiiUDownloader.exe

echo "Creating zip archive..."
cd dist
rm -f libx265-*.dll
zip -9 -r ../WiiUDownloader-Windows.zip .
cd ..

echo "Package complete: WiiUDownloader-Windows.zip"
