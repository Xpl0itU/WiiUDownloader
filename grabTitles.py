#!/bin/env python

import os
import pycurl

# Don't edit below this line

def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)

def cDownload(url, file):
    with open(file, 'wb') as f:
        c = pycurl.Curl()
        c.setopt(c.URL, url)
        c.setopt(c.WRITEDATA, f)
        c.setopt(c.FOLLOWLOCATION, True)
        c.setopt(c.USERAGENT, "NUSspliBuilder/2.2")
        c.setopt(c.ACCEPT_ENCODING, "")
        c.perform()
        c.close()

checkAndDeleteFile("db.go")
cDownload("https://napi.v10lator.de/db?t=go", "db.go")
