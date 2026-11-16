package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

var (
	tuiTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	tuiAccentStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	tuiDimStyle     = lipgloss.NewStyle().Faint(true)
	tuiErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	tuiSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("84"))
	tuiHelpStyle    = lipgloss.NewStyle().Faint(true)
	tuiLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)

type tuiMenuItem struct {
	label  string
	desc   string
	screen tuiScreen
	action string
}

func (i tuiMenuItem) Title() string       { return i.label }
func (i tuiMenuItem) Description() string { return i.desc }
func (i tuiMenuItem) FilterValue() string { return i.label }

type tuiTitleItem struct {
	entry wiiudownloader.TitleEntry
}

func (i tuiTitleItem) Title() string {
	if i.entry.Name == "" {
		return fmt.Sprintf("%016x", i.entry.TitleID)
	}
	return i.entry.Name
}

func (i tuiTitleItem) Description() string {
	return fmt.Sprintf("%s · %s · %016x",
		wiiudownloader.GetFormattedKind(i.entry.TitleID),
		formatRegion(i.entry.Region),
		i.entry.TitleID,
	)
}

func (i tuiTitleItem) FilterValue() string {
	return i.entry.Name + " " + fmt.Sprintf("%016x", i.entry.TitleID)
}

type tuiFileItem struct {
	file wiiudownloader.TitleFile
}

func (i tuiFileItem) Title() string { return i.file.Path }

func (i tuiFileItem) Description() string {
	if i.file.Shared {
		return "shared entry"
	}
	return formatBytes(int64(i.file.Size))
}

func (i tuiFileItem) FilterValue() string { return i.file.Path }

type tuiActionItem struct {
	label  string
	desc   string
	action string
}

func (i tuiActionItem) Title() string       { return i.label }
func (i tuiActionItem) Description() string { return i.desc }
func (i tuiActionItem) FilterValue() string { return i.label }

func newTUIList(items []list.Item, width, height int) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	model := list.New(items, delegate, width, height)
	model.SetShowTitle(false)
	model.SetShowStatusBar(false)
	model.SetShowHelp(false)
	model.SetShowPagination(false)
	model.SetFilteringEnabled(false)
	model.DisableQuitKeybindings()
	return model
}

func tuiHomeItems() []list.Item {
	return []list.Item{
		tuiMenuItem{label: "Search titles", desc: "Find games, updates, DLC and demos by name or title ID", screen: screenSearch},
		tuiMenuItem{label: "Download by title ID", desc: "Enter a 16-digit title ID directly", screen: screenTitleID},
		tuiMenuItem{label: "Decrypt a folder", desc: "Decrypt a title folder downloaded without decryption", screen: screenDecrypt},
		tuiMenuItem{label: "Update title database", desc: "Download the latest title list", action: "db-update"},
		tuiMenuItem{label: "Help", desc: "Show the interactive controls and tips", screen: screenHelp},
		tuiMenuItem{label: "Quit", desc: "Exit WiiUDownloader", action: "quit"},
	}
}

func tuiActionItems() []list.Item {
	return []list.Item{
		tuiActionItem{label: "Download", desc: "Choose a folder and download this title", action: "download"},
		tuiActionItem{label: "Show info", desc: "Fetch the TMD and show version, size and contents", action: "info"},
		tuiActionItem{label: "Browse files / extract", desc: "List the files inside the title and extract selected ones", action: "files"},
		tuiActionItem{label: "Back", desc: "Return to the title list", action: "back"},
	}
}

type tuiRegionChoice struct {
	label string
	mask  uint8
}

var tuiRegionChoices = []tuiRegionChoice{
	{label: "All", mask: allRegionMask},
	{label: "USA", mask: wiiudownloader.MCP_REGION_USA},
	{label: "Europe", mask: wiiudownloader.MCP_REGION_EUROPE},
	{label: "Japan", mask: wiiudownloader.MCP_REGION_JAPAN},
	{label: "USA + Europe", mask: wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_EUROPE},
}

type tuiTypeChoice struct {
	label    string
	category uint8
}

var tuiTypeChoices = []tuiTypeChoice{
	{label: "All", category: wiiudownloader.TITLE_CATEGORY_ALL},
	{label: "Game", category: wiiudownloader.TITLE_CATEGORY_GAME},
	{label: "Update", category: wiiudownloader.TITLE_CATEGORY_UPDATE},
	{label: "DLC", category: wiiudownloader.TITLE_CATEGORY_DLC},
	{label: "Demo", category: wiiudownloader.TITLE_CATEGORY_DEMO},
}

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	downloads := filepath.Join(home, "Downloads")
	if info, statErr := os.Stat(downloads); statErr == nil && info.IsDir() {
		return filepath.Join(downloads, "WiiUDownloader")
	}
	return filepath.Join(home, "WiiUDownloader")
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func validateVersionText(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "latest") {
		return "latest", nil
	}
	version, err := strconv.Atoi(value)
	if err != nil || version < 0 {
		return "", fmt.Errorf("version must be \"latest\" or a non-negative number")
	}
	return strconv.Itoa(version), nil
}

func tuiHelpText() string {
	return strings.Join([]string{
		tuiTitleStyle.Render("WiiUDownloader interactive mode"),
		"",
		tuiAccentStyle.Render("Main menu"),
		"  ↑/↓ or j/k   move",
		"  Enter        select",
		"  q            quit",
		"",
		tuiAccentStyle.Render("Search"),
		"  Type         search by name or title ID",
		"  Tab          move between search, region, type and results",
		"  ←/→          change the focused region or type filter",
		"  ↑/↓          move through results",
		"  Enter        open the selected title's actions",
		"  d / i / f    download, info, or browse files for the selected title",
		"  Esc          back to the main menu",
		"",
		tuiAccentStyle.Render("Download options"),
		"  ↑/↓, Tab     move between fields",
		"  Space        toggle decrypt / delete-encrypted",
		"  Enter        start the download or confirm a field",
		"  Esc          back",
		"",
		tuiAccentStyle.Render("During a transfer"),
		"  Esc          cancel the current download, extraction or decryption",
		"  Ctrl+C       cancel; press again to force quit",
		"",
		tuiAccentStyle.Render("File browser"),
		"  Tab          switch between the filter and the file list",
		"  Enter        extract the highlighted file",
		"  a            extract all currently filtered files",
		"  Esc          back",
		"",
		tuiAccentStyle.Render("Tips"),
		"  • Downloads resume automatically if interrupted.",
		"  • Decrypted output is ready for Cemu or a Wii U console.",
		"  • Use the title database update after long breaks to see new titles.",
		"  • Every regular `wiiudl` command still works outside the TUI.",
	}, "\n")
}
