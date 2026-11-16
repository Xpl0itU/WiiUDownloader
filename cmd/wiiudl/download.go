package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type downloadItem struct {
	entry   wiiudownloader.TitleEntry
	version int
	dir     string
}

type downloadResultJSON struct {
	Name            string  `json:"name"`
	TitleID         string  `json:"titleId"`
	Version         string  `json:"version"`
	Path            string  `json:"path"`
	Status          string  `json:"status"`
	Error           string  `json:"error,omitempty"`
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
}

func cmdDownload(application *app, args []string) error {
	fs := newFlagSet(application, "download", "Download one or more Wii U titles.", `Usage:
  wiiudl download [flags] <title...>

A title is a 16-digit title ID or a name/query. Multiple titles may be given.
Each title is written to a subdirectory of --output named
"<name> [<kind>] [<title id>]" (plus " [vN]" for a pinned version).

Flags:`)
	outputDir := "."
	fs.StringVar(&outputDir, "output", ".", "Parent directory for downloaded titles")
	fs.StringVar(&outputDir, "o", ".", "Alias for --output")
	version := fs.String("version", "latest", "Title version: latest or a non-negative number")
	decrypt := fs.Bool("decrypt", false, "Decrypt content after downloading")
	deleteEncrypted := fs.Bool("delete-encrypted", false, "Delete encrypted .app/.h3 files after decryption")
	decryptOutput := fs.String("decrypt-output", "", "Parent directory for decrypted output (a title-named subfolder is created)")
	failFast := fs.Bool("fail-fast", false, "Stop after the first failed title")
	dryRun := fs.Bool("dry-run", false, "Show what would be downloaded without downloading")
	first := fs.Bool("first", false, "Pick the first name match instead of prompting when ambiguous")
	region := fs.String("region", "all", "Regions used when resolving names: usa,eur,jpn,chn,kor,twn,all")
	category := fs.String("type", "all", "Content type used when resolving names: game, update, dlc, demo or all")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}

	if len(positional) == 0 {
		fs.Usage()
		return silentExit{code: 2}
	}
	if *deleteEncrypted && !*decrypt {
		return usageError{fmt.Errorf("--delete-encrypted requires --decrypt")}
	}
	if *decryptOutput != "" && !*decrypt {
		return usageError{fmt.Errorf("--decrypt-output requires --decrypt")}
	}
	regionMask, err := parseRegionMask(*region)
	if err != nil {
		return usageError{err: err}
	}
	categoryValue, err := parseCategory(*category)
	if err != nil {
		return usageError{err: err}
	}

	db, err := application.loadDBForTitles(positional)
	if err != nil {
		return err
	}

	outputDir = absolutePath(outputDir)
	if *decryptOutput != "" {
		*decryptOutput = absolutePath(*decryptOutput)
	}

	items, err := resolveDownloadItems(application, db, positional, outputDir, *version, regionMask, categoryValue, *first)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no titles to download")
	}

	if *dryRun {
		if application.json {
			results := make([]downloadResultJSON, 0, len(items))
			for _, item := range items {
				results = append(results, downloadResultJSON{
					Name:    displayName(item.entry),
					TitleID: fmt.Sprintf("%016x", item.entry.TitleID),
					Version: displayVersion(item.version),
					Path:    item.dir,
					Status:  "planned",
				})
			}
			return writeJSON(application.stdout, results)
		}
		fmt.Fprintln(application.stdout, "Would download:")
		for _, item := range items {
			fmt.Fprintf(application.stdout, "  %s (%016x), version %s\n    -> %s\n",
				displayName(item.entry), item.entry.TitleID, displayVersion(item.version), item.dir)
		}
		return nil
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	started := time.Now()
	downloaded := 0
	failed := 0
	cancelled := false
	results := make([]downloadResultJSON, 0, len(items))

	for index, item := range items {
		if !application.quiet {
			fmt.Fprintf(application.stderr, "[%d/%d] %s (%016x), version %s\n  -> %s\n",
				index+1, len(items), displayName(item.entry), item.entry.TitleID, displayVersion(item.version), item.dir)
		}

		progress := newTerminalProgress(application.stderr, isTerminal(os.Stderr), application.quiet)
		application.interrupts.set(progress)

		itemStarted := time.Now()
		downloadErr := wiiudownloader.DownloadTitle(
			fmt.Sprintf("%016x", item.entry.TitleID),
			item.dir,
			item.version,
			*decrypt,
			progress,
			*deleteEncrypted,
			application.client,
			*decryptOutput,
		)
		progress.Finish()
		application.interrupts.set(nil)

		if progress.Cancelled() {
			cancelled = true
			results = append(results, downloadResultJSON{
				Name:    displayName(item.entry),
				TitleID: fmt.Sprintf("%016x", item.entry.TitleID),
				Version: displayVersion(item.version),
				Path:    item.dir,
				Status:  "cancelled",
				Error:   "interrupted",
			})
			if !application.quiet {
				fmt.Fprintln(application.stderr, "Cancelled.")
			}
			break
		}
		if downloadErr != nil {
			failed++
			results = append(results, downloadResultJSON{
				Name:            displayName(item.entry),
				TitleID:         fmt.Sprintf("%016x", item.entry.TitleID),
				Version:         displayVersion(item.version),
				Path:            item.dir,
				Status:          "failed",
				Error:           downloadErr.Error(),
				DurationSeconds: time.Since(itemStarted).Seconds(),
			})
			fmt.Fprintf(application.stderr, "Failed: %s: %v\n", displayName(item.entry), downloadErr)
			if *failFast {
				break
			}
			continue
		}

		downloaded++
		results = append(results, downloadResultJSON{
			Name:            displayName(item.entry),
			TitleID:         fmt.Sprintf("%016x", item.entry.TitleID),
			Version:         displayVersion(item.version),
			Path:            item.dir,
			Status:          "downloaded",
			DurationSeconds: time.Since(itemStarted).Seconds(),
		})
		if !application.quiet {
			fmt.Fprintf(application.stderr, "Done in %s.\n", formatDuration(time.Since(itemStarted)))
		}
	}

	if application.json {
		if err := writeJSON(application.stdout, results); err != nil {
			return err
		}
	} else if !application.quiet || failed > 0 {
		elapsed := formatDuration(time.Since(started))
		fmt.Fprintf(application.stderr, "Downloaded %d of %d title(s) in %s.\n", downloaded, len(items), elapsed)
		fmt.Fprintf(application.stderr, "Destination: %s\n", outputDir)
	}
	if cancelled {
		return interruptedError{}
	}
	if failed > 0 {
		return silentExit{code: 1}
	}
	return nil
}

func resolveDownloadItems(application *app, db *titleDB, args []string, outputDir, version string, regionMask, category uint8, pickFirst bool) ([]downloadItem, error) {
	items := make([]downloadItem, 0, len(args))
	seen := make(map[string]struct{})
	for _, argument := range args {
		matches, err := resolveTitleArgument(application, db, argument, regionMask, category, true, pickFirst)
		if err != nil {
			return nil, err
		}
		for _, entry := range matches {
			titleVersion, err := parseTitleVersion(version, entry)
			if err != nil {
				return nil, err
			}
			key := fmt.Sprintf("%016x/%d", entry.TitleID, titleVersion)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, downloadItem{
				entry:   entry,
				version: titleVersion,
				dir:     filepath.Join(outputDir, titleDirName(entry, titleVersion)),
			})
		}
	}
	return items, nil
}

func displayName(entry wiiudownloader.TitleEntry) string {
	if entry.Name != "" {
		return entry.Name
	}
	return fmt.Sprintf("%016x", entry.TitleID)
}

func displayVersion(version int) string {
	if version < 0 {
		return "latest"
	}
	return fmt.Sprintf("%d", version)
}
