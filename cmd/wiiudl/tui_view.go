package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

const (
	tuiMinWidth   = 30
	tuiMinHeight  = 8
	tuiLabelWidth = 14
)

// ---------------------------------------------------------------------------
// Layout

// contentWidth is the usable width inside the one-column side margins.
func (m *tuiModel) contentWidth() int {
	width := m.width - 2
	if width < tuiMinWidth-2 {
		width = tuiMinWidth - 2
	}
	return width
}

// topPad reports whether there is room for a blank line under the header.
func (m *tuiModel) topPad() bool { return m.height >= 20 }

// bodyHeight is the number of lines available between header and footer.
func (m *tuiModel) bodyHeight() int {
	height := m.height - 4 // header, header rule, footer rule, footer
	if m.topPad() {
		height--
	}
	if height < 3 {
		height = 3
	}
	return height
}

// compact reports whether optional spacing and section headings should be
// dropped so forms still fit on short terminals.
func (m *tuiModel) compact() bool { return m.bodyHeight() < 18 }

// searchControlLines counts the lines above the result list: search box,
// filters (one or two lines) and the column header.
func (m *tuiModel) searchControlLines() int {
	if m.contentWidth() < 46 {
		return 4
	}
	return 3
}

func (m *tuiModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < tuiMinWidth || m.height < tuiMinHeight {
		msg := tuiWarningStyle.Render("Terminal too small") + "\n" +
			tuiDimStyle.Render(fmt.Sprintf("%d×%d, need %d×%d", m.width, m.height, tuiMinWidth, tuiMinHeight))
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
	}

	width := m.contentWidth()
	body := fitBlock(m.screenBody(width), width, m.bodyHeight())
	rule := tuiSubtleStyle.Render(strings.Repeat("─", m.width))

	lines := []string{m.headerView(), rule}
	if m.topPad() {
		lines = append(lines, "")
	}
	lines = append(lines, indentBlock(body, 1), rule, m.footerView())
	return strings.Join(lines, "\n")
}

// fitBlock truncates every line to width and pads or cuts the block to
// exactly height lines, so the footer always stays anchored at the bottom.
func fitBlock(block string, width, height int) string {
	lines := strings.Split(block, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	clip := lipgloss.NewStyle().MaxWidth(width)
	for index, line := range lines {
		if lipgloss.Width(line) > width {
			lines[index] = clip.Render(line)
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func indentBlock(block string, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		lines[index] = pad + line
	}
	return strings.Join(lines, "\n")
}

// padLines pads a rendered component to exactly height lines.
func padLines(view string, height int) string {
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Header and footer

func (m *tuiModel) headerView() string {
	brand := tuiBrandStyle.Render("WiiUDownloader")
	if m.width < 56 {
		brand = tuiBrandStyle.Render("WiiU")
	}
	right := ""
	if m.db != nil && m.width >= 64 {
		right = tuiDimStyle.Render(plural(len(m.db.entries), "title", "titles"))
	}
	avail := m.width - lipgloss.Width(brand) - lipgloss.Width(right) - 4
	crumbs := renderCrumbs(m.breadcrumb(), avail)
	gap := m.width - lipgloss.Width(brand) - 1 - lipgloss.Width(crumbs) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	return brand + " " + crumbs + strings.Repeat(" ", gap) + right
}

func renderCrumbs(parts []string, width int) string {
	if width <= 0 || len(parts) == 0 {
		return ""
	}
	const sep = " › "
	joinedWidth := func(items []string) int {
		return lipgloss.Width(strings.Join(items, sep))
	}
	// Drop leading crumbs first, but keep the last two (usually the title
	// and the current step) and shorten them instead.
	dropped := false
	for len(parts) > 2 && joinedWidth(parts)+2 > width {
		parts = parts[1:]
		dropped = true
	}
	prefix := ""
	if dropped {
		prefix = "…" + sep
	}
	parts = append([]string(nil), parts...)
	last := len(parts) - 1
	if last > 0 {
		overflow := lipgloss.Width(prefix) + joinedWidth(parts) - width
		if overflow > 0 {
			keep := lipgloss.Width(parts[last-1]) - overflow
			if keep < 8 {
				keep = 8
			}
			parts[last-1] = truncateText(parts[last-1], keep)
		}
	}
	parts[last] = truncateText(parts[last], width-lipgloss.Width(prefix)-joinedWidth(parts[:last])-lipgloss.Width(sep)*min(last, 1))

	var builder strings.Builder
	builder.WriteString(tuiDimStyle.Render(prefix))
	for index, part := range parts {
		if index > 0 {
			builder.WriteString(tuiSubtleStyle.Render(sep))
		}
		if index == last {
			builder.WriteString(tuiTitleStyle.Render(part))
		} else {
			builder.WriteString(tuiDimStyle.Render(part))
		}
	}
	return builder.String()
}

func (m *tuiModel) breadcrumb() []string {
	name := displayName(m.selected)
	switch m.screen {
	case screenHome:
		return []string{"Main menu"}
	case screenSearch:
		return []string{"Search"}
	case screenActions:
		return []string{"Search", name}
	case screenDownloadOptions:
		return []string{"Search", name, "Download"}
	case screenProgress:
		return []string{m.progressLabel(), m.progressTitle}
	case screenInfo:
		return []string{"Search", displayName(m.infoEntry), "Info"}
	case screenFiles:
		return []string{"Search", displayName(m.filesEntry), "Files"}
	case screenExtractOptions:
		return []string{"Search", displayName(m.filesEntry), "Files", "Extract"}
	case screenDecrypt:
		return []string{"Decrypt a folder"}
	case screenTitleID:
		return []string{"Download by title ID"}
	case screenDBUpdate:
		return []string{"Update title database"}
	case screenHelp:
		return []string{"Help"}
	case screenMessage:
		return []string{m.messageTitle}
	}
	return nil
}

type tuiKey struct {
	key  string
	desc string
}

func (m *tuiModel) footerView() string {
	keys := m.footerKeys()
	sep := tuiSubtleStyle.Render("  ·  ")
	render := func(items []tuiKey) string {
		parts := make([]string, 0, len(items))
		for _, key := range items {
			parts = append(parts, tuiKeyStyle.Render(key.key)+" "+tuiDimStyle.Render(key.desc))
		}
		return " " + strings.Join(parts, sep)
	}
	// Drop hints from the middle so the first (primary action) and the
	// last (usually "esc back") stay visible on narrow terminals.
	for len(keys) > 2 && lipgloss.Width(render(keys)) > m.width {
		keys = append(append([]tuiKey(nil), keys[:len(keys)-2]...), keys[len(keys)-1])
	}
	if lipgloss.Width(render(keys)) > m.width && len(keys) > 1 {
		keys = keys[:1]
	}
	return render(keys)
}

// footerKeys lists the shortcuts that apply to the focused control, most
// important first; trailing entries are dropped on narrow terminals.
func (m *tuiModel) footerKeys() []tuiKey {
	switch m.screen {
	case screenHome:
		return []tuiKey{{"↑/↓", "move"}, {"enter", "select"}, {"/", "search"}, {"?", "help"}, {"q", "quit"}}
	case screenSearch:
		switch m.searchFocus {
		case 0:
			return []tuiKey{{"type", "to search"}, {"enter", "open"}, {"↓", "results"}, {"tab", "filters"}, {"esc", "menu"}}
		case 1, 2:
			return []tuiKey{{"←/→", "change"}, {"tab", "next"}, {"↓", "results"}, {"↑", "search box"}, {"esc", "menu"}}
		default:
			return []tuiKey{{"↑/↓", "move"}, {"enter", "actions"}, {"d", "download"}, {"i", "info"}, {"f", "files"}, {"/", "search"}, {"esc", "menu"}}
		}
	case screenActions:
		return []tuiKey{{"↑/↓", "move"}, {"enter", "select"}, {"d/i/f", "shortcuts"}, {"esc", "back"}, {"?", "help"}}
	case screenDownloadOptions:
		switch m.downloadFocus {
		case 0, 1:
			return []tuiKey{{"type", "to edit"}, {"enter/↓", "next"}, {"↑", "previous"}, {"esc", "back"}}
		case 2, 3:
			return []tuiKey{{"space", "toggle"}, {"↑/↓", "move"}, {"esc", "back"}}
		default:
			return []tuiKey{{"enter", "press"}, {"←/→", "switch button"}, {"↑/↓", "move"}, {"esc", "back"}}
		}
	case screenProgress:
		if m.cancelling {
			return []tuiKey{{"ctrl+c", "force quit"}}
		}
		return []tuiKey{{"esc", "cancel"}, {"ctrl+c ×2", "force quit"}}
	case screenInfo:
		return []tuiKey{{"↑/↓", "scroll"}, {"pgup/pgdn", "page"}, {"esc", "back"}}
	case screenFiles:
		if m.filesLoading {
			return []tuiKey{{"esc", "back"}}
		}
		if m.fileFocus == 0 {
			return []tuiKey{{"type", "to filter"}, {"enter/↓", "list"}, {"tab", "list"}, {"esc", "back"}}
		}
		return []tuiKey{{"↑/↓", "move"}, {"enter", "extract file"}, {"a", "extract all shown"}, {"/", "filter"}, {"esc", "back"}}
	case screenExtractOptions:
		if m.extractFocus == 0 {
			return []tuiKey{{"type", "to edit"}, {"enter", "next"}, {"↑/↓", "move"}, {"esc", "back"}}
		}
		return []tuiKey{{"enter", "press"}, {"←/→", "switch button"}, {"↑/↓", "move"}, {"esc", "back"}}
	case screenDecrypt:
		switch m.decryptFocus {
		case 0:
			return []tuiKey{{"type", "to edit"}, {"enter/↓", "next"}, {"esc", "back"}}
		case 1:
			return []tuiKey{{"space", "toggle"}, {"↑/↓", "move"}, {"esc", "back"}}
		default:
			return []tuiKey{{"enter", "press"}, {"←/→", "switch button"}, {"↑/↓", "move"}, {"esc", "back"}}
		}
	case screenTitleID:
		return []tuiKey{{"enter", "continue"}, {"esc", "back"}}
	case screenDBUpdate:
		return []tuiKey{{"…", "please wait"}, {"ctrl+c", "quit"}}
	case screenHelp:
		return []tuiKey{{"↑/↓", "scroll"}, {"pgup/pgdn", "page"}, {"esc", "back"}}
	case screenMessage:
		return []tuiKey{{"enter", "continue"}}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared building blocks

func sectionHeading(title string) string {
	return tuiSectionStyle.Render(strings.ToUpper(title))
}

// titleCard renders a two-line summary of a title: name, then kind badge,
// region and ID.
func titleCard(entry wiiudownloader.TitleEntry, width int, extra string) string {
	name := tuiTitleStyle.Render(truncateText(displayName(entry), width))
	kind := "[" + wiiudownloader.GetFormattedKind(entry.TitleID) + "]"
	region := formatRegion(entry.Region)
	tid := fmt.Sprintf("%016x", entry.TitleID)
	// Drop the least important parts first when the line does not fit.
	if extra != "" && lipgloss.Width(kind+"  "+region+"  "+tid+"  ·  "+extra) > width {
		extra = ""
	}
	if lipgloss.Width(kind+"  "+region+"  "+tid) > width {
		region = truncateText(region, atLeast(width-lipgloss.Width(kind+"  "+"  "+tid), 0))
		if region == "" {
			tid = truncateText(tid, atLeast(width-lipgloss.Width(kind)-2, 0))
		}
	}
	meta := tuiBadgeStyle.Render(kind)
	if region != "" {
		meta += "  " + tuiTextStyle.Render(region)
	}
	if tid != "" {
		meta += "  " + tuiDimStyle.Render(tid)
	}
	if extra != "" {
		meta += tuiDimStyle.Render("  ·  " + extra)
	}
	return name + "\n" + meta
}

func focusMarker(focused bool) string {
	if focused {
		return tuiAccentStyle.Render("▸ ")
	}
	return "  "
}

func fieldLine(label, value string, focused bool) string {
	labelStyle := tuiLabelStyle
	bar := tuiSubtleStyle.Render("│ ")
	if focused {
		labelStyle = tuiAccentStyle
		bar = tuiAccentStyle.Render("│ ")
	}
	return focusMarker(focused) + labelStyle.Render(padText(label, tuiLabelWidth)) + " " + bar + value
}

// fieldNote renders a line aligned under a field's value.
func fieldNote(text string) string {
	return strings.Repeat(" ", 2+tuiLabelWidth+3) + text
}

func toggleLine(label string, value, focused, disabled bool, note string) string {
	box := "[ ]"
	if value {
		box = "[x]"
	}
	style := tuiTextStyle
	switch {
	case disabled:
		box = "[-]"
		style = tuiDimStyle
	case focused:
		style = tuiAccentStyle
	}
	line := focusMarker(focused) + style.Render(box+" "+label)
	if note != "" {
		line += "  " + tuiDimStyle.Render(note)
	}
	return line
}

// buttonRow renders buttons on one line. The focused button is inverted,
// marked with ▸ and keeps its brackets so focus is visible without color.
func buttonRow(labels []string, focused int) string {
	parts := make([]string, 0, len(labels))
	for index, label := range labels {
		text := "[ " + label + " ]"
		if index == focused {
			parts = append(parts, tuiAccentStyle.Render("▸")+tuiBrandStyle.Padding(0).Render(text))
		} else {
			parts = append(parts, " "+tuiTextStyle.Render(text))
		}
	}
	return " " + strings.Join(parts, "  ")
}

func errorLine(text string) string   { return tuiErrorStyle.Render("✗ " + text) }
func warningLine(text string) string { return tuiWarningStyle.Render("! " + text) }
func okLine(text string) string      { return tuiSuccessStyle.Render("✓ ") + tuiTextStyle.Render(text) }

// labeledPath shows a destination path after a label. The path wraps onto
// following lines (aligned under itself) so the whole folder name stays
// visible; in compact layouts it is shortened from the left instead.
func labeledPath(label, path string, width int, compact bool) string {
	prefix := label + "  "
	indent := lipgloss.Width(prefix)
	avail := width - indent
	if compact || avail < 16 {
		return tuiLabelStyle.Render(prefix) + tuiTextStyle.Render(truncateLeft(path, atLeast(avail, 8)))
	}
	runes := []rune(path)
	var lines []string
	for len(runes) > 0 && len(lines) < 3 {
		take := 0
		used := 0
		for take < len(runes) {
			w := lipgloss.Width(string(runes[take]))
			if used+w > avail {
				break
			}
			used += w
			take++
		}
		if take == 0 {
			break
		}
		if len(lines) == 2 && take < len(runes) {
			lines = append(lines, truncateLeft(string(runes), avail))
			break
		}
		lines = append(lines, string(runes[:take]))
		runes = runes[take:]
	}
	for index, line := range lines {
		if index == 0 {
			lines[index] = tuiLabelStyle.Render(prefix) + tuiTextStyle.Render(line)
		} else {
			lines[index] = strings.Repeat(" ", indent) + tuiTextStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

// hangingWrap wraps text to width, prefixing the first line with lead and the
// following lines with blank space of the same width.
func hangingWrap(lead, text string, width int, style lipgloss.Style) string {
	leadWidth := lipgloss.Width(lead)
	wrapped := strings.Split(wrapText(text, width-leadWidth), "\n")
	for index, line := range wrapped {
		prefix := strings.Repeat(" ", leadWidth)
		if index == 0 {
			prefix = lead
		}
		wrapped[index] = prefix + style.Render(strings.TrimRight(line, " "))
	}
	return strings.Join(wrapped, "\n")
}

func bullet(text string, width int) string {
	return hangingWrap(tuiDimStyle.Render("  • "), text, width, tuiDimStyle)
}

func wrapText(text string, width int) string {
	if width < 10 {
		width = 10
	}
	return lipgloss.NewStyle().Width(width).Render(text)
}

// listRange describes the visible slice of a list, e.g. "21–40 of 128 ▲▼".
func listRange(model list.Model) string {
	total := len(model.Items())
	if total == 0 {
		return ""
	}
	paginator := model.Paginator
	start, end := paginator.GetSliceBounds(total)
	if start == 0 && end >= total {
		return ""
	}
	text := fmt.Sprintf("%s–%s of %s", formatCount(start+1), formatCount(end), formatCount(total))
	arrows := ""
	if start > 0 {
		arrows += "▲"
	}
	if end < total {
		arrows += "▼"
	}
	return text + " " + arrows
}

// scrollHint describes a viewport's scroll position.
func scrollHint(view viewport.Model) string {
	if view.TotalLineCount() <= view.Height {
		return ""
	}
	switch {
	case view.AtTop():
		return "▼ more below"
	case view.AtBottom():
		return "▲ top · end of content"
	default:
		return fmt.Sprintf("▲▼ %3.0f%%", view.ScrollPercent()*100)
	}
}

// statusLine places left and right text on one line of the given width.
func statusLine(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// ---------------------------------------------------------------------------
// Screens

func (m *tuiModel) screenBody(width int) string {
	switch m.screen {
	case screenHome:
		return m.homeView(width)
	case screenSearch:
		return m.searchView(width)
	case screenActions:
		return m.actionsView(width)
	case screenDownloadOptions:
		return m.downloadOptionsView(width)
	case screenProgress:
		return m.progressView(width)
	case screenInfo:
		return m.infoScreenView(width)
	case screenFiles:
		return m.filesView(width)
	case screenExtractOptions:
		return m.extractOptionsView(width)
	case screenDecrypt:
		return m.decryptView(width)
	case screenTitleID:
		return m.titleIDView(width)
	case screenDBUpdate:
		return m.dbUpdateView(width)
	case screenHelp:
		return m.helpScreenView(width)
	case screenMessage:
		return m.messageView(width)
	}
	return ""
}

func (m *tuiModel) homeView(width int) string {
	var builder strings.Builder
	builder.WriteString(tuiTitleStyle.Render(truncateText("Download, decrypt and explore Wii U titles", width)) + "\n")
	switch {
	case m.db == nil || len(m.db.entries) == 0:
		builder.WriteString(warningLine("The title database is empty. Choose “Update title database”.") + "\n")
	default:
		summary := plural(len(m.db.entries), "title", "titles") + " in the database"
		if !m.db.fetchedAt.IsZero() {
			summary += " · updated " + m.db.fetchedAt.Local().Format("2006-01-02")
		}
		builder.WriteString(tuiDimStyle.Render(summary) + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(m.home.View())
	if m.err != "" {
		builder.WriteString("\n\n" + errorLine(m.err))
	}
	return builder.String()
}

func (m *tuiModel) searchView(width int) string {
	var builder strings.Builder
	builder.WriteString(focusMarker(m.searchFocus == 0))
	label := tuiLabelStyle
	bar := tuiSubtleStyle.Render("│ ")
	if m.searchFocus == 0 {
		label = tuiAccentStyle
		bar = tuiAccentStyle.Render("│ ")
	}
	builder.WriteString(label.Render(padText("Search", 8)) + bar + m.searchInput.View() + "\n")

	region := m.choiceView("Region", tuiRegionChoices[m.regionIndex].label, m.searchFocus == 1)
	kind := m.choiceView("Type", tuiTypeChoices[m.typeIndex].label, m.searchFocus == 2)
	if m.searchControlLines() == 4 {
		builder.WriteString(region + "\n" + kind + "\n")
	} else {
		builder.WriteString(region + "    " + kind + "\n")
	}

	listHeight := m.searchList.Height()
	if len(m.searchResults) > 0 {
		builder.WriteString(tuiLabelStyle.Render(titleListHeader(width)))
	}
	builder.WriteString("\n")
	if len(m.searchResults) == 0 {
		builder.WriteString(padLines(m.searchEmptyView(width), listHeight) + "\n")
	} else {
		builder.WriteString(padLines(m.searchList.View(), listHeight) + "\n")
	}
	builder.WriteString(tuiSubtleStyle.Render(strings.Repeat("─", width)) + "\n")

	var left string
	switch {
	case m.err != "":
		left = errorLine(m.err)
	case m.searchTotal == 0:
		left = tuiDimStyle.Render("No results")
	case m.searchTotal > tuiMaxSearchResults:
		left = warningLine(fmt.Sprintf("Showing the first %s of %s results. Type more to narrow them down.",
			formatCount(tuiMaxSearchResults), formatCount(m.searchTotal)))
	default:
		left = tuiTextStyle.Render(plural(m.searchTotal, "result", "results"))
	}
	builder.WriteString(statusLine(left, tuiDimStyle.Render(listRange(m.searchList)), width))
	return builder.String()
}

// choiceView renders a cycling filter. When focused it shows ◂ ▸ arrows to
// signal that ←/→ change it; otherwise the value sits in brackets.
func (m *tuiModel) choiceView(label, value string, focused bool) string {
	if focused {
		return focusMarker(true) + tuiAccentStyle.Render(padText(label, 7)) + tuiAccentStyle.Render("◂ "+value+" ▸")
	}
	return focusMarker(false) + tuiLabelStyle.Render(padText(label, 7)) + tuiTextStyle.Render("[ "+value+" ]")
}

func (m *tuiModel) searchEmptyView(width int) string {
	if m.db == nil || len(m.db.entries) == 0 {
		return warningLine("The title database is empty.") + "\n" +
			hangingWrap("  ", "Press esc and choose “Update title database” to download it.", width, tuiDimStyle)
	}
	query := strings.TrimSpace(m.searchInput.Value())
	var lines []string
	if query == "" {
		lines = append(lines, tuiTitleStyle.Render("No titles match the current filters."))
	} else {
		lines = append(lines, tuiTitleStyle.Render(truncateText("No titles match “"+query+"”.", width)))
	}
	lines = append(lines, "")
	if query != "" {
		lines = append(lines,
			bullet("Check the spelling, or try fewer or shorter words", width),
			bullet("Paste a 16-digit title ID to look it up directly", width),
		)
	}
	if m.regionIndex != 0 || m.typeIndex != 0 {
		lines = append(lines, bullet(fmt.Sprintf("Filters are active (Region: %s, Type: %s). Press tab to change them",
			tuiRegionChoices[m.regionIndex].label, tuiTypeChoices[m.typeIndex].label), width))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) actionsView(width int) string {
	extra := ""
	if m.selected.Version > 0 {
		extra = fmt.Sprintf("database version %d", m.selected.Version)
	}
	if m.db != nil {
		if _, known := m.db.findByID(m.selected.TitleID); !known {
			extra = "not in the local database"
		}
	}
	return titleCard(m.selected, width, extra) + "\n\n" + m.actionList.View()
}

func (m *tuiModel) downloadOptionsView(width int) string {
	compact := m.compact()
	var lines []string
	lines = append(lines, titleCard(m.selected, width, ""), "")

	if !compact {
		lines = append(lines, sectionHeading("Destination"))
	}
	lines = append(lines, fieldLine("Output folder", m.outputInput.View(), m.downloadFocus == 0))
	output := expandHomePath(m.outputInput.Value())
	if output == "" {
		lines = append(lines, fieldNote(errorLine("Choose an output folder")))
	}
	lines = append(lines, fieldLine("Version", m.versionInput.View(), m.downloadFocus == 1))
	version, versionErr := validateVersionText(m.versionInput.Value())
	switch {
	case versionErr != nil:
		lines = append(lines, fieldNote(errorLine("Use “latest” or a number")))
	case m.downloadFocus == 1:
		lines = append(lines, fieldNote(tuiDimStyle.Render("“latest” or a version number")))
	}

	if compact {
		lines = append(lines, "")
	} else {
		lines = append(lines, "", sectionHeading("Decryption"))
	}
	lines = append(lines, toggleLine("Decrypt contents", m.decrypt, m.downloadFocus == 2, false, "for Cemu or a console"))
	note := ""
	if !m.decrypt {
		note = "requires decrypt"
	}
	lines = append(lines, toggleLine("Delete encrypted files afterwards", m.deleteEncrypted, m.downloadFocus == 3, !m.decrypt, note))
	lines = append(lines, "")

	buttonFocus := -1
	if m.downloadFocus >= 4 {
		buttonFocus = m.downloadFocus - 4
	}
	lines = append(lines, buttonRow([]string{"Start download", "Back"}, buttonFocus))
	if m.err != "" {
		lines = append(lines, "", errorLine(m.err))
	}

	lines = append(lines, "")
	if output != "" && versionErr == nil {
		target := filepath.Join(absolutePath(output), titleDirName(m.selected, versionInt(version, m.selected)))
		lines = append(lines, labeledPath("Saves to", shortenHomePath(target), width, compact))
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			lines = append(lines, tuiDimStyle.Render(wrapText("Folder exists. Existing files are reused and the download resumes.", width)))
		}
	} else {
		lines = append(lines, tuiDimStyle.Render("Saves to —  (fix the fields above)"))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) progressLabel() string {
	switch m.progressOperation {
	case "extract":
		return "Extracting"
	case "decrypt":
		return "Decrypting"
	}
	if m.progressPhase == "decrypt" {
		return "Decrypting"
	}
	return "Downloading"
}

func (m *tuiModel) progressView(width int) string {
	var lines []string
	head := m.spinner.View() + " " + tuiAccentStyle.Render(m.progressLabel()) + "  "
	head += tuiTitleStyle.Render(truncateText(m.progressTitle, width-lipgloss.Width(head)))
	lines = append(lines, head)

	if len(m.progressSteps) > 1 {
		current := 0
		if m.progressPhase == "decrypt" {
			current = len(m.progressSteps) - 1
		}
		parts := make([]string, 0, len(m.progressSteps))
		for index, step := range m.progressSteps {
			switch {
			case index < current:
				parts = append(parts, tuiSuccessStyle.Render("✓ ")+tuiDimStyle.Render(step))
			case index == current:
				parts = append(parts, tuiAccentStyle.Render("▸ "+step))
			default:
				parts = append(parts, tuiDimStyle.Render("○ "+step))
			}
		}
		lines = append(lines, tuiDimStyle.Render(fmt.Sprintf("Step %d of %d  ", current+1, len(m.progressSteps)))+
			strings.Join(parts, tuiSubtleStyle.Render("  ›  ")))
	}
	lines = append(lines, "")

	fraction := 0.0
	var detail string
	waiting := false
	if m.progressPhase == "decrypt" {
		fraction = clampTUI(m.progressDecrypt)
		detail = "Decrypting contents"
		if m.progressOperation == "extract" {
			detail = "Extracting files"
		}
	} else {
		if m.progressTotal > 0 {
			fraction = clampTUI(float64(m.progressBytes) / float64(m.progressTotal))
		} else {
			waiting = true
		}
		parts := []string{formatBytes(m.progressBytes) + " of " + formatBytes(m.progressTotal)}
		if waiting {
			parts = []string{"Preparing… fetching title metadata"}
		}
		if m.progressSpeed > 0 && !waiting {
			parts = append(parts, formatRate(m.progressSpeed))
			if m.progressTotal > m.progressBytes {
				eta := time.Duration(float64(m.progressTotal-m.progressBytes)/m.progressSpeed) * time.Second
				parts = append(parts, "ETA "+formatDuration(eta))
			}
		}
		detail = strings.Join(parts, "  ·  ")
		if lipgloss.Width(detail) > width {
			detail = strings.Join(parts, "\n")
		}
	}
	lines = append(lines, m.progress.ViewAs(fraction)+" "+tuiTitleStyle.Render(fmt.Sprintf("%5.1f%%", fraction*100)))
	for _, part := range strings.Split(detail, "\n") {
		lines = append(lines, hangingWrap("", part, width, tuiTextStyle))
	}
	if !m.progressStart.IsZero() {
		lines = append(lines, tuiDimStyle.Render("Elapsed "+formatDuration(time.Since(m.progressStart))))
	}
	lines = append(lines, "")
	if m.cancelling {
		lines = append(lines, hangingWrap(tuiWarningStyle.Render("! "), "Cancelling… waiting for the current transfer to stop.", width, tuiWarningStyle),
			hangingWrap("  ", "Press ctrl+c to force quit.", width, tuiDimStyle))
	} else {
		hint := "Press esc to cancel."
		if m.progressOperation == "download" {
			hint += " Partial downloads resume next time."
		}
		lines = append(lines, hangingWrap("", hint, width, tuiDimStyle))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) infoScreenView(width int) string {
	var builder strings.Builder
	builder.WriteString(titleCard(m.infoEntry, width, "version "+displayVersion(m.infoVersion)) + "\n\n")
	if m.infoLoading {
		builder.WriteString(m.spinner.View() + " " + tuiTextStyle.Render("Fetching title metadata…") + "\n")
		builder.WriteString(hangingWrap("  ", "Downloading the TMD from Nintendo's servers. Press esc to go back.", width, tuiDimStyle))
		return builder.String()
	}
	if m.infoErr != nil {
		builder.WriteString(errorLine("Could not fetch metadata") + "\n")
		builder.WriteString(tuiDimStyle.Render(truncateText("  "+m.infoErr.Error(), width)) + "\n\n")
	}
	builder.WriteString(padLines(m.infoView.View(), m.infoView.Height) + "\n")
	builder.WriteString(tuiDimStyle.Render(scrollHint(m.infoView)))
	return builder.String()
}

func (m *tuiModel) infoContent() string {
	row := func(label, value string) string {
		return "  " + tuiLabelStyle.Render(padText(label, 18)) + tuiTextStyle.Render(value) + "\n"
	}
	var builder strings.Builder
	builder.WriteString(sectionHeading("Overview") + "\n")
	builder.WriteString(row("Title ID", fmt.Sprintf("%016x", m.infoEntry.TitleID)))
	builder.WriteString(row("Kind", wiiudownloader.GetFormattedKind(m.infoEntry.TitleID)))
	builder.WriteString(row("Region", formatRegion(m.infoEntry.Region)))
	builder.WriteString(row("Category", categoryName(categoryOf(m.infoEntry))))
	if m.infoEntry.Version > 0 {
		builder.WriteString(row("Database version", fmt.Sprintf("%d", m.infoEntry.Version)))
	}
	builder.WriteString(row("Requested", displayVersion(m.infoVersion)))
	if m.infoErr != nil {
		builder.WriteString(row("TMD", "unavailable"))
		return builder.String()
	}
	if m.infoTMD == nil {
		return builder.String()
	}
	builder.WriteString(row("Title version", fmt.Sprintf("%d", m.infoTMD.TitleVersion)))
	builder.WriteString(row("Contents", fmt.Sprintf("%d", len(m.infoTMD.Contents))))
	builder.WriteString(row("Download size", formatBytes(int64(m.infoTMD.CalculateTotalSize()))))

	builder.WriteString("\n" + sectionHeading(fmt.Sprintf("Contents (%d)", len(m.infoTMD.Contents))) + "\n")
	builder.WriteString("  " + tuiLabelStyle.Render(fmt.Sprintf("%-12s %12s   %s", "CONTENT ID", "SIZE", "HASHED")) + "\n")
	for _, content := range m.infoTMD.Contents {
		hashed := "no"
		if content.Type&wiiudownloader.CONTENT_TYPE_HASHED != 0 {
			hashed = "yes"
		}
		builder.WriteString("  " + tuiTextStyle.Render(fmt.Sprintf("%08x     %12s   %s", content.ID, formatBytes(content.DownloadSize()), hashed)) + "\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func (m *tuiModel) filesView(width int) string {
	var builder strings.Builder
	builder.WriteString(titleCard(m.filesEntry, width, "version "+displayVersion(m.filesVersion)) + "\n\n")
	if m.filesLoading {
		builder.WriteString(m.spinner.View() + " " + tuiTextStyle.Render("Fetching the file list…") + "\n")
		builder.WriteString(hangingWrap("  ", "Reading the title's file table from Nintendo's servers. This can take a few seconds. Press esc to go back.", width, tuiDimStyle))
		return builder.String()
	}

	label := tuiLabelStyle
	bar := tuiSubtleStyle.Render("│ ")
	if m.fileFocus == 0 {
		label = tuiAccentStyle
		bar = tuiAccentStyle.Render("│ ")
	}
	builder.WriteString(focusMarker(m.fileFocus == 0) + label.Render(padText("Filter", 8)) + bar + m.fileFilter.View() + "\n")

	listHeight := m.filesList.Height()
	switch {
	case len(m.allFiles) == 0:
		builder.WriteString(padLines("\n"+tuiDimStyle.Render("  This title has no files to show."), listHeight) + "\n")
	case len(m.filteredFiles) == 0:
		empty := "\n" + tuiTitleStyle.Render(truncateText("No files match “"+strings.TrimSpace(m.fileFilter.Value())+"”.", width)) + "\n" +
			hangingWrap("  ", "Press / to edit the filter, or clear it to see all files.", width, tuiDimStyle)
		builder.WriteString(padLines(empty, listHeight) + "\n")
	default:
		builder.WriteString(padLines(m.filesList.View(), listHeight) + "\n")
	}

	var left string
	if m.status != "" {
		left = warningLine(m.status)
	} else {
		total := uint64(0)
		for _, file := range m.filteredFiles {
			if !file.Shared {
				total += file.Size
			}
		}
		count := plural(len(m.filteredFiles), "file", "files")
		if len(m.filteredFiles) != len(m.allFiles) {
			count = fmt.Sprintf("%s of %s files", formatCount(len(m.filteredFiles)), formatCount(len(m.allFiles)))
		}
		left = tuiTextStyle.Render(count) + tuiDimStyle.Render("  ·  "+formatBytes(int64(total)))
	}
	builder.WriteString(statusLine(left, tuiDimStyle.Render(listRange(m.filesList)), width))
	return builder.String()
}

func (m *tuiModel) extractOptionsView(width int) string {
	compact := m.compact()
	sizes := make(map[string]uint64, len(m.allFiles))
	for _, file := range m.allFiles {
		sizes[file.Path] = file.Size
	}
	total := uint64(0)
	for _, path := range m.extractPaths {
		total += sizes[path]
	}

	var lines []string
	lines = append(lines, titleCard(m.filesEntry, width, "version "+displayVersion(m.filesVersion)), "")
	lines = append(lines, sectionHeading(fmt.Sprintf("Extract %s", plural(len(m.extractPaths), "file", "files")))+
		tuiDimStyle.Render("  ·  "+formatBytes(int64(total))))

	preview := 5
	if compact {
		preview = 2
	}
	for index, path := range m.extractPaths {
		if index == preview {
			lines = append(lines, tuiDimStyle.Render(fmt.Sprintf("  … and %s more", formatCount(len(m.extractPaths)-preview))))
			break
		}
		lines = append(lines, tuiDimStyle.Render("  • ")+tuiTextStyle.Render(truncateLeft(path, width-4)))
	}
	lines = append(lines, "")

	lines = append(lines, fieldLine("Output folder", m.extractOutput.View(), m.extractFocus == 0))
	output := expandHomePath(m.extractOutput.Value())
	if output == "" {
		lines = append(lines, fieldNote(errorLine("Choose an output folder")))
	}
	lines = append(lines, "")
	buttonFocus := -1
	if m.extractFocus >= 1 {
		buttonFocus = m.extractFocus - 1
	}
	lines = append(lines, buttonRow([]string{"Start extraction", "Back"}, buttonFocus))
	if m.err != "" {
		lines = append(lines, "", errorLine(m.err))
	}
	if output != "" {
		target := filepath.Join(absolutePath(output), titleDirName(m.filesEntry, m.filesVersion))
		lines = append(lines, "", labeledPath("Files go to", shortenHomePath(target), width, compact))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) decryptView(width int) string {
	var lines []string
	lines = append(lines, tuiTextStyle.Render(wrapText("Decrypt a folder that contains title.tmd, title.tik and .app files, for example one downloaded with decryption turned off.", width)), "")
	lines = append(lines, fieldLine("Folder", m.decryptInput.View(), m.decryptFocus == 0))
	if note := decryptFolderNote(expandHomePath(m.decryptInput.Value())); note != "" {
		lines = append(lines, fieldNote(note))
	}
	lines = append(lines, toggleLine("Delete encrypted .app/.h3 files afterwards", m.decryptDelete, m.decryptFocus == 1, false, ""))
	lines = append(lines, "")
	buttonFocus := -1
	if m.decryptFocus >= 2 {
		buttonFocus = m.decryptFocus - 2
	}
	lines = append(lines, buttonRow([]string{"Start decryption", "Back"}, buttonFocus))
	if m.err != "" {
		lines = append(lines, "", errorLine(m.err))
	}
	return strings.Join(lines, "\n")
}

// decryptFolderNote gives live feedback about the folder being typed.
func decryptFolderNote(path string) string {
	if path == "" {
		return tuiDimStyle.Render("Type or paste a folder path (~ is expanded)")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return errorLine("Folder not found")
	}
	_, tmdErr := os.Stat(filepath.Join(path, "title.tmd"))
	_, tikErr := os.Stat(filepath.Join(path, "title.tik"))
	switch {
	case tmdErr == nil && tikErr == nil:
		return okLine("title.tmd and title.tik found")
	case tmdErr != nil:
		return warningLine("No title.tmd in this folder")
	default:
		return warningLine("No title.tik in this folder")
	}
}

func (m *tuiModel) titleIDView(width int) string {
	var lines []string
	lines = append(lines, tuiTextStyle.Render(wrapText("Enter a 16-digit hexadecimal title ID. To find a title by name, go back and use Search.", width)), "")
	lines = append(lines, fieldLine("Title ID", m.titleIDInput.View(), true))
	lines = append(lines, fieldNote(m.titleIDNote(width-(2+tuiLabelWidth+3))))
	if m.err != "" {
		lines = append(lines, "", errorLine(m.err))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) titleIDNote(width int) string {
	raw := strings.TrimSpace(m.titleIDInput.Value())
	if raw == "" {
		return tuiDimStyle.Render(truncateText("Example: 00050000101c9500 (dashes and 0x are fine)", width))
	}
	if id, ok := parseTitleID(raw); ok {
		if m.db == nil {
			return okLine("Valid title ID")
		}
		if entry, found := m.db.findByID(id); found {
			return okLine(truncateText(fmt.Sprintf("%s · %s · %s", displayName(entry),
				wiiudownloader.GetFormattedKind(id), formatRegion(entry.Region)), width-2))
		}
		return warningLine(truncateText("Valid ID, but not in the local database. It will be tried anyway.", width-2))
	}
	digits := strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")
	digits = strings.ReplaceAll(digits, "-", "")
	for _, r := range digits {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return errorLine("Only 0–9 and a–f are allowed")
		}
	}
	return tuiDimStyle.Render(fmt.Sprintf("%d of 16 digits", len(digits)))
}

func (m *tuiModel) dbUpdateView(width int) string {
	if !m.dbUpdating {
		return tuiTextStyle.Render("Starting database update…")
	}
	lines := []string{m.spinner.View() + " " + tuiTextStyle.Render(m.dbStatus), ""}
	if m.db != nil {
		lines = append(lines, tuiDimStyle.Render("Current database: "+plural(len(m.db.entries), "title", "titles")))
	}
	lines = append(lines, tuiDimStyle.Render(wrapText("The new list is cached and used automatically next time.", width)))
	return strings.Join(lines, "\n")
}

func (m *tuiModel) helpScreenView(width int) string {
	return padLines(m.helpView.View(), m.helpView.Height) + "\n" + tuiDimStyle.Render(scrollHint(m.helpView))
}

func (m *tuiModel) messageView(width int) string {
	icon, color := "✓", tuiColorSuccess
	switch m.messageKind {
	case "error":
		icon, color = "✗", tuiColorError
	case "cancel":
		icon, color = "■", tuiColorWarning
	}
	panelWidth := width
	if panelWidth > 78 {
		panelWidth = 78
	}
	bordered := width >= 40
	inner := panelWidth
	if bordered {
		inner = panelWidth - 4
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(color).Render(icon + " " + m.messageTitle)
	body := tuiTextStyle.Render(wrapText(m.messageBody, inner))
	hint := m.messageHint
	if hint == "" {
		hint = "Press enter to return to " + tuiScreenName(m.messageReturn) + "."
	}
	content := title + "\n\n" + body + "\n\n" + tuiDimStyle.Render(wrapText(hint, inner))
	if !bordered {
		return content
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(panelWidth - 2).
		Render(content)
}

func tuiScreenName(screen tuiScreen) string {
	switch screen {
	case screenHome:
		return "the main menu"
	case screenSearch:
		return "the search results"
	case screenActions:
		return "the title actions"
	case screenFiles:
		return "the file list"
	default:
		return "the previous screen"
	}
}
