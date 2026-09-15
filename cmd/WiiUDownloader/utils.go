package main

import (
	_ "embed"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

// setDarkTheme drives libadwaita's style manager, which owns both the palette
// and the dark variant of the stylesheet. Setting GTK_THEME instead (as the
// GTK3 version did) overrides libadwaita's own CSS and breaks its widgets.
func setDarkTheme(darkMode bool) {
	scheme := adw.ColorSchemeForceLight
	if darkMode {
		scheme = adw.ColorSchemeForceDark
	}
	manager := adw.StyleManagerGetDefault()
	if manager == nil {
		return
	}
	manager.SetColorScheme(scheme)
}

func applyStyling() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		log.Printf("failed to get default display")
		return
	}
	provider := gtk.NewCSSProvider()
	// Parse failures surface through the parsing-error signal.
	provider.ConnectParsingError(func(_ *gtk.CSSSection, err error) {
		log.Printf("failed to load CSS styling: %v", err)
	})
	provider.LoadFromString(styleCSS)
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	pathInfo, err := os.Stat(path)
	return err == nil && pathInfo != nil && pathInfo.IsDir()
}

func ShowErrorDialog(window *gtk.Window, err error) {
	if err == nil {
		return
	}
	showAlert(window, WINDOW_TITLE_PREFIX+"Error", err.Error())
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
