package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

func cmdDB(application *app, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(application.stdout, dbUsage)
		return nil
	}

	switch args[0] {
	case "update":
		return cmdDBUpdate(application, args[1:])
	case "info":
		return cmdDBInfo(application, args[1:])
	case "clear":
		return cmdDBClear(application, args[1:])
	case "path":
		path, err := titleDBCachePath()
		if err != nil {
			return err
		}
		fmt.Fprintln(application.stdout, path)
		return nil
	default:
		fmt.Fprintf(application.stderr, "%s: unknown db command %q\n\n%s\n", cliName, args[0], dbUsage)
		return silentExit{code: 2}
	}
}

func cmdDBUpdate(application *app, args []string) error {
	fs := newFlagSet(application, "db update", "Download and cache the latest title database.", `Usage:
  wiiudl db update [--url URL]

Flags:`)
	url := fs.String("url", titleDBURL(), "Database URL (defaults to WIIUD_DB_URL or the public endpoint)")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) > 0 {
		fs.Usage()
		return silentExit{code: 2}
	}

	cachePath, err := titleDBCachePath()
	if err != nil {
		return fmt.Errorf("determining cache path: %w", err)
	}

	if !application.quiet {
		fmt.Fprintf(application.stderr, "Downloading title database from %s...\n", *url)
	}
	entries, source, err := fetchTitleDatabase(application.client, *url)
	if err != nil {
		return fmt.Errorf("downloading title database: %w", err)
	}
	now := time.Now().UTC()
	cache := titleDBCache{
		FetchedAt: now,
		Source:    source,
		Entries:   titleEntriesToJSON(entries),
	}
	if err := writeTitleDBCache(cachePath, cache); err != nil {
		return fmt.Errorf("writing cache: %w", err)
	}

	if application.json {
		return writeJSON(application.stdout, struct {
			Entries   int       `json:"entries"`
			Source    string    `json:"source"`
			Path      string    `json:"path"`
			FetchedAt time.Time `json:"fetchedAt"`
		}{len(entries), source, cachePath, now})
	}

	fmt.Fprintf(application.stdout, "Cached %d titles from %s\n", len(entries), source)
	fmt.Fprintf(application.stdout, "Cache: %s\n", cachePath)
	return nil
}

func cmdDBInfo(application *app, args []string) error {
	fs := newFlagSet(application, "db info", "Show which title database is in use.", `Usage:
  wiiudl db info

Flags:`)
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) > 0 {
		fs.Usage()
		return silentExit{code: 2}
	}

	if application.dbPath != "" {
		data, err := os.ReadFile(application.dbPath)
		if err != nil {
			return err
		}
		entries, err := parseTitleDatabase(data)
		if err != nil {
			return err
		}
		fmt.Fprintf(application.stdout, "Source: %s (--db)\nTitles: %d\n", application.dbPath, len(entries))
		return nil
	}

	cachePath, cacheErr := titleDBCachePath()
	if cacheErr == nil {
		cache, err := readTitleDBCache(cachePath)
		if err == nil {
			fmt.Fprintf(application.stdout, "Source: cache\nCache:  %s\nTitles: %d\n", cachePath, len(cache.Entries))
			if !cache.FetchedAt.IsZero() {
				fmt.Fprintf(application.stdout, "Age:    %s (fetched %s)\n", formatAge(time.Since(cache.FetchedAt)), cache.FetchedAt.Local().Format(time.RFC1123))
			}
			if cache.Source != "" {
				fmt.Fprintf(application.stdout, "Origin: %s\n", cache.Source)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(application.stderr, "warning: cache at %s is unreadable: %v\n", cachePath, err)
		}
	}

	if len(wiiudownloader.TitleDatabase) > 0 {
		db := newTitleDB(wiiudownloader.TitleDatabase, "built-in")
		fmt.Fprintf(application.stdout, "Source: built into this binary\nTitles: %d\n", len(db.entries))
		if cacheErr == nil {
			fmt.Fprintf(application.stdout, "Cache:  %s (not populated)\n", cachePath)
		}
		return nil
	}

	fmt.Fprintln(application.stdout, "Source: none")
	if cacheErr == nil {
		fmt.Fprintf(application.stdout, "Cache:  %s (missing)\n", cachePath)
	}
	fmt.Fprintf(application.stdout, "Run `%s db update` to download it.\n", cliName)
	return nil
}

func cmdDBClear(application *app, args []string) error {
	fs := newFlagSet(application, "db clear", "Delete the cached title database.", `Usage:
  wiiudl db clear

Flags:`)
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) > 0 {
		fs.Usage()
		return silentExit{code: 2}
	}
	if application.dbPath != "" {
		return usageError{fmt.Errorf("refusing to delete an explicit --db file")}
	}

	cachePath, err := titleDBCachePath()
	if err != nil {
		return err
	}
	if err := os.Remove(cachePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(application.stdout, "No cached database at %s\n", cachePath)
			return nil
		}
		return err
	}
	fmt.Fprintf(application.stdout, "Removed %s\n", cachePath)
	return nil
}

func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return pluralAge(int(d.Minutes()), "minute")
	}
	if d < 24*time.Hour {
		return pluralAge(int(d.Hours()), "hour")
	}
	return pluralAge(int(d.Hours()/24), "day")
}

func pluralAge(value int, unit string) string {
	if value == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", value, unit)
}
