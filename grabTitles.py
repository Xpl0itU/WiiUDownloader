#!/bin/env python

import os
import urllib.request

# Don't edit below this line

def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)

opener = urllib.request.build_opener()
opener.addheaders = [("User-agent", "NUSspliBuilder/2.1")]
urllib.request.install_opener(opener)

checkAndDeleteFile("gtitles/gtitles.c")
urllib.request.urlretrieve("https://napi.nbg01.v10lator.de/db", "gtitles/gtitles.c")
os.system("gcc -c -Wall -fpic -Igtitles -o gtitles/gitles.o gtitles/gtitles.c")
os.system("gcc -shared -o gtitles/libgtitles.so gtitles/gitles.o")

os.system("gcc -c -Wall -fpic -Icdecrypt cdecrypt/*.c")
os.system("gcc -shared -o cdecrypt/libcdecrypt.so cdecrypt/*.o")
