package main

import (
	"os/exec"
	"runtime"
	"strings"
)

func isDarkMode() bool {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "Dark" {
			return true
		}

	case "windows":
		cmd := exec.Command("reg", "query", "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize", "/v", "AppsUseLightTheme")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "REG_DWORD") && strings.Contains(string(output), "0x0") {
			return true
		}

	case "linux":
		cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "gtk-theme")
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "'Adwaita-dark'" {
			return true
		}
	}

	return false
}
