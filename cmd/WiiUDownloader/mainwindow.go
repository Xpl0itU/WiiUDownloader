package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/Xpl0itU/dialog"
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

type MainWindow struct {
	window                          *gtk.Window
	queuePane                       *QueuePane
	treeView                        *gtk.TreeView
	searchEntry                     *gtk.Entry
	downloadQueueButton             *gtk.Button
	decryptContentsCheckbox         *gtk.CheckButton
	deleteEncryptedContentsCheckbox *gtk.CheckButton
	deleteEncryptedContents         bool
	progressWindow                  *ProgressWindow
	configWindow                    *ConfigWindow
	lastSearchText                  string
	categoryButtons                 []*gtk.ToggleButton
	titles                          []wiiudownloader.TitleEntry
	decryptContents                 bool
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
	win.SetDefaultSize(870, 400)
	win.SetDecorated(true)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.Connect("destroy", func() {
		os.Exit(0) // Hacky way to close the program
	})

	searchEntry, err := gtk.EntryNew()
	if err != nil {
		log.Fatalln("Unable to create entry:", err)
	}
	searchEntry.SetPlaceholderText("Search...")
	searchEntry.SetHExpand(true)
	searchEntry.SetWidthChars(24)

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
	mw.decryptContents = config.DecryptContents
	mw.deleteEncryptedContents = config.DeleteEncryptedContents
	mw.currentRegion = config.SelectedRegion
}

func (mw *MainWindow) BuildUI() {
	if mw.uiBuilt {
		return
	}
	mw.uiBuilt = true
	// Use OS-provided window decorations

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
		tidStr, _ := val.GetString()
		tid, _ := strconv.ParseUint(tidStr, 16, 64)

		nameVal, _ := model.GetValue(iter, NAME_COLUMN)
		nameStr, _ := nameVal.GetString()

		// 1. Category Filter
		if mw.currentCategory != wiiudownloader.TITLE_CATEGORY_ALL {
			kindVal, _ := model.GetValue(iter, KIND_COLUMN)
			kindStr, _ := kindVal.GetString()
			if kindStr != wiiudownloader.GetFormattedKind(tid) {
				// This shouldn't happen if data is consistent, but let's be safe
				return false
			}
			// Map category
			cat := wiiudownloader.GetCategoryFromFormattedCategory(kindStr)
			if cat != mw.currentCategory {
				return false
			}
		}

		// 2. Region Filter
		for _, t := range allTitles {
			if t.TitleID == tid {
				if (mw.currentRegion & t.Region) == 0 {
					return false
				}
				break
			}
		}

		// 3. Search Filter
		if mw.lastSearchText != "" {
			search := strings.ToLower(mw.lastSearchText)
			if !strings.Contains(strings.ToLower(nameStr), search) &&
				!strings.Contains(strings.ToLower(tidStr), search) {
				return false
			}
		}

		return true
	})

	// Create a sort model that wraps the filter model
	sortModel, err := gtk.TreeModelSortNew(mw.filterModel.ToTreeModel())
	if err != nil {
		log.Fatalln("Unable to create sort model:", err)
	}
	mw.sortModel = sortModel

	// Set sort column IDs for each column
	sortModel.SetSortColumnId(KIND_COLUMN, gtk.SORT_ASCENDING)
	sortModel.SetSortColumnId(TITLE_ID_COLUMN, gtk.SORT_ASCENDING)
	sortModel.SetSortColumnId(REGION_COLUMN, gtk.SORT_ASCENDING)
	sortModel.SetSortColumnId(NAME_COLUMN, gtk.SORT_ASCENDING)

	mw.treeView, err = gtk.TreeViewNewWithModel(sortModel)
	if err != nil {
		log.Fatalln("Unable to create tree view:", err)
	}
	mw.treeView.SetHeadersClickable(true)

	// Enable multiple selection
	selection, err := mw.treeView.GetSelection()
	if err != nil {
		log.Fatalln("Unable to get selection:", err)
	}
	selection.SetMode(gtk.SELECTION_MULTIPLE)

	toggleRenderer, err := gtk.CellRendererToggleNew()
	if err != nil {
		log.Fatalln("Unable to create cell renderer toggle:", err)
	}
	// on click, add or remove from queue
	toggleRenderer.Connect("toggled", func(renderer *gtk.CellRendererToggle, path string) {
		pathObj, err := gtk.TreePathNewFromString(path)
		if err != nil {
			log.Fatalln("Unable to create tree path:", err)
		}

		// Convert sort path to filter path
		filterPath := mw.sortModel.ConvertPathToChildPath(pathObj)
		if filterPath == nil {
			return
		}

		// Convert filter path to child path
		childPath := mw.filterModel.ConvertPathToChildPath(filterPath)
		if childPath == nil {
			return
		}

		// Get iter from child store
		iter, err := mw.childStore.ToTreeModel().GetIter(childPath)
		if err != nil {
			return
		}

		inQueueVal, err := mw.childStore.ToTreeModel().GetValue(iter, IN_QUEUE_COLUMN)
		if err != nil {
			log.Fatalln("Unable to get value:", err)
		}
		isInQueue, err := inQueueVal.GoValue()
		if err != nil {
			log.Fatalln("Unable to get value:", err)
		}

		// Get all selected rows
		sel, err := mw.treeView.GetSelection()
		if err != nil {
			return
		}

		// Get count of selected rows
		selectedCount := sel.CountSelectedRows()

		// If only one row is selected, toggle just that one
		// If multiple rows are selected, add/remove all of them together
		if selectedCount <= 1 {
			tid, err := mw.childStore.ToTreeModel().GetValue(iter, TITLE_ID_COLUMN)
			if err != nil {
				log.Fatalln("Unable to get value:", err)
			}
			tidStr, err := tid.GetString()
			if err != nil {
				log.Fatalln("Unable to get value:", err)
			}
			parsedTid, err := strconv.ParseUint(tidStr, 16, 64)
			if err != nil {
				log.Fatalln("Unable to parse title ID:", err)
			}
			if isInQueue.(bool) {
				mw.queuePane.RemoveTitle(wiiudownloader.TitleEntry{TitleID: parsedTid})
			} else {
				mw.queuePane.AddTitle(wiiudownloader.GetTitleEntryFromTid(parsedTid))
			}
		} else {
			// Multiple rows selected - toggle all of them
			paths := sel.GetSelectedRows(mw.sortModel)
			for l := paths; l != nil; l = l.Next() {
				data := l.Data()
				sortPath, ok := data.(*gtk.TreePath)
				if !ok || sortPath == nil {
					continue
				}

				filterPathMulti := mw.sortModel.ConvertPathToChildPath(sortPath)
				if filterPathMulti == nil {
					continue
				}

				childPathMulti := mw.filterModel.ConvertPathToChildPath(filterPathMulti)
				if childPathMulti == nil {
					continue
				}

				iterMulti, err := mw.childStore.ToTreeModel().GetIter(childPathMulti)
				if err != nil {
					continue
				}

				tidMulti, err := mw.childStore.ToTreeModel().GetValue(iterMulti, TITLE_ID_COLUMN)
				if err != nil {
					continue
				}
				tidStrMulti, err := tidMulti.GetString()
				if err != nil {
					continue
				}
				parsedTidMulti, err := strconv.ParseUint(tidStrMulti, 16, 64)
				if err != nil {
					continue
				}

				// Use the same state as the clicked row for all selected rows
				if isInQueue.(bool) {
					mw.queuePane.RemoveTitle(wiiudownloader.TitleEntry{TitleID: parsedTidMulti})
				} else {
					mw.queuePane.AddTitle(wiiudownloader.GetTitleEntryFromTid(parsedTidMulti))
				}
			}
		}

		mw.updateTitlesInQueue()
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

	mainvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}
	mainvBox.SetMarginTop(6)
	mainvBox.SetMarginBottom(6)
	mainvBox.SetMarginStart(6)
	mainvBox.SetMarginEnd(6)
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
		button.SetMode(false) // Make it look like a regular button
		buttonStyle, _ := button.GetStyleContext()
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

	mw.decryptContentsCheckbox, err = gtk.CheckButtonNewWithLabel("Decrypt contents")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	mw.decryptContentsCheckbox.SetActive(mw.decryptContents)

	mw.deleteEncryptedContentsCheckbox, err = gtk.CheckButtonNewWithLabel("Delete encrypted contents after decryption")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	mw.deleteEncryptedContentsCheckbox.SetSensitive(mw.decryptContents)
	mw.deleteEncryptedContentsCheckbox.SetActive(mw.deleteEncryptedContents)
	mw.deleteEncryptedContentsCheckbox.Connect("clicked", func() {
		config, err := loadConfig()
		if err != nil {
			return
		}
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

		var selectedPath string
		if config.RememberLastPath {
			if isValidPath(config.LastSelectedPath) {
				selectedPath = config.LastSelectedPath
			} else {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, fmt.Errorf("Saved download path not found: %s", config.LastSelectedPath))
				})
				// fall through to dialog
			}
		}
		if selectedPath == "" {
			if isValidPath(config.LastSelectedPath) {
				dialog.SetStartDir(config.LastSelectedPath)
			}
			chosen, err := dialog.Browse()
			if err != nil {
				glib.IdleAdd(func() {
					mw.progressWindow.Window.Hide()
				})
				return
			}
			selectedPath = chosen
			config.LastSelectedPath = chosen
			if err := config.Save(); err != nil {
				glib.IdleAdd(func() {
					ShowErrorDialog(mw.window, err)
				})
			}
		}

		mw.progressWindow.Window.ShowAll()

		// Capture state on main thread before spawning goroutine
		decryptContents := mw.decryptContents
		deleteEncryptedContents := mw.getDeleteEncryptedContents()

		go func() {
			// Disable the window while downloading to prevent multiple clicks
			glib.IdleAdd(func() {
				mw.treeView.SetSensitive(false)
				for _, button := range mw.categoryButtons {
					button.SetSensitive(false)
				}
				mw.searchEntry.SetSensitive(false)
				mw.downloadQueueButton.SetSensitive(false)
				mw.deleteEncryptedContentsCheckbox.SetSensitive(false)
				mw.decryptContentsCheckbox.SetSensitive(false)
				mw.queuePane.removeFromQueueButton.SetSensitive(false)
			})

			defer glib.IdleAdd(func() {
				mw.treeView.SetSensitive(true)
				for _, button := range mw.categoryButtons {
					button.SetSensitive(true)
				}
				mw.searchEntry.SetSensitive(true)
				mw.downloadQueueButton.SetSensitive(true)
				mw.deleteEncryptedContentsCheckbox.SetSensitive(true)
				mw.decryptContentsCheckbox.SetSensitive(true)
				mw.queuePane.removeFromQueueButton.SetSensitive(true)
			})

			if err := mw.onDownloadQueueClicked(selectedPath, decryptContents, deleteEncryptedContents, config); err != nil {
				glib.IdleAdd(func() {
					mw.showError(err)
				})
				return
			}

			// Show errors dialog if there were any errors
			errors := mw.progressWindow.GetErrors()
			if len(errors) > 0 {
				glib.IdleAdd(func() {
					mw.showErrorsDialog(errors)
				})
			}
		}()
	})
	mw.decryptContentsCheckbox.Connect("clicked", mw.onDecryptContentsClicked)
	bottomhBox.PackStart(mw.downloadQueueButton, false, false, 0)

	checkboxvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}
	checkboxvBox.PackStart(mw.decryptContentsCheckbox, false, false, 0)
	checkboxvBox.PackEnd(mw.deleteEncryptedContentsCheckbox, false, false, 0)

	bottomhBox.PackStart(checkboxvBox, false, false, 0)

	japanButton, err := gtk.CheckButtonNewWithLabel("Japan")
	japanButton.SetActive(mw.currentRegion&wiiudownloader.MCP_REGION_JAPAN != 0)
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	japanButton.Connect("clicked", func() {
		mw.onRegionChange(japanButton, wiiudownloader.MCP_REGION_JAPAN)
	})
	bottomhBox.PackEnd(japanButton, false, false, 0)

	usaButton, err := gtk.CheckButtonNewWithLabel("USA")
	usaButton.SetActive(mw.currentRegion&wiiudownloader.MCP_REGION_USA != 0)
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	usaButton.Connect("clicked", func() {
		mw.onRegionChange(usaButton, wiiudownloader.MCP_REGION_USA)
	})
	bottomhBox.PackEnd(usaButton, false, false, 0)

	europeButton, err := gtk.CheckButtonNewWithLabel("Europe")
	europeButton.SetActive(mw.currentRegion&wiiudownloader.MCP_REGION_EUROPE != 0)
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	europeButton.Connect("clicked", func() {
		mw.onRegionChange(europeButton, wiiudownloader.MCP_REGION_EUROPE)
	})
	bottomhBox.PackEnd(europeButton, false, false, 0)

	mainvBox.PackEnd(bottomhBox, false, false, 0)

	splitPane, err := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	if err != nil {
		log.Fatalln("Unable to create paned:", err)
	}
	splitPane.Pack1(mw.queuePane.GetContainer(), true, false)
	splitPane.Pack2(mainvBox, true, true)

	splitPane.SetMarginBottom(2)
	splitPane.SetMarginEnd(2)
	splitPane.SetMarginStart(2)
	splitPane.SetMarginTop(2)

	mw.window.Add(splitPane)

	splitPane.ShowAll()
}

func (mw *MainWindow) onRegionChange(button *gtk.CheckButton, region uint8) {
	if button.GetActive() {
		mw.currentRegion = region | mw.currentRegion
	} else {
		mw.currentRegion = region ^ mw.currentRegion
	}
	mw.filterModel.Refilter()
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

func (mw *MainWindow) onSearchEntryChanged() {
	if mw.searchTimer != nil {
		mw.searchTimer.Stop()
	}
	mw.searchTimer = time.AfterFunc(200*time.Millisecond, func() {
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

func (mw *MainWindow) onDecryptContentsMenuItemClicked(selectedPath string) error {
	err := wiiudownloader.DecryptContents(selectedPath, mw.progressWindow, false)

	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
		// Show decryption errors if ContinueOnError is enabled
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
	mw.decryptContents = !mw.decryptContents
	mw.deleteEncryptedContentsCheckbox.SetSensitive(mw.decryptContents)
	config, err := loadConfig()
	if err != nil {
		return
	}
	config.DecryptContents = mw.decryptContents
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
				tidNum, err := strconv.ParseUint(tidStr, 16, 64)
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
	dialog.SetDefaultSize(600, 400)

	contentArea, err := dialog.GetContentArea()
	if err != nil {
		return
	}

	// Create a label for the header
	headerLabel, err := gtk.LabelNew(fmt.Sprintf("The following %d title(s) failed to download:", len(errors)))
	if err != nil {
		return
	}
	headerLabel.SetMarginTop(10)
	headerLabel.SetMarginStart(10)
	contentArea.PackStart(headerLabel, false, false, 0)

	// Create a scrolled window
	scrolledWindow, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return
	}
	scrolledWindow.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)
	scrolledWindow.SetMarginStart(10)
	scrolledWindow.SetMarginEnd(10)
	contentArea.PackStart(scrolledWindow, true, true, 0)

	// Create a ListBox to display errors
	listBox, err := gtk.ListBoxNew()
	if err != nil {
		return
	}
	listBox.SetSelectionMode(gtk.SELECTION_NONE)
	scrolledWindow.Add(listBox)

	// Add each error to the list
	for _, dlErr := range errors {
		row, err := gtk.ListBoxRowNew()
		if err != nil {
			continue
		}

		box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
		if err != nil {
			continue
		}
		box.SetMarginTop(5)
		box.SetMarginBottom(5)
		box.SetMarginStart(5)
		box.SetMarginEnd(5)

		// Title
		titleLabel, err := gtk.LabelNew("")
		if err != nil {
			continue
		}
		titleLabel.SetMarkup(fmt.Sprintf("<b>%s</b> [%s]", escapeMarkup(dlErr.Title), dlErr.TidStr))
		titleLabel.SetXAlign(0)
		box.PackStart(titleLabel, false, false, 0)

		// Error Type
		if dlErr.ErrorType != "" {
			errorTypeLabel, err := gtk.LabelNew("")
			if err != nil {
				continue
			}
			errorTypeLabel.SetMarkup(fmt.Sprintf("<i>Error Type: %s</i>", escapeMarkup(dlErr.ErrorType)))
			errorTypeLabel.SetXAlign(0)
			box.PackStart(errorTypeLabel, false, false, 0)
		}

		// Error message
		errorLabel, err := gtk.LabelNew(dlErr.Error)
		if err != nil {
			continue
		}
		errorLabel.SetXAlign(0)
		errorLabel.SetLineWrap(true)
		errorLabel.SetLineWrapMode(pango.WRAP_WORD)
		box.PackStart(errorLabel, false, false, 0)

		// Separator
		separator, err := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
		if err != nil {
			continue
		}
		box.PackStart(separator, false, false, 0)

		row.Add(box)
		listBox.Add(row)
	}

	// Add close button
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

	// Clear errors and reset at the start of the queue
	mw.progressWindow.ResetTotalsAndErrors()

	mw.queuePane.ForEachRemoving(func(title wiiudownloader.TitleEntry) bool {
		// Check for cancellation before starting next download
		if mw.progressWindow.Cancelled() {
			return false
		}

		errGroup.Go(func() error {
			if mw.progressWindow.cancelled {
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
					// Record error but continue with next download
					queueStatusChan <- true
					return nil
				}
				// If not continuing on error, signal to remove from queue and stop processing
				queueStatusChan <- false
				return downloadErr
			}

			queueStatusChan <- true
			return nil
		})

		if err = errGroup.Wait(); err != nil {
			if mw.progressWindow.Cancelled() {
				err = nil               // Suppress error so it doesn't show popup
				queueStatusChan <- true // Processed (cancelled), so remove this one
				return <-queueStatusChan
			} else {
				// Error occurred and ContinueOnError is false, stop processing
				return <-queueStatusChan
			}
		} else {
			// No error from goroutine, read the status from channel
			return <-queueStatusChan
		}
	})

	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
		mw.updateTitlesInQueue()
	})

	return err
}
