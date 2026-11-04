---
name: wiiudownloader-cli
description: >
  Operate the WiiUDownloader command-line interface (CLI) to search, inspect,
  download, and decrypt Wii U games, updates, DLCs, and demos from Nintendo's CDN.
  Use when: "wiiudownloader cli", "download wii u game", "decrypt wii u", "wiiu cli",
  "download title id", "wii u update", "wii u dlc", "generate fake ticket", "wiiudownloader",
  or when automating Wii U downloads, decryption, or NUS asset extraction.
---

# WiiUDownloader CLI — Agent Skill

This skill guides AI agents on how to use the `wiiudownloader` command-line tool to search, inspect, download, and decrypt Wii U titles (Games, Updates, DLCs, Demos) directly from Nintendo's CDN without requiring external title keys.

---

## 1. Binary Availability & Build

Check if `wiiudownloader` exists in the repository root or in `$PATH`:

```bash
# Check existing binary
./wiiudownloader --version
```

If not compiled yet, build it in seconds with pure Go (requires zero CGo or GTK dependencies):

```bash
# Compile standalone CLI in root
go build -o wiiudownloader ./cmd/wiiudownloader
```

> [!NOTE]
> The GUI executable (`WiiUDownloader`) also delegates to the CLI automatically when invoked with any CLI subcommand (e.g. `./WiiUDownloader search ...` or `./WiiUDownloader download ...`), so agents can use whichever binary is available.

---

## 2. Agent Best Practices

When running commands as an agent:
1. **Always use `--json`** for read-only query commands (`search`, `list`, `info`, `files`) to get structured, machine-parseable output.
2. **Always use `-q` or `--quiet`** when downloading or decrypting to avoid spamming the tool output with carriage return (`\r`) progress bar updates.
3. **Use `-d` (`--decrypt`)** if the user wants playable files for Cemu or console extraction.
4. **Use `--delete-encrypted`** with `-d` if the user wants to conserve disk space by removing raw `.app` and `.h3` files after decryption.

---

## 3. Core Workflows

### 3.1 Searching and Finding Titles

Search the built-in database (3,700+ titles) by name or partial Title ID:

```bash
# Search by name with region and category filters (JSON output)
./wiiudownloader search "Mario Kart 8" -r usa -t game --json

# Human-readable table
./wiiudownloader search "Zelda" -r usa -t game
```

**Filter Options:**
- `-r, --region`: `usa`, `eur`, `jpn`, or comma-separated `usa,eur` (default: all)
- `-t, --type, --category`: `game`, `update`, `dlc`, `demo`, `all` (default: all)
- `-l, --limit`: max results (default: 50, 0 for all)

---

### 3.2 Inspecting Title Details & CDN Metadata

Fetch metadata, exact CDN payload size, content count, and related updates/DLC:

```bash
# Inspect title and query Nintendo CDN for size and contents
./wiiudownloader info 0005000010143500 --json
```

**Key Information Returned:**
- `title_id`: 16-character hexadecimal Title ID
- `name`: Official title name
- `region`: USA / Europe / Japan / All
- `category`: Game / Update / DLC / Demo
- `total_size`: Total byte size of all contents on Nintendo CDN
- `contents`: Array of content files with IDs, uncompressed sizes, and hash flags
- `related_update`: Matching Update Title ID and name
- `related_dlc`: Matching DLC Title ID and name

---

### 3.3 Downloading Titles

#### Standard Download & Decrypt
```bash
# Download and decrypt to a specific folder
./wiiudownloader download 0005000010143500 -d -o ./games -q
```

#### Download Game + Update + DLC in One Go
Use `-a` or `--all` to automatically resolve and queue the base game, its latest update, and all DLCs:
```bash
./wiiudownloader download 0005000010143500 -d --all -o ./games -q
```
Or selectively include updates/DLC:
- `-u, --with-update`: Only game + update
- `-c, --with-dlc`: Only game + DLC

#### Save Disk Space (Delete Encrypted After Decryption)
```bash
./wiiudownloader download 0005000010143500 -d --delete-encrypted -o ./games -q
```

#### Separate Decryption Output
To keep encrypted assets in one directory and decrypted playable files in another (e.g. Cemu `mlc01`):
```bash
./wiiudownloader download 0005000010143500 -d -o ./downloads --decrypt-out /path/to/cemu/games -q
```

#### Download a Specific Title Version
```bash
./wiiudownloader download 0005000e10143500 -v 64 -d -q
```

---

### 3.4 Batch Downloads

#### From a Text File
Create a file (e.g. `titles.txt`) containing Title IDs (one per line, `#` comments ignored):
```text
# Zelda Wind Waker HD
0005000010143500
# Mario Kart 8
000500001010EC00
```
Run:
```bash
./wiiudownloader download --file titles.txt -d --all -o ./downloads -q
```

#### From GUI Queue
If the user previously queued titles in the WiiUDownloader graphical app:
```bash
# View items in queue
./wiiudownloader queue list

# Download all queued items
./wiiudownloader download --from-queue -d --delete-encrypted -q

# Clear the queue
./wiiudownloader queue clear
```

---

### 3.5 Downloading Specific Files (Fast Partial Extraction)

Inspect internal game files inside the FST before downloading:
```bash
# Inspect FST file tree
./wiiudownloader files 0005000010143500 -s meta --json
```

Download and decrypt **only** the selected files (e.g. icons, XML metadata, or specific assets):
```bash
./wiiudownloader download 0005000010143500 --files "meta/meta.xml,meta/iconTex.tga" -o ./extracted -q
```

---

### 3.6 Decrypting Existing Folders

Decrypt one or more title folders that were already downloaded in encrypted format:
```bash
# Decrypt in place
./wiiudownloader decrypt /path/to/game_folder -q

# Decrypt and delete raw encrypted files (.app, .h3)
./wiiudownloader decrypt /path/to/game_folder --delete-encrypted -q

# Decrypt to a separate destination directory
./wiiudownloader decrypt /path/to/game_folder -o /path/to/cemu/games -q
```

---

### 3.7 Generating Fake Tickets and Certificates

Generate `title.tik` and `title.cert` without downloading the full game:
```bash
# From Title ID (fetches remote TMD to determine keys)
./wiiudownloader ticket 0005000010143500 -o ./output

# From a local title.tmd file
./wiiudownloader ticket /path/to/title.tmd
```

---

### 3.8 Database Management

```bash
# View database statistics
./wiiudownloader db stats

# Force refresh/update title database from server
./wiiudownloader db update
```

---

## 4. Output Folder Structure

Downloaded titles are stored in canonical folders matching the GUI naming scheme:
```
<OutputDirectory>/<NormalizedName> [<Kind>] [<TitleID>] [v<Version>]
```
Example:
```
./games/THE LEGEND OF ZELDA The Wind Waker HD [Game] [0005000010143500]/
├── code/
│   ├── app.xml
│   └── cos.xml
├── content/
│   └── ...
└── meta/
    ├── meta.xml
    └── iconTex.tga
```
If not decrypted (`-d` omitted), the folder contains raw NUS files:
```
├── 00000000.app
├── 00000001.app
├── 00000001.h3
├── title.cert
├── title.tik
└── title.tmd
```
