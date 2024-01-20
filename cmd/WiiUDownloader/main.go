package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
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

		bundlePath := filepath.Dir(filepath.Dir(execPath))
		filePath := filepath.Join(bundlePath, "Resources", "lib", "share", "glib-schemas")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			logger.Warning("glib-schemas not found")
		} else {
			os.Setenv("GSETTINGS_SCHEMA_DIR", filePath)
		}
	}
	gtk.Init(nil)

	app, err := gtk.ApplicationNew("io.github.xpl0itu.wiiudownloader", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		logger.Fatal(err.Error())
	}

	tmpDir, err := os.MkdirTemp("", "wiiudownloader")
	if err != nil {
		logger.Fatal(err.Error())
	}
	defer os.RemoveAll(tmpDir)
	ariaSessionPath := filepath.Join(tmpDir, "wiiudownloader.session")

	app.Connect("activate", func() {
		win := NewMainWindow(app, wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME), logger, ariaSessionPath)
		win.ShowAll()
		app.AddWindow(win.window)
		app.GetActiveWindow().Show()
		gtk.Main()
	})
	app.Run(nil)
	app.Quit()
}
