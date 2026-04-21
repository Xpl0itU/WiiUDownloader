#!/bin/env python

import os
import urllib.request
import ssl
import re

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

dest_path = "db.go"
checkAndDeleteFile(dest_path)

print("Downloading db.go...")
temp_db = "db_temp.go"
urllib.request.urlretrieve("https://napi.v10lator.de/db?t=go", temp_db)

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
