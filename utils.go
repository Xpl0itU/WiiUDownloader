package wiiudownloader

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
)

func min[T cmp.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func isThisDecryptedFile(path string) bool {
	return strings.Contains(path, "code") || strings.Contains(path, "content") || strings.Contains(path, "meta")
}

func doDeleteEncryptedContents(path string) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && !isThisDecryptedFile(filePath) {
			if err := os.Remove(filePath); err != nil {
				return err
			}
		}
		return nil
	})
}
