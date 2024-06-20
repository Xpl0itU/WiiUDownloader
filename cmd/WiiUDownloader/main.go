package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	// Check if user is running macOS
	if runtime.GOOS == "darwin" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatal(err.Error())
		}

		bundlePath := filepath.Dir(filepath.Dir(execPath))
		filePath := filepath.Join(bundlePath, "Resources", "lib", "share", "glib-schemas")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.Println("glib-schemas not found")
		} else {
			os.Setenv("GSETTINGS_SCHEMA_DIR", filePath)
		}
	}

	gtk.Init(nil)

	app, err := gtk.ApplicationNew("io.github.xpl0itu.wiiudownloader", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		log.Fatal("Error creating application.")
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			MaxConnsPerHost:       100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	config, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	win := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME), client, config)
	config.saveConfigCallback = func() {
		win.applyConfig(config)
	}

	app.Connect("activate", func(app *gtk.Application) {
		if !config.DidInitialSetup {
			// Open the initial setup assistant
			assistant, err := NewInitialSetupAssistantWindow(config)
			if err != nil {
				log.Fatal(err)
			}
			assistant.SetPostSetupCallback(func() {
				win.SetApplicationForGTKWindow(app)
				glib.IdleAddPriority(glib.PRIORITY_HIGH, func() {
					win.ShowAll()
					app.AddWindow(win.window)
					app.GetActiveWindow().Show()
				})
			})
			glib.IdleAddPriority(glib.PRIORITY_HIGH, func() {
				assistant.assistantWindow.ShowAll()
				app.AddWindow(assistant.assistantWindow)
				app.GetActiveWindow().Show()
				win.window.Hide()
			})
		} else {
			win.SetApplicationForGTKWindow(app)
			glib.IdleAddPriority(glib.PRIORITY_HIGH, func() {
				win.ShowAll()
				app.AddWindow(win.window)
				app.GetActiveWindow().Show()
			})
		}
	})
	app.ConnectAfter("activate", func(app *gtk.Application) {
		gtk.Main()
	})
	glib.ApplicationGetDefault().Run(os.Args)
}
