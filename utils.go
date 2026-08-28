package wiiudownloader

import (
	"fmt"
	"io"
	"net/http"
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

func FetchTMDSize(titleID uint64, version int, client *http.Client) (uint64, error) {
	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%016x", titleID)
	tmdURL := fmt.Sprintf("%s/tmd", baseURL)
	if version >= 0 {
		tmdURL = fmt.Sprintf("%s/tmd.%d", baseURL, version)
	}

	resp, err := client.Get(tmdURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to fetch TMD: status %d", resp.StatusCode)
	}

	tmdData, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read TMD data: %w", err)
	}

	tmd, err := ParseTMD(tmdData)
	if err != nil {
		return 0, fmt.Errorf("failed to parse TMD: %w", err)
	}

	return tmd.CalculateTotalSize(), nil
}
