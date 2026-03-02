package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	NETWORK_DIAL_TIMEOUT         = 30 * time.Second
	NETWORK_DIAL_KEEP_ALIVE      = 30 * time.Second
	FALLBACK_DNS_DIAL_TIMEOUT    = 10 * time.Second
	HTTP_MAX_IDLE_CONNS          = 100
	HTTP_MAX_IDLE_CONNS_PER_HOST = 100
	HTTP_MAX_CONNS_PER_HOST      = 100
	HTTP_IDLE_CONN_TIMEOUT       = 90 * time.Second
	HTTP_TLS_HANDSHAKE_TIMEOUT   = 10 * time.Second
	HTTP_RESPONSE_HEADER_TIMEOUT = 10 * time.Second
	HTTP_EXPECT_CONTINUE_TIMEOUT = 1 * time.Second
	// Used only as DNS fallback when system resolver fails.
	FALLBACK_DNS_RESOLVER_ENDPOINT = "1.1.1.1:53"
)

func main() {
	runtime.LockOSThread()
	runtime.GOMAXPROCS(runtime.NumCPU())
	configureMacOSEnvironment()
	gtk.Init(nil)

	app, err := gtk.ApplicationNew("io.github.xpl0itu.wiiudownloader", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		showFatalDialogAndLog("Error creating application", err)
		return
	}

	if runtime.GOOS == "darwin" {
		quitAction := glib.SimpleActionNew("quit", nil)
		quitAction.Connect("activate", func() {
			app.Quit()
		})
		app.AddAction(quitAction)
		app.SetAccelsForAction("app.quit", []string{"<Primary>q"})
	}

	client := buildHTTPClient()
	config, err := loadConfig()
	if err != nil {
		log.Printf("error loading config: %v", err)
		errorDialog := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_WARNING, gtk.BUTTONS_OK, "Error loading config: %v\n\nStarting with default settings.", err)
		errorDialog.Run()
		errorDialog.Destroy()
	}
	if config == nil {
		config = getDefaultConfig()
	}

	if settings, err := gtk.SettingsGetDefault(); err != nil {
		log.Printf("error getting gtk settings: %v", err)
	} else if settings != nil {
		settings.SetProperty("gtk-theme-name", "Adwaita")
		settings.SetProperty("gtk-application-prefer-dark-theme", config.DarkMode)
	}

	win := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME), client, config)
	config.saveConfigCallback = func() {
		glib.IdleAdd(func() {
			win.applyConfig(config)
		})
	}

	app.Connect("activate", func(app *gtk.Application) {
		if !config.DidInitialSetup {
			assistant, err := NewInitialSetupAssistantWindow(config)
			if err != nil {
				showFatalDialogAndLog("Error creating setup assistant", err)
				return
			}
			assistant.SetPostSetupCallback(func() {
				showMainWindow(app, win)
			})
			assistant.assistantWindow.ShowAll()
			app.AddWindow(assistant.assistantWindow)
			if win.window != nil {
				win.window.Hide()
			}
			return
		}

		showMainWindow(app, win)
	})

	app.Run(os.Args)
}

func configureMacOSEnvironment() {
	if runtime.GOOS != "darwin" {
		return
	}
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("error getting executable path: %v", err)
		return
	}
	if filepath.Base(filepath.Dir(execPath)) != "MacOS" {
		return
	}

	bundlePath := filepath.Dir(filepath.Dir(execPath))
	os.Unsetenv("DYLD_LIBRARY_PATH")
	os.Unsetenv("DYLD_FALLBACK_LIBRARY_PATH")
	os.Unsetenv("DYLD_INSERT_LIBRARIES")
	os.Unsetenv("PKG_CONFIG_PATH")

	glibPath := filepath.Join(bundlePath, "Resources", "share", "glib-2.0", "schemas")
	if _, err := os.Stat(glibPath); err == nil {
		os.Setenv("GSETTINGS_SCHEMA_DIR", glibPath)
	}

	os.Setenv("GDK_PIXBUF_MODULE_DIR", filepath.Join(bundlePath, "MacOS", "lib", "loaders"))
	cachePath := filepath.Join(bundlePath, "Resources", "loaders.cache")
	if _, err := os.Stat(cachePath); err == nil {
		os.Setenv("GDK_PIXBUF_MODULE_FILE", cachePath)
	}

	gioModPath := filepath.Join(bundlePath, "MacOS", "lib", "gio-modules")
	os.Setenv("GIO_MODULE_DIR", gioModPath)
	os.Setenv("GIO_EXTRA_MODULES", gioModPath)

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

func buildHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: NETWORK_DIAL_TIMEOUT, KeepAlive: NETWORK_DIAL_KEEP_ALIVE}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err == nil {
					return conn, nil
				}
				if !strings.Contains(err.Error(), "no such host") && !strings.Contains(err.Error(), "lookup") {
					return nil, err
				}
				log.Printf("DNS lookup failed for %s, retrying with 1.1.1.1...", addr)
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
						d := net.Dialer{Timeout: FALLBACK_DNS_DIAL_TIMEOUT}
						return d.DialContext(ctx, "udp", FALLBACK_DNS_RESOLVER_ENDPOINT)
					},
				}
				host, port, splitErr := net.SplitHostPort(addr)
				if splitErr != nil {
					return nil, err
				}
				ips, lookupErr := resolver.LookupIPAddr(ctx, host)
				if lookupErr != nil {
					log.Printf("fallback DNS lookup failed: %v", lookupErr)
					return nil, err
				}
				if len(ips) == 0 {
					return nil, err
				}
				targetAddr := net.JoinHostPort(ips[0].String(), port)
				return dialer.DialContext(ctx, network, targetAddr)
			},
			MaxIdleConns:          HTTP_MAX_IDLE_CONNS,
			MaxIdleConnsPerHost:   HTTP_MAX_IDLE_CONNS_PER_HOST,
			MaxConnsPerHost:       HTTP_MAX_CONNS_PER_HOST,
			IdleConnTimeout:       HTTP_IDLE_CONN_TIMEOUT,
			TLSHandshakeTimeout:   HTTP_TLS_HANDSHAKE_TIMEOUT,
			ResponseHeaderTimeout: HTTP_RESPONSE_HEADER_TIMEOUT,
			ExpectContinueTimeout: HTTP_EXPECT_CONTINUE_TIMEOUT,
		},
	}
}

func showMainWindow(app *gtk.Application, win *MainWindow) {
	win.SetApplicationForGTKWindow(app)
	win.BuildUI()
	app.AddWindow(win.window)
	if win.window != nil {
		win.window.Show()
	}
}

func showFatalDialogAndLog(prefix string, err error) {
	log.Printf("%s: %v", prefix, err)
	d := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s: %v", prefix, err)
	d.Run()
	d.Destroy()
}
