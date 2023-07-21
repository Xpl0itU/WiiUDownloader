package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

func main() {
	logger, err := wiiudownloader.NewLogger("log.txt")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	// Check if user is running macOS
	if runtime.GOOS == "darwin" {
		execPath, err := os.Executable()
		if err != nil {
			logger.Error(err.Error())
			return
		}

		bundlePath := filepath.Join(filepath.Dir(filepath.Dir(execPath)))
		filePath := filepath.Join(bundlePath, "Resources/lib/share/glib-schemas")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			logger.Warning("glib-schemas not found")
		} else {
			os.Setenv("GSETTINGS_SCHEMA_DIR", filePath)
		}
	}
	win := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME), logger)

	win.ShowAll()
	Main()
}
