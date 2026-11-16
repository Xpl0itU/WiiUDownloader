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
	err    error
	paths  []string
	target string
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
	home        list.Model
	homeTwoLine bool

	// Search
	searchInput   textinput.Model
	searchFocus   int
	regionIndex   int
	typeIndex     int
	searchList    list.Model
	searchResults []wiiudownloader.TitleEntry
	searchTotal   int

	// Selected title / actions
	selected       wiiudownloader.TitleEntry
	actionList     list.Model
	actionsTwoLine bool

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
	progressSteps     []string
	progressStart     time.Time
	progressElapsed   time.Duration
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
	helpReturn    tuiScreen
	messageTitle  string
	messageBody   string
	messageHint   string
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
		homeTwoLine:     true,
		actionsTwoLine:  true,
	}

	model.spinner = spinner.New()
	model.spinner.Spinner = spinner.MiniDot
	model.spinner.Style = tuiAccentStyle

	model.progress = progress.New(progress.WithGradient("#0EA5E9", "#6366F1"), progress.WithoutPercentage())
	model.progress.Width = 60

	model.home = newTUIMenuList(tuiHomeItems(), 80, 18, false)
	model.searchInput = newTUIInput("Search by name or title ID…", 50)
	model.searchList = newTUIList(nil, 80, 12)
	model.actionList = newTUIMenuList(tuiActionItems(), 80, 12, false)
	model.outputInput = newTUIInput("Download folder", 58)
	model.outputInput.SetValue(defaultDownloadDir())
	model.versionInput = newTUIInput("latest", 16)
	model.versionInput.SetValue("latest")
	model.infoView = viewport.New(80, 18)
	model.helpView = viewport.New(80, 18)
	model.helpView.SetContent(tuiHelpText(80))
	model.fileFilter = newTUIInput("type to filter by path…", 42)
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
	model.resize()
	return model
}

func newTUIInput(placeholder string, width int) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.CharLimit = 1024
	input.Width = width
	input.Prompt = ""
	input.TextStyle = tuiTextStyle
	input.PlaceholderStyle = tuiDimStyle
	input.Cursor.Style = tuiAccentStyle
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
		m.resize()
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
			m.messageHint = "Check your network connection and try again. Press enter to return to the title actions."
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
			m.messageHint = "Files downloaded so far were kept, so retrying resumes where it stopped. Press enter to return to the search results."
		} else if m.reporter != nil && m.reporter.Cancelled() {
			m.showMessage("Download cancelled", "The partial download was kept and will resume next time.", screenSearch, "cancel")
		} else {
			body := "Saved to:\n" + msg.target + "\n\n" + m.transferSummary()
			m.showMessage("Download complete", body, screenSearch, "success")
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
			body := fmt.Sprintf("Extracted %s to:\n%s\n\n%s", plural(len(msg.paths), "file", "files"), msg.target, m.transferSummary())
			m.showMessage("Extraction complete", body, screenFiles, "success")
		}
		m.reporter = nil
		m.progressCh = nil
		return m, nil
	case tuiDecryptDoneMsg:
		m.finishProgress()
		if msg.err != nil {
			m.showMessage("Decryption failed", msg.err.Error(), screenHome, "error")
			m.messageHint = "Make sure the folder contains title.tmd, title.tik and every .app file. Press enter to return to the main menu."
		} else if m.reporter != nil && m.reporter.Cancelled() {
			m.showMessage("Decryption cancelled", "The operation was interrupted.", screenHome, "cancel")
		} else {
			body := "Decrypted files are ready in:\n" + msg.path + "\n\nFinished in " + formatDuration(m.progressElapsed) + "."
			m.showMessage("Decryption complete", body, screenHome, "success")
		}
		m.reporter = nil
		m.progressCh = nil
		return m, nil
	case tuiDBUpdatedMsg:
		m.dbUpdating = false
		if msg.err != nil {
			m.showMessage("Database update failed", msg.err.Error(), screenHome, "error")
			m.messageHint = "The current title database is still loaded. Check your network connection and try again."
			return m, nil
		}
		oldCount := 0
		if m.db != nil {
			oldCount = len(m.db.entries)
		}
		newDB := newTitleDB(msg.entries, "cache")
		newDB.setGlobal()
		m.db = newDB
		m.app.db = newDB
		body := fmt.Sprintf("Loaded %s from:\n%s", plural(len(newDB.entries), "title", "titles"), msg.source)
		if delta := len(newDB.entries) - oldCount; delta > 0 && oldCount > 0 {
			body += fmt.Sprintf("\n\n%s since the previous database.", plural(delta, "new title", "new titles"))
		}
		if msg.warning != "" {
			body += "\n\n" + msg.warning
		}
		m.refreshSearch()
		m.showMessage("Title database updated", body, screenHome, "success")
		return m, nil
	}

	return m, m.updateFocusedInput(msg)
}

// transferSummary describes a finished transfer's size and duration.
func (m *tuiModel) transferSummary() string {
	summary := "Finished in " + formatDuration(m.progressElapsed)
	if m.progressTotal > 0 {
		summary = formatBytes(m.progressTotal) + " · " + strings.ToLower(summary[:1]) + summary[1:]
	}
	return summary + "."
}

func (m *tuiModel) resize() {
	width := m.contentWidth()
	bodyHeight := m.bodyHeight()

	// Menus show descriptions when there is room, otherwise one line each.
	homeHeight := atLeast(bodyHeight-3, 3)
	m.homeTwoLine = homeHeight >= 2*len(m.home.Items())
	m.home.SetDelegate(&tuiRowDelegate{twoLine: m.homeTwoLine, focused: true})
	m.home.SetSize(width, homeHeight)

	actionHeight := atLeast(bodyHeight-3, 3)
	m.actionsTwoLine = actionHeight >= 2*len(m.actionList.Items())
	m.actionList.SetDelegate(&tuiRowDelegate{twoLine: m.actionsTwoLine, focused: true})
	m.actionList.SetSize(width, actionHeight)

	m.searchList.SetSize(width, atLeast(bodyHeight-m.searchControlLines()-2, 2))
	m.filesList.SetSize(width, atLeast(bodyHeight-5, 2))

	infoChrome := 4
	if m.infoErr != nil {
		infoChrome += 3
	}
	m.infoView.Width = width
	m.infoView.Height = atLeast(bodyHeight-infoChrome, 2)
	m.helpView.Width = width
	m.helpView.Height = atLeast(bodyHeight-1, 2)
	m.helpView.SetContent(tuiHelpText(width))

	m.progress.Width = width - 8
	if m.progress.Width > 60 {
		m.progress.Width = 60
	}
	if m.progress.Width < 10 {
		m.progress.Width = 10
	}

	fieldWidth := atLeast(width-(2+tuiLabelWidth+3)-1, 10)
	for _, input := range []*textinput.Model{&m.outputInput, &m.extractOutput, &m.decryptInput, &m.titleIDInput} {
		input.Width = fieldWidth
	}
	m.versionInput.Width = atLeast(fieldWidth, 10)
	if m.versionInput.Width > 16 {
		m.versionInput.Width = 16
	}
	m.titleIDInput.Width = atLeast(fieldWidth, 10)
	if m.titleIDInput.Width > 28 {
		m.titleIDInput.Width = 28
	}
	searchWidth := atLeast(width-(2+8+2)-1, 10)
	m.searchInput.Width = searchWidth
	m.fileFilter.Width = searchWidth
}

func atLeast(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
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
	case "/":
		return m, m.openSearch()
	case "?":
		m.openHelp(screenHome)
		return m, nil
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
			m.openHelp(screenHome)
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
	case 1, 2:
		switch key {
		case "left", "right", " ", "enter":
			if m.searchFocus == 1 {
				m.cycleRegion(key != "left")
			} else {
				m.cycleType(key != "left")
			}
			return m, m.refreshSearch()
		case "down":
			m.setSearchFocus(3)
			return m, nil
		case "up":
			m.setSearchFocus(0)
			return m, textinput.Blink
		}
	case 3:
		switch key {
		case "/":
			m.setSearchFocus(0)
			return m, textinput.Blink
		case "?":
			m.openHelp(screenSearch)
			return m, nil
		case "up":
			if m.searchList.Index() == 0 {
				m.setSearchFocus(0)
				return m, textinput.Blink
			}
		}
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
	case "d":
		return m.openDownloadOptions(m.selected)
	case "i":
		return m.startInfo(m.selected)
	case "f":
		return m.startFiles(m.selected)
	case "?":
		m.openHelp(screenActions)
		return m, nil
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
		m.err = ""
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
	case 4, 5:
		switch key {
		case "left", "right":
			m.setDownloadFocus(9 - m.downloadFocus)
			return m, nil
		case "enter", " ":
			if m.downloadFocus == 4 {
				return m, m.beginDownload()
			}
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
	if key == "esc" {
		m.closeTree()
		return m, m.openActionsCmd(m.selected)
	}
	if m.filesLoading {
		return m, nil
	}
	switch key {
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

	m.status = ""
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
		m.err = ""
		return m, m.updateFocusedInput(msg)
	case 1, 2:
		switch key {
		case "left", "right":
			m.setExtractFocus(3 - m.extractFocus)
		case "enter", " ":
			if m.extractFocus == 1 {
				return m, m.beginExtract()
			}
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
		m.err = ""
		return m, m.updateFocusedInput(msg)
	case 1:
		if key == " " || key == "left" || key == "right" || key == "enter" {
			m.decryptDelete = !m.decryptDelete
		}
	case 2, 3:
		switch key {
		case "left", "right":
			m.setDecryptFocus(5 - m.decryptFocus)
		case "enter", " ":
			if m.decryptFocus == 2 {
				return m, m.beginDecrypt()
			}
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
	switch msg.String() {
	case "esc", "?":
		return m, m.closeHelp()
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

func (m *tuiModel) openHome() tea.Cmd {
	m.blurAllInputs()
	m.closeTree()
	m.screen = screenHome
	m.err = ""
	m.status = ""
	return nil
}

func (m *tuiModel) openHelp(from tuiScreen) {
	m.blurAllInputs()
	m.helpReturn = from
	m.screen = screenHelp
	m.helpView.GotoTop()
}

func (m *tuiModel) closeHelp() tea.Cmd {
	switch m.helpReturn {
	case screenSearch:
		return m.returnToSearch()
	case screenActions:
		m.screen = screenActions
		return nil
	default:
		return m.openHome()
	}
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
	m.actionList = newTUIMenuList(tuiActionItems(), m.actionList.Width(), m.actionList.Height(), !m.actionsTwoLine)
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
	m.status = ""
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
	m.messageHint = ""
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
	m.searchList.SetDelegate(&tuiRowDelegate{focused: focus == 3})
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
	m.filesList.SetDelegate(&tuiRowDelegate{focused: focus == 1})
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
	m.resize()
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
	// The file count is rendered by the view; status holds transient notices.
	m.status = ""
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

// resetProgress prepares the transfer screen for a new operation.
func (m *tuiModel) resetProgress(title, operation, phase string, steps []string) {
	m.progressCh = make(chan tuiProgressMsg, 256)
	m.reporter = newTUIProgress(m.progressCh)
	m.progressTitle = title
	m.progressOperation = operation
	m.progressSteps = steps
	m.progressStart = time.Now()
	m.progressElapsed = 0
	m.progressBytes = 0
	m.progressTotal = 0
	m.progressDecrypt = 0
	m.progressPhase = phase
	m.progressSpeed = 0
	m.progressLastBytes = 0
	m.progressLastTime = time.Time{}
	m.cancelling = false
	m.err = ""
	m.status = ""
	m.screen = screenProgress
	m.blurAllInputs()
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

	steps := []string{"Download"}
	if decrypt {
		steps = append(steps, "Decrypt")
	}
	m.resetProgress(displayName(entry), "download", "download", steps)

	reporter := m.reporter
	ch := m.progressCh
	return tea.Batch(waitForTUIProgress(ch), m.spinner.Tick, func() tea.Msg {
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

	m.resetProgress(displayName(m.filesEntry), "extract", "download", []string{"Extract"})

	reporter := m.reporter
	ch := m.progressCh
	return tea.Batch(waitForTUIProgress(ch), m.spinner.Tick, func() tea.Msg {
		extractErr := tree.DownloadFiles(target, paths, reporter)
		return tuiExtractDoneMsg{err: extractErr, paths: paths, target: target}
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

	m.resetProgress(filepath.Base(path), "decrypt", "decrypt", []string{"Decrypt"})

	reporter := m.reporter
	ch := m.progressCh
	return tea.Batch(waitForTUIProgress(ch), m.spinner.Tick, func() tea.Msg {
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
	if !m.progressStart.IsZero() {
		m.progressElapsed = time.Since(m.progressStart)
	}
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
	case screenProgress:
		return m.reporter != nil
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
