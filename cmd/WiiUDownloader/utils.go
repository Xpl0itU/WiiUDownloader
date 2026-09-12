package main

import (
	_ "embed"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func formatBytes(bytes uint64) string {
	const unit = 1000

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	value := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	value = math.Round(value*100) / 100
	return fmt.Sprintf("%.2f %s", value, units[unitIndex])
}

func normalizeFilename(filename string) string {
	var out strings.Builder
	shouldAppend := true
	firstChar := true

	for _, c := range filename {
		switch {
		case c == '_':
			if shouldAppend {
				out.WriteRune('_')
				shouldAppend = false
			}
			firstChar = false
		case c == ' ':
			if shouldAppend && !firstChar {
				out.WriteRune(' ')
				shouldAppend = false
			}
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
			out.WriteRune(c)
			shouldAppend = true
			firstChar = false
		}
	}

	result := out.String()
	if len(result) > 0 && result[len(result)-1] == '_' {
		result = result[:len(result)-1]
	}

	return result
}

var desiredDark atomic.Bool
var guardConnected sync.Once

func setDarkTheme(darkMode bool) {
	desiredDark.Store(darkMode)

	if runtime.GOOS == "darwin" {
		if darkMode {
			os.Setenv("GTK_THEME", "Adwaita:dark")
		} else {
			os.Setenv("GTK_THEME", "Adwaita")
		}
	}

	gSettings, err := gtk.SettingsGetDefault()
	if err != nil || gSettings == nil {
		return
	}

	gSettings.SetProperty("gtk-application-prefer-dark-theme", darkMode)
	gSettings.SetProperty("gtk-theme-name", "Adwaita")

	if runtime.GOOS == "darwin" {
		guardConnected.Do(func() {
			gSettings.Connect("notify::gtk-theme-name", func() {
				if cur := readThemeName(gSettings); cur == "Adwaita" {
					return
				}
				gSettings.SetProperty("gtk-theme-name", "Adwaita")
			})
		})
	}
}

func readThemeName(s *gtk.Settings) string {
	v, err := s.GetProperty("gtk-theme-name")
	if err != nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case *glib.Value:
		if str, err := x.GetString(); err == nil {
			return str
		}
	}
	return ""
}

func applyStyling() {
	provider, err := gtk.CssProviderNew()
	if err != nil {
		log.Printf("failed to create CSS provider: %v", err)
		return
	}
	if err := provider.LoadFromData(styleCSS); err != nil {
		log.Printf("failed to load CSS styling: %v", err)
	}
	screen, err := gdk.ScreenGetDefault()
	if err != nil {
		log.Printf("failed to get default screen: %v", err)
		return
	}
	gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	pathInfo, err := os.Stat(path)
	return err == nil && pathInfo != nil && pathInfo.IsDir()
}

func ShowErrorDialog(window *gtk.Window, err error) {
	flags := gtk.DIALOG_MODAL
	if window == nil {
		flags = 0
	}
	dialog := gtk.MessageDialogNew(window, flags, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", err.Error())
	dialog.Run()
	dialog.Destroy()
}

const (
	queueDownloadIcon = "folder-download-symbolic"
	supportMeIcon     = "starred-symbolic"
)

//go:embed style.css
var styleCSS string

func detectErrorType(errorMsg string) string {
	errorLower := strings.ToLower(errorMsg)

	if strings.Contains(errorLower, "tmd") || strings.Contains(errorLower, "title.tmd") {
		return "TMD Download"
	}
	if strings.Contains(errorLower, "tik") || strings.Contains(errorLower, "cetk") || strings.Contains(errorLower, "title.tik") {
		return "Ticket Download"
	}
	if strings.Contains(errorLower, "cert") || strings.Contains(errorLower, "certificate") {
		return "Certificate Download"
	}
	if strings.Contains(errorLower, "decrypt") {
		return "Decryption"
	}
	if strings.Contains(errorLower, ".app") || strings.Contains(errorLower, ".h3") {
		return "Content Download"
	}
	if strings.Contains(errorLower, "content not found") {
		return "Decryption"
	}
	if strings.Contains(errorLower, "status code") || strings.Contains(errorLower, "connection") ||
		strings.Contains(errorLower, "timeout") || strings.Contains(errorLower, "network") {
		return "Network Error"
	}
	if strings.Contains(errorLower, "permission") || strings.Contains(errorLower, "no such file") {
		return "File I/O Error"
	}

	return "Download Error"
}
