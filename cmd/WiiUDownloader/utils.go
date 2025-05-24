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
