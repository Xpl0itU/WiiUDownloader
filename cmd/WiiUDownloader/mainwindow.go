package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

const (
	MAIN_WINDOW_WIDTH        = 1040
	MAIN_WINDOW_HEIGHT       = 700
	SEARCH_ENTRY_WIDTH_CHARS = 14
	// AdwToolbarView logs "exceeds AdwWindow" instead of propagating its
	// content's minimum, so the window happily shrinks into a layout that no
	// longer fits. Pinning the smallest usable size stops that; the smoke run
	// asserts the content minimum stays inside it.
	MIN_WINDOW_WIDTH  = 640
	MIN_WINDOW_HEIGHT = 480
	// One compact layout below this width: the queue pane is dropped, the bottom
	// bar stacks and the search entry shrinks. It has to be a single breakpoint —
	// libadwaita only applies the last matching one, so a second, narrower
	// breakpoint would never fire.
	//
	// The value is not free: the queue pane is only visible *above* it, so it has
	// to clear the pane-visible layout minimum or AdwToolbarView starts logging
	// "exceeds AdwWindow" again. The smoke run asserts exactly that.
	COMPACT_WINDOW_BREAKPOINT       = 920
	NARROW_SEARCH_ENTRY_WIDTH_CHARS = 9

	UI_MARGIN_SMALL               = 6
	SPLIT_PANE_MARGIN             = 2
	DOWNLOAD_PANE_MIN_WIDTH       = 300
	QUEUE_PANE_MIN_WIDTH          = 200
	SEARCH_DEBOUNCE_DELAY         = 200 * time.Millisecond
	PARSE_UINT_BASE_16            = 16
	PARSE_UINT_BITS_64            = 64
	DONATION_BAR_SPACING          = 20
	QUEUE_ROW_MAX_HEIGHT          = 44
	QUEUE_SMOKE_SIZE_BYTES        = 1234567
	RELATED_DIALOG_WIDTH          = 620
	RELATED_DIALOG_HEIGHT         = 420
	ERROR_DIALOG_WIDTH            = 600
	ERROR_DIALOG_HEIGHT           = 400
	DIALOG_MARGIN                 = 10
	DIALOG_CONTENT_SPACING        = 12
	DIALOG_CONTENT_MARGIN         = 12
	RELATED_ROW_HORIZONTAL_MARGIN = 16
	RELATED_ROW_VERTICAL_MARGIN   = 12
	RELATED_ROW_SPACING           = 12
	ERROR_ROW_MARGIN              = 5
	MAX_CONCURRENT_SIZE_FETCHES   = 8
)

type MainWindow struct {
	window                          *gtk.Window
	adwWindow                       *adw.Window
	headerBar                       *adw.HeaderBar
	toolbarView                     *adw.ToolbarView
	titleStatusPage                 *adw.StatusPage
	queuePane                       *QueuePane
	titleView                       *gtk.ColumnView
	titleSelection                  *gtk.MultiSelection
	titleSortModel                  *gtk.SortListModel
	titleFilter                     *gtk.CustomFilter
	titleScroll                     *gtk.ScrolledWindow
	rows                            *gtk.StringList
	titleRows                       map[string]*titleRow
	boundChecks                     map[string]*gtk.CheckButton
	checkRowKeys                    map[uintptr]string
	searchEntry                     *gtk.SearchEntry
	downloadQueueButton             *gtk.Button
	decryptContentsCheckbox         *gtk.CheckButton
	deleteEncryptedContentsCheckbox *gtk.CheckButton
	decryptContentsToggleHandle     glib.SignalHandle
	deleteEncryptedContentsHandle   glib.SignalHandle
	japanRegionCheckbox             *gtk.CheckButton
	usaRegionCheckbox               *gtk.CheckButton
	europeRegionCheckbox            *gtk.CheckButton
	japanRegionToggleHandle         glib.SignalHandle
	usaRegionToggleHandle           glib.SignalHandle
	europeRegionToggleHandle        glib.SignalHandle
	deleteEncryptedContents         bool
	progressWindow                  *ProgressWindow
	// downloadUI is the surface the current run reports to; set by
	// startDownloadRun from Config.UseInlineDownloadUI.
	downloadUI            DownloadUI
	configWindow          *ConfigWindow
	lastSearchText        string
	categoryButtons       []*gtk.ToggleButton
	titles                []wiiudownloader.TitleEntry
	decryptContents       bool
	suggestRelatedContent bool
	currentRegion         uint8
	currentCategory       uint8
	client                *http.Client
	uiBuilt               bool
	searchTimer           *time.Timer
	menuButton            *gtk.MenuButton
	toolbar               *gtk.Box
	categoryBox           *gtk.Box
	bottomBar             *gtk.Box
	donationBar           *gtk.Box
	donationLabel         *gtk.Label
	donationSubLabel      *gtk.Label
	supporterLabels       []*gtk.Label
	supporterCount        int
	showDonationBar       bool
	sizeFetchSemaphore    chan struct{}
}

func NewMainWindow(entries []wiiudownloader.TitleEntry, client *http.Client, config *Config) *MainWindow {
	adwWin := adw.NewWindow()
	win := &adwWin.Window
	win.SetTitle(APP_NAME)
	win.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
	win.SetSizeRequest(MIN_WINDOW_WIDTH, MIN_WINDOW_HEIGHT)
	win.SetDecorated(true)
	win.ConnectCloseRequest(func() bool {
		os.Exit(0)
		return true
	})

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search...")
	searchEntry.SetHExpand(false)
	searchEntry.SetHAlign(gtk.AlignEnd)
	searchEntry.SetWidthChars(SEARCH_ENTRY_WIDTH_CHARS)
	// Cap the natural width too, so typing never re-flows the toolbar.
	searchEntry.SetMaxWidthChars(SEARCH_ENTRY_WIDTH_CHARS)
	SetupEntryAccessibility(searchEntry, "Search titles", "Enter a game title or title ID to search. You can use the category buttons above to filter by type.")

	queuePane, err := NewQueuePane()
	if err != nil {
		log.Fatalln("Unable to create queue pane:", err)
	}

	mainWindow := MainWindow{
		window:             win,
		adwWindow:          adwWin,
		queuePane:          queuePane,
		titles:             entries,
		searchEntry:        searchEntry,
		currentRegion:      wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA,
		lastSearchText:     "",
		client:             client,
		supporterCount:     fallbackSupporterCount,
		sizeFetchSemaphore: make(chan struct{}, MAX_CONCURRENT_SIZE_FETCHES),
	}

	queuePane.updateFunc = mainWindow.updateTitlesInQueue
	queuePane.SetSetVersionRequested(mainWindow.onSetVersionRequested)

	mainWindow.applyConfig(config)
	applyStyling()

	searchEntry.ConnectChanged(mainWindow.onSearchEntryChanged)

	mainWindow.queuePane.SetDownloadCallback(mainWindow.onDownloadQueueButtonClicked)

	// Best effort; on failure the manually-set fallback count stays.
	go mainWindow.refreshSupporterCount()

	return &mainWindow
}

func (mw *MainWindow) SetApplicationForGTKWindow(app *gtk.Application) {
	mw.window.SetApplication(app)
}

func (mw *MainWindow) createConfigWindow(config *Config) error {
	// One settings window at a time. The previous one used to be dropped and
	// left for the Go GC to destroy, which tears a realized window down at an
	// arbitrary moment; on macOS that is frame-callback churn GDK is not fond
	// of. Closing it here keeps the teardown deterministic.
	if previous := mw.configWindow; previous != nil && previous.Window != nil {
		mw.configWindow = nil
		uiIdleAdd(func() { previous.Window.Close() })
	}

	configWindow, err := NewConfigWindow(config)
	if err != nil {
		return err
	}
	mw.configWindow = configWindow
	return nil
}

func (mw *MainWindow) applyConfig(config *Config) {
	setDarkTheme(config.DarkMode)
	mw.applyDownloadOptionState(config.DecryptContents, config.DeleteEncryptedContents)
	mw.suggestRelatedContent = config.SuggestRelatedContent
	mw.applyRegionSelection(config.SelectedRegion)
	mw.setDonationBarVisible(config.ShowDonationBar)
}

func (mw *MainWindow) BuildUI() {
	if mw.uiBuilt {
		return
	}
	mw.uiBuilt = true

	mw.buildTitleList()

	mainvBox := gtk.NewBox(gtk.OrientationVertical, 6)
	mainvBox.SetMarginTop(UI_MARGIN_SMALL)
	mainvBox.SetMarginBottom(UI_MARGIN_SMALL)
	mainvBox.SetMarginStart(UI_MARGIN_SMALL)
	mainvBox.SetMarginEnd(UI_MARGIN_SMALL)

	// App-level menu lives in the header bar as a primary GtkMenuButton, the
	// GTK4 pattern; handlers sit in the window's action group.
	menuActions := gio.NewSimpleActionGroup()

	decryptContentsAction := gio.NewSimpleAction("decrypt-contents", nil)
	decryptContentsAction.ConnectActivate(func(*glib.Variant) {
		chooseFolders(mw.window, WINDOW_TITLE_PREFIX+"Select Game Folders", "", func(selectedPaths []string) {
			mw.runDecryptContents(selectedPaths)
		})
	})
	menuActions.Insert(decryptContentsAction)

	generateFakeTicketAction := gio.NewSimpleAction("generate-fake-ticket", nil)
	generateFakeTicketAction.ConnectActivate(func(*glib.Variant) {
		chooseFile(mw.window, WINDOW_TITLE_PREFIX+"Select TMD File", "tmd", []string{"*.tmd"}, func(tmdPath string) {
			if tmdPath == "" {
				return
			}
			mw.runGenerateFakeTicketAndCert(tmdPath)
		})
	})
	menuActions.Insert(generateFakeTicketAction)

	addByTitleIDAction := gio.NewSimpleAction("add-by-title-id", nil)
	addByTitleIDAction.ConnectActivate(func(*glib.Variant) {
		mw.showAddByTitleIDDialog()
	})
	menuActions.Insert(addByTitleIDAction)

	openSettingsAction := gio.NewSimpleAction("open-settings", nil)
	openSettingsAction.ConnectActivate(func(*glib.Variant) {
		mw.openSettingsWindow()
	})
	menuActions.Insert(openSettingsAction)

	toolsMenu := gio.NewMenu()
	toolsMenu.Append("Decrypt Contents", "win.decrypt-contents")
	toolsMenu.Append("Generate Fake Ticket and Cert", "win.generate-fake-ticket")
	toolsMenu.Append("Add by Title ID", "win.add-by-title-id")

	settingsMenu := gio.NewMenu()
	settingsMenu.Append("Settings", "win.open-settings")

	rootMenu := gio.NewMenu()
	rootMenu.AppendSection("Tools", toolsMenu)
	rootMenu.AppendSection("", settingsMenu)

	menuButton := gtk.NewMenuButton()
	mw.menuButton = menuButton
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetMenuModel(rootMenu)
	menuButton.SetTooltipText(composeAccessibleText("Main menu", "Tools and application settings", " - "))
	menuButton.SetVAlign(gtk.AlignCenter)
	menuButton.SetMarginStart(4)
	menuButton.SetMarginEnd(2)
	menuButton.AddCSSClass("flat")

	mw.window.InsertActionGroup("win", menuActions)

	// Same nav bar as the other windows, so the app looks like one piece.
	headerBar := adw.NewHeaderBar()
	mw.headerBar = headerBar
	headerBar.PackEnd(menuButton)

	tophBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	mw.toolbar = tophBox

	// GTK3 used SetMode(false) radio buttons, i.e. toggle buttons; a grouped,
	// linked set of GtkToggleButtons is the same widget without the removed one.
	categoryBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	mw.categoryBox = categoryBox
	categoryBox.AddCSSClass("linked")
	categoryBox.SetVAlign(gtk.AlignCenter)

	var firstCategory *gtk.ToggleButton
	mw.categoryButtons = make([]*gtk.ToggleButton, 0)
	for _, cat := range []string{"Game", "Update", "DLC", "Demo", "All"} {
		button := gtk.NewToggleButtonWithLabel(cat)
		if firstCategory == nil {
			firstCategory = button
		} else {
			button.SetGroup(firstCategory)
		}
		button.AddCSSClass("category-toggle")
		categoryBox.Append(button)
		button.ConnectToggled(func() {
			mw.onCategoryToggled(button, cat)
		})
		if cat == "Game" {
			button.SetActive(true)
		}
		SetupToggleButtonAccessibility(button, "Filter titles by category: "+cat)
		mw.categoryButtons = append(mw.categoryButtons, button)
	}
	tophBox.Append(categoryBox)

	// The search entry is a sibling of the pills in one horizontal box, so it
	// always renders to their right; a box never wraps onto a second line.
	toolbarSpacer := gtk.NewLabel("")
	toolbarSpacer.SetHExpand(true)
	tophBox.Append(toolbarSpacer)
	tophBox.Append(mw.searchEntry)
	mainvBox.Append(tophBox)

	scrollable := gtk.NewScrolledWindow()
	scrollable.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrollable.SetVExpand(true)
	scrollable.SetChild(mw.titleView)
	mw.titleScroll = scrollable

	// Empty state: an empty list otherwise just looks like a broken window.
	statusPage := adw.NewStatusPage()
	statusPage.SetIconName("edit-find-symbolic")
	statusPage.SetTitle("No titles match")
	statusPage.SetDescription("Try a different search term, or pick another category or region.")
	statusPage.SetVisible(false)
	mw.titleStatusPage = statusPage

	titleOverlay := gtk.NewOverlay()
	titleOverlay.SetChild(scrollable)
	titleOverlay.AddOverlay(statusPage)
	mainvBox.Append(titleOverlay)

	// A GtkActionBar paints its background as a rounded card under libadwaita, so
	// the fill visibly stops short of the content area. A plain box with our own
	// border spans the full width the way the GTK3 build's action bar did.
	bottomhBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	bottomhBox.AddCSSClass("bottom-bar")
	mw.bottomBar = bottomhBox

	mw.downloadQueueButton = mw.queuePane.downloadButton
	SetupButtonAccessibility(mw.downloadQueueButton, "Start downloading all titles in your queue")

	mw.decryptContentsCheckbox = gtk.NewCheckButtonWithLabel("Decrypt contents")
	SetupCheckButtonAccessibility(mw.decryptContentsCheckbox, "When checked, downloaded game contents will be decrypted after download completes")

	mw.deleteEncryptedContentsCheckbox = gtk.NewCheckButtonWithLabel("Delete encrypted contents after decryption")
	SetupCheckButtonAccessibility(mw.deleteEncryptedContentsCheckbox, "When checked and decrypt contents is enabled, encrypted files will be deleted after successful decryption")
	mw.deleteEncryptedContentsHandle = mw.deleteEncryptedContentsCheckbox.ConnectToggled(func() {
		config, err := loadConfig()
		if err != nil {
			return
		}
		mw.deleteEncryptedContents = mw.getDeleteEncryptedContents()
		config.DeleteEncryptedContents = mw.getDeleteEncryptedContents()
		if err := config.Save(); err != nil {
			ShowErrorDialog(mw.window, err)
			return
		}
	})

	mw.decryptContentsToggleHandle = mw.decryptContentsCheckbox.ConnectToggled(mw.onDecryptContentsClicked)
	mw.applyDownloadOptionState(mw.decryptContents, mw.deleteEncryptedContents)

	checkboxvBox := gtk.NewBox(gtk.OrientationVertical, 0)
	checkboxvBox.Append(mw.decryptContentsCheckbox)
	checkboxvBox.Append(mw.deleteEncryptedContentsCheckbox)

	bottomhBox.Append(checkboxvBox)

	bottomSpacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	bottomSpacer.SetHExpand(true)
	bottomhBox.Append(bottomSpacer)

	regionBox := gtk.NewBox(gtk.OrientationHorizontal, 12)

	europeButton := gtk.NewCheckButtonWithLabel("Europe")
	mw.europeRegionCheckbox = europeButton
	mw.europeRegionToggleHandle = europeButton.ConnectToggled(func() {
		mw.onRegionChange(europeButton, wiiudownloader.MCP_REGION_EUROPE)
	})
	regionBox.Append(europeButton)

	usaButton := gtk.NewCheckButtonWithLabel("USA")
	mw.usaRegionCheckbox = usaButton
	mw.usaRegionToggleHandle = usaButton.ConnectToggled(func() {
		mw.onRegionChange(usaButton, wiiudownloader.MCP_REGION_USA)
	})
	regionBox.Append(usaButton)

	japanButton := gtk.NewCheckButtonWithLabel("Japan")
	mw.japanRegionCheckbox = japanButton
	mw.japanRegionToggleHandle = japanButton.ConnectToggled(func() {
		mw.onRegionChange(japanButton, wiiudownloader.MCP_REGION_JAPAN)
	})
	regionBox.Append(japanButton)

	bottomhBox.Append(regionBox)
	mw.syncRegionCheckboxes()

	// GTK3 pack_end stacks upward from the bottom edge, so the donation bar sat
	// above the action bar; GtkBox lays out in append order, so append it first.
	mw.setupDonationBar()
	if mw.donationBar != nil {
		mainvBox.Append(mw.donationBar)
	}

	mainvBox.Append(bottomhBox)

	splitPane := gtk.NewPaned(gtk.OrientationHorizontal)
	splitPane.SetStartChild(mw.queuePane.GetContainer())
	splitPane.SetResizeStartChild(false)
	// Shrinking below the minimum clips the child instead of reflowing it, which
	// is exactly the "content gets cut off" resize bug.
	splitPane.SetShrinkStartChild(false)
	splitPane.SetEndChild(mainvBox)
	splitPane.SetResizeEndChild(true)
	splitPane.SetShrinkEndChild(false)

	splitPane.SetMarginBottom(SPLIT_PANE_MARGIN)
	splitPane.SetMarginEnd(SPLIT_PANE_MARGIN)
	splitPane.SetMarginStart(SPLIT_PANE_MARGIN)
	splitPane.SetMarginTop(SPLIT_PANE_MARGIN)

	view := adw.NewToolbarView()
	mw.toolbarView = view
	view.AddTopBar(headerBar)
	view.SetContent(splitPane)
	mw.adwWindow.SetContent(view)

	splitPane.SetPosition(280) // Set default width for QueuePane

	// Narrow windows drop the queue pane instead of squeezing the title list. The
	// two bottom-bar groups would otherwise set a minimum as wide as the window
	// itself, so they stack, and the search entry gives up the most room.
	compact := adw.NewBreakpoint(adw.NewBreakpointConditionLength(
		adw.BreakpointConditionMaxWidth, COMPACT_WINDOW_BREAKPOINT, adw.LengthUnitPx))
	compact.AddSetter(mw.queuePane.GetContainer(), "visible", false)
	compact.AddSetter(bottomhBox, "orientation", gtk.OrientationVertical)
	compact.AddSetter(mw.searchEntry, "width-chars", NARROW_SEARCH_ENTRY_WIDTH_CHARS)
	mw.adwWindow.AddBreakpoint(compact)
}

// openSettingsWindow shows the settings window as a transient of the main one.
func (mw *MainWindow) openSettingsWindow() {
	config, err := loadConfig()
	if err != nil {
		return
	}
	if err := mw.createConfigWindow(config); err != nil {
		return
	}
	if mw.configWindow != nil && mw.window != nil {
		mw.configWindow.Window.SetTransientFor(mw.window)
		mw.configWindow.Window.SetDecorated(true)
	}
	mw.configWindow.Window.Present()
}

// runDecryptContents decrypts one or more folders in the background. A folder
// that fails is skipped so the rest of the batch still runs, and every failure
// is reported together once the batch ends.
func (mw *MainWindow) runDecryptContents(selectedPaths []string) {
	if len(selectedPaths) == 0 {
		return
	}

	progressWindow, err := createProgressWindow(mw.window)
	if err != nil {
		log.Printf("Failed to create progress window: %v", err)
		return
	}
	mw.progressWindow = progressWindow
	mw.progressWindow.SetGameTitle(fmt.Sprintf("Decrypting %d folder(s)...", len(selectedPaths)))
	mw.progressWindow.ResetTotals()
	progressWindow.Window.Present()

	go func() {
		var failed []DownloadError
		for _, selectedPath := range selectedPaths {
			if mw.progressWindow.Cancelled() {
				break
			}
			if err := mw.decryptFolder(selectedPath); err != nil {
				log.Printf("Decryption failed for %s: %v", selectedPath, err)
				failed = append(failed, DownloadError{
					Title: filepath.Base(selectedPath),
					Error: err.Error(),
				})
			}
		}

		uiIdleAdd(func() {
			mw.progressWindow.Window.SetVisible(false)
			// A decryption is not a download, so it never raises the "Download
			// Complete" dialog; only the failures are worth a dialog.
			if len(failed) > 0 {
				mw.showDecryptErrorsDialog(failed)
			}
		})
	}()
}

// decryptFolder decrypts a single folder into the configured output path.
func (mw *MainWindow) decryptFolder(selectedPath string) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	decryptOut := ""
	if config.DecryptOutputPath != "" {
		decryptOut = filepath.Join(config.DecryptOutputPath, filepath.Base(selectedPath))
	}
	return wiiudownloader.DecryptContents(selectedPath, mw.progressWindow, false, decryptOut)
}

// runGenerateFakeTicketAndCert generates ticket+cert files for a TMD path.
func (mw *MainWindow) runGenerateFakeTicketAndCert(tmdPath string) {
	progressWindow, err := createProgressWindow(mw.window)
	if err != nil {
		log.Printf("Failed to create progress window: %v", err)
		return
	}
	mw.progressWindow = progressWindow
	mw.progressWindow.SetGameTitle("Generating Ticket and Cert...")
	mw.progressWindow.ResetTotals()
	progressWindow.Window.Present()

	go func() {
		defer uiIdleAdd(func() {
			mw.progressWindow.Window.SetVisible(false)
		})

		parentDir := filepath.Dir(tmdPath)
		tmdData, err := os.ReadFile(tmdPath)
		if err != nil {
			uiIdleAdd(func() {
				ShowErrorDialog(mw.window, err)
			})
			return
		}

		tmd, err := wiiudownloader.ParseTMD(tmdData)
		if err != nil {
			uiIdleAdd(func() {
				ShowErrorDialog(mw.window, err)
			})
			return
		}

		titleIDHex := fmt.Sprintf("%016x", tmd.TitleID)
		titleEntry := wiiudownloader.GetTitleEntryFromTid(tmd.TitleID)
		titleKeyType := uint8(wiiudownloader.TITLE_KEY_mypass)
		if titleEntry.TitleID == tmd.TitleID {
			titleKeyType = titleEntry.Key
		}
		titleKey, err := wiiudownloader.GenerateKeyWithType(titleIDHex, titleKeyType)
		if err != nil {
			uiIdleAdd(func() {
				ShowErrorDialog(mw.window, err)
			})
			return
		}
		if err := wiiudownloader.GenerateTicket(filepath.Join(parentDir, "title.tik"), tmd.TitleID, titleKey, tmd.TitleVersion); err != nil {
			uiIdleAdd(func() {
				ShowErrorDialog(mw.window, err)
			})
			return
		}

		if err := wiiudownloader.GenerateCert(tmd, filepath.Join(parentDir, "title.cert"), mw.progressWindow, http.DefaultClient); err != nil {
			uiIdleAdd(func() {
				ShowErrorDialog(mw.window, err)
			})
			return
		}

		uiIdleAdd(func() {
			showAlert(mw.window, WINDOW_TITLE_PREFIX+"Success", "Successfully generated fake ticket and cert.")
		})
	}()
}

func (mw *MainWindow) PostShowInit() {
	mw.focusTitleList()
	mw.window.SetFocus(mw.titleView)
	mw.window.SetDefaultWidget(mw.downloadQueueButton)
}

func (mw *MainWindow) onRegionChange(button *gtk.CheckButton, region uint8) {
	mw.currentRegion = updateRegionMask(mw.currentRegion, region, button.Active())
	mw.refreshTitleFilter()
	config, err := loadConfig()
	if err != nil {
		return
	}
	config.SelectedRegion = mw.currentRegion
	if err := config.Save(); err != nil {
		ShowErrorDialog(mw.window, err)
		return
	}
}

func updateRegionMask(current, region uint8, active bool) uint8 {
	if active {
		return current | region
	}
	return current &^ region
}

func regionCheckboxStates(regionMask uint8) (europe, usa, japan bool) {
	return regionMask&wiiudownloader.MCP_REGION_EUROPE != 0,
		regionMask&wiiudownloader.MCP_REGION_USA != 0,
		regionMask&wiiudownloader.MCP_REGION_JAPAN != 0
}

func (mw *MainWindow) applyRegionSelection(regionMask uint8) {
	mw.currentRegion = regionMask
	mw.syncRegionCheckboxes()
	mw.refreshTitleFilter()
}

func (mw *MainWindow) syncRegionCheckboxes() {
	if mw.europeRegionCheckbox == nil || mw.usaRegionCheckbox == nil || mw.japanRegionCheckbox == nil {
		return
	}

	europeActive, usaActive, japanActive := regionCheckboxStates(mw.currentRegion)
	setCheckButtonActiveWithoutSignal(mw.europeRegionCheckbox, mw.europeRegionToggleHandle, europeActive)
	setCheckButtonActiveWithoutSignal(mw.usaRegionCheckbox, mw.usaRegionToggleHandle, usaActive)
	setCheckButtonActiveWithoutSignal(mw.japanRegionCheckbox, mw.japanRegionToggleHandle, japanActive)
}

func (mw *MainWindow) onSearchEntryChanged() {
	if mw.searchTimer != nil {
		mw.searchTimer.Stop()
	}
	mw.searchTimer = time.AfterFunc(SEARCH_DEBOUNCE_DELAY, func() {
		uiIdleAdd(func() {
			mw.lastSearchText = mw.searchEntry.Text()
			mw.refreshTitleFilter()
		})
	})
}

func (mw *MainWindow) onCategoryToggled(button *gtk.ToggleButton, category string) {
	if !button.Active() {
		return
	}
	mw.currentCategory = wiiudownloader.GetCategoryFromFormattedCategory(category)
	uiIdleAdd(func() {
		mw.refreshTitleFilter()
	})
}

func (mw *MainWindow) setDownloadControlsSensitive(sensitive bool) {
	mw.titleView.SetSensitive(sensitive)
	for _, button := range mw.categoryButtons {
		button.SetSensitive(sensitive)
	}
	mw.searchEntry.SetSensitive(sensitive)
	mw.downloadQueueButton.SetSensitive(sensitive)
	mw.deleteEncryptedContentsCheckbox.SetSensitive(sensitive)
	mw.decryptContentsCheckbox.SetSensitive(sensitive)
	// Derived, not forced: the pane also has to have a live selection.
	mw.queuePane.SetControlsSensitive(sensitive)
}

func (mw *MainWindow) showSuccessDialog(count int, downloadPath string, decryptOutputPath string) {
	dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Download Complete")
	dialog.SetDefaultSize(420, -1)

	contentArea := dialog.Content()

	header := gtk.NewLabel("")
	header.SetMarkup("<span size='x-large' weight='bold' foreground='#16a34a'>Downloads Finished!</span>")
	header.SetMarginTop(12)
	contentArea.Append(header)

	showDual := decryptOutputPath != "" && decryptOutputPath != downloadPath
	infoLabel := gtk.NewLabel("")
	if showDual {
		infoLabel.SetMarkup(fmt.Sprintf("Successfully processed %d items.\n<span size='small'>Download: %s</span>\n<span size='small'>Decrypted: %s</span>", count, glib.MarkupEscapeText(downloadPath), glib.MarkupEscapeText(decryptOutputPath)))
	} else {
		infoLabel.SetMarkup(fmt.Sprintf("Successfully processed %d items.\nSaved to: <span size='small'>%s</span>", count, glib.MarkupEscapeText(downloadPath)))
	}
	infoLabel.SetWrap(true)
	infoLabel.SetEllipsize(pango.EllipsizeMiddle)
	infoLabel.SetMaxWidthChars(60)
	infoLabel.SetXAlign(0.5)
	infoLabel.SetJustify(gtk.JustifyCenter)
	contentArea.Append(infoLabel)

	if showDual {
		linkedBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
		linkedBox.SetHAlign(gtk.AlignCenter)
		linkedBox.SetMarginBottom(12)
		linkedBox.AddCSSClass("linked")

		dlBtn := newIconLabelButton("folder-open-symbolic", "Open Downloads")
		dlBtn.ConnectClicked(func() {
			openURL(downloadPath)
		})
		linkedBox.Append(dlBtn)

		decBtn := newIconLabelButton("folder-open-symbolic", "Open Decrypted")
		decBtn.AddCSSClass("confirm-action")
		decBtn.ConnectClicked(func() {
			openURL(decryptOutputPath)
		})
		linkedBox.Append(decBtn)

		contentArea.Append(linkedBox)
	} else {
		openBtn := newIconLabelButton("folder-open-symbolic", "Open Download Folder")
		openBtn.SetHAlign(gtk.AlignCenter)
		openBtn.SetMarginBottom(12)
		openBtn.AddCSSClass("confirm-action")
		openBtn.ConnectClicked(func() {
			openURL(downloadPath)
		})
		contentArea.Append(openBtn)
	}

	if mw.showDonationBar {
		donationBox := gtk.NewBox(gtk.OrientationVertical, 8)
		donationBox.AddCSSClass("donation-highlight")
		donationBox.SetMarginStart(12)
		donationBox.SetMarginEnd(12)
		donationBox.SetMarginBottom(12)
		donationBox.SetMarginTop(6)

		nudgeLabel := gtk.NewLabel("")
		// rough retail value per item; Wii U games typically sell for $40+
		retailValue := count * 40
		nudgeLabel.SetMarkup(fmt.Sprintf("<span size='medium'><b>You just grabbed $%d+ of games for free.</b> A coffee is a fraction of that.</span>", retailValue))
		nudgeLabel.SetWrap(true)
		nudgeLabel.SetWrapMode(pango.WrapWord)
		nudgeLabel.SetXAlign(0.5)
		nudgeLabel.SetJustify(gtk.JustifyCenter)
		donationBox.Append(nudgeLabel)

		if kofiBtn := newKofiButton(); kofiBtn != nil {
			kofiBtn.SetHAlign(gtk.AlignCenter)
			donationBox.Append(kofiBtn)
		}

		if supporterSmall := mw.newSupporterLabel(); supporterSmall != nil {
			donationBox.Append(supporterSmall)
		}

		contentArea.Append(donationBox)
	}

	dialog.AddButton("Close", nil)
	dialog.Present()
}

func openURL(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = execCommand("xdg-open", url)
	case "darwin":
		err = execCommand("open", url)
	case "windows":
		err = execCommand("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		log.Printf("unsupported platform for opening URL: %s", url)
	}
	if err != nil {
		log.Printf("failed to open URL %s: %v", url, err)
	}
}

func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

func (mw *MainWindow) onDecryptContentsClicked() {
	mw.applyDownloadOptionState(mw.decryptContentsCheckbox.Active(), mw.getDeleteEncryptedContents())
	config, err := loadConfig()
	if err != nil {
		return
	}
	config.DecryptContents = mw.decryptContents
	config.DeleteEncryptedContents = mw.deleteEncryptedContents
	if err := config.Save(); err != nil {
		ShowErrorDialog(mw.window, err)
		return
	}
}

func (mw *MainWindow) getDeleteEncryptedContents() bool {
	if mw.deleteEncryptedContentsCheckbox.Sensitive() {
		return mw.deleteEncryptedContentsCheckbox.Active()
	}
	return false
}

func downloadOptionCheckboxState(decryptContents, deleteEncryptedContents bool) (decryptActive, deleteActive, deleteSensitive bool) {
	decryptActive = decryptContents
	deleteSensitive = decryptContents
	deleteActive = decryptContents && deleteEncryptedContents
	return decryptActive, deleteActive, deleteSensitive
}

func (mw *MainWindow) applyDownloadOptionState(decryptContents, deleteEncryptedContents bool) {
	decryptActive, deleteActive, deleteSensitive := downloadOptionCheckboxState(decryptContents, deleteEncryptedContents)

	mw.decryptContents = decryptActive
	mw.deleteEncryptedContents = deleteActive

	if mw.decryptContentsCheckbox != nil {
		setCheckButtonActiveWithoutSignal(mw.decryptContentsCheckbox, mw.decryptContentsToggleHandle, decryptActive)
	}
	if mw.deleteEncryptedContentsCheckbox != nil {
		mw.deleteEncryptedContentsCheckbox.SetSensitive(deleteSensitive)
		setCheckButtonActiveWithoutSignal(mw.deleteEncryptedContentsCheckbox, mw.deleteEncryptedContentsHandle, deleteActive)
	}
}

func setCheckButtonActiveWithoutSignal(button *gtk.CheckButton, handle glib.SignalHandle, active bool) {
	if button == nil || button.Active() == active {
		return
	}

	if handle == 0 {
		button.SetActive(active)
		return
	}

	handler := gtk.BaseWidget(button)
	handler.HandlerBlock(handle)
	defer handler.HandlerUnblock(handle)
	button.SetActive(active)
}

func (mw *MainWindow) collectRelatedCandidates(originals []wiiudownloader.TitleEntry) []wiiudownloader.TitleEntry {
	candidates := make([]wiiudownloader.TitleEntry, 0)
	exclude := make(map[uint64]struct{})

	for _, queued := range mw.queuePane.GetTitleQueue() {
		exclude[queued.TitleID] = struct{}{}
	}
	for _, original := range originals {
		exclude[original.TitleID] = struct{}{}
	}

	for _, original := range originals {
		high := wiiudownloader.GetTitleIDHigh(original.TitleID)
		targets := wiiudownloader.GetRelatedTypeTargets(high)
		for _, targetHigh := range targets {
			related, found := wiiudownloader.FindRelatedTitleByHighAndLow(original, targetHigh, exclude)
			if !found {
				continue
			}
			exclude[related.TitleID] = struct{}{}
			candidates = append(candidates, related)
		}
	}

	return candidates
}

// showRelatedTitlesDialog presents related-content candidates; onDone gets the
// accepted entries, or nil when skipped.
func (mw *MainWindow) showRelatedTitlesDialog(originals, candidates []wiiudownloader.TitleEntry, onDone func(chosen []wiiudownloader.TitleEntry)) {
	dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Add Related Content")
	dialog.SetDefaultSize(RELATED_DIALOG_WIDTH, RELATED_DIALOG_HEIGHT)

	contentArea := dialog.Content()

	headerLabel := gtk.NewLabel("")
	headerLabel.SetMarkup("<span font='14' weight='bold'>Related content found</span>")
	headerLabel.SetHAlign(gtk.AlignStart)
	contentArea.Append(headerLabel)

	descLabel := gtk.NewLabel(fmt.Sprintf("You added %d title(s). Select related Game/DLC/Update items to add to the queue.", len(originals)))
	descLabel.SetHAlign(gtk.AlignStart)
	descLabel.SetWrap(true)
	contentArea.Append(descLabel)

	scrolledWindow := gtk.NewScrolledWindow()
	scrolledWindow.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolledWindow.SetVExpand(true)
	contentArea.Append(scrolledWindow)

	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	listBox.SetActivateOnSingleClick(false)
	scrolledWindow.SetChild(listBox)

	type rowOption struct {
		entry *wiiudownloader.TitleEntry
		check *gtk.CheckButton
	}
	options := make([]rowOption, 0, len(candidates))

	for i := range candidates {
		candidate := candidates[i]

		row := gtk.NewListBoxRow()
		row.SetSelectable(false)

		outerContainer := gtk.NewBox(gtk.OrientationHorizontal, 0)
		outerContainer.SetMarginStart(RELATED_ROW_HORIZONTAL_MARGIN)
		outerContainer.SetMarginEnd(RELATED_ROW_HORIZONTAL_MARGIN)
		outerContainer.SetMarginTop(RELATED_ROW_VERTICAL_MARGIN)
		outerContainer.SetMarginBottom(RELATED_ROW_VERTICAL_MARGIN)
		outerContainer.SetSpacing(RELATED_ROW_SPACING)
		row.SetChild(outerContainer)

		check := gtk.NewCheckButtonWithLabel("")
		check.SetActive(true)
		check.SetVAlign(gtk.AlignStart)
		SetupCheckButtonAccessibility(check, fmt.Sprintf("Add %s", candidate.Name))
		outerContainer.Append(check)

		textBox := gtk.NewBox(gtk.OrientationVertical, 0)
		textBox.SetSpacing(2)
		textBox.SetHExpand(true)

		mainLabel := gtk.NewLabel("")
		mainLabel.SetMarkup(fmt.Sprintf("<span font='12' weight='600'>%s</span>", glib.MarkupEscapeText(candidate.Name)))
		mainLabel.SetHAlign(gtk.AlignStart)
		textBox.Append(mainLabel)

		subLabel := gtk.NewLabel("")
		subLabel.SetMarkup(fmt.Sprintf(
			"<span font='10' alpha='80%%'>%s | %s | %016x</span>",
			glib.MarkupEscapeText(wiiudownloader.GetFormattedKind(candidate.TitleID)),
			glib.MarkupEscapeText(wiiudownloader.GetFormattedRegion(candidate.Region)),
			candidate.TitleID,
		))
		subLabel.SetWrap(true)
		subLabel.SetHAlign(gtk.AlignStart)
		textBox.Append(subLabel)

		outerContainer.Append(textBox)
		listBox.Append(row)

		candidateCopy := candidate
		options = append(options, rowOption{entry: &candidateCopy, check: check})
	}

	dialog.AddButton("Skip", func() { onDone(nil) })
	dialog.AddActionButton("Add Selected", "suggested-action", func() {
		selected := make([]wiiudownloader.TitleEntry, 0, len(options))
		for _, option := range options {
			if option.check.Active() {
				selected = append(selected, *option.entry)
			}
		}
		onDone(selected)
	})
	dialog.Present()
}

func (mw *MainWindow) showError(err error) {
	if err == nil {
		return
	}
	uiIdleAdd(func() {
		if mw.progressWindow != nil && mw.progressWindow.Window != nil {
			mw.progressWindow.Window.SetVisible(false)
		}
		ShowErrorDialog(mw.window, err)
	})
}

func (mw *MainWindow) showErrorsDialog(errors []DownloadError) {
	dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Download Errors")
	dialog.SetDefaultSize(ERROR_DIALOG_WIDTH, ERROR_DIALOG_HEIGHT)

	contentArea := dialog.Content()

	headerLabel := gtk.NewLabel(fmt.Sprintf("The following %d title(s) failed to download:", len(errors)))
	contentArea.Append(headerLabel)

	scrolledWindow := gtk.NewScrolledWindow()
	scrolledWindow.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolledWindow.SetVExpand(true)
	contentArea.Append(scrolledWindow)

	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	scrolledWindow.SetChild(listBox)

	for _, dlErr := range errors {
		listBox.Append(newErrorOverviewRow(dlErr))
	}

	infoBox := gtk.NewBox(gtk.OrientationVertical, 2)
	infoBox.SetMarginTop(DIALOG_MARGIN)
	infoBox.SetMarginBottom(DIALOG_MARGIN)
	infoBox.SetHAlign(gtk.AlignCenter)

	serverLabel := gtk.NewLabel("")
	serverLabel.SetMarkup("<span size='small' alpha='70%'>Nintendo servers might be down.</span>")
	serverLabel.SetHAlign(gtk.AlignCenter)
	infoBox.Append(serverLabel)

	linkBtn := gtk.NewLinkButtonWithLabel("https://www.nintendo.co.jp/netinfo/en_US/index.html", "View Server Status")
	linkBtn.SetHAlign(gtk.AlignCenter)
	infoBox.Append(linkBtn)
	contentArea.Append(infoBox)

	dialog.AddActionButton("Add Failed to Queue", "warn-action", func() {
		mw.queuePane.Clear()
		var titles []wiiudownloader.TitleEntry
		for _, e := range errors {
			tid, err := strconv.ParseUint(e.TidStr, PARSE_UINT_BASE_16, PARSE_UINT_BITS_64)
			if err != nil {
				continue
			}
			entry := wiiudownloader.GetTitleEntryFromTid(tid)
			if entry.TitleID != 0 {
				entry.Version = e.Version
				titles = append(titles, entry)
			}
		}
		if len(titles) > 0 {
			mw.addTitlesToQueue(titles)
			mw.updateTitlesInQueue()
		}
	})
	dialog.AddButton("Close", nil)
	dialog.Present()
}

// newErrorOverviewRow renders one failure as title, optional type and message.
func newErrorOverviewRow(dlErr DownloadError) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()

	box := gtk.NewBox(gtk.OrientationVertical, 5)
	box.SetMarginTop(ERROR_ROW_MARGIN)
	box.SetMarginBottom(ERROR_ROW_MARGIN)
	box.SetMarginStart(ERROR_ROW_MARGIN)
	box.SetMarginEnd(ERROR_ROW_MARGIN)

	title := glib.MarkupEscapeText(dlErr.Title)
	if dlErr.TidStr != "" {
		title = fmt.Sprintf("%s [%s]", title, glib.MarkupEscapeText(dlErr.TidStr))
	}
	titleLabel := gtk.NewLabel("")
	titleLabel.SetMarkup(fmt.Sprintf("<b>%s</b>", title))
	titleLabel.SetXAlign(0)
	box.Append(titleLabel)

	if dlErr.ErrorType != "" {
		errorTypeLabel := gtk.NewLabel("")
		errorTypeLabel.SetMarkup(fmt.Sprintf("<i>Error Type: %s</i>", glib.MarkupEscapeText(dlErr.ErrorType)))
		errorTypeLabel.SetXAlign(0)
		box.Append(errorTypeLabel)
	}

	errorLabel := gtk.NewLabel(dlErr.Error)
	errorLabel.SetXAlign(0)
	errorLabel.SetWrap(true)
	errorLabel.SetWrapMode(pango.WrapWord)
	box.Append(errorLabel)

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	row.SetChild(box)
	return row
}

// showDecryptErrorsDialog lists the folders a decryption batch could not handle.
func (mw *MainWindow) showDecryptErrorsDialog(errors []DownloadError) {
	dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Decryption Errors")
	dialog.SetDefaultSize(ERROR_DIALOG_WIDTH, ERROR_DIALOG_HEIGHT)

	contentArea := dialog.Content()
	contentArea.Append(gtk.NewLabel(fmt.Sprintf("The following %d folder(s) could not be decrypted:", len(errors))))

	scrolledWindow := gtk.NewScrolledWindow()
	scrolledWindow.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolledWindow.SetVExpand(true)
	contentArea.Append(scrolledWindow)

	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	scrolledWindow.SetChild(listBox)
	for _, decryptErr := range errors {
		listBox.Append(newErrorOverviewRow(decryptErr))
	}

	dialog.AddButton("Close", nil)
	dialog.Present()
}

func (mw *MainWindow) showAddByTitleIDDialog() {
	dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Add by Title ID")

	contentArea := dialog.Content()

	label := gtk.NewLabel("Enter Title ID (16-character hex):")
	contentArea.Append(label)

	entry := gtk.NewEntry()
	entry.SetWidthChars(20)
	entry.SetActivatesDefault(true)
	contentArea.Append(entry)

	dialog.AddButton("Cancel", nil)
	dialog.AddActionButton("Add", "suggested-action", func() {
		tidStr := strings.TrimSpace(entry.Text())
		if len(tidStr) != 16 {
			ShowErrorDialog(mw.window, fmt.Errorf("invalid Title ID length: expected 16 characters, got %d", len(tidStr)))
			return
		}

		tid, err := strconv.ParseUint(tidStr, 16, 64)
		if err != nil {
			ShowErrorDialog(mw.window, fmt.Errorf("failed to parse Title ID: %v", err))
			return
		}

		titleEntry := wiiudownloader.GetTitleEntryFromTid(tid)
		if titleEntry.TitleID == 0 {
			// Not in database, create placeholder
			titleEntry = wiiudownloader.TitleEntry{
				Name:    tidStr,
				TitleID: tid,
				Region:  0, // Unknown
				Key:     uint8(wiiudownloader.TITLE_KEY_mypass),
			}
		}

		mw.addTitlesToQueue([]wiiudownloader.TitleEntry{titleEntry})
		mw.updateTitlesInQueue()
	})
	dialog.Present()
}
