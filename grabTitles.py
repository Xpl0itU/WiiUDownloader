#!/bin/env python

import os
import urllib.request
import ssl

# Don't edit below this line

def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)

# Disable certificate verification
ssl_context = ssl.create_default_context()
ssl_context.check_hostname = False
ssl_context.verify_mode = ssl.CERT_NONE

opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=ssl_context))
opener.addheaders = [("User-agent", "NUSspliBuilder/2.1")]
urllib.request.install_opener(opener)

checkAndDeleteFile("gtitles/gtitles.c")
urllib.request.urlretrieve("https://napi.nbg01.v10lator.de/db", "gtitles/gtitles.c")
os.system("gcc -c -Wall -fpic -Igtitles -o gtitles/gtitles.o gtitles/gtitles.c")
os.system("ar rcs libgtitles.a gtitles/gtitles.o")
os.system("gcc -shared -o gtitles/libgtitles.so gtitles/gtitles.o")

os.system("gcc -c -Wall -fpic -Icdecrypt cdecrypt/*.c")
os.system("ar rcs libcdecrypt.a *.o")
os.system("gcc -shared -o cdecrypt/libcdecrypt.so *.o")
