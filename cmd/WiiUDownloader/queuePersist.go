package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

const queueFilename = "queue.json"

// persistedQueueEntry stores the minimal state needed to rebuild the queue.
// The Title ID is a 16-char hex string for portability and readability.
type persistedQueueEntry struct {
	TID     string `json:"tid"`
	Version int    `json:"version"`
}

func queueFilePath() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := configDirPath(userConfigDir)
	if err := os.MkdirAll(dir, CONFIG_DIR_PERM); err != nil {
		return "", err
	}
	return filepath.Join(dir, queueFilename), nil
}

// saveQueueFile writes the queue atomically (tmp + rename) so a crash cannot
// leave a half-written file behind.
func saveQueueFile(path string, titles []wiiudownloader.TitleEntry) error {
	entries := make([]persistedQueueEntry, 0, len(titles))
	for _, t := range titles {
		entries = append(entries, persistedQueueEntry{
			TID:     fmt.Sprintf("%016x", t.TitleID),
			Version: t.Version,
		})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, CONFIG_FILE_PERM); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadQueueFile(path string) ([]wiiudownloader.TitleEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []persistedQueueEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("invalid queue file %s: %w", path, err)
	}
	titles := make([]wiiudownloader.TitleEntry, 0, len(entries))
	for _, e := range entries {
		tid, err := strconv.ParseUint(e.TID, 16, 64)
		if err != nil {
			log.Printf("queue restore: skipping invalid title ID %q", e.TID)
			continue
		}
		if dbEntry := wiiudownloader.GetTitleEntryFromTid(tid); dbEntry.TitleID == tid {
			dbEntry.Version = e.Version
			titles = append(titles, dbEntry)
			continue
		}
		titles = append(titles, wiiudownloader.TitleEntry{
			Name:    e.TID,
			TitleID: tid,
			Version: e.Version,
			Key:     uint8(wiiudownloader.TITLE_KEY_mypass),
		})
	}
	return titles, nil
}

// persistQueue best-effort saves the queue snapshot; failures are logged.
func persistQueue(titles []wiiudownloader.TitleEntry) {
	path, err := queueFilePath()
	if err != nil {
		log.Printf("queue persist: cannot resolve path: %v", err)
		return
	}
	if err := saveQueueFile(path, titles); err != nil {
		log.Printf("queue persist: %v", err)
	}
}

// restorePersistedQueue re-queues titles saved by a previous session through
// the normal add path (dedup, version remap, size fetch all apply).
func (mw *MainWindow) restorePersistedQueue() {
	path, err := queueFilePath()
	if err != nil {
		return
	}
	titles, err := loadQueueFile(path)
	if err != nil || len(titles) == 0 {
		if err != nil && !os.IsNotExist(err) {
			log.Printf("queue restore: %v", err)
		}
		return
	}
	mw.addTitlesToQueue(titles)
	mw.updateTitlesInQueue()
}
