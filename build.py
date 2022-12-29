#!/bin/env python

import os
import shutil
import urllib.request

# Don't edit below this line

def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)

opener = urllib.request.build_opener()
opener.addheaders = [("User-agent", "NUSspliBuilder/2.1")]
urllib.request.install_opener(opener)

checkAndDeleteFile("src/gtitles.c")
urllib.request.urlretrieve("https://napi.nbg01.v10lator.de/db", "src/gtitles.c")

try:
    os.mkdir("build")
except:
    pass
os.chdir("build")
os.system("cmake ..")
os.system("cmake --build .")
if os.name == 'nt':
    os.makedirs("dist/lib/gdk-pixbuf-2.0")
    os.system("ldd build/WiiUDownloader.exe | grep '\/mingw.*\.dll' -o | xargs -I{} cp "{}" ./dist")
    shutil.copy("/mingw64/lib/gdk-pixbuf-2.0", "dist/lib/gdk-pixbuf-2.0")
    os.makedirs("dist/share/icons")
    shutil.copytree("/mingw64/share/icons/", "dist/share/icons/")
    os.makedirs("dist/share/glib-2.0/schemas/")
    shutil.copytree("/mingw64/share/glib-2.0/schemas/", "dist/share/glib-2.0/schemas/")
    os.system("glib-compile-schemas.exe build/dist/share/glib-2.0/schemas/")