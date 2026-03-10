package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/Xpl0itU/dialog"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
	"golang.org/x/sync/errgroup"
)

const (
	IN_QUEUE_COLUMN = iota
	KIND_COLUMN
	TITLE_ID_COLUMN
	REGION_COLUMN
	NAME_COLUMN
)

const (
	MAIN_WINDOW_WIDTH             = 870
	MAIN_WINDOW_HEIGHT            = 400
	SEARCH_ENTRY_WIDTH_CHARS      = 24
	UI_MARGIN_SMALL               = 6
	SPLIT_PANE_MARGIN             = 2
	DOWNLOAD_PANE_MIN_WIDTH       = 400
	SEARCH_DEBOUNCE_DELAY         = 200 * time.Millisecond
	PARSE_UINT_BASE_16            = 16
	PARSE_UINT_BITS_64            = 64
	RELATED_DIALOG_WIDTH          = 620
	RELATED_DIALOG_HEIGHT         = 420
	ERROR_DIALOG_WIDTH            = 600
	ERROR_DIALOG_HEIGHT           = 400
	DIALOG_MARGIN                 = 10
	RELATED_ROW_HORIZONTAL_MARGIN = 16
	RELATED_ROW_VERTICAL_MARGIN   = 12
	RELATED_ROW_SPACING           = 12
	ERROR_ROW_MARGIN              = 5
)

type MainWindow struct {
	window                          *gtk.Window
	queuePane                       *QueuePane
	treeView                        *gtk.TreeView
	searchEntry                     *gtk.Entry
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
	configWindow                    *ConfigWindow
	lastSearchText                  string
	categoryButtons                 []*gtk.ToggleButton
	titles                          []wiiudownloader.TitleEntry
	decryptContents                 bool
	suggestRelatedContent           bool
	currentRegion                   uint8
	currentCategory                 uint8
	client                          *http.Client
	uiBuilt                         bool
	searchTimer                     *time.Timer
	filterModel                     *gtk.TreeModelFilter
	sortModel                       *gtk.TreeModelSort
	childStore                      *gtk.ListStore
}

func NewMainWindow(entries []wiiudownloader.TitleEntry, client *http.Client, config *Config) *MainWindow {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatalln("Unable to create window:", err)
	}

	win.SetTitle("WiiUDownloader")
	win.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
	win.SetDecorated(true)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.Connect("destroy", func() {
		os.Exit(0)
	})

	searchEntry, err := gtk.EntryNew()
	if err != nil {
		log.Fatalln("Unable to create entry:", err)
	}
	searchEntry.SetPlaceholderText("Search...")
	searchEntry.SetHExpand(true)
	searchEntry.SetWidthChars(SEARCH_ENTRY_WIDTH_CHARS)
	SetupEntryAccessibility(searchEntry, "Search titles", "Enter a game title or title ID to search. You can use the category buttons above to filter by type.")

	queuePane, err := NewQueuePane()
	if err != nil {
		log.Fatalln("Unable to create queue pane:", err)
	}

	mainWindow := MainWindow{
		window:         win,
		queuePane:      queuePane,
		titles:         entries,
		searchEntry:    searchEntry,
		currentRegion:  wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA,
		lastSearchText: "",
		client:         client,
	}

	queuePane.updateFunc = mainWindow.updateTitlesInQueue

	mainWindow.applyConfig(config)
	applyStyling()

	searchEntry.Connect("changed", mainWindow.onSearchEntryChanged)

	return &mainWindow
}

func (mw *MainWindow) SetApplicationForGTKWindow(app *gtk.Application) {
	mw.window.SetApplication(app)
}

func (mw *MainWindow) createConfigWindow(config *Config) error {
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
}

func (mw *MainWindow) BuildUI() {
	if mw.uiBuilt {
		return
	}
	mw.uiBuilt = true

	var err error
	mw.childStore, err = gtk.ListStoreNew(glib.TYPE_BOOLEAN, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		log.Fatalln("Unable to create list store:", err)
	}

	allTitles := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL)
	for _, entry := range allTitles {
		iter := mw.childStore.Append()
		err = mw.childStore.Set(iter,
			[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
			[]interface{}{mw.queuePane.IsTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
		)
		if err != nil {
			log.Fatalln("Unable to set values:", err)
		}
	}

	mw.filterModel, err = mw.childStore.ToTreeModel().FilterNew(nil)
	if err != nil {
		log.Fatalln("Unable to create filter model:", err)
	}

	mw.filterModel.SetVisibleFunc(func(model *gtk.TreeModel, iter *gtk.TreeIter) bool {
		val, err := model.GetValue(iter, TITLE_ID_COLUMN)
		if err != nil {
			return true
		}
		tidStr, err := val.GetString()
		if err != nil {
			return true
		}
		tid, err := strconv.ParseUint(tidStr, PARSE_UINT_BASE_16, PARSE_UINT_BITS_64)
		if err != nil {
			return true
		}

		nameVal, err := model.GetValue(iter, NAME_COLUMN)
		if err != nil {
			return true
		}
		nameStr, err := nameVal.GetString()
		if err != nil {
			return true
		}

		if mw.currentCategory != wiiudownloader.TITLE_CATEGORY_ALL {
			kindVal, err := model.GetValue(iter, KIND_COLUMN)
			if err != nil {
				return true
			}
			kindStr, err := kindVal.GetString()
			if err != nil {
				return true
			}
			if kindStr != wiiudownloader.GetFormattedKind(tid) {
				return false
			}
			cat := wiiudownloader.GetCategoryFromFormattedCategory(kindStr)
			if cat != mw.currentCategory {
				return false
			}
		}

		for _, t := range allTitles {
			if t.TitleID == tid {
				if (mw.currentRegion & t.Region) == 0 {
					return false
				}
				break
			}
		}

		if mw.lastSearchText != "" {
			if !titleMatchesSearch(mw.lastSearchText, nameStr, tidStr) {
				return false
			}
		}

		return true
	})

	sortModel, err := gtk.TreeModelSortNew(mw.filterModel.ToTreeModel())
	if err != nil {
		log.Fatalln("Unable to create sort model:", err)
	}
	mw.sortModel = sortModel

	sortModel.SetSortColumnId(KIND_COLUMN, gtk.SORT_ASCENDING)
	sortModel.SetSortColumnId(TITLE_ID_COLUMN, gtk.SORT_ASCENDING)
	sortModel.SetSortColumnId(REGION_COLUMN, gtk.SORT_ASCENDING)
	sortModel.SetSortColumnId(NAME_COLUMN, gtk.SORT_ASCENDING)

	mw.treeView, err = gtk.TreeViewNewWithModel(sortModel)
	if err != nil {
		log.Fatalln("Unable to create tree view:", err)
	}
	mw.treeView.SetHeadersClickable(true)

	selection, err := mw.treeView.GetSelection()
	if err != nil {
		log.Fatalln("Unable to get selection:", err)
	}
	selection.SetMode(gtk.SELECTION_MULTIPLE)

	toggleRenderer, err := gtk.CellRendererToggleNew()
	if err != nil {
		log.Fatalln("Unable to create cell renderer toggle:", err)
	}
	toggleRenderer.Connect("toggled", func(renderer *gtk.CellRendererToggle, path string) {
		pathObj, err := gtk.TreePathNewFromString(path)
		if err != nil {
			log.Fatalln("Unable to create tree path:", err)
		}
		mw.toggleQueueForSortPath(pathObj)
	})
	column, err := gtk.TreeViewColumnNewWithAttribute("Queue", toggleRenderer, "active", IN_QUEUE_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		log.Fatalln("Unable to create cell renderer:", err)
	}

	column, err = gtk.TreeViewColumnNewWithAttribute("Kind", renderer, "text", KIND_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	column.SetResizable(true)
	column.SetSortColumnID(KIND_COLUMN)
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Title ID", renderer, "text", TITLE_ID_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	column.SetResizable(true)
	column.SetSortColumnID(TITLE_ID_COLUMN)
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Region", renderer, "text", REGION_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	column.SetResizable(true)
	column.SetSortColumnID(REGION_COLUMN)
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Name", renderer, "text", NAME_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	column.SetResizable(true)
	column.SetSortColumnID(NAME_COLUMN)
	mw.treeView.AppendColumn(column)

	SetupTreeViewAccessibility(mw.treeView)
	mw.treeView.ToWidget().SetProperty("tooltip-text", "Game titles list. Use arrow keys to navigate, space or enter to toggle queue status for selected titles, or click checkboxes to add/remove titles.")
	mw.treeView.Connect("key-press-event", func(treeView *gtk.TreeView, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		if !isKeyboardActivationKey(keyEvent.KeyVal()) {
			return false
		}
		return mw.toggleQueueFromKeyboard()
	})
	mw.ensureTreeViewCursor()
	mw.window.SetFocusChild(mw.treeView.ToWidget())

	mainvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}
	mainvBox.SetMarginTop(UI_MARGIN_SMALL)
	mainvBox.SetMarginBottom(UI_MARGIN_SMALL)
	mainvBox.SetMarginStart(UI_MARGIN_SMALL)
	mainvBox.SetMarginEnd(UI_MARGIN_SMALL)
	menuBar, err := gtk.MenuBarNew()
	if err != nil {
		log.Fatalln("Unable to create menu bar:", err)
	}
	toolsSubMenu, err := gtk.MenuNew()
	if err != nil {
		log.Fatalln("Unable to create menu:", err)
	}

	toolsMenu, err := gtk.MenuItemNewWithLabel("Tools")
	if err != nil {
		log.Fatalln("Unable to create menu item:", err)
	}
	decryptContentsMenuItem, err := gtk.MenuItemNewWithLabel("Decrypt contents")
	if err != nil {
		log.Fatalln("Unable to create menu item:", err)
	}
	decryptContentsMenuItem.ToWidget().SetProperty("tooltip-text", "Decrypt contents - Select a game directory to decrypt its contents")
	decryptContentsMenuItem.Connect("activate", func() {
		mw.progressWindow, err = createProgressWindow(mw.window)
		if err != nil {
			return
		}
		selectedPath, err := dialog.Directory().Title("Select the game path").Browse()
		if err != nil {
			glib.IdleAdd(func() {
				mw.progressWindow.Window.Hide()
			})
			return
		}

		mw.progressWindow.Window.ShowAll()
		go func() {
			if err := mw.onDecryptContentsMenuItemClicked(selectedPath); err != nil {
				glib.IdleAdd(func() {
					mw.showError(err)
				})
			}
		}()
	})
	toolsSubMenu.Append(decryptContentsMenuItem)

	generateFakeTicketCert, err := gtk.MenuItemNewWithLabel("Generate fake ticket and cert")
	if err != nil {
		log.Fatalln("Unable to create menu item:", err)
	}
	generateFakeTicketCert.ToWidget().SetProperty("tooltip-text", "Generate fake ticket and cert - Create ticket and certificate files for a game")
	generateFakeTicketCert.Connect("activate", func() {
		tmdPath, err := dialog.File().Title("Select the game's tmd file").Filter("tmd", "tmd").Load()
		if err != nil {
			return
		}

		mw.progressWindow, err = createProgressWindow(mw.window)
		if err != nil {
			log.Printf("Failed to create progress window: %v", err)
			return
		}
		mw.progressWindow.Window.ShowAll()
		mw.progressWindow.SetGameTitle("Generating Ticket and Cert...")
		mw.progressWindow.ResetTotals()

		go func() {
			defer glib.IdleAdd(func() {
				mw.progressWindow.Window.Hide()
			})

			parentDir := filepath.Dir(tmdPath)
			tmdData, err := os.ReadFile(tmdPath)
			if err != nil {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, err)
				})
				return
			}

			tmd, err := wiiudownloader.ParseTMD(tmdData)
			if err != nil {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, err)
				})
				return
			}

			titleKey, err := wiiudownloader.GenerateKey(fmt.Sprintf("%016x", tmd.TitleID))
			if err != nil {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, err)
				})
				return
			}

			if err := wiiudownloader.GenerateTicket(filepath.Join(parentDir, "title.tik"), tmd.TitleID, titleKey, tmd.TitleVersion); err != nil {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, err)
				})
				return
			}

			if err := wiiudownloader.GenerateCert(tmd, filepath.Join(parentDir, "title.cert"), mw.progressWindow, http.DefaultClient); err != nil {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, err)
				})
				return
			}

			glib.IdleAdd(func() {
				infoDialog := gtk.MessageDialogNew(mw.window, gtk.DIALOG_MODAL, gtk.MESSAGE_INFO, gtk.BUTTONS_OK, "Successfully generated fake ticket and cert.")
				infoDialog.Run()
				infoDialog.Destroy()
			})
		}()
	})
	toolsSubMenu.Append(generateFakeTicketCert)

	toolsMenu.SetSubmenu(toolsSubMenu)
	menuBar.Append(toolsMenu)
	configSubMenu, err := gtk.MenuNew()
	if err != nil {
		log.Fatalln("Unable to create menu:", err)
	}
	configMenuOption, err := gtk.MenuItemNewWithLabel("Settings")
	if err != nil {
		log.Fatalln("Unable to create menu item:", err)
	}
	configMenuOption.SetSubmenu(configSubMenu)
	configOption, err := gtk.MenuItemNewWithLabel("Settings")
	if err != nil {
		log.Fatalln("Unable to create menu item:", err)
	}
	configOption.ToWidget().SetProperty("tooltip-text", "Settings - Configure download path and other preferences")
	configOption.Connect("activate", func() {
		config, err := loadConfig()
		if err != nil {
			return
		}
		if err := mw.createConfigWindow(config); err != nil {
			return
		}
		if mw.configWindow != nil && mw.window != nil {
			mw.configWindow.Window.SetTransientFor(mw.window)
			mw.configWindow.Window.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
			mw.configWindow.Window.SetDecorated(true)
		}
		mw.configWindow.Window.ShowAll()
	})
	configSubMenu.Append(configOption)
	menuBar.Append(configMenuOption)
	mainvBox.PackStart(menuBar, false, false, 0)
	tophBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}

	var firstRadio *gtk.RadioButton
	mw.categoryButtons = make([]*gtk.ToggleButton, 0)
	for _, cat := range []string{"Game", "Update", "DLC", "Demo", "All"} {
		var (
			button *gtk.RadioButton
			err    error
		)
		if firstRadio == nil {
			button, err = gtk.RadioButtonNewWithLabel(nil, cat)
			firstRadio = button
		} else {
			button, err = gtk.RadioButtonNewWithLabelFromWidget(firstRadio, cat)
		}
		if err != nil {
			log.Fatalln("Unable to create radio button:", err)
		}
		button.SetMode(false)
		buttonStyle, err := button.GetStyleContext()
		if err != nil {
			log.Fatalln("Unable to get button style context:", err)
		}
		if buttonStyle != nil {
			buttonStyle.AddClass("category-toggle")
		}
		tophBox.PackStart(button, false, false, 0)
		button.Connect("toggled", func() {
			mw.onCategoryToggled(&button.ToggleButton)
		})
		buttonLabel, err := button.GetLabel()
		if err != nil {
			log.Fatalln("Unable to get label:", err)
		}
		if buttonLabel == "Game" {
			button.SetActive(true)
		}
		SetupToggleButtonAccessibility(&button.ToggleButton, "Filter titles by category: "+cat)
		mw.categoryButtons = append(mw.categoryButtons, &button.ToggleButton)
	}
	tophBox.PackEnd(mw.searchEntry, true, true, 0)

	mainvBox.PackStart(tophBox, false, false, 0)

	scrollable, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		log.Fatalln("Unable to create scrolled window:", err)
	}
	scrollable.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrollable.Add(mw.treeView)

	mainvBox.PackStart(scrollable, true, true, 0)

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}

	mw.downloadQueueButton, err = gtk.ButtonNewWithLabel("Download Queue")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	mw.downloadQueueButton.SetCanDefault(true)
	mw.downloadQueueButton.GrabDefault()
	SetupButtonAccessibility(mw.downloadQueueButton, "Start downloading all titles in your queue. You can select multiple titles by clicking checkboxes and they will be added to your download queue.")

	mw.decryptContentsCheckbox, err = gtk.CheckButtonNewWithLabel("Decrypt contents")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	SetupCheckButtonAccessibility(mw.decryptContentsCheckbox, "When checked, downloaded game contents will be decrypted after download completes")

	mw.deleteEncryptedContentsCheckbox, err = gtk.CheckButtonNewWithLabel("Delete encrypted contents after decryption")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	SetupCheckButtonAccessibility(mw.deleteEncryptedContentsCheckbox, "When checked and decrypt contents is enabled, encrypted files will be deleted after successful decryption")
	mw.deleteEncryptedContentsHandle = mw.deleteEncryptedContentsCheckbox.Connect("toggled", func() {
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

	mw.downloadQueueButton.Connect("clicked", func() {
		if mw.queuePane.IsQueueEmpty() {
			return
		}
		mw.progressWindow, err = createProgressWindow(mw.window)
		if err != nil {
			return
		}
		dialog := dialog.Directory().Title("Select a path to save the games to")
		config, err := loadConfig()
		if err != nil {
			return
		}

		selectedPath, err := mw.resolveDownloadPath(config, dialog.SetStartDir, dialog.Browse)
		if err != nil {
			glib.IdleAdd(func() {
				mw.progressWindow.Window.Hide()
			})
			return
		}

		mw.progressWindow.Window.ShowAll()
		decryptContents := mw.decryptContents
		deleteEncryptedContents := mw.getDeleteEncryptedContents()

		go func() {
			glib.IdleAdd(func() {
				mw.setDownloadControlsSensitive(false)
			})

			defer glib.IdleAdd(func() {
				mw.setDownloadControlsSensitive(true)
			})

			runErr := mw.onDownloadQueueClicked(selectedPath, decryptContents, deleteEncryptedContents, config)
			if runErr != nil {
				glib.IdleAdd(func() {
					mw.showError(runErr)
				})
				return
			}

			errors := mw.progressWindow.GetErrors()
			if shouldShowQueueErrorSummary(runErr, errors) {
				glib.IdleAdd(func() {
					mw.showErrorsDialog(errors)
				})
			}
		}()
	})
	mw.decryptContentsToggleHandle = mw.decryptContentsCheckbox.Connect("toggled", mw.onDecryptContentsClicked)
	mw.applyDownloadOptionState(mw.decryptContents, mw.deleteEncryptedContents)
	bottomhBox.PackStart(mw.downloadQueueButton, false, false, 0)

	checkboxvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}
	checkboxvBox.PackStart(mw.decryptContentsCheckbox, false, false, 0)
	checkboxvBox.PackEnd(mw.deleteEncryptedContentsCheckbox, false, false, 0)

	bottomhBox.PackStart(checkboxvBox, false, false, 0)

	japanButton, err := gtk.CheckButtonNewWithLabel("Japan")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	mw.japanRegionCheckbox = japanButton
	mw.japanRegionToggleHandle = japanButton.Connect("toggled", func() {
		mw.onRegionChange(japanButton, wiiudownloader.MCP_REGION_JAPAN)
	})
	bottomhBox.PackEnd(japanButton, false, false, 0)

	usaButton, err := gtk.CheckButtonNewWithLabel("USA")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	mw.usaRegionCheckbox = usaButton
	mw.usaRegionToggleHandle = usaButton.Connect("toggled", func() {
		mw.onRegionChange(usaButton, wiiudownloader.MCP_REGION_USA)
	})
	bottomhBox.PackEnd(usaButton, false, false, 0)

	europeButton, err := gtk.CheckButtonNewWithLabel("Europe")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	mw.europeRegionCheckbox = europeButton
	mw.europeRegionToggleHandle = europeButton.Connect("toggled", func() {
		mw.onRegionChange(europeButton, wiiudownloader.MCP_REGION_EUROPE)
	})
	bottomhBox.PackEnd(europeButton, false, false, 0)
	mw.syncRegionCheckboxes()

	mainvBox.PackEnd(bottomhBox, false, false, 0)

	bottomhBox.SetSizeRequest(DOWNLOAD_PANE_MIN_WIDTH, -1)

	splitPane, err := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	if err != nil {
		log.Fatalln("Unable to create paned:", err)
	}
	splitPane.Pack1(mw.queuePane.GetContainer(), true, false)
	splitPane.Pack2(mainvBox, true, true)

	splitPane.SetMarginBottom(SPLIT_PANE_MARGIN)
	splitPane.SetMarginEnd(SPLIT_PANE_MARGIN)
	splitPane.SetMarginStart(SPLIT_PANE_MARGIN)
	splitPane.SetMarginTop(SPLIT_PANE_MARGIN)

	mw.window.Add(splitPane)

	splitPane.ShowAll()
}

func (mw *MainWindow) onRegionChange(button *gtk.CheckButton, region uint8) {
	mw.currentRegion = updateRegionMask(mw.currentRegion, region, button.GetActive())
	if mw.filterModel != nil {
		mw.filterModel.Refilter()
	}
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
	if mw.filterModel != nil {
		mw.filterModel.Refilter()
	}
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
		glib.IdleAdd(func() {
			text, err := mw.searchEntry.GetText()
			if err != nil {
				log.Printf("Unable to get text: %v", err)
				return
			}
			mw.lastSearchText = text
			mw.filterModel.Refilter()
		})
	})
}

func (mw *MainWindow) onCategoryToggled(button *gtk.ToggleButton) {
	if !button.GetActive() {
		return
	}
	category, err := button.GetLabel()
	if err != nil {
		log.Println("Unable to get label:", err)
		return
	}
	mw.currentCategory = wiiudownloader.GetCategoryFromFormattedCategory(category)
	glib.IdleAdd(func() {
		mw.filterModel.Refilter()
	})
}

func (mw *MainWindow) setDownloadControlsSensitive(sensitive bool) {
	mw.treeView.SetSensitive(sensitive)
	for _, button := range mw.categoryButtons {
		button.SetSensitive(sensitive)
	}
	mw.searchEntry.SetSensitive(sensitive)
	mw.downloadQueueButton.SetSensitive(sensitive)
	mw.deleteEncryptedContentsCheckbox.SetSensitive(sensitive)
	mw.decryptContentsCheckbox.SetSensitive(sensitive)
	mw.queuePane.removeFromQueueButton.SetSensitive(sensitive)
}

func (mw *MainWindow) resolveDownloadPath(config *Config, setStartDir func(string) *dialog.DirectoryBuilder, browse func() (string, error)) (string, error) {
	if config.RememberLastPath && isValidPath(config.LastSelectedPath) {
		return config.LastSelectedPath, nil
	}
	if isValidPath(config.LastSelectedPath) {
		setStartDir(config.LastSelectedPath)
	}
	chosen, err := browse()
	if err != nil {
		return "", err
	}
	config.LastSelectedPath = chosen
	if saveErr := config.Save(); saveErr != nil {
		glib.IdleAdd(func() {
			ShowErrorDialog(mw.window, saveErr)
		})
	}
	return chosen, nil
}

func shouldShowQueueErrorSummary(runErr error, errors []DownloadError) bool {
	return runErr == nil && len(errors) > 0
}

func (mw *MainWindow) onDecryptContentsMenuItemClicked(selectedPath string) error {
	err := wiiudownloader.DecryptContents(selectedPath, mw.progressWindow, false)

	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
		config, loadErr := loadConfig()
		if loadErr != nil {
			return
		}

		errors := mw.progressWindow.GetErrors()
		if len(errors) > 0 && config.ContinueOnError {
			mw.showErrorsDialog(errors)
		}
	})
	return err
}

func (mw *MainWindow) onDecryptContentsClicked() {
	mw.applyDownloadOptionState(mw.decryptContentsCheckbox.GetActive(), mw.getDeleteEncryptedContents())
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
	if mw.deleteEncryptedContentsCheckbox.GetSensitive() {
		return mw.deleteEncryptedContentsCheckbox.GetActive()
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
	if button == nil || button.GetActive() == active {
		return
	}

	if handle == 0 {
		button.SetActive(active)
		return
	}

	button.HandlerBlock(handle)
	defer button.HandlerUnblock(handle)
	button.SetActive(active)
}

func (mw *MainWindow) getTitleEntryFromChildIter(iter *gtk.TreeIter) (wiiudownloader.TitleEntry, bool) {
	tidVal, err := mw.childStore.ToTreeModel().GetValue(iter, TITLE_ID_COLUMN)
	if err != nil {
		return wiiudownloader.TitleEntry{}, false
	}
	tidStr, err := tidVal.GetString()
	if err != nil {
		return wiiudownloader.TitleEntry{}, false
	}
	parsedTid, err := strconv.ParseUint(tidStr, PARSE_UINT_BASE_16, PARSE_UINT_BITS_64)
	if err != nil {
		return wiiudownloader.TitleEntry{}, false
	}

	entry := wiiudownloader.GetTitleEntryFromTid(parsedTid)
	if entry.TitleID == 0 {
		return wiiudownloader.TitleEntry{}, false
	}
	return entry, true
}

func (mw *MainWindow) collectSelectedEntriesForToggle(clickedIter *gtk.TreeIter, selectedCount int) []wiiudownloader.TitleEntry {
	result := make([]wiiudownloader.TitleEntry, 0)
	seen := make(map[uint64]struct{})

	addIfUnique := func(entry wiiudownloader.TitleEntry) {
		if entry.TitleID == 0 {
			return
		}
		if _, exists := seen[entry.TitleID]; exists {
			return
		}
		seen[entry.TitleID] = struct{}{}
		result = append(result, entry)
	}

	if selectedCount <= 1 {
		entry, ok := mw.getTitleEntryFromChildIter(clickedIter)
		if ok {
			addIfUnique(entry)
		}
		return result
	}

	selection, err := mw.treeView.GetSelection()
	if err != nil {
		return result
	}

	paths := selection.GetSelectedRows(mw.sortModel)
	for l := paths; l != nil; l = l.Next() {
		data := l.Data()
		sortPath, ok := data.(*gtk.TreePath)
		if !ok || sortPath == nil {
			continue
		}

		filterPath := mw.sortModel.ConvertPathToChildPath(sortPath)
		if filterPath == nil {
			continue
		}

		childPath := mw.filterModel.ConvertPathToChildPath(filterPath)
		if childPath == nil {
			continue
		}

		iter, err := mw.childStore.ToTreeModel().GetIter(childPath)
		if err != nil {
			continue
		}

		entry, ok := mw.getTitleEntryFromChildIter(iter)
		if ok {
			addIfUnique(entry)
		}
	}

	if len(result) == 0 {
		entry, ok := mw.getTitleEntryFromChildIter(clickedIter)
		if ok {
			addIfUnique(entry)
		}
	}

	return result
}

func (mw *MainWindow) toggleQueueForSortPath(sortPath *gtk.TreePath) bool {
	if sortPath == nil {
		return false
	}

	filterPath := mw.sortModel.ConvertPathToChildPath(sortPath)
	if filterPath == nil {
		return false
	}

	childPath := mw.filterModel.ConvertPathToChildPath(filterPath)
	if childPath == nil {
		return false
	}

	iter, err := mw.childStore.ToTreeModel().GetIter(childPath)
	if err != nil {
		return false
	}

	inQueueVal, err := mw.childStore.ToTreeModel().GetValue(iter, IN_QUEUE_COLUMN)
	if err != nil {
		log.Fatalln("Unable to get value:", err)
	}
	isInQueue, err := inQueueVal.GoValue()
	if err != nil {
		log.Fatalln("Unable to get value:", err)
	}

	selection, err := mw.treeView.GetSelection()
	if err != nil {
		return false
	}

	selectedCount := selection.CountSelectedRows()
	selectedEntries := mw.collectSelectedEntriesForToggle(iter, selectedCount)

	if isInQueue.(bool) {
		for _, entry := range selectedEntries {
			mw.queuePane.RemoveTitle(entry)
		}
	} else {
		for _, entry := range selectedEntries {
			if !mw.queuePane.IsTitleInQueue(entry) {
				mw.queuePane.AddTitle(entry)
			}
		}

		if mw.suggestRelatedContent {
			candidates := mw.collectRelatedCandidates(selectedEntries)
			if len(candidates) > 0 {
				chosenRelated, accepted := mw.showRelatedTitlesDialog(selectedEntries, candidates)
				if accepted {
					for _, entry := range chosenRelated {
						if !mw.queuePane.IsTitleInQueue(entry) {
							mw.queuePane.AddTitle(entry)
						}
					}
				}
			}
		}
	}

	mw.updateTitlesInQueue()
	return true
}

func (mw *MainWindow) toggleQueueFromKeyboard() bool {
	if mw.treeView == nil || mw.sortModel == nil {
		return false
	}

	sortPath, _ := mw.treeView.GetCursor()
	if sortPath == nil {
		if !mw.ensureTreeViewCursor() {
			return false
		}
		sortPath, _ = mw.treeView.GetCursor()
	}
	if sortPath == nil {
		return false
	}

	selection, err := mw.treeView.GetSelection()
	if err != nil {
		return false
	}
	if selection.CountSelectedRows() <= 1 && !selection.PathIsSelected(sortPath) {
		selection.UnselectAll()
		selection.SelectPath(sortPath)
	}

	return mw.toggleQueueForSortPath(sortPath)
}

func (mw *MainWindow) ensureTreeViewCursor() bool {
	if mw.treeView == nil || mw.sortModel == nil {
		return false
	}

	sortPath, _ := mw.treeView.GetCursor()
	if sortPath != nil {
		return true
	}

	firstPath, err := gtk.TreePathNewFirst()
	if err != nil {
		return false
	}
	if _, err := mw.sortModel.ToTreeModel().GetIter(firstPath); err != nil {
		return false
	}

	mw.treeView.SetCursor(firstPath, nil, false)
	return true
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

func (mw *MainWindow) showRelatedTitlesDialog(originals, candidates []wiiudownloader.TitleEntry) ([]wiiudownloader.TitleEntry, bool) {
	dialog, err := gtk.DialogNew()
	if err != nil {
		log.Printf("Error creating related titles dialog: %v", err)
		return nil, false
	}
	defer dialog.Destroy()

	dialog.SetTitle("Add Related Content")
	dialog.SetModal(true)
	dialog.SetTransientFor(mw.window)
	dialog.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	dialog.SetDefaultSize(RELATED_DIALOG_WIDTH, RELATED_DIALOG_HEIGHT)
	SetupDialogAccessibility(dialog, "Add related content")

	dialog.AddButton("Skip", gtk.RESPONSE_CANCEL)
	dialog.AddButton("Add Selected", gtk.RESPONSE_ACCEPT)
	dialog.SetDefaultResponse(gtk.RESPONSE_ACCEPT)

	contentArea, err := dialog.GetContentArea()
	if err != nil {
		return nil, false
	}
	contentArea.SetSpacing(8)

	headerLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, false
	}
	headerLabel.SetMarkup("<span font='14' weight='bold'>Related content found</span>")
	headerLabel.SetHAlign(gtk.ALIGN_START)
	headerLabel.SetMarginTop(DIALOG_MARGIN)
	headerLabel.SetMarginStart(DIALOG_MARGIN)
	contentArea.PackStart(headerLabel, false, false, 0)

	descLabel, err := gtk.LabelNew(fmt.Sprintf("You added %d title(s). Select related Game/DLC/Update items to add to the queue.", len(originals)))
	if err != nil {
		return nil, false
	}
	descLabel.SetHAlign(gtk.ALIGN_START)
	descLabel.SetLineWrap(true)
	descLabel.SetMarginStart(DIALOG_MARGIN)
	descLabel.SetMarginEnd(DIALOG_MARGIN)
	contentArea.PackStart(descLabel, false, false, 0)

	scrolledWindow, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return nil, false
	}
	scrolledWindow.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)
	scrolledWindow.SetMarginStart(DIALOG_MARGIN)
	scrolledWindow.SetMarginEnd(DIALOG_MARGIN)
	scrolledWindow.SetMarginBottom(DIALOG_MARGIN)
	contentArea.PackStart(scrolledWindow, true, true, 0)

	listBox, err := gtk.ListBoxNew()
	if err != nil {
		return nil, false
	}
	listBox.SetSelectionMode(gtk.SELECTION_NONE)
	listBox.SetActivateOnSingleClick(false)
	scrolledWindow.Add(listBox)

	type rowOption struct {
		entry *wiiudownloader.TitleEntry
		check *gtk.CheckButton
	}
	options := make([]rowOption, 0, len(candidates))

	for i := range candidates {
		candidate := candidates[i]

		row, err := gtk.ListBoxRowNew()
		if err != nil {
			continue
		}
		row.SetSelectable(false)

		outerContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
		if err != nil {
			continue
		}
		outerContainer.SetMarginStart(RELATED_ROW_HORIZONTAL_MARGIN)
		outerContainer.SetMarginEnd(RELATED_ROW_HORIZONTAL_MARGIN)
		outerContainer.SetMarginTop(RELATED_ROW_VERTICAL_MARGIN)
		outerContainer.SetMarginBottom(RELATED_ROW_VERTICAL_MARGIN)
		outerContainer.SetSpacing(RELATED_ROW_SPACING)
		row.Add(outerContainer)

		check, err := gtk.CheckButtonNewWithLabel("")
		if err != nil {
			continue
		}
		check.SetActive(true)
		check.SetVAlign(gtk.ALIGN_START)
		SetupCheckButtonAccessibility(check, fmt.Sprintf("Add %s", candidate.Name))
		outerContainer.PackStart(check, false, false, 0)

		textBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
		if err != nil {
			continue
		}
		textBox.SetSpacing(2)

		mainLabel, err := gtk.LabelNew("")
		if err != nil {
			continue
		}
		mainLabel.SetMarkup(fmt.Sprintf("<span font='12' weight='600'>%s</span>", escapeMarkup(candidate.Name)))
		mainLabel.SetHAlign(gtk.ALIGN_START)
		textBox.PackStart(mainLabel, false, false, 0)

		subLabel, err := gtk.LabelNew("")
		if err != nil {
			continue
		}
		subLabel.SetMarkup(fmt.Sprintf(
			"<span font='10' alpha='80%%'>%s | %s | %016x</span>",
			escapeMarkup(wiiudownloader.GetFormattedKind(candidate.TitleID)),
			escapeMarkup(wiiudownloader.GetFormattedRegion(candidate.Region)),
			candidate.TitleID,
		))
		subLabel.SetLineWrap(true)
		subLabel.SetHAlign(gtk.ALIGN_START)
		textBox.PackStart(subLabel, false, false, 0)

		outerContainer.PackStart(textBox, true, true, 0)
		listBox.Add(row)

		candidateCopy := candidate
		options = append(options, rowOption{entry: &candidateCopy, check: check})
	}

	contentArea.ShowAll()
	response := dialog.Run()
	if response != gtk.RESPONSE_ACCEPT {
		return nil, false
	}

	selected := make([]wiiudownloader.TitleEntry, 0, len(options))
	for _, option := range options {
		if option.check.GetActive() {
			selected = append(selected, *option.entry)
		}
	}

	return selected, true
}

func (mw *MainWindow) updateTitlesInQueue() {
	if mw.childStore == nil {
		return
	}
	storeRef := mw.childStore

	iter, ok := storeRef.GetIterFirst()
	if !ok {
		log.Println("Unable to get first iter")
		return
	}
	for iter != nil {
		tid, err := storeRef.GetValue(iter, TITLE_ID_COLUMN)
		if err != nil {
			continue
		}
		if tid != nil {
			if tidStr, err := tid.GetString(); err == nil {
				tidNum, err := strconv.ParseUint(tidStr, PARSE_UINT_BASE_16, PARSE_UINT_BITS_64)
				if err != nil {
					continue
				}
				isInQueue := mw.queuePane.IsTitleInQueue(wiiudownloader.TitleEntry{TitleID: tidNum})
				storeRef.SetValue(iter, IN_QUEUE_COLUMN, isInQueue)
				tid.Unset()
			}
		}
		if !storeRef.IterNext(iter) {
			break
		}
	}
	mw.queuePane.Update(false)
}

func (mw *MainWindow) showError(err error) {
	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
	})
	errorDialog := gtk.MessageDialogNew(mw.window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", err.Error())
	errorDialog.Run()
	errorDialog.Destroy()
}

func (mw *MainWindow) showErrorsDialog(errors []DownloadError) {
	dialog, err := gtk.DialogNew()
	if err != nil {
		log.Printf("Error creating dialog: %v", err)
		return
	}
	defer dialog.Destroy()

	dialog.SetTitle("Download Errors")
	dialog.SetModal(true)
	dialog.SetTransientFor(mw.window)
	dialog.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	dialog.SetDefaultSize(ERROR_DIALOG_WIDTH, ERROR_DIALOG_HEIGHT)

	contentArea, err := dialog.GetContentArea()
	if err != nil {
		return
	}

	headerLabel, err := gtk.LabelNew(fmt.Sprintf("The following %d title(s) failed to download:", len(errors)))
	if err != nil {
		return
	}
	headerLabel.SetMarginTop(DIALOG_MARGIN)
	headerLabel.SetMarginStart(DIALOG_MARGIN)
	contentArea.PackStart(headerLabel, false, false, 0)

	scrolledWindow, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return
	}
	scrolledWindow.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)
	scrolledWindow.SetMarginStart(DIALOG_MARGIN)
	scrolledWindow.SetMarginEnd(DIALOG_MARGIN)
	contentArea.PackStart(scrolledWindow, true, true, 0)

	listBox, err := gtk.ListBoxNew()
	if err != nil {
		return
	}
	listBox.SetSelectionMode(gtk.SELECTION_NONE)
	scrolledWindow.Add(listBox)

	for _, dlErr := range errors {
		row, err := gtk.ListBoxRowNew()
		if err != nil {
			continue
		}

		box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
		if err != nil {
			continue
		}
		box.SetMarginTop(ERROR_ROW_MARGIN)
		box.SetMarginBottom(ERROR_ROW_MARGIN)
		box.SetMarginStart(ERROR_ROW_MARGIN)
		box.SetMarginEnd(ERROR_ROW_MARGIN)

		titleLabel, err := gtk.LabelNew("")
		if err != nil {
			continue
		}
		titleLabel.SetMarkup(fmt.Sprintf("<b>%s</b> [%s]", escapeMarkup(dlErr.Title), dlErr.TidStr))
		titleLabel.SetXAlign(0)
		box.PackStart(titleLabel, false, false, 0)

		if dlErr.ErrorType != "" {
			errorTypeLabel, err := gtk.LabelNew("")
			if err != nil {
				continue
			}
			errorTypeLabel.SetMarkup(fmt.Sprintf("<i>Error Type: %s</i>", escapeMarkup(dlErr.ErrorType)))
			errorTypeLabel.SetXAlign(0)
			box.PackStart(errorTypeLabel, false, false, 0)
		}

		errorLabel, err := gtk.LabelNew(dlErr.Error)
		if err != nil {
			continue
		}
		errorLabel.SetXAlign(0)
		errorLabel.SetLineWrap(true)
		errorLabel.SetLineWrapMode(pango.WRAP_WORD)
		box.PackStart(errorLabel, false, false, 0)

		separator, err := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
		if err != nil {
			continue
		}
		box.PackStart(separator, false, false, 0)

		row.Add(box)
		listBox.Add(row)
	}

	dialog.AddButton("Close", gtk.RESPONSE_OK)

	contentArea.ShowAll()
	dialog.Run()
}

func (mw *MainWindow) onDownloadQueueClicked(selectedPath string, decryptContents, deleteEncryptedContents bool, config *Config) error {
	if mw.queuePane.IsQueueEmpty() {
		return nil
	}

	var err error = nil

	queueStatusChan := make(chan bool, 1)
	defer close(queueStatusChan)
	errGroup := errgroup.Group{}

	mw.progressWindow.ResetTotalsAndErrors()

	mw.queuePane.ForEachRemoving(func(title wiiudownloader.TitleEntry) bool {
		if mw.progressWindow.Cancelled() {
			return false
		}

		errGroup.Go(func() error {
			if mw.progressWindow.Cancelled() {
				queueStatusChan <- true
				return nil
			}
			tidStr := fmt.Sprintf("%016x", title.TitleID)
			titlePath := filepath.Join(selectedPath, fmt.Sprintf("%s [%s] [%s]", normalizeFilename(title.Name), wiiudownloader.GetFormattedKind(title.TitleID), tidStr))
			downloadErr := wiiudownloader.DownloadTitle(tidStr, titlePath, decryptContents, mw.progressWindow, deleteEncryptedContents, mw.client)

			if downloadErr != nil && downloadErr != context.Canceled {
				errorType := detectErrorType(downloadErr.Error())
				mw.progressWindow.AddErrorWithType(title.Name, downloadErr.Error(), tidStr, errorType)

				if config.ContinueOnError {
					queueStatusChan <- true
					return nil
				}
				queueStatusChan <- false
				return downloadErr
			}

			queueStatusChan <- true
			return nil
		})

		if err = errGroup.Wait(); err != nil {
			if mw.progressWindow.Cancelled() {
				err = nil
				queueStatusChan <- true
				return <-queueStatusChan
			} else {
				return <-queueStatusChan
			}
		} else {
			return <-queueStatusChan
		}
	})

	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
		mw.updateTitlesInQueue()
	})

	return err
}
