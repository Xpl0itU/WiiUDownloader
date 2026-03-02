package main

import (
	"os/exec"
	"runtime"
	"strings"
)

const (
	WINDOWS_THEME_REG_VALUE_TYPE = "REG_DWORD"
	WINDOWS_DARK_MODE_REG_VALUE  = "0x0"
	LINUX_DARK_THEME_SUFFIX      = "-dark"
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
		if err == nil && strings.Contains(string(output), WINDOWS_THEME_REG_VALUE_TYPE) && strings.Contains(string(output), WINDOWS_DARK_MODE_REG_VALUE) {
			return true
		}

	case "linux":
		cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "gtk-theme")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), LINUX_DARK_THEME_SUFFIX) {
			return true
		}
	}

	return false
}
