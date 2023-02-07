#!/bin/bash

# Does not grab `gtitles.c`, use build.py for that.

mkdir build 2>/dev/null
cmake -B build
cmake --build build
mkdir dist 2>/dev/null
mkdir dist/lib 2>/dev/null
mkdir dist/share 2>/dev/null
mkdir dist/share/icons 2>/dev/null
mkdir dist/share/glib-2.0 2>/dev/null
mkdir dist/share/glib-2.0/schemas/ 2>/dev/null
for ff in $(${MSYSTEM_PREFIX}/bin/ntldd -R build/WiiUDownloader.exe  | tr '\\' '/' | grep -io "$(cygpath -m ${MSYSTEM_PREFIX}).\+\.dll" | sort -u); do
    cp $(cygpath -u "$ff") dist/
done
cp build/WiiUDownloader.exe dist/
cp build/regFixLongPaths.exe dist/
cp -r ${MSYSTEM_PREFIX}/lib/gdk-pixbuf-2.0 ./dist/lib/gdk-pixbuf-2.0
cp -r ${MSYSTEM_PREFIX}/share/icons/* ./dist/share/icons/
cp ${MSYSTEM_PREFIX}/share/glib-2.0/schemas/* dist/share/glib-2.0/schemas/
glib-compile-schemas.exe dist/share/glib-2.0/schemas/
