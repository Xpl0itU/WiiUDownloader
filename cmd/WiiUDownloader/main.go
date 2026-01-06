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

	// Check if user is running macOS and inside a bundle
	if runtime.GOOS == "darwin" {
		execPath, err := os.Executable()
		if err != nil {
			log.Fatal(err.Error())
		}

		// Check if we are inside a .app bundle
		// path/to/WiiUDownloader.app/Contents/MacOS/WiiUDownloader
		if filepath.Base(filepath.Dir(execPath)) == "MacOS" {
			bundlePath := filepath.Dir(filepath.Dir(execPath))

			// 1. Isolation: Clear variables to prevent Homebrew leaks
			os.Unsetenv("DYLD_LIBRARY_PATH")
			os.Unsetenv("DYLD_FALLBACK_LIBRARY_PATH")
			os.Unsetenv("DYLD_INSERT_LIBRARIES")
			os.Unsetenv("PKG_CONFIG_PATH")

			// 2. Set GSettings Schema Dir
			glibPath := filepath.Join(bundlePath, "Resources", "share", "glib-2.0", "schemas")
			if _, err := os.Stat(glibPath); err == nil {
				os.Setenv("GSETTINGS_SCHEMA_DIR", glibPath)
			}

			// 3. Set GdkPixbuf Module Dir (Crucial for icons)
			// Our new script puts them in lib/loaders
			os.Setenv("GDK_PIXBUF_MODULE_DIR", filepath.Join(bundlePath, "MacOS", "lib", "loaders"))
			// Also point to the cache located in Resources
			cachePath := filepath.Join(bundlePath, "Resources", "loaders.cache")
			if _, err := os.Stat(cachePath); err == nil {
				os.Setenv("GDK_PIXBUF_MODULE_FILE", cachePath)
			}

			// 4. Set GIO Module Dir
			gioModPath := filepath.Join(bundlePath, "MacOS", "lib", "gio-modules")
			os.Setenv("GIO_MODULE_DIR", gioModPath)
			os.Setenv("GIO_EXTRA_MODULES", gioModPath)

			// 5. Set XDG_DATA_DIRS for icons and themes
			sharePath := filepath.Join(bundlePath, "Resources", "share")
			if _, err := os.Stat(sharePath); err == nil {
				os.Setenv("XDG_DATA_DIRS", sharePath)
				if isDarkMode() {
					os.Setenv("GTK_THEME", "Adwaita:dark")
				} else {
					os.Setenv("GTK_THEME", "Adwaita")
				}
			}
		}
	}

	gtk.Init(nil)

	// Robust Dark Mode application
	settings, _ := gtk.SettingsGetDefault()
	if isDarkMode() {
		settings.SetProperty("gtk-application-prefer-dark-theme", true)
		settings.SetProperty("gtk-theme-name", "Adwaita")
	} else {
		settings.SetProperty("gtk-application-prefer-dark-theme", false)
		settings.SetProperty("gtk-theme-name", "Adwaita")
	}

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
