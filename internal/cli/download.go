package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type downloadTarget struct {
	TitleID uint64
	Name    string
	Version int
}

// RunDownload handles the "download" subcommand.
func RunDownload(args []string) int {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	var (
		outputDir       string
		decrypt         bool
		deleteEncrypted bool
		decryptOut      string
		version         int
		withUpdate      bool
		withDLC         bool
		allRelated      bool
		filesStr        string
		filePath        string
		fromQueue       bool
		continueOnError bool
		regionStr       string
		quiet           bool
	)

	fs.StringVar(&outputDir, "o", "./downloads", "Output directory for downloaded files")
	fs.StringVar(&outputDir, "output", "./downloads", "Output directory for downloaded files")
	fs.BoolVar(&decrypt, "d", false, "Decrypt contents after downloading")
	fs.BoolVar(&decrypt, "decrypt", false, "Decrypt contents after downloading")
	fs.BoolVar(&deleteEncrypted, "delete-encrypted", false, "Delete encrypted files (.app, .h3) after decryption")
	fs.StringVar(&decryptOut, "decrypt-out", "", "Separate output directory for decrypted files")
	fs.IntVar(&version, "v", wiiudownloader.VersionLatest, "Specific title version (-1 for latest)")
	fs.IntVar(&version, "version", wiiudownloader.VersionLatest, "Specific title version (-1 for latest)")
	fs.BoolVar(&withUpdate, "u", false, "Also download the related update for each game")
	fs.BoolVar(&withUpdate, "with-update", false, "Also download the related update for each game")
	fs.BoolVar(&withDLC, "c", false, "Also download the related DLC for each game")
	fs.BoolVar(&withDLC, "with-dlc", false, "Also download the related DLC for each game")
	fs.BoolVar(&allRelated, "a", false, "Download game + update + DLC")
	fs.BoolVar(&allRelated, "all", false, "Download game + update + DLC")
	fs.StringVar(&filesStr, "files", "", "Download only specific files (comma-separated, e.g. 'code/app.xml,meta/meta.xml')")
	fs.StringVar(&filePath, "file", "", "Text file containing Title IDs to download (one per line)")
	fs.BoolVar(&fromQueue, "from-queue", false, "Download titles saved in WiiUDownloader GUI queue")
	fs.BoolVar(&continueOnError, "continue-on-error", true, "Continue to next title if an error occurs")
	fs.StringVar(&regionStr, "r", "", "Filter region when searching by name (usa, eur, jpn)")
	fs.StringVar(&regionStr, "region", "", "Filter region when searching by name (usa, eur, jpn)")
	fs.BoolVar(&quiet, "q", false, "Quiet mode: minimal output")
	fs.BoolVar(&quiet, "quiet", false, "Quiet mode: minimal output")

	boolFlags := map[string]bool{
		"d":                  true,
		"decrypt":            true,
		"delete-encrypted":   true,
		"u":                  true,
		"with-update":        true,
		"c":                  true,
		"with-dlc":           true,
		"a":                  true,
		"all":                true,
		"from-queue":         true,
		"continue-on-error":  true,
		"q":                  true,
		"quiet":              true,
	}
	if err := fs.Parse(ReorderArgs(args, boolFlags)); err != nil {
		return 1
	}

	if err := EnsureTitleDatabase(quiet); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	var rawTargets []string
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", filePath, err)
			return 1
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				rawTargets = append(rawTargets, line)
			}
		}
	}

	rawTargets = append(rawTargets, fs.Args()...)

	var targets []downloadTarget

	if fromQueue {
		qTitles, err := GetPersistedQueue()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading queue: %v\n", err)
			return 1
		}
		for _, qt := range qTitles {
			targets = append(targets, downloadTarget{
				TitleID: qt.TitleID,
				Name:    qt.Name,
				Version: qt.Version,
			})
		}
	}

	// Process raw targets (IDs or names)
	regionMask := parseRegionFilter(regionStr)
	for _, raw := range rawTargets {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		if tid, err := wiiudownloader.ParseTitleID(raw); err == nil && tid > 0 {
			entry := wiiudownloader.GetTitleEntryFromTid(tid)
			name := entry.Name
			if name == "" {
				name = fmt.Sprintf("%016x", tid)
			}
			targets = append(targets, downloadTarget{
				TitleID: tid,
				Name:    name,
				Version: version,
			})
		} else {
			// Search by name
			entries := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL)
			rawLower := strings.ToLower(raw)
			var found []wiiudownloader.TitleEntry
			for _, e := range entries {
				if e.Region&regionMask == 0 {
					continue
				}
				if strings.EqualFold(e.Name, raw) {
					found = []wiiudownloader.TitleEntry{e}
					break
				}
				if strings.Contains(strings.ToLower(e.Name), rawLower) {
					found = append(found, e)
				}
			}

			if len(found) == 0 {
				fmt.Fprintf(os.Stderr, "Error: title %q not found in database\n", raw)
				if !continueOnError {
					return 1
				}
				continue
			}

			if len(found) > 1 {
				fmt.Printf("Multiple titles matched %q:\n", raw)
				for i, f := range found {
					if i >= 10 {
						fmt.Printf("  ... and %d more (use search or specify Title ID)\n", len(found)-10)
						break
					}
					fmt.Printf("  - %016x  [%s]  %s (%s)\n",
						f.TitleID,
						wiiudownloader.GetFormattedKind(f.TitleID),
						f.Name,
						wiiudownloader.GetFormattedRegion(f.Region),
					)
				}
				fmt.Printf("Selecting first match: %016x (%s)\n\n", found[0].TitleID, found[0].Name)
			}

			targets = append(targets, downloadTarget{
				TitleID: found[0].TitleID,
				Name:    found[0].Name,
				Version: version,
			})
		}
	}

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no valid Title IDs or titles specified to download")
		fmt.Fprintln(os.Stderr, "Usage: wiiudownloader download [options] <title_id...>")
		return 1
	}

	// Expand related items if requested
	if withUpdate || withDLC || allRelated {
		var expanded []downloadTarget
		seen := make(map[uint64]struct{})
		for _, t := range targets {
			if _, ok := seen[t.TitleID]; !ok {
				seen[t.TitleID] = struct{}{}
				expanded = append(expanded, t)
			}

			entry := wiiudownloader.GetTitleEntryFromTid(t.TitleID)
			if entry.TitleID == 0 {
				continue
			}

			if withUpdate || allRelated {
				if upd, ok := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_UPDATE, seen); ok {
					seen[upd.TitleID] = struct{}{}
					expanded = append(expanded, downloadTarget{
						TitleID: upd.TitleID,
						Name:    upd.Name,
						Version: wiiudownloader.VersionLatest,
					})
				}
			}

			if withDLC || allRelated {
				if dlc, ok := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_DLC, seen); ok {
					seen[dlc.TitleID] = struct{}{}
					expanded = append(expanded, downloadTarget{
						TitleID: dlc.TitleID,
						Name:    dlc.Name,
						Version: wiiudownloader.VersionLatest,
					})
				}
			}
		}
		targets = expanded
	}

	// Parse specific files if requested
	var specificPaths []string
	if filesStr != "" {
		for _, p := range strings.Split(filesStr, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				specificPaths = append(specificPaths, p)
			}
		}
	}

	if !quiet {
		fmt.Printf("Queued %d title(s) for download:\n", len(targets))
		for i, t := range targets {
			vStr := "latest"
			if t.Version >= 0 {
				vStr = fmt.Sprintf("v%d", t.Version)
			}
			fmt.Printf("  %d. %016x  [%s]  %s [%s]\n",
				i+1,
				t.TitleID,
				wiiudownloader.GetFormattedKind(t.TitleID),
				t.Name,
				vStr,
			)
		}
		fmt.Printf("\nOutput folder: %s\n", outputDir)
		if decrypt {
			fmt.Printf("Decryption: enabled (delete encrypted: %v)\n", deleteEncrypted)
			if decryptOut != "" {
				fmt.Printf("Decrypted output folder: %s\n", decryptOut)
			}
		}
		fmt.Println()
	}

	client := wiiudownloader.BuildHTTPClient()
	reporter := NewTerminalProgressReporter(quiet)
	defer reporter.Close()

	var successCount, failCount int
	for i, t := range targets {
		if reporter.Cancelled() {
			break
		}

		tidStr := fmt.Sprintf("%016x", t.TitleID)
		folderName := wiiudownloader.FormatTitleFolderName(t.Name, t.TitleID, t.Version)
		titleDst := filepath.Join(outputDir, folderName)

		if !quiet {
			fmt.Printf("\n[%d/%d] Starting: %s (%s)\n", i+1, len(targets), t.Name, tidStr)
		}

		var downloadErr error
		if len(specificPaths) > 0 {
			tree, err := wiiudownloader.FetchTitleFileTree(t.TitleID, t.Version, client)
			if err != nil {
				downloadErr = err
			} else {
				downloadErr = tree.DownloadFiles(titleDst, specificPaths, reporter)
				tree.Close()
			}
		} else {
			downloadErr = wiiudownloader.DownloadTitle(
				tidStr,
				titleDst,
				t.Version,
				decrypt,
				reporter,
				deleteEncrypted,
				client,
				decryptOut,
			)
		}

		if downloadErr != nil {
			reporter.ClearLine()
			if reporter.Cancelled() {
				break
			}
			fmt.Fprintf(os.Stderr, "\n✗ Error downloading %s: %v\n", t.Name, downloadErr)
			failCount++
			if !continueOnError {
				return 1
			}
		} else {
			if !reporter.Cancelled() {
				reporter.ClearLine()
				if !quiet {
					fmt.Printf("✓ Finished: %s\n", folderName)
				}
				successCount++
			}
		}
	}

	if !quiet {
		fmt.Println()
		fmt.Printf("Download Complete: %d succeeded, %d failed.\n", successCount, failCount)
	}

	if failCount > 0 {
		return 1
	}
	return 0
}
