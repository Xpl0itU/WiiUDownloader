package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tuiScreen int

const (
	screenHome tuiScreen = iota
	screenSearch
	screenActions
	screenDownloadOptions
	screenProgress
	screenInfo
	screenFiles
	screenExtractOptions
	screenDecrypt
	screenTitleID
	screenDBUpdate
	screenHelp
	screenMessage
)

const tuiMaxSearchResults = 500

type tuiDownloadDoneMsg struct {
	err    error
	target string
}

type tuiExtractDoneMsg struct {
	err   error
	paths []string
}

type tuiDecryptDoneMsg struct {
	err  error
	path string
}

type tuiInfoLoadedMsg struct {
	tmd *wiiudownloader.TMD
	err error
}

type tuiFilesLoadedMsg struct {
	tree *wiiudownloader.TitleFileTree
	err  error
}

type tuiDBUpdatedMsg struct {
	entries []wiiudownloader.TitleEntry
	source  string
	warning string
	err     error
}

type tuiModel struct {
	app *app
	db  *titleDB

	width, height int
	screen        tuiScreen
	spinner       spinner.Model
	err           string
	status        string

	// Home
	home list.Model

	// Search
	searchInput   textinput.Model
	searchFocus   int
	regionIndex   int
	typeIndex     int
	searchList    list.Model
	searchResults []wiiudownloader.TitleEntry
	searchTotal   int

	// Selected title / actions
	selected   wiiudownloader.TitleEntry
	actionList list.Model

	// Download options
	outputInput     textinput.Model
	versionInput    textinput.Model
	downloadFocus   int
	decrypt         bool
	deleteEncrypted bool

	// Active transfer
	progress          progress.Model
	progressCh        chan tuiProgressMsg
	reporter          *tuiProgress
	progressTitle     string
	progressOperation string
	progressStart     time.Time
	progressBytes     int64
	progressTotal     int64
	progressDecrypt   float64
	progressPhase     string
	progressSpeed     float64
	progressLastBytes int64
	progressLastTime  time.Time
	cancelling        bool

	// Info
	infoView    viewport.Model
	infoLoading bool
	infoEntry   wiiudownloader.TitleEntry
	infoVersion int
	infoTMD     *wiiudownloader.TMD
	infoErr     error

	// Files
	filesLoading  bool
	tree          *wiiudownloader.TitleFileTree
	allFiles      []wiiudownloader.TitleFile
	filteredFiles []wiiudownloader.TitleFile
	filesEntry    wiiudownloader.TitleEntry
	filesVersion  int
	fileFilter    textinput.Model
	fileFocus     int
	filesList     list.Model

	// Extract options
	extractOutput textinput.Model
	extractFocus  int
	extractPaths  []string

	// Decrypt
	decryptInput  textinput.Model
	decryptDelete bool
	decryptFocus  int

	// Download by ID
	titleIDInput textinput.Model

	// Database update
	dbUpdating bool
	dbStatus   string

	// Help and message screens
	helpView      viewport.Model
	messageTitle  string
	messageBody   string
	messageReturn tuiScreen
	messageKind   string
}

func newTUIModel(application *app, db *titleDB) *tuiModel {
	model := &tuiModel{
		app:             application,
		db:              db,
		width:           100,
		height:          30,
		screen:          screenHome,
		decrypt:         true,
		deleteEncrypted: false,
		regionIndex:     0,
		typeIndex:       0,
	}

	model.spinner = spinner.New()
	model.spinner.Spinner = spinner.Dot
	model.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	model.progress = progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())
	model.progress.Width = 60

	model.home = newTUIList(tuiHomeItems(), 80, 18)
	model.searchInput = newTUIInput("Search by name or title ID…", 50)
	model.searchList = newTUIList(nil, 80, 12)
	model.actionList = newTUIList(tuiActionItems(), 80, 12)
	model.outputInput = newTUIInput("Download folder", 58)
	model.outputInput.SetValue(defaultDownloadDir())
	model.versionInput = newTUIInput("latest", 16)
	model.versionInput.SetValue("latest")
	model.infoView = viewport.New(80, 18)
	model.helpView = viewport.New(80, 18)
	model.helpView.SetContent(tuiHelpText())
	model.fileFilter = newTUIInput("type to filter…", 42)
	model.filesList = newTUIList(nil, 80, 12)
	model.extractOutput = newTUIInput("Output folder", 58)
	model.extractOutput.SetValue(defaultDownloadDir())
	model.decryptInput = newTUIInput("~/Downloads/Game [Game] [00050000101c9500]", 58)
	model.titleIDInput = newTUIInput("00050000101c9500", 24)
	model.setSearchFocus(0)
	model.setDownloadFocus(0)
	model.setExtractFocus(0)
	model.setDecryptFocus(0)
	model.titleIDInput.Focus()
	model.blurAllInputs()
	return model
}

func newTUIInput(placeholder string, width int) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.CharLimit = 1024
	input.Width = width
	input.Prompt = ""
	return input
}

func cmdTUI(application *app, args []string) error {
	fs := newFlagSet(application, "tui", "Start the full-screen interactive interface.", `Usage:
  wiiudl tui

The TUI requires an interactive terminal. It provides guided search,
downloads, metadata and file browsing, extraction, decryption and
title database updates.

Flags:`)
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) > 0 {
		fs.Usage()
		return silentExit{code: 2}
	}
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return fmt.Errorf("the TUI requires an interactive terminal (use the regular commands when piping input or output)")
	}

	// Keep the alternate screen clean: warnings from the database load happen
	// before the program starts, and core log output is suppressed below.
	if !application.quiet {
		fmt.Fprintln(application.stderr, "Loading title database…")
	}
	db, err := application.loadDB()
	if err != nil {
		return err
	}

	oldLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogWriter)

	model := newTUIModel(application, db)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func (m *tuiModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		if !m.loading() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case cursor.BlinkMsg:
		return m, m.updateFocusedInput(msg)
	case tuiProgressMsg:
		if m.screen != screenProgress {
			return m, nil
		}
		m.applyProgress(msg)
		return m, waitForTUIProgress(m.progressCh)
	case tuiInfoLoadedMsg:
		m.infoLoading = false
		if m.screen != screenInfo {
			return m, nil
		}
		m.infoTMD = msg.tmd
		m.infoErr = msg.err
		m.infoView.SetContent(m.infoContent())
		m.infoView.GotoTop()
		return m, nil
	case tuiFilesLoadedMsg:
		m.filesLoading = false
		if m.screen != screenFiles {
			if msg.tree != nil {
				_ = msg.tree.Close()
			}
			return m, nil
		}
		if msg.err != nil {
			m.showMessage("Could not load files", msg.err.Error(), screenActions, "error")
			return m, nil
		}
		m.tree = msg.tree
		m.allFiles = nil
		if msg.tree != nil {
			m.allFiles = msg.tree.Files
		}
		m.rebuildFilesList()
		m.setFileFocus(1)
		return m, nil
	case tuiDownloadDoneMsg:
		m.finishProgress()
		if msg.err != nil {
			m.showMessage("Download failed", msg.err.Error(), screenSearch, "error")
		} else if m.reporter != nil && m.reporter.Cancelled() {
			m.showMessage("Download cancelled", "The partial download was kept and will resume next time.", screenSearch, "cancel")
		} else {
			m.showMessage("Download complete", "Saved to:\n"+msg.target, screenSearch, "success")
		}
		m.reporter = nil
		m.progressCh = nil
		return m, nil
	case tuiExtractDoneMsg:
		m.finishProgress()
		if msg.err != nil {
			m.showMessage("Extraction failed", msg.err.Error(), screenFiles, "error")
		} else if m.reporter != nil && m.reporter.Cancelled() {
			m.showMessage("Extraction cancelled", "No further files were extracted.", screenFiles, "cancel")
		} else {
			m.showMessage("Extraction complete", fmt.Sprintf("Extracted %d file(s).", len(msg.paths)), screenFiles, "success")
		}
		m.reporter = nil
		m.progressCh = nil
		return m, nil
	case tuiDecryptDoneMsg:
		m.finishProgress()
		if msg.err != nil {
			m.showMessage("Decryption failed", msg.err.Error(), screenHome, "error")
		} else if m.reporter != nil && m.reporter.Cancelled() {
			m.showMessage("Decryption cancelled", "The operation was interrupted.", screenHome, "cancel")
		} else {
			m.showMessage("Decryption complete", "Decrypted files are ready in:\n"+msg.path, screenHome, "success")
		}
		m.reporter = nil
		m.progressCh = nil
		return m, nil
	case tuiDBUpdatedMsg:
		m.dbUpdating = false
		if msg.err != nil {
			m.showMessage("Database update failed", msg.err.Error(), screenHome, "error")
			return m, nil
		}
		newDB := newTitleDB(msg.entries, "cache")
		newDB.setGlobal()
		m.db = newDB
		m.app.db = newDB
		body := fmt.Sprintf("Loaded %d titles from:\n%s", len(newDB.entries), msg.source)
		if msg.warning != "" {
			body += "\n\n" + msg.warning
		}
		m.showMessage("Title database updated", body, screenHome, "success")
		return m, nil
	}

	return m, m.updateFocusedInput(msg)
}

func (m *tuiModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	title, body := m.screenView()
	header := tuiTitleStyle.Render(" WiiUDownloader ")
	header += " " + tuiDimStyle.Render(title)
	if m.db != nil {
		header += tuiDimStyle.Render(fmt.Sprintf("  ·  %d titles", len(m.db.entries)))
	}
	footer := tuiHelpStyle.Render(m.footer())
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
}

func (m *tuiModel) resize() {
	width := m.width - 4
	if width < 30 {
		width = 30
	}
	height := m.height - 8
	if height < 6 {
		height = 6
	}

	m.home.SetSize(width, height)
	m.actionList.SetSize(width, height)
	m.searchList.SetSize(width, height-3)
	if m.searchList.Height() < 4 {
		m.searchList.SetSize(width, 4)
	}
	m.filesList.SetSize(width, height-4)
	if m.filesList.Height() < 4 {
		m.filesList.SetSize(width, 4)
	}
	m.infoView.Width = width
	m.infoView.Height = height
	m.helpView.Width = width
	m.helpView.Height = height

	m.progress.Width = width - 12
	if m.progress.Width > 56 {
		m.progress.Width = 56
	}
	if m.progress.Width < 20 {
		m.progress.Width = 20
	}

	for _, input := range []*textinput.Model{
		&m.searchInput, &m.outputInput, &m.versionInput, &m.fileFilter,
		&m.extractOutput, &m.decryptInput, &m.titleIDInput,
	} {
		input.Width = width - 16
		if input.Width < 16 {
			input.Width = 16
		}
	}
}

func (m *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.screen == screenProgress && m.reporter != nil {
			if m.cancelling {
				return m, tea.Quit
			}
			return m, m.cancelCurrent()
		}
		return m, tea.Quit
	}

	switch m.screen {
	case screenHome:
		return m.handleHomeKey(msg)
	case screenSearch:
		return m.handleSearchKey(msg)
	case screenActions:
		return m.handleActionsKey(msg)
	case screenDownloadOptions:
		return m.handleDownloadOptionsKey(msg)
	case screenProgress:
		return m.handleProgressKey(msg)
	case screenInfo:
		return m.handleInfoKey(msg)
	case screenFiles:
		return m.handleFilesKey(msg)
	case screenExtractOptions:
		return m.handleExtractOptionsKey(msg)
	case screenDecrypt:
		return m.handleDecryptKey(msg)
	case screenTitleID:
		return m.handleTitleIDKey(msg)
	case screenDBUpdate:
		return m, nil
	case screenHelp:
		return m.handleHelpKey(msg)
	case screenMessage:
		return m.handleMessageKey(msg)
	default:
		return m, nil
	}
}

func (m *tuiModel) handleHomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "enter":
		item, ok := m.home.SelectedItem().(tuiMenuItem)
		if !ok {
			return m, nil
		}
		switch item.action {
		case "quit":
			return m, tea.Quit
		case "db-update":
			return m, m.startDBUpdate()
		}
		switch item.screen {
		case screenSearch:
			return m, m.openSearch()
		case screenTitleID:
			return m, m.openTitleID()
		case screenDecrypt:
			return m, m.openDecrypt()
		case screenHelp:
			m.screen = screenHelp
			m.helpView.GotoTop()
			return m, nil
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		return m, cmd
	}
}

func (m *tuiModel) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.blurAllInputs()
		m.screen = screenHome
		return m, nil
	case "tab":
		m.setSearchFocus((m.searchFocus + 1) % 4)
		return m, textinput.Blink
	case "shift+tab":
		m.setSearchFocus((m.searchFocus + 3) % 4)
		return m, textinput.Blink
	}

	switch m.searchFocus {
	case 0:
		switch key {
		case "down":
			m.setSearchFocus(3)
			return m, nil
		case "enter":
			if entry, ok := m.currentSearchEntry(); ok {
				return m.openActions(entry)
			}
			return m, nil
		}
		cmd := m.updateFocusedInput(msg)
		return m, tea.Batch(cmd, m.refreshSearch())
	case 1:
		if key == "left" || key == "right" || key == " " || key == "enter" {
			m.cycleRegion(key != "left")
			return m, m.refreshSearch()
		}
	case 2:
		if key == "left" || key == "right" || key == " " || key == "enter" {
			m.cycleType(key != "left")
			return m, m.refreshSearch()
		}
	case 3:
		if entry, ok := m.currentSearchEntry(); ok {
			switch key {
			case "enter":
				return m.openActions(entry)
			case "d":
				return m.openDownloadOptions(entry)
			case "i":
				return m.startInfo(entry)
			case "f":
				return m.startFiles(entry)
			}
		}
		var cmd tea.Cmd
		m.searchList, cmd = m.searchList.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *tuiModel) handleActionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, m.returnToSearch()
	case "enter":
		item, ok := m.actionList.SelectedItem().(tuiActionItem)
		if !ok {
			return m, nil
		}
		switch item.action {
		case "download":
			return m.openDownloadOptions(m.selected)
		case "info":
			return m.startInfo(m.selected)
		case "files":
			return m.startFiles(m.selected)
		case "back":
			return m, m.returnToSearch()
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.actionList, cmd = m.actionList.Update(msg)
		return m, cmd
	}
}

func (m *tuiModel) handleDownloadOptionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		return m, m.openActionsCmd(m.selected)
	case "up", "shift+tab":
		m.setDownloadFocus((m.downloadFocus + 5) % 6)
		return m, textinput.Blink
	case "down", "tab":
		m.setDownloadFocus((m.downloadFocus + 1) % 6)
		return m, textinput.Blink
	}

	switch m.downloadFocus {
	case 0, 1:
		if key == "enter" {
			m.setDownloadFocus((m.downloadFocus + 1) % 6)
			return m, textinput.Blink
		}
		return m, m.updateFocusedInput(msg)
	case 2:
		if key == " " || key == "left" || key == "right" || key == "enter" {
			m.decrypt = !m.decrypt
			if !m.decrypt {
				m.deleteEncrypted = false
			}
		}
		return m, nil
	case 3:
		if key == " " || key == "left" || key == "right" || key == "enter" {
			if m.decrypt {
				m.deleteEncrypted = !m.deleteEncrypted
			}
		}
		return m, nil
	case 4:
		if key == "enter" || key == " " {
			return m, m.beginDownload()
		}
		return m, nil
	case 5:
		if key == "enter" || key == " " {
			return m, m.openActionsCmd(m.selected)
		}
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) handleProgressKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m, m.cancelCurrent()
	}
	return m, nil
}

func (m *tuiModel) handleInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m, m.openActionsCmd(m.selected)
	}
	var cmd tea.Cmd
	m.infoView, cmd = m.infoView.Update(msg)
	return m, cmd
}

func (m *tuiModel) handleFilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.closeTree()
		return m, m.openActionsCmd(m.selected)
	case "tab":
		m.setFileFocus((m.fileFocus + 1) % 2)
		return m, textinput.Blink
	case "/":
		m.setFileFocus(0)
		return m, textinput.Blink
	case "enter":
		if m.fileFocus == 0 {
			m.setFileFocus(1)
			return m, textinput.Blink
		}
		item, ok := m.filesList.SelectedItem().(tuiFileItem)
		if !ok {
			return m, nil
		}
		if item.file.Shared {
			m.status = "Shared entries have no extractable payload."
			return m, nil
		}
		return m.openExtractOptions([]string{item.file.Path}), nil
	case "a":
		if m.fileFocus == 1 {
			paths := make([]string, 0, len(m.filteredFiles))
			for _, file := range m.filteredFiles {
				if !file.Shared {
					paths = append(paths, file.Path)
				}
			}
			if len(paths) == 0 {
				m.status = "No extractable files in the current filter."
				return m, nil
			}
			return m.openExtractOptions(paths), nil
		}
	}

	if m.fileFocus == 0 {
		if key == "down" {
			m.setFileFocus(1)
			return m, nil
		}
		cmd := m.updateFocusedInput(msg)
		return m, tea.Batch(cmd, m.rebuildFilesListCmd())
	}

	var cmd tea.Cmd
	m.filesList, cmd = m.filesList.Update(msg)
	return m, cmd
}

func (m *tuiModel) handleExtractOptionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		return m, m.backToFiles()
	case "up", "shift+tab":
		m.setExtractFocus((m.extractFocus + 2) % 3)
		return m, textinput.Blink
	case "down", "tab":
		m.setExtractFocus((m.extractFocus + 1) % 3)
		return m, textinput.Blink
	}

	switch m.extractFocus {
	case 0:
		if key == "enter" {
			m.setExtractFocus(1)
			return m, textinput.Blink
		}
		return m, m.updateFocusedInput(msg)
	case 1:
		if key == "enter" || key == " " {
			return m, m.beginExtract()
		}
	case 2:
		if key == "enter" || key == " " {
			return m, m.backToFiles()
		}
	}
	return m, nil
}

func (m *tuiModel) handleDecryptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		return m, m.openHome()
	case "up", "shift+tab":
		m.setDecryptFocus((m.decryptFocus + 3) % 4)
		return m, textinput.Blink
	case "down", "tab":
		m.setDecryptFocus((m.decryptFocus + 1) % 4)
		return m, textinput.Blink
	}

	switch m.decryptFocus {
	case 0:
		if key == "enter" {
			m.setDecryptFocus(1)
			return m, textinput.Blink
		}
		return m, m.updateFocusedInput(msg)
	case 1:
		if key == " " || key == "left" || key == "right" || key == "enter" {
			m.decryptDelete = !m.decryptDelete
		}
	case 2:
		if key == "enter" || key == " " {
			return m, m.beginDecrypt()
		}
	case 3:
		if key == "enter" || key == " " {
			return m, m.openHome()
		}
	}
	return m, nil
}

func (m *tuiModel) handleTitleIDKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, m.openHome()
	case "enter":
		id, ok := parseTitleID(m.titleIDInput.Value())
		if !ok {
			m.err = "Enter a 16-digit hexadecimal title ID (for example 00050000101c9500)."
			return m, nil
		}
		entry, found := m.db.findByID(id)
		if !found {
			entry = wiiudownloader.TitleEntry{TitleID: id}
		}
		return m.openActions(entry)
	default:
		m.err = ""
		return m, m.updateFocusedInput(msg)
	}
}

func (m *tuiModel) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		return m, m.openHome()
	}
	var cmd tea.Cmd
	m.helpView, cmd = m.helpView.Update(msg)
	return m, cmd
}

func (m *tuiModel) handleMessageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc", " ":
		return m.returnFromMessage()
	}
	return m, nil
}

func (m *tuiModel) screenView() (string, string) {
	switch m.screen {
	case screenHome:
		body := m.home.View()
		if m.err != "" {
			body += "\n" + tuiErrorStyle.Render(m.err)
		}
		return "Main menu", body
	case screenSearch:
		return "Search", m.searchView()
	case screenActions:
		header := fmt.Sprintf("Selected: %s", displayName(m.selected))
		header += "\n" + tuiDimStyle.Render(fmt.Sprintf("%016x · %s · %s",
			m.selected.TitleID,
			wiiudownloader.GetFormattedKind(m.selected.TitleID),
			formatRegion(m.selected.Region),
		))
		return "Actions", header + "\n\n" + m.actionList.View()
	case screenDownloadOptions:
		return "Download", m.downloadOptionsView()
	case screenProgress:
		return "Transfer", m.progressView()
	case screenInfo:
		return "Info", m.infoView2()
	case screenFiles:
		return "Files", m.filesView()
	case screenExtractOptions:
		return "Extract files", m.extractOptionsView()
	case screenDecrypt:
		return "Decrypt", m.decryptView()
	case screenTitleID:
		return "Download by title ID", m.titleIDView()
	case screenDBUpdate:
		return "Title database", m.dbUpdateView()
	case screenHelp:
		return "Help", m.helpView.View()
	case screenMessage:
		return "Done", m.messageView()
	default:
		return "", ""
	}
}

func (m *tuiModel) searchView() string {
	searchLabel := tuiLabelStyle.Render("Search ")
	if m.searchFocus == 0 {
		searchLabel = tuiAccentStyle.Render("Search ")
	}
	regionLabel := tuiLabelStyle.Render("Region ")
	if m.searchFocus == 1 {
		regionLabel = tuiAccentStyle.Render("Region ")
	}
	typeLabel := tuiLabelStyle.Render("Type ")
	if m.searchFocus == 2 {
		typeLabel = tuiAccentStyle.Render("Type ")
	}

	line1 := searchLabel + m.searchInput.View()
	line2 := regionLabel + tuiAccentStyle.Render("["+tuiRegionChoices[m.regionIndex].label+"]") +
		"   " + typeLabel + tuiAccentStyle.Render("["+tuiTypeChoices[m.typeIndex].label+"]")

	status := fmt.Sprintf("%d results", m.searchTotal)
	if m.searchTotal > tuiMaxSearchResults {
		status = fmt.Sprintf("showing first %d of %d results — refine the search", tuiMaxSearchResults, m.searchTotal)
	}
	if m.searchTotal == 0 {
		status = "no results"
	}
	if m.err != "" {
		status = tuiErrorStyle.Render(m.err)
	}
	return line1 + "\n" + line2 + "\n" + m.searchList.View() + "\n" + tuiDimStyle.Render(status)
}

func (m *tuiModel) downloadOptionsView() string {
	var builder strings.Builder
	builder.WriteString(tuiAccentStyle.Render("Download ") + displayName(m.selected) + "\n")
	builder.WriteString(tuiDimStyle.Render(fmt.Sprintf("%016x · %s · %s", m.selected.TitleID,
		wiiudownloader.GetFormattedKind(m.selected.TitleID), formatRegion(m.selected.Region))) + "\n\n")

	builder.WriteString(m.fieldLine("Output folder", m.outputInput.View(), m.downloadFocus == 0))
	builder.WriteString(m.fieldLine("Version", m.versionInput.View(), m.downloadFocus == 1))
	builder.WriteString(m.toggleLine("Decrypt contents", m.decrypt, m.downloadFocus == 2))
	builder.WriteString(m.toggleLine("Delete encrypted files after decryption", m.deleteEncrypted, m.downloadFocus == 3))
	if !m.decrypt {
		builder.WriteString(tuiDimStyle.Render("  (delete encrypted requires decrypt)") + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(m.actionLine("Start download", m.downloadFocus == 4))
	builder.WriteString(m.actionLine("Back", m.downloadFocus == 5))
	if m.err != "" {
		builder.WriteString("\n" + tuiErrorStyle.Render(m.err) + "\n")
	}

	version, err := validateVersionText(m.versionInput.Value())
	if err == nil {
		dir := titleDirName(m.selected, versionInt(version, m.selected))
		builder.WriteString("\n" + tuiDimStyle.Render("Destination: "+filepath.Join(expandHomePath(m.outputInput.Value()), dir)))
	}
	return builder.String()
}

func (m *tuiModel) progressView() string {
	var builder strings.Builder
	label := "Downloading"
	switch m.progressOperation {
	case "extract":
		label = "Extracting"
	case "decrypt":
		label = "Decrypting"
	}
	if m.progressPhase == "decrypt" {
		label = "Decrypting"
	}
	builder.WriteString(tuiAccentStyle.Render(label+": ") + m.progressTitle + "\n")
	if !m.progressStart.IsZero() {
		builder.WriteString(tuiDimStyle.Render("Elapsed "+formatDuration(time.Since(m.progressStart))) + "\n")
	}
	builder.WriteString("\n")

	fraction := 0.0
	if m.progressPhase == "decrypt" {
		fraction = m.progressDecrypt
		builder.WriteString(m.progress.ViewAs(fraction) + "\n")
		builder.WriteString(fmt.Sprintf("Decrypting  %5.1f%%\n", fraction*100))
	} else {
		if m.progressTotal > 0 {
			fraction = float64(m.progressBytes) / float64(m.progressTotal)
		}
		builder.WriteString(m.progress.ViewAs(clampTUI(fraction)) + "\n")
		builder.WriteString(fmt.Sprintf("%s / %s", formatBytes(m.progressBytes), formatBytes(m.progressTotal)))
		if m.progressSpeed > 0 {
			builder.WriteString("   " + formatRate(m.progressSpeed))
			if m.progressTotal > m.progressBytes {
				eta := time.Duration(float64(m.progressTotal-m.progressBytes)/m.progressSpeed) * time.Second
				builder.WriteString("   ETA " + formatDuration(eta))
			}
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	if m.cancelling {
		builder.WriteString(tuiErrorStyle.Render("Cancelling…") + "\n")
	} else {
		builder.WriteString(tuiDimStyle.Render("Esc cancels the current transfer. Partial downloads resume later.") + "\n")
	}
	return builder.String()
}

func (m *tuiModel) infoView2() string {
	var builder strings.Builder
	builder.WriteString(tuiAccentStyle.Render(displayName(m.infoEntry)) + "\n")
	builder.WriteString(tuiDimStyle.Render(fmt.Sprintf("%016x · %s · %s",
		m.infoEntry.TitleID,
		wiiudownloader.GetFormattedKind(m.infoEntry.TitleID),
		formatRegion(m.infoEntry.Region),
	)) + "\n\n")
	if m.infoLoading {
		builder.WriteString(m.spinner.View() + " Fetching title metadata…")
		return builder.String()
	}
	if m.infoErr != nil {
		builder.WriteString(tuiErrorStyle.Render("Could not fetch metadata: "+m.infoErr.Error()) + "\n\n")
	}
	builder.WriteString(m.infoView.View())
	return builder.String()
}

func (m *tuiModel) infoContent() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Title ID:        %016x\n", m.infoEntry.TitleID)
	fmt.Fprintf(&builder, "Kind:            %s\n", wiiudownloader.GetFormattedKind(m.infoEntry.TitleID))
	fmt.Fprintf(&builder, "Region:          %s\n", formatRegion(m.infoEntry.Region))
	fmt.Fprintf(&builder, "Category:        %s\n", categoryName(categoryOf(m.infoEntry)))
	if m.infoEntry.Version > 0 {
		fmt.Fprintf(&builder, "Database version: %d\n", m.infoEntry.Version)
	}
	fmt.Fprintf(&builder, "Requested:       %s\n", displayVersion(m.infoVersion))
	if m.infoErr != nil {
		fmt.Fprintf(&builder, "TMD:             unavailable (%v)\n", m.infoErr)
		return builder.String()
	}
	if m.infoTMD == nil {
		return builder.String()
	}
	fmt.Fprintf(&builder, "Title version:   %d\n", m.infoTMD.TitleVersion)
	fmt.Fprintf(&builder, "Contents:        %d\n", len(m.infoTMD.Contents))
	fmt.Fprintf(&builder, "Download size:   %s\n", formatBytes(int64(m.infoTMD.CalculateTotalSize())))
	builder.WriteString("\nCONTENT ID   SIZE         HASHED\n")
	for _, content := range m.infoTMD.Contents {
		hashed := "no"
		if content.Type&wiiudownloader.CONTENT_TYPE_HASHED != 0 {
			hashed = "yes"
		}
		fmt.Fprintf(&builder, "%08x     %-12s %s\n", content.ID, formatBytes(content.DownloadSize()), hashed)
	}
	return builder.String()
}

func (m *tuiModel) filesView() string {
	var builder strings.Builder
	builder.WriteString(tuiAccentStyle.Render(displayName(m.filesEntry)) + "\n")
	builder.WriteString(tuiDimStyle.Render(fmt.Sprintf("%016x · version %s", m.filesEntry.TitleID, displayVersion(m.filesVersion))) + "\n\n")
	if m.filesLoading {
		builder.WriteString(m.spinner.View() + " Fetching file list…")
		return builder.String()
	}
	filterLabel := tuiLabelStyle.Render("Filter ")
	if m.fileFocus == 0 {
		filterLabel = tuiAccentStyle.Render("Filter ")
	}
	builder.WriteString(filterLabel + m.fileFilter.View() + "\n")
	builder.WriteString(m.filesList.View() + "\n")
	status := fmt.Sprintf("%d file(s)", len(m.filteredFiles))
	if m.status != "" {
		status = m.status
	}
	builder.WriteString(tuiDimStyle.Render(status))
	return builder.String()
}

func (m *tuiModel) extractOptionsView() string {
	var builder strings.Builder
	total := uint64(0)
	for _, path := range m.extractPaths {
		for _, file := range m.allFiles {
			if file.Path == path {
				total += file.Size
				break
			}
		}
	}
	builder.WriteString(tuiAccentStyle.Render(fmt.Sprintf("Extract %d file(s)", len(m.extractPaths))) + "\n")
	builder.WriteString(tuiDimStyle.Render(fmt.Sprintf("About %s", formatBytes(int64(total)))) + "\n\n")
	builder.WriteString(m.fieldLine("Output folder", m.extractOutput.View(), m.extractFocus == 0))
	builder.WriteString("\n")
	builder.WriteString(m.actionLine("Start extraction", m.extractFocus == 1))
	builder.WriteString(m.actionLine("Back", m.extractFocus == 2))
	if m.err != "" {
		builder.WriteString("\n" + tuiErrorStyle.Render(m.err) + "\n")
	}
	return builder.String()
}

func (m *tuiModel) decryptView() string {
	var builder strings.Builder
	builder.WriteString(tuiDimStyle.Render("Decrypt a folder that contains title.tmd, title.tik and .app files.") + "\n\n")
	builder.WriteString(m.fieldLine("Folder", m.decryptInput.View(), m.decryptFocus == 0))
	builder.WriteString(m.toggleLine("Delete encrypted .app/.h3 files", m.decryptDelete, m.decryptFocus == 1))
	builder.WriteString("\n")
	builder.WriteString(m.actionLine("Start decryption", m.decryptFocus == 2))
	builder.WriteString(m.actionLine("Back", m.decryptFocus == 3))
	if m.err != "" {
		builder.WriteString("\n" + tuiErrorStyle.Render(m.err) + "\n")
	}
	return builder.String()
}

func (m *tuiModel) titleIDView() string {
	var builder strings.Builder
	builder.WriteString(tuiDimStyle.Render("Enter a 16-digit title ID, or go back and use Search for a name.") + "\n\n")
	builder.WriteString(m.fieldLine("Title ID", m.titleIDInput.View(), true))
	if m.err != "" {
		builder.WriteString("\n" + tuiErrorStyle.Render(m.err) + "\n")
	}
	return builder.String()
}

func (m *tuiModel) dbUpdateView() string {
	if !m.dbUpdating {
		return "Starting database update…"
	}
	return m.spinner.View() + " " + m.dbStatus
}

func (m *tuiModel) messageView() string {
	var builder strings.Builder
	style := tuiSuccessStyle
	if m.messageKind == "error" {
		style = tuiErrorStyle
	} else if m.messageKind == "cancel" {
		style = tuiErrorStyle
	}
	builder.WriteString(style.Render(m.messageTitle) + "\n\n")
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	builder.WriteString(lipgloss.NewStyle().Width(width).Render(m.messageBody) + "\n\n")
	builder.WriteString(tuiDimStyle.Render("Press Enter to continue."))
	return builder.String()
}

func (m *tuiModel) footer() string {
	switch m.screen {
	case screenHome:
		return "↑/↓ move · Enter select · q quit"
	case screenSearch:
		return "type to search · Tab focus · ←/→ filter · ↑/↓ results · Enter actions · d/i/f shortcuts · Esc back"
	case screenActions:
		return "↑/↓ move · Enter select · Esc back"
	case screenDownloadOptions:
		return "↑/↓ or Tab move · Space toggles · Enter start · Esc back"
	case screenProgress:
		return "Esc cancel · Ctrl+C twice to force quit"
	case screenInfo:
		return "↑/↓ scroll · PgUp/PgDn page · Esc back"
	case screenFiles:
		return "Tab filter/list · Enter extract selected · a extract filtered · Esc back"
	case screenExtractOptions:
		return "↑/↓ or Tab move · Enter start · Esc back"
	case screenDecrypt:
		return "↑/↓ or Tab move · Space toggle · Enter start · Esc back"
	case screenTitleID:
		return "Enter continue · Esc back"
	case screenDBUpdate:
		return "Please wait · Ctrl+C quit"
	case screenHelp:
		return "↑/↓ scroll · Esc back"
	case screenMessage:
		return "Enter continue"
	default:
		return ""
	}
}

func (m *tuiModel) fieldLine(label, value string, focused bool) string {
	labelStyle := tuiLabelStyle
	if focused {
		labelStyle = tuiAccentStyle
	}
	return labelStyle.Render(fmt.Sprintf("%-16s ", label+":")) + value + "\n"
}

func (m *tuiModel) toggleLine(label string, value, focused bool) string {
	marker := "[ ]"
	if value {
		marker = "[x]"
	}
	style := tuiLabelStyle
	if focused {
		style = tuiAccentStyle
	}
	return style.Render(fmt.Sprintf("%-16s ", label+":")) + style.Render(marker) + "\n"
}

func (m *tuiModel) actionLine(label string, focused bool) string {
	if focused {
		return tuiAccentStyle.Render("> "+label) + "\n"
	}
	return "  " + label + "\n"
}

func (m *tuiModel) openHome() tea.Cmd {
	m.blurAllInputs()
	m.closeTree()
	m.screen = screenHome
	m.err = ""
	m.status = ""
	return nil
}

func (m *tuiModel) openSearch() tea.Cmd {
	m.closeTree()
	m.blurAllInputs()
	m.screen = screenSearch
	m.err = ""
	m.status = ""
	m.setSearchFocus(0)
	cmd := m.refreshSearch()
	return tea.Batch(cmd, textinput.Blink)
}

func (m *tuiModel) returnToSearch() tea.Cmd {
	m.blurAllInputs()
	m.screen = screenSearch
	m.setSearchFocus(3)
	return nil
}

func (m *tuiModel) openActions(entry wiiudownloader.TitleEntry) (tea.Model, tea.Cmd) {
	return m, m.openActionsCmd(entry)
}

func (m *tuiModel) openActionsCmd(entry wiiudownloader.TitleEntry) tea.Cmd {
	m.blurAllInputs()
	m.selected = entry
	m.actionList = newTUIList(tuiActionItems(), m.actionList.Width(), m.actionList.Height())
	m.actionList.Select(0)
	m.screen = screenActions
	m.err = ""
	m.status = ""
	return nil
}

func (m *tuiModel) openDownloadOptions(entry wiiudownloader.TitleEntry) (tea.Model, tea.Cmd) {
	m.blurAllInputs()
	m.selected = entry
	m.screen = screenDownloadOptions
	m.err = ""
	m.status = ""
	if strings.TrimSpace(m.outputInput.Value()) == "" {
		m.outputInput.SetValue(defaultDownloadDir())
	}
	m.setDownloadFocus(0)
	return m, textinput.Blink
}

func (m *tuiModel) openTitleID() tea.Cmd {
	m.blurAllInputs()
	m.screen = screenTitleID
	m.titleIDInput.Focus()
	m.err = ""
	m.status = ""
	return textinput.Blink
}

func (m *tuiModel) openDecrypt() tea.Cmd {
	m.blurAllInputs()
	m.screen = screenDecrypt
	m.setDecryptFocus(0)
	m.err = ""
	m.status = ""
	return textinput.Blink
}

func (m *tuiModel) openExtractOptions(paths []string) *tuiModel {
	m.blurAllInputs()
	m.extractPaths = paths
	m.extractOutput.SetValue(defaultDownloadDir())
	m.screen = screenExtractOptions
	m.err = ""
	m.setExtractFocus(0)
	return m
}

func (m *tuiModel) backToFiles() tea.Cmd {
	m.blurAllInputs()
	m.screen = screenFiles
	m.setFileFocus(1)
	m.err = ""
	m.status = ""
	return nil
}

func (m *tuiModel) showMessage(title, body string, returnTo tuiScreen, kind string) {
	m.screen = screenMessage
	m.messageTitle = title
	m.messageBody = body
	m.messageReturn = returnTo
	m.messageKind = kind
	m.blurAllInputs()
}

func (m *tuiModel) returnFromMessage() (tea.Model, tea.Cmd) {
	m.blurAllInputs()
	m.screen = m.messageReturn
	m.err = ""
	m.status = ""
	switch m.screen {
	case screenSearch:
		m.setSearchFocus(3)
	case screenFiles:
		m.setFileFocus(1)
	case screenHome:
		m.err = ""
	}
	return m, nil
}

func (m *tuiModel) setSearchFocus(focus int) {
	m.searchFocus = focus
	m.searchInput.Blur()
	if focus == 0 {
		m.searchInput.Focus()
	}
}

func (m *tuiModel) setDownloadFocus(focus int) {
	m.downloadFocus = focus
	m.outputInput.Blur()
	m.versionInput.Blur()
	if focus == 0 {
		m.outputInput.Focus()
	} else if focus == 1 {
		m.versionInput.Focus()
	}
}

func (m *tuiModel) setExtractFocus(focus int) {
	m.extractFocus = focus
	m.extractOutput.Blur()
	if focus == 0 {
		m.extractOutput.Focus()
	}
}

func (m *tuiModel) setDecryptFocus(focus int) {
	m.decryptFocus = focus
	m.decryptInput.Blur()
	m.titleIDInput.Blur()
	m.fileFilter.Blur()
	if focus == 0 {
		m.decryptInput.Focus()
	}
}

func (m *tuiModel) setFileFocus(focus int) {
	m.fileFocus = focus
	m.fileFilter.Blur()
	if focus == 0 {
		m.fileFilter.Focus()
	}
}

func (m *tuiModel) blurAllInputs() {
	m.searchInput.Blur()
	m.outputInput.Blur()
	m.versionInput.Blur()
	m.fileFilter.Blur()
	m.extractOutput.Blur()
	m.decryptInput.Blur()
	m.titleIDInput.Blur()
}

func (m *tuiModel) updateFocusedInput(msg tea.Msg) tea.Cmd {
	switch m.screen {
	case screenSearch:
		if m.searchFocus == 0 {
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return cmd
		}
	case screenDownloadOptions:
		switch m.downloadFocus {
		case 0:
			var cmd tea.Cmd
			m.outputInput, cmd = m.outputInput.Update(msg)
			return cmd
		case 1:
			var cmd tea.Cmd
			m.versionInput, cmd = m.versionInput.Update(msg)
			return cmd
		}
	case screenFiles:
		if m.fileFocus == 0 {
			var cmd tea.Cmd
			m.fileFilter, cmd = m.fileFilter.Update(msg)
			return cmd
		}
	case screenExtractOptions:
		if m.extractFocus == 0 {
			var cmd tea.Cmd
			m.extractOutput, cmd = m.extractOutput.Update(msg)
			return cmd
		}
	case screenDecrypt:
		if m.decryptFocus == 0 {
			var cmd tea.Cmd
			m.decryptInput, cmd = m.decryptInput.Update(msg)
			return cmd
		}
	case screenTitleID:
		var cmd tea.Cmd
		m.titleIDInput, cmd = m.titleIDInput.Update(msg)
		return cmd
	}
	return nil
}

func (m *tuiModel) cycleRegion(forward bool) {
	if forward {
		m.regionIndex = (m.regionIndex + 1) % len(tuiRegionChoices)
	} else {
		m.regionIndex = (m.regionIndex + len(tuiRegionChoices) - 1) % len(tuiRegionChoices)
	}
}

func (m *tuiModel) cycleType(forward bool) {
	if forward {
		m.typeIndex = (m.typeIndex + 1) % len(tuiTypeChoices)
	} else {
		m.typeIndex = (m.typeIndex + len(tuiTypeChoices) - 1) % len(tuiTypeChoices)
	}
}

func (m *tuiModel) refreshSearch() tea.Cmd {
	if m.db == nil {
		return nil
	}
	query := m.searchInput.Value()
	region := tuiRegionChoices[m.regionIndex].mask
	category := tuiTypeChoices[m.typeIndex].category
	matches := filterTitles(m.db.entries, query, region, category)
	sortTitles(matches, sortByName, false)
	m.searchTotal = len(matches)
	if len(matches) > tuiMaxSearchResults {
		matches = matches[:tuiMaxSearchResults]
	}
	m.searchResults = matches
	items := make([]list.Item, 0, len(matches))
	for _, entry := range matches {
		items = append(items, tuiTitleItem{entry: entry})
	}
	cmd := m.searchList.SetItems(items)
	if len(items) > 0 {
		m.searchList.Select(0)
	}
	return cmd
}

func (m *tuiModel) currentSearchEntry() (wiiudownloader.TitleEntry, bool) {
	if len(m.searchResults) == 0 {
		return wiiudownloader.TitleEntry{}, false
	}
	index := m.searchList.Index()
	if index < 0 || index >= len(m.searchResults) {
		index = 0
	}
	return m.searchResults[index], true
}

func (m *tuiModel) startInfo(entry wiiudownloader.TitleEntry) (tea.Model, tea.Cmd) {
	version, err := parseTitleVersion("latest", entry)
	if err != nil {
		version = wiiudownloader.VersionLatest
	}
	m.selected = entry
	m.infoEntry = entry
	m.infoVersion = version
	m.infoTMD = nil
	m.infoErr = nil
	m.infoLoading = true
	m.screen = screenInfo
	m.blurAllInputs()
	client := m.app.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		tmd, fetchErr := wiiudownloader.FetchTitleTMD(entry.TitleID, version, client)
		return tuiInfoLoadedMsg{tmd: tmd, err: fetchErr}
	})
}

func (m *tuiModel) startFiles(entry wiiudownloader.TitleEntry) (tea.Model, tea.Cmd) {
	version, err := parseTitleVersion("latest", entry)
	if err != nil {
		version = wiiudownloader.VersionLatest
	}
	m.closeTree()
	m.blurAllInputs()
	m.selected = entry
	m.filesEntry = entry
	m.filesVersion = version
	m.filesLoading = true
	m.allFiles = nil
	m.filteredFiles = nil
	m.fileFilter.SetValue("")
	m.filesList = newTUIList(nil, m.filesList.Width(), m.filesList.Height())
	m.setFileFocus(1)
	m.screen = screenFiles
	m.err = ""
	m.status = ""
	client := m.app.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		tree, fetchErr := wiiudownloader.FetchTitleFileTree(entry.TitleID, version, client)
		return tuiFilesLoadedMsg{tree: tree, err: fetchErr}
	})
}

func (m *tuiModel) rebuildFilesListCmd() tea.Cmd {
	return m.rebuildFilesList()
}

func (m *tuiModel) rebuildFilesList() tea.Cmd {
	needle := strings.ToLower(strings.TrimSpace(m.fileFilter.Value()))
	filtered := make([]wiiudownloader.TitleFile, 0, len(m.allFiles))
	for _, file := range m.allFiles {
		if needle == "" || strings.Contains(strings.ToLower(file.Path), needle) {
			filtered = append(filtered, file)
		}
	}
	m.filteredFiles = filtered
	items := make([]list.Item, 0, len(filtered))
	for _, file := range filtered {
		items = append(items, tuiFileItem{file: file})
	}
	cmd := m.filesList.SetItems(items)
	if len(items) > 0 {
		m.filesList.Select(0)
	}
	m.status = fmt.Sprintf("%d of %d file(s)", len(filtered), len(m.allFiles))
	return cmd
}

func (m *tuiModel) closeTree() {
	if m.tree != nil {
		_ = m.tree.Close()
		m.tree = nil
	}
	m.allFiles = nil
	m.filteredFiles = nil
}

func (m *tuiModel) beginDownload() tea.Cmd {
	entry := m.selected
	output := expandHomePath(m.outputInput.Value())
	if output == "" {
		m.err = "Choose an output folder."
		return nil
	}
	versionText, err := validateVersionText(m.versionInput.Value())
	if err != nil {
		m.err = err.Error()
		return nil
	}
	version, err := parseTitleVersion(versionText, entry)
	if err != nil {
		m.err = err.Error()
		return nil
	}
	target := filepath.Join(absolutePath(output), titleDirName(entry, version))
	decrypt := m.decrypt
	deleteEncrypted := m.deleteEncrypted && decrypt
	client := m.app.client

	m.progressCh = make(chan tuiProgressMsg, 256)
	m.reporter = newTUIProgress(m.progressCh)
	m.progressTitle = displayName(entry)
	m.progressOperation = "download"
	m.progressStart = time.Now()
	m.progressBytes = 0
	m.progressTotal = 0
	m.progressDecrypt = 0
	m.progressPhase = "download"
	m.progressSpeed = 0
	m.progressLastBytes = 0
	m.progressLastTime = time.Time{}
	m.cancelling = false
	m.err = ""
	m.status = ""
	m.screen = screenProgress
	m.blurAllInputs()

	reporter := m.reporter
	ch := m.progressCh
	return tea.Batch(waitForTUIProgress(ch), func() tea.Msg {
		downloadErr := wiiudownloader.DownloadTitle(
			fmt.Sprintf("%016x", entry.TitleID),
			target,
			version,
			decrypt,
			reporter,
			deleteEncrypted,
			client,
			"",
		)
		return tuiDownloadDoneMsg{err: downloadErr, target: target}
	})
}

func (m *tuiModel) beginExtract() tea.Cmd {
	if m.tree == nil {
		m.err = "The file list is no longer available."
		return nil
	}
	output := expandHomePath(m.extractOutput.Value())
	if output == "" {
		m.err = "Choose an output folder."
		return nil
	}
	target := filepath.Join(absolutePath(output), titleDirName(m.filesEntry, m.filesVersion))
	paths := append([]string(nil), m.extractPaths...)
	tree := m.tree

	m.progressCh = make(chan tuiProgressMsg, 256)
	m.reporter = newTUIProgress(m.progressCh)
	m.progressTitle = displayName(m.filesEntry)
	m.progressOperation = "extract"
	m.progressStart = time.Now()
	m.progressBytes = 0
	m.progressTotal = 0
	m.progressDecrypt = 0
	m.progressPhase = "download"
	m.progressSpeed = 0
	m.progressLastBytes = 0
	m.progressLastTime = time.Time{}
	m.cancelling = false
	m.err = ""
	m.status = ""
	m.screen = screenProgress
	m.blurAllInputs()

	reporter := m.reporter
	ch := m.progressCh
	return tea.Batch(waitForTUIProgress(ch), func() tea.Msg {
		extractErr := tree.DownloadFiles(target, paths, reporter)
		return tuiExtractDoneMsg{err: extractErr, paths: paths}
	})
}

func (m *tuiModel) beginDecrypt() tea.Cmd {
	path := expandHomePath(m.decryptInput.Value())
	if path == "" {
		m.err = "Choose a folder to decrypt."
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		m.err = "Folder not found: " + path
		return nil
	}
	deleteEncrypted := m.decryptDelete

	m.progressCh = make(chan tuiProgressMsg, 256)
	m.reporter = newTUIProgress(m.progressCh)
	m.progressTitle = filepath.Base(path)
	m.progressOperation = "decrypt"
	m.progressStart = time.Now()
	m.progressBytes = 0
	m.progressTotal = 0
	m.progressDecrypt = 0
	m.progressPhase = "decrypt"
	m.progressSpeed = 0
	m.progressLastBytes = 0
	m.progressLastTime = time.Time{}
	m.cancelling = false
	m.err = ""
	m.status = ""
	m.screen = screenProgress
	m.blurAllInputs()

	reporter := m.reporter
	ch := m.progressCh
	return tea.Batch(waitForTUIProgress(ch), func() tea.Msg {
		decryptErr := wiiudownloader.DecryptContents(path, reporter, deleteEncrypted, "")
		return tuiDecryptDoneMsg{err: decryptErr, path: path}
	})
}

func (m *tuiModel) startDBUpdate() tea.Cmd {
	m.blurAllInputs()
	if m.app.dbPath != "" {
		m.showMessage("Database update unavailable", "This session is using an explicit --db file. Remove --db to use the cached title database.", screenHome, "error")
		return nil
	}
	m.dbUpdating = true
	m.dbStatus = "Downloading the latest title database…"
	m.screen = screenDBUpdate
	m.err = ""
	m.status = ""
	client := m.app.client
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		entries, source, err := fetchTitleDatabase(client, titleDBURL())
		if err != nil {
			return tuiDBUpdatedMsg{err: err}
		}
		warning := ""
		if cachePath, cacheErr := titleDBCachePath(); cacheErr == nil {
			if writeErr := writeTitleDBCache(cachePath, titleDBCache{
				FetchedAt: time.Now().UTC(),
				Source:    source,
				Entries:   titleEntriesToJSON(entries),
			}); writeErr != nil {
				warning = "The database was downloaded but could not be cached: " + writeErr.Error()
			}
		} else {
			warning = "The database was downloaded but the cache path could not be determined: " + cacheErr.Error()
		}
		return tuiDBUpdatedMsg{entries: entries, source: source, warning: warning}
	})
}

func (m *tuiModel) applyProgress(msg tuiProgressMsg) {
	now := time.Now()
	if !m.progressLastTime.IsZero() {
		elapsed := now.Sub(m.progressLastTime).Seconds()
		if elapsed > 0 {
			instant := float64(msg.Current-m.progressLastBytes) / elapsed
			if instant < 0 {
				instant = 0
			}
			if m.progressSpeed == 0 {
				m.progressSpeed = instant
			} else {
				m.progressSpeed = 0.3*instant + 0.7*m.progressSpeed
			}
		}
	}
	m.progressLastTime = now
	m.progressLastBytes = msg.Current
	m.progressBytes = msg.Current
	m.progressTotal = msg.Total
	m.progressPhase = msg.Phase
	m.progressDecrypt = msg.Decrypt
}

func (m *tuiModel) cancelCurrent() tea.Cmd {
	if m.reporter == nil || m.cancelling {
		return nil
	}
	m.reporter.SetCancelled()
	m.cancelling = true
	m.status = "Cancelling…"
	return nil
}

func (m *tuiModel) finishProgress() {
	m.cancelling = false
	m.status = ""
}

func waitForTUIProgress(ch <-chan tuiProgressMsg) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *tuiModel) loading() bool {
	switch m.screen {
	case screenInfo:
		return m.infoLoading
	case screenFiles:
		return m.filesLoading
	case screenDBUpdate:
		return m.dbUpdating
	default:
		return false
	}
}

func clampTUI(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func versionInt(value string, entry wiiudownloader.TitleEntry) int {
	version, err := parseTitleVersion(value, entry)
	if err != nil {
		return wiiudownloader.VersionLatest
	}
	return version
}

var _ tea.Model = (*tuiModel)(nil)
