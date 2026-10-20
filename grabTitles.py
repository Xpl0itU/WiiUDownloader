#!/bin/env python

import os
import pycurl
import re

# Don't edit below this line


def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)


def cDownload(url, file):
    with open(file, "wb") as f:
        c = pycurl.Curl()
        c.setopt(c.URL, url)
        c.setopt(c.WRITEDATA, f)
        c.setopt(c.FOLLOWLOCATION, True)
        c.setopt(c.USERAGENT, "NUSspliBuilder/2.2")
        c.setopt(c.ACCEPT_ENCODING, "")
        c.perform()
        c.close()


dest_path = "db.go"
checkAndDeleteFile(dest_path)

print("Downloading db.go...")
temp_db = "db_temp.go"
cDownload("https://napi.v10lator.de/db?t=go", temp_db)

with open(temp_db, "r", encoding="utf-8") as f:
    content = f.read()

# Defensive post-processing
if "package wiiudownloader" not in content:
    content = "package wiiudownloader\n\n" + content

# Remove TitleEntry struct definition if present to avoid clashing with gtitles.go
struct_pattern = r"type TitleEntry struct \{.*?\}"
content = re.sub(struct_pattern, "", content, flags=re.DOTALL)

# Populate the library's TitleDatabase via init()
if "var titleEntry = " in content:
    content = content.replace("var titleEntry = ", "func init() {\n\tTitleDatabase = ")
    content += "\n}\n"

with open(dest_path, "w", encoding="utf-8") as f:
    f.write(content)

os.remove(temp_db)
print(f"Database saved to {dest_path}")