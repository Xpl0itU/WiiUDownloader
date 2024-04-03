#!/bin/env python

import os
import requests

# Don't edit below this line


def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)


def downloadFile(url, file, headers={}):
    print(f"Downloading {file}")
    with requests.get(url, headers=headers) as r:
        with open(file, "wb") as f:
            f.write(r.content)


checkAndDeleteFile("db.go")
downloadFile(
    "https://napi.v10lator.de/db?t=go",
    "db.go",
    {"User-Agent": "NUSspliBuilder/2.2", "Accept-Encoding": "br, gzip, deflate"},
)
