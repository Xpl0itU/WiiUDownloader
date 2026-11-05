#!/bin/env python

import gzip
import os
import re
import subprocess
import urllib.request

# Don't edit below this line


def checkAndDeleteFile(file):
    if os.path.exists(file):
        print(f"Deleting {file}")
        os.remove(file)


def cDownload(url, file):
    # Try curl first if available (handles HTTP/1.1 and TLS negotiation with napi server)
    try:
        subprocess.run(
            ["curl", "-s", "--compressed", "--http1.1", "-Lo", file, "-H", "User-Agent: NUSspliBuilder/2.2", url],
            check=True,
        )
        if os.path.exists(file) and os.path.getsize(file) > 0:
            return
    except Exception:
        pass

    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": "NUSspliBuilder/2.2",
            "Accept-Encoding": "gzip, deflate",
        },
    )
    with urllib.request.urlopen(req) as response:
        content = response.read()
        if response.headers.get("Content-Encoding") == "gzip" or content[:2] == b"\x1f\x8b":
            content = gzip.decompress(content)
        with open(file, "wb") as f:
            f.write(content)


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