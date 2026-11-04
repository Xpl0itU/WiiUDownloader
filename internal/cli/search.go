package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type TitleResultJSON struct {
	TitleID  string `json:"title_id"`
	TIDHex   string `json:"tid_hex"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	Category string `json:"category"`
	Key      uint8  `json:"key"`
}

func parseRegionFilter(regionStr string) uint8 {
	if regionStr == "" || strings.EqualFold(regionStr, "all") {
		return wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN
	}

	var mask uint8
	parts := strings.Split(regionStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		switch p {
		case "usa", "us", "na":
			mask |= wiiudownloader.MCP_REGION_USA
		case "eur", "europe", "eu", "pal":
			mask |= wiiudownloader.MCP_REGION_EUROPE
		case "jpn", "japan", "jp":
			mask |= wiiudownloader.MCP_REGION_JAPAN
		case "all":
			mask |= wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN
		}
	}
	if mask == 0 {
		return wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN
	}
	return mask
}

func parseCategoryFilter(catStr string) uint8 {
	switch strings.ToLower(strings.TrimSpace(catStr)) {
	case "game", "games":
		return wiiudownloader.TITLE_CATEGORY_GAME
	case "update", "updates":
		return wiiudownloader.TITLE_CATEGORY_UPDATE
	case "dlc", "dlcs":
		return wiiudownloader.TITLE_CATEGORY_DLC
	case "demo", "demos":
		return wiiudownloader.TITLE_CATEGORY_DEMO
	case "disc":
		return wiiudownloader.TITLE_CATEGORY_DISC
	default:
		return wiiudownloader.TITLE_CATEGORY_ALL
	}
}

// RunSearch handles the "search" subcommand.
func RunSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	var (
		regionStr string
		catStr    string
		limit     int
		jsonOut   bool
	)
	fs.StringVar(&regionStr, "r", "", "Filter by region (usa, eur, jpn, all)")
	fs.StringVar(&regionStr, "region", "", "Filter by region (usa, eur, jpn, all)")
	fs.StringVar(&catStr, "t", "all", "Filter by category (game, update, dlc, demo, all)")
	fs.StringVar(&catStr, "type", "all", "Filter by category (game, update, dlc, demo, all)")
	fs.StringVar(&catStr, "category", "all", "Filter by category (game, update, dlc, demo, all)")
	fs.IntVar(&limit, "l", 50, "Limit maximum number of results (0 for all)")
	fs.IntVar(&limit, "limit", 50, "Limit maximum number of results (0 for all)")
	fs.BoolVar(&jsonOut, "json", false, "Output results in JSON format")

	boolFlags := map[string]bool{"json": true}
	if err := fs.Parse(ReorderArgs(args, boolFlags)); err != nil {
		return 1
	}

	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "Error: search query required")
		fmt.Fprintln(os.Stderr, "Usage: wiiudownloader search [options] <query>")
		return 1
	}

	if err := EnsureTitleDatabase(jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	regionMask := parseRegionFilter(regionStr)
	category := parseCategoryFilter(catStr)
	entries := wiiudownloader.GetTitleEntries(category)

	queryLower := strings.ToLower(query)
	var matched []wiiudownloader.TitleEntry

	for _, entry := range entries {
		if entry.Region&regionMask == 0 {
			continue
		}

		tidHex := fmt.Sprintf("%016x", entry.TitleID)
		if strings.Contains(tidHex, queryLower) || strings.Contains(strings.ToLower(entry.Name), queryLower) {
			matched = append(matched, entry)
			if limit > 0 && len(matched) >= limit {
				break
			}
		}
	}

	return outputSearchResults(matched, jsonOut)
}

// RunList handles the "list" subcommand.
func RunList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var (
		regionStr string
		catStr    string
		limit     int
		jsonOut   bool
	)
	fs.StringVar(&regionStr, "r", "", "Filter by region (usa, eur, jpn, all)")
	fs.StringVar(&regionStr, "region", "", "Filter by region (usa, eur, jpn, all)")
	fs.StringVar(&catStr, "t", "game", "Filter by category (game, update, dlc, demo, all)")
	fs.StringVar(&catStr, "type", "game", "Filter by category (game, update, dlc, demo, all)")
	fs.StringVar(&catStr, "category", "game", "Filter by category (game, update, dlc, demo, all)")
	fs.IntVar(&limit, "l", 100, "Limit maximum number of results (0 for all)")
	fs.IntVar(&limit, "limit", 100, "Limit maximum number of results (0 for all)")
	fs.BoolVar(&jsonOut, "json", false, "Output results in JSON format")

	boolFlags := map[string]bool{"json": true}
	if err := fs.Parse(ReorderArgs(args, boolFlags)); err != nil {
		return 1
	}

	if err := EnsureTitleDatabase(jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	regionMask := parseRegionFilter(regionStr)
	category := parseCategoryFilter(catStr)
	entries := wiiudownloader.GetTitleEntries(category)

	var matched []wiiudownloader.TitleEntry
	for _, entry := range entries {
		if entry.Region&regionMask == 0 {
			continue
		}
		matched = append(matched, entry)
		if limit > 0 && len(matched) >= limit {
			break
		}
	}

	return outputSearchResults(matched, jsonOut)
}

func outputSearchResults(matched []wiiudownloader.TitleEntry, jsonOut bool) int {
	if jsonOut {
		results := make([]TitleResultJSON, len(matched))
		for i, e := range matched {
			tidStr := fmt.Sprintf("%016x", e.TitleID)
			results[i] = TitleResultJSON{
				TitleID:  tidStr,
				TIDHex:   "0x" + tidStr,
				Name:     e.Name,
				Region:   wiiudownloader.GetFormattedRegion(e.Region),
				Category: wiiudownloader.GetFormattedKind(e.TitleID),
				Key:      e.Key,
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return 0
	}

	if len(matched) == 0 {
		fmt.Println("No titles found matching the criteria.")
		return 0
	}

	fmt.Printf("%-18s %-10s %-12s %s\n", "TITLE ID", "REGION", "CATEGORY", "NAME")
	fmt.Println(strings.Repeat("-", 80))
	for _, e := range matched {
		tidStr := fmt.Sprintf("%016x", e.TitleID)
		regionStr := wiiudownloader.GetFormattedRegion(e.Region)
		catStr := wiiudownloader.GetFormattedKind(e.TitleID)
		fmt.Printf("%-18s %-10s %-12s %s\n", tidStr, regionStr, catStr, e.Name)
	}
	fmt.Printf("\nFound %d title(s).\n", len(matched))
	return 0
}
