package main

import (
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
	css := `
	headerbar {
		padding: 6px;
	}
	treeview.view {
		padding: 6px;
	}
	.button, button {
		padding: 5px 12px;
	}
	entry {
		padding: 5px 10px;
	}
	button.category-toggle {
		border-radius: 8px;
		padding: 5px 12px;
		transition: all 0.2s ease-in-out;
	}
	checkbutton {
		padding: 4px 8px;
	}
	button.category-toggle:hover,
	radio.category-toggle:hover {
		background: shade(@theme_bg_color, 0.95);
	}
	button.category-toggle:checked,
	radio.category-toggle:checked,
	button.category-toggle:active,
	radio.category-toggle:active {
		background: @theme_selected_bg_color;
		background-color: #3584e4; /* Explicit fallback blue (Adwaita blue) */
		color: @theme_selected_fg_color;
		color: white;
	}
	.settings-window label {
		font-weight: 600;
	}
	.settings-grid entry {
		padding: 6px 10px;
	}
	.settings-grid button {
		padding: 6px 10px;
	}
	.gratitude-footer {
		border-top: 2px solid #00a2ed;
		background: shade(@theme_bg_color, 0.94);
		padding: 16px;
		font-size: 1.1em;
	}
	button.kofi-btn {
		background-image: none;
		background-color: #ff813f;
		color: #ffffff;
		font-weight: bold;
		border-radius: 8px;
		padding: 6px 16px;
		font-size: 1.1em;
		transition: all 0.2s ease-in-out;
	}
	button.kofi-btn:hover {
		background-image: none;
		background-color: #ff9359;
	}
	.kofi-btn {
		box-shadow: 0 1px 2px rgba(0,0,0,0.12);
	}
	.supporter-count {
		font-size: 0.9em;
		color: @theme_unfocused_fg_color;
		margin-left: 8px;
	}
	.donation-highlight {
		border-top: 2px solid #00a2ed;
		background: shade(@theme_bg_color, 0.96);
		padding: 16px;
		font-size: 1.2em;
	}
	.success-flash {
		background-color: #00a2ed;
		color: white;
	}
	.total-size-label {
		font-weight: bold;
		font-size: 1.1em;
		padding: 8px 12px;
		color: @theme_fg_color;
	}
	.queue-pane-vbox {
		background: @theme_bg_color;
	}
	notebook {
		padding: 0;
	}
	notebook stack {
		background: @theme_bg_color;
		padding: 12px;
	}
	.title {
		font-size: 1.2em;
		font-weight: 500;
	}
	.subtitle {
		font-size: 0.95em;
		color: @theme_unfocused_fg_color;
	}
	.dim-label {
		color: @theme_unfocused_fg_color;
		opacity: 0.7;
	}
	.sidebar {
		background-color: @theme_bg_color;
		border-right: 1px solid shade(@theme_bg_color, 0.9);
	}
	.linked button {
		border-radius: 0;
	}
	.linked button:first-child {
		border-top-left-radius: 6px;
		border-bottom-left-radius: 6px;
	}
	.linked button:last-child {
		border-top-right-radius: 6px;
		border-bottom-right-radius: 6px;
		border-left: none;
	}
	`

	if err := provider.LoadFromData(css); err != nil {
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

func escapeMarkup(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")
	return text
}

func queueDownloadIconName() string {
	return "folder-download-symbolic"
}

func supportMeIconName() string {
	return "starred-symbolic"
}

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
