package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type persistedQueueItem struct {
	TID     string `json:"tid"`
	Version int    `json:"version"`
}

func getQueueFilePath() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userConfigDir, "WiiUDownloader", "queue.json"), nil
}

// GetPersistedQueue reads titles stored in the GUI queue.
func GetPersistedQueue() ([]wiiudownloader.TitleEntry, error) {
	path, err := getQueueFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []persistedQueueItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("invalid queue file %s: %w", path, err)
	}

	titles := make([]wiiudownloader.TitleEntry, 0, len(items))
	for _, it := range items {
		tid, err := strconv.ParseUint(it.TID, 16, 64)
		if err != nil {
			continue
		}
		entry := wiiudownloader.GetTitleEntryFromTid(tid)
		if entry.TitleID == 0 {
			entry = wiiudownloader.TitleEntry{
				Name:     it.TID,
				TitleID:  tid,
				Category: wiiudownloader.TITLE_CATEGORY_GAME,
				Key:      uint8(wiiudownloader.TITLE_KEY_mypass),
			}
		}
		entry.Version = it.Version
		titles = append(titles, entry)
	}
	return titles, nil
}

// ClearPersistedQueue empties the GUI queue file.
func ClearPersistedQueue() error {
	path, err := getQueueFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte("[]"), 0o644)
}

// RunQueue handles the "queue" subcommand.
func RunQueue(args []string) int {
	if len(args) == 0 || args[0] == "list" {
		if err := EnsureTitleDatabase(false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		titles, err := GetPersistedQueue()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading queue: %v\n", err)
			return 1
		}
		if len(titles) == 0 {
			fmt.Println("Queue is empty.")
			return 0
		}
		fmt.Printf("%-18s %-10s %-12s %-8s %s\n", "TITLE ID", "REGION", "CATEGORY", "VERSION", "NAME")
		fmt.Println(strings.Repeat("-", 80))
		for _, t := range titles {
			tidStr := fmt.Sprintf("%016x", t.TitleID)
			vStr := "latest"
			if t.Version >= 0 {
				vStr = fmt.Sprintf("v%d", t.Version)
			}
			fmt.Printf("%-18s %-10s %-12s %-8s %s\n",
				tidStr,
				wiiudownloader.GetFormattedRegion(t.Region),
				wiiudownloader.GetFormattedKind(t.TitleID),
				vStr,
				t.Name,
			)
		}
		fmt.Printf("\nTotal: %d item(s) in queue.\n", len(titles))
		return 0
	}

	switch args[0] {
	case "clear":
		if err := ClearPersistedQueue(); err != nil {
			fmt.Fprintf(os.Stderr, "Error clearing queue: %v\n", err)
			return 1
		}
		fmt.Println("Queue cleared successfully.")
		return 0
	case "download":
		// Delegate to RunDownload with --from-queue flag
		downloadArgs := append([]string{"--from-queue"}, args[1:]...)
		return RunDownload(downloadArgs)
	case "help", "-h", "--help":
		fmt.Println("Usage: wiiudownloader queue [command]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list        List items currently queued in WiiUDownloader GUI (default)")
		fmt.Println("  download    Download all items currently in the queue")
		fmt.Println("  clear       Empty the queue file")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown queue command: %s\n", args[0])
		return 1
	}
}
