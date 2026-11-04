package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type ContentInfoJSON struct {
	ID     string `json:"id"`
	Index  uint16 `json:"index"`
	Type   uint16 `json:"type"`
	Size   uint64 `json:"size"`
	Hashed bool   `json:"hashed"`
}

type TitleInfoJSON struct {
	TitleID         string            `json:"title_id"`
	Name            string            `json:"name"`
	Region          string            `json:"region"`
	Category        string            `json:"category"`
	KeyType         uint8             `json:"key_type"`
	Version         int               `json:"version"`
	TMDVersion      *uint16           `json:"tmd_version,omitempty"`
	TotalSize       *uint64           `json:"total_size,omitempty"`
	FormattedSize   string            `json:"formatted_size,omitempty"`
	RelatedUpdate   *RelatedTitleJSON `json:"related_update,omitempty"`
	RelatedDLC      *RelatedTitleJSON `json:"related_dlc,omitempty"`
	RelatedGame     *RelatedTitleJSON `json:"related_game,omitempty"`
	Contents        []ContentInfoJSON `json:"contents,omitempty"`
}

type RelatedTitleJSON struct {
	TitleID  string `json:"title_id"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	Category string `json:"category"`
}

// RunInfo handles the "info" subcommand.
func RunInfo(args []string) int {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	var (
		version int
		remote  bool
		jsonOut bool
	)
	fs.IntVar(&version, "v", wiiudownloader.VersionLatest, "Version to inspect (-1 for latest)")
	fs.IntVar(&version, "version", wiiudownloader.VersionLatest, "Version to inspect (-1 for latest)")
	fs.BoolVar(&remote, "remote", true, "Fetch remote TMD metadata from CDN (size, contents)")
	fs.BoolVar(&jsonOut, "json", false, "Output results in JSON format")

	boolFlags := map[string]bool{"remote": true, "json": true}
	if err := fs.Parse(ReorderArgs(args, boolFlags)); err != nil {
		return 1
	}

	target := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(target) == "" {
		fmt.Fprintln(os.Stderr, "Error: title ID or name required")
		fmt.Fprintln(os.Stderr, "Usage: wiiudownloader info [options] <title_id | name>")
		return 1
	}

	if err := EnsureTitleDatabase(jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading database: %v\n", err)
		return 1
	}

	var entry wiiudownloader.TitleEntry
	if tid, err := wiiudownloader.ParseTitleID(target); err == nil && tid > 0 {
		entry = wiiudownloader.GetTitleEntryFromTid(tid)
		if entry.TitleID == 0 {
			entry.TitleID = tid
			entry.Name = fmt.Sprintf("%016x", tid)
		}
	} else {
		// Search by name
		entries := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL)
		targetLower := strings.ToLower(target)
		for _, e := range entries {
			if strings.EqualFold(e.Name, target) {
				entry = e
				break
			}
			if strings.Contains(strings.ToLower(e.Name), targetLower) && entry.TitleID == 0 {
				entry = e
			}
		}
		if entry.TitleID == 0 {
			fmt.Fprintf(os.Stderr, "Error: title not found for query %q\n", target)
			return 1
		}
	}

	info := TitleInfoJSON{
		TitleID:  fmt.Sprintf("%016x", entry.TitleID),
		Name:     entry.Name,
		Region:   wiiudownloader.GetFormattedRegion(entry.Region),
		Category: wiiudownloader.GetFormattedKind(entry.TitleID),
		KeyType:  entry.Key,
		Version:  version,
	}

	// Related titles
	high := wiiudownloader.GetTitleIDHigh(entry.TitleID)
	if high == wiiudownloader.TID_HIGH_GAME {
		if upd, ok := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_UPDATE, nil); ok {
			info.RelatedUpdate = &RelatedTitleJSON{
				TitleID:  fmt.Sprintf("%016x", upd.TitleID),
				Name:     upd.Name,
				Region:   wiiudownloader.GetFormattedRegion(upd.Region),
				Category: "Update",
			}
		}
		if dlc, ok := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_DLC, nil); ok {
			info.RelatedDLC = &RelatedTitleJSON{
				TitleID:  fmt.Sprintf("%016x", dlc.TitleID),
				Name:     dlc.Name,
				Region:   wiiudownloader.GetFormattedRegion(dlc.Region),
				Category: "DLC",
			}
		}
	} else if high == wiiudownloader.TID_HIGH_UPDATE || high == wiiudownloader.TID_HIGH_DLC {
		if game, ok := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_GAME, nil); ok {
			info.RelatedGame = &RelatedTitleJSON{
				TitleID:  fmt.Sprintf("%016x", game.TitleID),
				Name:     game.Name,
				Region:   wiiudownloader.GetFormattedRegion(game.Region),
				Category: "Game",
			}
		}
	}

	// Remote CDN query
	if remote {
		client := wiiudownloader.BuildHTTPClient()
		tmd, err := wiiudownloader.FetchTitleTMD(entry.TitleID, version, client)
		if err == nil {
			info.TMDVersion = &tmd.TitleVersion
			totalSize := tmd.CalculateTotalSize()
			info.TotalSize = &totalSize
			info.FormattedSize = wiiudownloader.FormatBytes(totalSize)

			for _, c := range tmd.Contents {
				idx := uint16(0)
				if len(c.Index) >= 2 {
					idx = uint16(c.Index[0])<<8 | uint16(c.Index[1])
				}
				info.Contents = append(info.Contents, ContentInfoJSON{
					ID:     fmt.Sprintf("%08X", c.ID),
					Index:  idx,
					Type:   c.Type,
					Size:   c.Size,
					Hashed: (c.Type & wiiudownloader.CONTENT_TYPE_HASHED) != 0,
				})
			}
		}
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(info)
		return 0
	}

	fmt.Println("Title Information:")
	fmt.Printf("  Title ID:        %s (0x%s)\n", info.TitleID, info.TitleID)
	fmt.Printf("  Name:            %s\n", info.Name)
	fmt.Printf("  Region:          %s\n", info.Region)
	fmt.Printf("  Category:        %s\n", info.Category)
	fmt.Printf("  Key Type:        %d\n", info.KeyType)

	if info.TMDVersion != nil {
		fmt.Printf("  TMD Version:     v%d\n", *info.TMDVersion)
	}
	if info.TotalSize != nil {
		fmt.Printf("  Total Size:      %s (%d bytes)\n", info.FormattedSize, *info.TotalSize)
		fmt.Printf("  Contents:        %d files\n", len(info.Contents))
	}

	if info.RelatedUpdate != nil {
		fmt.Printf("  Related Update:  %s - %s (%s)\n", info.RelatedUpdate.TitleID, info.RelatedUpdate.Name, info.RelatedUpdate.Region)
	}
	if info.RelatedDLC != nil {
		fmt.Printf("  Related DLC:     %s - %s (%s)\n", info.RelatedDLC.TitleID, info.RelatedDLC.Name, info.RelatedDLC.Region)
	}
	if info.RelatedGame != nil {
		fmt.Printf("  Base Game:       %s - %s (%s)\n", info.RelatedGame.TitleID, info.RelatedGame.Name, info.RelatedGame.Region)
	}

	if len(info.Contents) > 0 {
		fmt.Println("\nContents List:")
		fmt.Printf("  %-10s %-8s %-12s %s\n", "CONTENT ID", "INDEX", "SIZE", "HASHED")
		fmt.Println("  " + strings.Repeat("-", 45))
		for _, c := range info.Contents {
			fmt.Printf("  %-10s %-8d %-12s %v\n", c.ID, c.Index, wiiudownloader.FormatBytes(c.Size), c.Hashed)
		}
	}

	return 0
}

func parseUint64(s string) uint64 {
	val, _ := strconv.ParseUint(s, 0, 64)
	return val
}
