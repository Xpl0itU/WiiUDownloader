package wiiudownloader

import (
	"os"
	"path/filepath"
	"strings"
)

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func doDeleteEncryptedContents(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".app") ||
			strings.HasSuffix(name, ".h3") ||
			name == "title.tmd" ||
			name == "title.tik" ||
			name == "title.cert" {
			if err := os.Remove(filepath.Join(path, name)); err != nil {
				return err
			}
		}
	}
	return nil
}
