package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"

	"github.com/valyala/fasthttp"
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

	client := &fasthttp.Client{
		MaxConnsPerHost:     1024,
		MaxIdleConnDuration: 30 * time.Second,
		TLSConfig:           nil,
		ReadBufferSize:      wiiudownloader.BUFFER_SIZE,
		WriteBufferSize:     wiiudownloader.BUFFER_SIZE,
		MaxConnWaitTimeout:  30 * time.Second,
		StreamResponseBody:  true,
		ConnPoolStrategy:    fasthttp.LIFO,
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
		Name:                "WiiUDownloader",
		DialDualStack:       true,
		RetryIf: func(request *fasthttp.Request) bool {
			return true
		},
	}

	app.Connect("activate", func() {
		win := NewMainWindow(app, wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME), logger, client)
		win.ShowAll()
		app.AddWindow(win.window)
		app.GetActiveWindow().Show()
		gtk.Main()
	})
	app.Run(nil)
	app.Quit()
}
