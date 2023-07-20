package main

import (
	"fmt"
	"os"
	"path/filepath"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

func main() {
	// Check if user is running macOS
	if os.Getenv("GOOS") != "darwin" {
		execPath, err := os.Executable()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		bundlePath := filepath.Join(filepath.Dir(filepath.Dir(execPath)))
		filePath := filepath.Join(bundlePath, "MacOS/lib/share/glib-2.0/schemas")
		if _, err := os.Stat("/path/to/whatever"); os.IsExist(err) {
			os.Setenv("GSETTINGS_SCHEMA_DIR", filePath)
		}
	}
	win := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME))

	win.ShowAll()
	Main()
}
