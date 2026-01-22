package main

import (
	"log"
	"os"
	"strings"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

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

func setDarkTheme(darkMode bool) {
	gSettings, err := gtk.SettingsGetDefault()
	if err != nil {
		log.Println(err.Error())
	}
	gSettings.SetProperty("gtk-application-prefer-dark-theme", darkMode)
}

func applyStyling() {
	provider, _ := gtk.CssProviderNew()
	css := `
	headerbar {
		padding: 6px;
	}
	treeview.view {
		padding: 6px;
	}
	.button {
		padding: 4px 10px;
	}
	entry {
		padding: 4px 8px;
	}
	button.category-toggle {
		border-radius: 8px;
		padding: 4px 10px;
		transition: all 0.2s ease-in-out;
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
	`
	_ = provider.LoadFromData(css)
	screen, _ := gdk.ScreenGetDefault()
	gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}

func isValidPath(path string) bool {
	if path == "" {
		return false
	}
	if pathInfo, err := os.Stat(path); os.IsNotExist(err) || !pathInfo.IsDir() {
		return false
	}
	return true
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
