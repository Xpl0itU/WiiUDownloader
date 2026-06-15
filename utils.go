package wiiudownloader

import (
	"os"
	"path/filepath"
	"strings"
)

var removableEncryptedExtensions = []string{".app", ".h3"}

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
		if hasRemovableEncryptedExtension(name) ||
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

func hasRemovableEncryptedExtension(name string) bool {
	for _, ext := range removableEncryptedExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
