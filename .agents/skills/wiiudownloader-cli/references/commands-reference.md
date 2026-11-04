# WiiUDownloader CLI — Complete Flag & Command Reference

## Global Flags

| Flag | Description |
|---|---|
| `-h, --help` | Show general help or command-specific help |
| `-v, --version` | Display current CLI version |

---

## 1. `download` (alias: `d`)

Download titles from Nintendo CDN.

```text
Usage: wiiudownloader download [flags] <title_id...>
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-o, --output` | `string` | `./downloads` | Destination directory |
| `-d, --decrypt` | `bool` | `false` | Decrypt contents after download |
| `--delete-encrypted` | `bool` | `false` | Remove raw encrypted `.app` and `.h3` files after decryption |
| `--decrypt-out` | `string` | `""` | Separate directory to place decrypted files into |
| `-v, --version` | `int` | `-1` | Title version (-1 selects latest) |
| `-u, --with-update` | `bool` | `false` | Also download related update |
| `-c, --with-dlc` | `bool` | `false` | Also download related DLC |
| `-a, --all` | `bool` | `false` | Download game + latest update + DLC |
| `--files` | `string` | `""` | Comma-separated list of specific FST files to extract |
| `--file` | `string` | `""` | Path to text file containing Title IDs |
| `--from-queue` | `bool` | `false` | Download titles currently in GUI queue (`queue.json`) |
| `--continue-on-error` | `bool` | `true` | Continue next download if current item fails |
| `-r, --region` | `string` | `""` | Region filter when specifying title by name (`usa`, `eur`, `jpn`) |
| `-q, --quiet` | `bool` | `false` | Disable animated terminal progress bar |

---

## 2. `search` (alias: `s`)

Search local title database by name or Title ID.

```text
Usage: wiiudownloader search [flags] <query>
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-r, --region` | `string` | `""` | Filter by region: `usa`, `eur`, `jpn`, `all` (can comma-separate) |
| `-t, --type, --category` | `string` | `"all"` | Filter by category: `game`, `update`, `dlc`, `demo`, `all` |
| `-l, --limit` | `int` | `50` | Maximum results to display (0 for unlimited) |
| `--json` | `bool` | `false` | Output results in JSON format |

---

## 3. `list` (alias: `ls`)

List titles from the database.

```text
Usage: wiiudownloader list [flags]
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-r, --region` | `string` | `""` | Filter by region: `usa`, `eur`, `jpn`, `all` |
| `-t, --type, --category` | `string` | `"game"` | Filter by category: `game`, `update`, `dlc`, `demo`, `all` |
| `-l, --limit` | `int` | `100` | Maximum results to display (0 for unlimited) |
| `--json` | `bool` | `false` | Output results in JSON format |

---

## 4. `info`

Inspect metadata and query Nintendo CDN for size and contents.

```text
Usage: wiiudownloader info [flags] <title_id | name>
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-v, --version` | `int` | `-1` | Version to inspect (-1 for latest) |
| `--remote` | `bool` | `true` | Fetch remote TMD metadata from CDN |
| `--json` | `bool` | `false` | Output results in JSON format |

---

## 5. `decrypt`

Decrypt one or more local folders containing encrypted titles.

```text
Usage: wiiudownloader decrypt [flags] <folder...>
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-o, --output` | `string` | `""` | Separate output directory for decrypted files |
| `--delete-encrypted` | `bool` | `false` | Remove raw `.app`, `.h3`, `.tik`, `.tmd` files after extraction |
| `-q, --quiet` | `bool` | `false` | Suppress progress output |

---

## 6. `ticket`

Generate fake ticket (`title.tik`) and certificate (`title.cert`).

```text
Usage: wiiudownloader ticket [flags] <title_id | path_to_tmd>
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-o, --output` | `string` | `""` | Output directory (default: current directory or TMD directory) |
| `-v, --version` | `int` | `-1` | Title version (when providing Title ID) |

---

## 7. `files`

Inspect the internal FST directory tree without downloading full title.

```text
Usage: wiiudownloader files [flags] <title_id>
```

### Flags
| Flag | Type | Default | Description |
|---|---|---|---|
| `-v, --version` | `int` | `-1` | Title version (-1 for latest) |
| `-s, --search` | `string` | `""` | Filter files by path or extension (e.g. `.xml` or `meta/`) |
| `--json` | `bool` | `false` | Output in JSON format |

---

## 8. `queue`

Manage the saved GUI download queue (`~/.config/WiiUDownloader/queue.json`).

```text
Usage: wiiudownloader queue <list | download | clear> [flags]
```

---

## 9. `db`

Manage the local title database and cache.

```text
Usage: wiiudownloader db <stats | update>
```
