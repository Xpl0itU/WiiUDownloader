package main

import (
	"log"
	"os"
	"strings"

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
	dialog := gtk.MessageDialogNew(window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", err.Error())
	dialog.Run()
	dialog.Destroy()
}
