package main

import (
	"bytes"
	_ "embed"
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	REPO_URL    = "https://github.com/Xpl0itU/WiiUDownloader"
	ISSUES_URL  = REPO_URL + "/issues"
	DOCS_URL    = "https://xpl0itu.github.io/WiiUDownloaderDocs/docs/"
	DISCORD_URL = "https://discord.gg/UrsB2XnUDM"
)

//go:embed appicon.png
var appIconPNG []byte

func installAppIcon() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return
	}

	themeDir := filepath.Join(cacheDir, APP_NAME, "icons")
	iconDir := filepath.Join(themeDir, "hicolor", "512x512", "apps")
	target := filepath.Join(iconDir, APP_ID+".png")

	if current, err := os.ReadFile(target); err != nil || !bytes.Equal(current, appIconPNG) {
		if err := os.MkdirAll(iconDir, 0o755); err != nil {
			log.Printf("Failed to publish the app icon: %v", err)
			return
		}
		if err := os.WriteFile(target, appIconPNG, 0o644); err != nil {
			log.Printf("Failed to publish the app icon: %v", err)
			return
		}
	}

	gtk.IconThemeGetForDisplay(display).AddSearchPath(themeDir)
}

func newAboutDialog() *adw.AboutDialog {
	dialog := adw.NewAboutDialog()
	dialog.SetApplicationName(APP_NAME)
	dialog.SetApplicationIcon(APP_ID)
	dialog.SetVersion(APP_VERSION)
	dialog.SetDeveloperName("Xpl0itU")
	dialog.SetCopyright("Copyright 2022-2026 Xpl0itU")
	dialog.SetLicenseType(gtk.LicenseGPL30)
	dialog.SetIssueURL(ISSUES_URL)
	dialog.AddLink("GitHub", REPO_URL)
	dialog.AddLink("Usage Guide", DOCS_URL)
	dialog.AddLink("Discord", DISCORD_URL)
	return dialog
}

func (mw *MainWindow) showAboutDialog() *adw.AboutDialog {
	dialog := newAboutDialog()
	dialog.Present(mw.window)
	return dialog
}
