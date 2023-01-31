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

checkAndDeleteFile("src/gtitles.c")
urllib.request.urlretrieve("https://napi.nbg01.v10lator.de/db", "src/gtitles.c")

try:
    os.mkdir("build")
except:
    pass
os.chdir("build")
os.system("cmake .. -DBUILD_LIBAPPIMAGEUPDATE_ONLY=ON")
os.system("cmake --build .")
