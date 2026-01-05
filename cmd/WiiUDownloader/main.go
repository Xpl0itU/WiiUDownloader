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
	runtime.LockOSThread() // macOS Crash Fix

	// Initialize Go's threading system to avoid races
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Check if user is running macOS
	// Check if user is running macOS
	if runtime.GOOS == "darwin" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatal(err.Error())
		}

		bundlePath := filepath.Dir(filepath.Dir(execPath))

		// Set GSettings Schema Dir
		glibPath := filepath.Join(bundlePath, "Resources", "share", "glib-2.0", "schemas")
		// Older logic path check just in case or if structure changed
		if _, err := os.Stat(glibPath); os.IsNotExist(err) {
			// Try old path just in case
			glibPath = filepath.Join(bundlePath, "Resources", "lib", "share", "glib-schemas")
		}

		if _, err := os.Stat(glibPath); err == nil {
			os.Setenv("GSETTINGS_SCHEMA_DIR", glibPath)
		} else {
			log.Println("Warning: glib-schemas not found in bundle.")
		}

		// Set GdkPixbuf Module Dir
		// We bundle them in Contents/MacOS/lib/gdk-pixbuf-2.0/2.10.0/loaders (usually)
		// But in create_bundle.py we copied 'lib/gdk-pixbuf-2.0' to 'Contents/MacOS/lib/gdk-pixbuf-2.0'
		// We need to find the directory containing 'loaders'.
		gdkLibPath := filepath.Join(bundlePath, "MacOS", "lib", "gdk-pixbuf-2.0")
		filepath.Walk(gdkLibPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() && info.Name() == "loaders" {
				os.Setenv("GDK_PIXBUF_MODULE_DIR", path)
				return filepath.SkipDir // Found it
			}
			return nil
		})

		// Set XDG_DATA_DIRS for icons
		// Bundle: Contents/Resources/share
		sharePath := filepath.Join(bundlePath, "Resources", "share")
		if _, err := os.Stat(sharePath); err == nil {
			os.Setenv("XDG_DATA_DIRS", sharePath)
			// Force GTK to see the theme if needed?
			// GTK3 usually checks XDG_DATA_DIRS for icons/themes.
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
				win.ShowAll()
				app.AddWindow(win.window)
				app.GetActiveWindow().Show()
			})

			assistant.assistantWindow.ShowAll()
			app.AddWindow(assistant.assistantWindow)
			app.GetActiveWindow().Show()
			win.window.Hide()
		} else {
			win.SetApplicationForGTKWindow(app)
			win.ShowAll()
			app.AddWindow(win.window)
			app.GetActiveWindow().Show()
		}
	})

	app.Run(os.Args)
}
