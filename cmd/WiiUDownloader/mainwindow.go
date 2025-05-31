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

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/Xpl0itU/dialog"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
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
	deleteEncryptedContentsCheckbox *gtk.CheckButton
	deleteEncryptedContents         bool
	progressWindow                  *ProgressWindow
	configWindow                    *ConfigWindow
	lastSearchText                  string
	categoryButtons                 []*gtk.ToggleButton
	titles                          []wiiudownloader.TitleEntry
	decryptContents                 bool
	currentRegion                   uint8
	client                          *http.Client
}

func NewMainWindow(entries []wiiudownloader.TitleEntry, client *http.Client, config *Config) *MainWindow {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatalln("Unable to create window:", err)
	}

	win.SetTitle("WiiUDownloader")
	win.SetDefaultSize(870, 400)
	win.Connect("destroy", func() {
		os.Exit(0) // Hacky way to close the program
	})

	searchEntry, err := gtk.EntryNew()
	if err != nil {
		log.Fatalln("Unable to create entry:", err)
	}
	searchEntry.SetPlaceholderText("Search...")
	searchEntry.SetHExpand(false)

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

	searchEntry.Connect("changed", mainWindow.onSearchEntryChanged)

	return &mainWindow
}

func (mw *MainWindow) SetApplicationForGTKWindow(app *gtk.Application) {
	mw.window.SetApplication(app)
}

func (mw *MainWindow) updateTitles(titles []wiiudownloader.TitleEntry) {
	store, err := gtk.ListStoreNew(glib.TYPE_BOOLEAN, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		log.Fatalln("Unable to create list store:", err)
	}

	for _, entry := range titles {
		if (mw.currentRegion & entry.Region) == 0 {
			continue
		}
		iter := store.Append()
		if err := store.Set(iter,
			[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
			[]interface{}{mw.queuePane.IsTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
		); err != nil {
			log.Fatalln("Unable to set values:", err)
		}
	}
	mw.treeView.SetModel(store)
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

func (mw *MainWindow) ShowAll() {
	store, err := gtk.ListStoreNew(glib.TYPE_BOOLEAN, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		log.Fatalln("Unable to create list store:", err)
	}

	for _, entry := range mw.titles {
		if (mw.currentRegion & entry.Region) == 0 {
			continue
		}
		iter := store.Append()
		err = store.Set(iter,
			[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
			[]interface{}{mw.queuePane.IsTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
		)
		if err != nil {
			log.Fatalln("Unable to set values:", err)
		}
	}

	mw.treeView, err = gtk.TreeViewNew()
	if err != nil {
		log.Fatalln("Unable to create tree view:", err)
	}

	selection, err := mw.treeView.GetSelection()
	if err != nil {
		log.Fatalln("Unable to get selection:", err)
	}
	selection.SetMode(gtk.SELECTION_MULTIPLE)

	mw.treeView.SetModel(store)

	toggleRenderer, err := gtk.CellRendererToggleNew()
	if err != nil {
		log.Fatalln("Unable to create cell renderer toggle:", err)
	}
	// on click, add or remove from queue
	toggleRenderer.Connect("toggled", func(renderer *gtk.CellRendererToggle, path string) {
		store, err := mw.treeView.GetModel()
		if err != nil {
			log.Fatalln("Unable to get model:", err)
		}
		pathObj, err := gtk.TreePathNewFromString(path)
		if err != nil {
			log.Fatalln("Unable to create tree path:", err)
		}
		iter, err := store.ToTreeModel().GetIter(pathObj)
		if err != nil {
			log.Fatalln("Unable to get iter:", err)
		}
		inQueueVal, err := store.ToTreeModel().GetValue(iter, IN_QUEUE_COLUMN)
		if err != nil {
			log.Fatalln("Unable to get value:", err)
		}
		isInQueue, err := inQueueVal.GoValue()
		if err != nil {
			log.Fatalln("Unable to get value:", err)
		}
		tid, err := store.ToTreeModel().GetValue(iter, TITLE_ID_COLUMN)
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
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Title ID", renderer, "text", TITLE_ID_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Region", renderer, "text", REGION_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Name", renderer, "text", NAME_COLUMN)
	if err != nil {
		log.Fatalln("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	mainvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}
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
		parentDir := filepath.Dir(tmdPath)
		tmdData, err := os.ReadFile(tmdPath)
		if err != nil {
			return
		}

		tmd, err := wiiudownloader.ParseTMD(tmdData)
		if err != nil {
			return
		}

		titleKey, err := wiiudownloader.GenerateKey(fmt.Sprintf("%016x", tmd.TitleID))
		if err != nil {
			return
		}

		wiiudownloader.GenerateTicket(filepath.Join(parentDir, "title.tik"), tmd.TitleID, titleKey, tmd.TitleVersion)

		if err := wiiudownloader.GenerateCert(tmd, filepath.Join(parentDir, "title.cert"), mw.progressWindow, http.DefaultClient); err != nil {
			return
		}
	})
	toolsSubMenu.Append(generateFakeTicketCert)

	toolsMenu.SetSubmenu(toolsSubMenu)
	menuBar.Append(toolsMenu)
	configSubMenu, err := gtk.MenuNew()
	if err != nil {
		log.Fatalln("Unable to create menu:", err)
	}
	configMenuOption, err := gtk.MenuItemNewWithLabel("Config")
	if err != nil {
		log.Fatalln("Unable to create menu item:", err)
	}
	configMenuOption.SetSubmenu(configSubMenu)
	configOption, err := gtk.MenuItemNewWithLabel("Config")
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
		mw.configWindow.Window.ShowAll()
	})
	configSubMenu.Append(configOption)
	menuBar.Append(configMenuOption)
	mainvBox.PackStart(menuBar, false, false, 0)
	tophBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}

	mw.categoryButtons = make([]*gtk.ToggleButton, 0)
	for _, cat := range []string{"Game", "Update", "DLC", "Demo", "All"} {
		button, err := gtk.ToggleButtonNewWithLabel(cat)
		if err != nil {
			log.Fatalln("Unable to create toggle button:", err)
		}
		tophBox.PackStart(button, false, false, 0)
		button.Connect("pressed", mw.onCategoryToggled)
		buttonLabel, err := button.GetLabel()
		if err != nil {
			log.Fatalln("Unable to get label:", err)
		}
		if buttonLabel == "Game" {
			button.SetActive(true)
		}
		mw.categoryButtons = append(mw.categoryButtons, button)
	}
	tophBox.PackEnd(mw.searchEntry, false, false, 0)

	mainvBox.PackStart(tophBox, false, false, 0)

	scrollable, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		log.Fatalln("Unable to create scrolled window:", err)
	}
	scrollable.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrollable.Add(mw.treeView)

	mainvBox.PackStart(scrollable, true, true, 0)

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}

	downloadQueueButton, err := gtk.ButtonNewWithLabel("Download Queue")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}

	decryptContentsCheckbox, err := gtk.CheckButtonNewWithLabel("Decrypt contents")
	if err != nil {
		log.Fatalln("Unable to create button:", err)
	}
	decryptContentsCheckbox.SetActive(mw.decryptContents)

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
			return
		}
	})

	downloadQueueButton.Connect("clicked", func() {
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

		if !config.RememberLastPath || !(isValidPath(config.LastSelectedPath)) {
			if config.LastSelectedPath != "" {
				dialog.SetStartDir(config.LastSelectedPath) // no need to check validity, startDir only works if it is a valid existing directory, else works as if not set
			}

			selectedPath, err := dialog.Browse()
			if err != nil {
				glib.IdleAdd(func() {
					mw.progressWindow.Window.Hide()
				})
				return
			}
			config.LastSelectedPath = selectedPath
			_ = config.Save() // ignore error on saving new path
		}

		mw.progressWindow.Window.ShowAll()

		go func() {
			if err := mw.onDownloadQueueClicked(config.LastSelectedPath); err != nil {
				glib.IdleAdd(func() {
					mw.showError(err)
				})
			}
		}()
	})
	decryptContentsCheckbox.Connect("clicked", mw.onDecryptContentsClicked)
	bottomhBox.PackStart(downloadQueueButton, false, false, 0)

	checkboxvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		log.Fatalln("Unable to create box:", err)
	}
	checkboxvBox.PackStart(decryptContentsCheckbox, false, false, 0)
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
	mw.updateTitles(mw.titles)
	mw.filterTitles(mw.lastSearchText)
	config, err := loadConfig()
	if err != nil {
		return
	}
	config.SelectedRegion = mw.currentRegion
	if err := config.Save(); err != nil {
		return
	}
}

func (mw *MainWindow) onSearchEntryChanged() {
	text, err := mw.searchEntry.GetText()
	if err != nil {
		log.Fatalln("Unable to get text:", err)
	}
	mw.lastSearchText = text
	mw.filterTitles(text)
}

func (mw *MainWindow) filterTitles(filterText string) {
	store, err := mw.treeView.GetModel()
	if err != nil {
		log.Fatalln("Unable to get tree view model:", err)
	}

	storeRef := store.(*gtk.ListStore)
	storeRef.Clear()

	for _, entry := range mw.titles {
		if strings.Contains(strings.ToLower(entry.Name), strings.ToLower(filterText)) ||
			strings.Contains(strings.ToLower(fmt.Sprintf("%016x", entry.TitleID)), strings.ToLower(filterText)) {
			if (mw.currentRegion & entry.Region) == 0 {
				continue
			}
			iter := storeRef.Append()
			if err := storeRef.Set(iter,
				[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
				[]interface{}{mw.queuePane.IsTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
			); err != nil {
				log.Fatalln("Unable to set values:", err)
			}
		}
	}
}

func (mw *MainWindow) onCategoryToggled(button *gtk.ToggleButton) {
	category, err := button.GetLabel()
	if err != nil {
		log.Fatalln("Unable to get label:", err)
	}
	mw.titles = wiiudownloader.GetTitleEntries(wiiudownloader.GetCategoryFromFormattedCategory(category))
	mw.updateTitles(mw.titles)
	mw.filterTitles(mw.lastSearchText)
	for _, catButton := range mw.categoryButtons {
		catButton.SetActive(false)
	}
	button.Activate()
}

func (mw *MainWindow) onDecryptContentsMenuItemClicked(selectedPath string) error {
	err := wiiudownloader.DecryptContents(selectedPath, mw.progressWindow, false)

	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
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
	store, err := mw.treeView.GetModel()
	if err != nil {
		log.Fatalln("Unable to get tree view model:", err)
	}

	storeRef := store.(*gtk.ListStore)

	iter, ok := storeRef.GetIterFirst()
	if !ok {
		log.Fatalln("Unable to get first iter:", err)
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
	errorDialog := gtk.MessageDialogNew(mw.window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, err.Error())
	errorDialog.Run()
	errorDialog.Destroy()
}

func (mw *MainWindow) onDownloadQueueClicked(selectedPath string) error {
	if mw.queuePane.IsQueueEmpty() {
		return nil
	}

	var err error = nil

	queueStatusChan := make(chan bool, 1)
	defer close(queueStatusChan)
	errGroup := errgroup.Group{}

	mw.queuePane.ForEachRemoving(func(title wiiudownloader.TitleEntry) {
		errGroup.Go(func() error {
			if mw.progressWindow.cancelled {
				queueStatusChan <- true
				return nil
			}
			tidStr := fmt.Sprintf("%016x", title.TitleID)
			titlePath := filepath.Join(selectedPath, fmt.Sprintf("%s [%s] [%s]", normalizeFilename(title.Name), wiiudownloader.GetFormattedKind(title.TitleID), tidStr))
			if err := wiiudownloader.DownloadTitle(tidStr, titlePath, mw.decryptContents, mw.progressWindow, mw.getDeleteEncryptedContents(), mw.client); err != nil && err != context.Canceled {
				return err
			}

			queueStatusChan <- true
			return nil
		})

		if err = errGroup.Wait(); err != nil {
			queueStatusChan <- false
		}

		if !<-queueStatusChan {
			return
		}
	})

	glib.IdleAdd(func() {
		mw.progressWindow.Window.Hide()
	})
	mw.updateTitlesInQueue()

	return err
}
