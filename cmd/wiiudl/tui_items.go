package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Palette. Every color adapts to light and dark terminal backgrounds; state is
// never conveyed by color alone (markers, brackets and icons accompany it).
var (
	tuiColorText    = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E7EB"}
	tuiColorMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	tuiColorSubtle  = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#3F4654"}
	tuiColorAccent  = lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#38BDF8"}
	tuiColorAccent2 = lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#7DD3FC"}
	tuiColorOnBrand = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0B1220"}
	tuiColorSuccess = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	tuiColorError   = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	tuiColorWarning = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
)

var (
	tuiBrandStyle   = lipgloss.NewStyle().Bold(true).Foreground(tuiColorOnBrand).Background(tuiColorAccent).Padding(0, 1)
	tuiTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(tuiColorText)
	tuiTextStyle    = lipgloss.NewStyle().Foreground(tuiColorText)
	tuiAccentStyle  = lipgloss.NewStyle().Bold(true).Foreground(tuiColorAccent)
	tuiDimStyle     = lipgloss.NewStyle().Foreground(tuiColorMuted)
	tuiSubtleStyle  = lipgloss.NewStyle().Foreground(tuiColorSubtle)
	tuiErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(tuiColorError)
	tuiWarningStyle = lipgloss.NewStyle().Foreground(tuiColorWarning)
	tuiSuccessStyle = lipgloss.NewStyle().Bold(true).Foreground(tuiColorSuccess)
	tuiLabelStyle   = lipgloss.NewStyle().Foreground(tuiColorMuted)
	tuiSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(tuiColorAccent2)
	tuiKeyStyle     = lipgloss.NewStyle().Bold(true).Foreground(tuiColorAccent)
	tuiBadgeStyle   = lipgloss.NewStyle().Foreground(tuiColorAccent2)
)

// tuiRow is implemented by every list item so a single delegate can render
// menus, titles and files consistently.
type tuiRow interface {
	list.Item
	// rowParts returns the main text and right-aligned metadata for a row of
	// the given width. Metadata is dropped by the delegate when space is short.
	rowParts(width int) (main, meta string)
	rowDesc() string
	rowDim() bool
}

type tuiMenuItem struct {
	label  string
	desc   string
	key    string
	screen tuiScreen
	action string
}

func (i tuiMenuItem) Title() string                 { return i.label }
func (i tuiMenuItem) Description() string           { return i.desc }
func (i tuiMenuItem) FilterValue() string           { return i.label }
func (i tuiMenuItem) rowParts(int) (string, string) { return i.label, i.key }
func (i tuiMenuItem) rowDesc() string               { return i.desc }
func (i tuiMenuItem) rowDim() bool                  { return false }

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

func (i tuiTitleItem) rowParts(width int) (string, string) {
	kind := padText(truncateText(wiiudownloader.GetFormattedKind(i.entry.TitleID), 10), 10)
	region := padText(truncateText(formatRegion(i.entry.Region), 16), 16)
	tid := fmt.Sprintf("%016x", i.entry.TitleID)
	switch {
	case width >= 84:
		return i.Title(), kind + "  " + region + "  " + tid
	case width >= 58:
		return i.Title(), kind + "  " + region
	case width >= 36:
		return i.Title(), strings.TrimSpace(kind)
	default:
		return i.Title(), ""
	}
}

// titleListHeader labels the columns produced by tuiTitleItem.rowParts for a
// list of the given total width.
func titleListHeader(width int) string {
	inner := width - 2
	var meta string
	switch {
	case inner >= 84:
		meta = padText("TYPE", 10) + "  " + padText("REGION", 16) + "  " + padText("TITLE ID", 16)
	case inner >= 58:
		meta = padText("TYPE", 10) + "  " + padText("REGION", 16)
	case inner >= 36:
		meta = "TYPE"
	}
	return "  " + padText("TITLE", inner-lipgloss.Width(meta)) + meta
}

func (i tuiTitleItem) rowDesc() string { return i.Description() }
func (i tuiTitleItem) rowDim() bool    { return false }

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

func (i tuiFileItem) rowParts(width int) (string, string) {
	if width < 30 {
		return i.file.Path, ""
	}
	return i.file.Path, fmt.Sprintf("%10s", i.Description())
}

func (i tuiFileItem) rowDesc() string { return i.Description() }
func (i tuiFileItem) rowDim() bool    { return i.file.Shared }

type tuiActionItem struct {
	label  string
	desc   string
	key    string
	action string
}

func (i tuiActionItem) Title() string                 { return i.label }
func (i tuiActionItem) Description() string           { return i.desc }
func (i tuiActionItem) FilterValue() string           { return i.label }
func (i tuiActionItem) rowParts(int) (string, string) { return i.label, i.key }
func (i tuiActionItem) rowDesc() string               { return i.desc }
func (i tuiActionItem) rowDim() bool                  { return false }

// tuiRowDelegate renders rows as one line (dense lists) or two lines (menus
// with descriptions). The selected row is marked with a glyph as well as
// color; when the list does not own keyboard focus, the marker is muted so the
// user can still see which entry Enter will act on.
type tuiRowDelegate struct {
	twoLine bool
	focused bool
}

func (d *tuiRowDelegate) Height() int {
	if d.twoLine {
		return 2
	}
	return 1
}

func (d *tuiRowDelegate) Spacing() int { return 0 }

func (d *tuiRowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d *tuiRowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(tuiRow)
	if !ok {
		return
	}
	width := m.Width()
	if width < 8 {
		width = 8
	}
	selected := index == m.Index()
	inner := width - 2

	main, meta := row.rowParts(inner)
	metaWidth := lipgloss.Width(meta)
	mainWidth := inner - metaWidth
	if meta != "" {
		mainWidth -= 2
	}
	if mainWidth < 10 {
		meta, metaWidth, mainWidth = "", 0, inner
	}
	main = truncateText(main, mainWidth)
	gap := inner - lipgloss.Width(main) - metaWidth
	if gap < 0 {
		gap = 0
	}

	marker, descMarker := "  ", "  "
	mainStyle := tuiTextStyle
	metaStyle := tuiDimStyle
	if row.rowDim() {
		mainStyle = tuiDimStyle
	}
	if selected {
		if d.focused {
			mainStyle = tuiAccentStyle
			metaStyle = tuiBadgeStyle
			if d.twoLine {
				marker = tuiAccentStyle.Render("┃ ")
				descMarker = marker
			} else {
				marker = tuiAccentStyle.Render("▸ ")
			}
		} else {
			mainStyle = mainStyle.Bold(true)
			marker = tuiDimStyle.Render("› ")
		}
	}

	line := marker + mainStyle.Render(main) + strings.Repeat(" ", gap) + metaStyle.Render(meta)
	if d.twoLine {
		desc := truncateText(row.rowDesc(), inner)
		descStyle := tuiDimStyle
		if selected && d.focused {
			descStyle = tuiTextStyle
		}
		line += "\n" + descMarker + descStyle.Render(desc)
	}
	fmt.Fprint(w, line)
}

func newTUIList(items []list.Item, width, height int) list.Model {
	return newTUIListWith(&tuiRowDelegate{focused: true}, items, width, height)
}

func newTUIMenuList(items []list.Item, width, height int, compact bool) list.Model {
	return newTUIListWith(&tuiRowDelegate{twoLine: !compact, focused: true}, items, width, height)
}

func newTUIListWith(delegate *tuiRowDelegate, items []list.Item, width, height int) list.Model {
	model := list.New(items, delegate, width, height)
	model.SetShowTitle(false)
	model.SetShowStatusBar(false)
	model.SetShowHelp(false)
	model.SetShowPagination(false)
	model.SetFilteringEnabled(false)
	model.DisableQuitKeybindings()
	model.Styles.NoItems = tuiDimStyle
	return model
}

func tuiHomeItems() []list.Item {
	return []list.Item{
		tuiMenuItem{label: "Search titles", desc: "Find games, updates, DLC and demos by name or title ID", key: "/", screen: screenSearch},
		tuiMenuItem{label: "Download by title ID", desc: "Enter a 16-digit title ID directly", screen: screenTitleID},
		tuiMenuItem{label: "Decrypt a folder", desc: "Decrypt a title folder downloaded without decryption", screen: screenDecrypt},
		tuiMenuItem{label: "Update title database", desc: "Download the latest title list", action: "db-update"},
		tuiMenuItem{label: "Help", desc: "Show the interactive controls and tips", key: "?", screen: screenHelp},
		tuiMenuItem{label: "Quit", desc: "Exit WiiUDownloader", key: "q", action: "quit"},
	}
}

func tuiActionItems() []list.Item {
	return []list.Item{
		tuiActionItem{label: "Download", desc: "Choose a folder and download this title", key: "d", action: "download"},
		tuiActionItem{label: "Show info", desc: "Fetch the TMD and show version, size and contents", key: "i", action: "info"},
		tuiActionItem{label: "Browse files / extract", desc: "List the files inside the title and extract selected ones", key: "f", action: "files"},
		tuiActionItem{label: "Back", desc: "Return to the title list", key: "esc", action: "back"},
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

// shortenHomePath replaces the home directory prefix with ~ for display.
func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
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

// truncateText shortens plain (unstyled) text to width cells, adding an
// ellipsis when something was cut.
func truncateText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	var builder strings.Builder
	used := 0
	for _, r := range text {
		w := lipgloss.Width(string(r))
		if used+w > width-1 {
			break
		}
		builder.WriteRune(r)
		used += w
	}
	return builder.String() + "…"
}

// truncateLeft keeps the end of text (useful for paths), prefixing an ellipsis.
func truncateLeft(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	used := 1
	start := len(runes)
	for start > 0 {
		w := lipgloss.Width(string(runes[start-1]))
		if used+w > width {
			break
		}
		used += w
		start--
	}
	return "…" + string(runes[start:])
}

func padText(text string, width int) string {
	gap := width - lipgloss.Width(text)
	if gap <= 0 {
		return text
	}
	return text + strings.Repeat(" ", gap)
}

// formatCount renders an integer with thousands separators.
func formatCount(n int) string {
	text := strconv.Itoa(n)
	if n < 0 {
		return text
	}
	var builder strings.Builder
	for index, r := range text {
		if index > 0 && (len(text)-index)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return formatCount(n) + " " + singular
	}
	return formatCount(n) + " " + pluralForm
}

type tuiHelpEntry struct {
	keys string
	desc string
}

type tuiHelpSection struct {
	title   string
	entries []tuiHelpEntry
}

func tuiHelpSections() []tuiHelpSection {
	return []tuiHelpSection{
		{"Everywhere", []tuiHelpEntry{
			{"esc", "go back one screen"},
			{"ctrl+c", "quit (during a transfer: cancel, press again to force quit)"},
		}},
		{"Main menu", []tuiHelpEntry{
			{"↑/↓  j/k", "move"},
			{"enter", "select"},
			{"/", "search titles"},
			{"?", "this help"},
			{"q", "quit"},
		}},
		{"Search", []tuiHelpEntry{
			{"type", "search by name or title ID"},
			{"tab  shift+tab", "cycle search → region → type → results"},
			{"←/→  space", "change the focused region or type filter"},
			{"↓", "jump from the search box to the results"},
			{"↑/↓  pgup/pgdn", "move through results (↑ at the top returns to the search box)"},
			{"enter", "open the selected title's actions"},
			{"d  i  f", "download, info, or browse files (results focused)"},
			{"/", "back to the search box (results focused)"},
		}},
		{"Title actions", []tuiHelpEntry{
			{"↑/↓  enter", "choose an action"},
			{"d  i  f", "download, info, or browse files directly"},
		}},
		{"Download options", []tuiHelpEntry{
			{"↑/↓  tab", "move between fields"},
			{"space  ←/→", "toggle decrypt / delete encrypted"},
			{"enter", "next field, or press a button"},
		}},
		{"During a transfer", []tuiHelpEntry{
			{"esc", "cancel the download, extraction or decryption"},
			{"ctrl+c", "cancel; press again to force quit"},
		}},
		{"Info", []tuiHelpEntry{
			{"↑/↓  pgup/pgdn", "scroll"},
		}},
		{"File browser", []tuiHelpEntry{
			{"tab  /", "switch to the filter / focus the filter"},
			{"↓  enter", "from the filter, move to the list"},
			{"enter", "extract the highlighted file"},
			{"a", "extract every file in the current filter"},
		}},
	}
}

func tuiHelpText(width int) string {
	keyWidth := 16
	if width < 60 {
		keyWidth = 11
	}
	descWidth := width - keyWidth - 4
	if descWidth < 12 {
		descWidth = 12
	}
	var builder strings.Builder
	builder.WriteString(tuiTitleStyle.Render("Keyboard controls") + "\n")
	builder.WriteString(tuiDimStyle.Render(strings.TrimRight(lipgloss.NewStyle().Width(width).Render(
		"The footer always lists the shortcuts for the focused control."), " ")) + "\n")
	for _, section := range tuiHelpSections() {
		builder.WriteString("\n" + tuiSectionStyle.Render(strings.ToUpper(section.title)) + "\n")
		for _, entry := range section.entries {
			keys := padText(truncateText(entry.keys, keyWidth), keyWidth)
			desc := lipgloss.NewStyle().Width(descWidth).Render(entry.desc)
			descLines := strings.Split(desc, "\n")
			for index, line := range descLines {
				prefix := strings.Repeat(" ", keyWidth)
				if index == 0 {
					prefix = tuiKeyStyle.Render(keys)
				}
				builder.WriteString("  " + prefix + "  " + tuiTextStyle.Render(strings.TrimRight(line, " ")) + "\n")
			}
		}
	}
	builder.WriteString("\n" + tuiSectionStyle.Render("TIPS") + "\n")
	for _, tip := range []string{
		"Downloads resume automatically if interrupted.",
		"Decrypted output is ready for Cemu or a Wii U console.",
		"Update the title database after long breaks to see new titles.",
		"Every regular `wiiudl` command still works outside the TUI.",
	} {
		wrapped := lipgloss.NewStyle().Width(width - 4).Render(tip)
		for index, line := range strings.Split(wrapped, "\n") {
			bullet := "  "
			if index == 0 {
				bullet = tuiDimStyle.Render("• ")
			}
			builder.WriteString("  " + bullet + tuiTextStyle.Render(strings.TrimRight(line, " ")) + "\n")
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}
