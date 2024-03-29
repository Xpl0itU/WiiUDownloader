package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
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
	window                          *gtk.ApplicationWindow
	treeView                        *gtk.TreeView
	logger                          *wiiudownloader.Logger
	searchEntry                     *gtk.Entry
	deleteEncryptedContentsCheckbox *gtk.CheckButton
	addToQueueButton                *gtk.Button
	progressWindow                  *ProgressWindow
	lastSearchText                  string
	titleQueue                      []wiiudownloader.TitleEntry
	categoryButtons                 []*gtk.ToggleButton
	titles                          []wiiudownloader.TitleEntry
	decryptContents                 bool
	currentRegion                   uint8
	client                          *http.Client
}

func NewMainWindow(app *gtk.Application, entries []wiiudownloader.TitleEntry, logger *wiiudownloader.Logger, client *http.Client) *MainWindow {
	gSettings, err := gtk.SettingsGetDefault()
	if err != nil {
		logger.Error(err.Error())
	}
	gSettings.SetProperty("gtk-application-prefer-dark-theme", isDarkMode())

	win, err := gtk.ApplicationWindowNew(app)
	if err != nil {
		logger.Fatal("Unable to create window:", err)
	}

	win.SetTitle("WiiUDownloader")
	win.SetDefaultSize(716, 400)
	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	searchEntry, err := gtk.EntryNew()
	if err != nil {
		logger.Fatal("Unable to create search entry:", err)
	}

	mainWindow := MainWindow{
		window:         win,
		titles:         entries,
		searchEntry:    searchEntry,
		currentRegion:  wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA,
		logger:         logger,
		lastSearchText: "",
		client:         client,
	}

	searchEntry.Connect("changed", mainWindow.onSearchEntryChanged)

	return &mainWindow
}

func (mw *MainWindow) updateTitles(titles []wiiudownloader.TitleEntry) {
	store, err := gtk.ListStoreNew(glib.TYPE_BOOLEAN, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		mw.logger.Fatal("Unable to create list store:", err)
	}

	for _, entry := range titles {
		if (mw.currentRegion & entry.Region) == 0 {
			continue
		}
		iter := store.Append()
		if err := store.Set(iter,
			[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
			[]interface{}{mw.isTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
		); err != nil {
			mw.logger.Fatal("Unable to set values:", err)
		}
	}
	mw.treeView.SetModel(store)
}

func (mw *MainWindow) ShowAll() {
	store, err := gtk.ListStoreNew(glib.TYPE_BOOLEAN, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		mw.logger.Fatal("Unable to create list store:", err)
	}

	for _, entry := range mw.titles {
		if (mw.currentRegion & entry.Region) == 0 {
			continue
		}
		iter := store.Append()
		err = store.Set(iter,
			[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
			[]interface{}{mw.isTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
		)
		if err != nil {
			mw.logger.Fatal("Unable to set values:", err)
		}
	}

	mw.treeView, err = gtk.TreeViewNew()
	if err != nil {
		mw.logger.Fatal("Unable to create tree view:", err)
	}

	selection, err := mw.treeView.GetSelection()
	if err != nil {
		mw.logger.Fatal("Unable to get selection:", err)
	}
	selection.SetMode(gtk.SELECTION_MULTIPLE)

	mw.treeView.SetModel(store)

	toggleRenderer, err := gtk.CellRendererToggleNew()
	if err != nil {
		mw.logger.Fatal("Unable to create cell renderer toggle:", err)
	}
	column, err := gtk.TreeViewColumnNewWithAttribute("Queue", toggleRenderer, "active", IN_QUEUE_COLUMN)
	if err != nil {
		mw.logger.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		mw.logger.Fatal("Unable to create cell renderer:", err)
	}
	column, err = gtk.TreeViewColumnNewWithAttribute("Kind", renderer, "text", KIND_COLUMN)
	if err != nil {
		mw.logger.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	renderer, err = gtk.CellRendererTextNew()
	if err != nil {
		mw.logger.Fatal("Unable to create cell renderer:", err)
	}
	column, err = gtk.TreeViewColumnNewWithAttribute("Title ID", renderer, "text", TITLE_ID_COLUMN)
	if err != nil {
		mw.logger.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Region", renderer, "text", REGION_COLUMN)
	if err != nil {
		mw.logger.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	renderer, err = gtk.CellRendererTextNew()
	if err != nil {
		mw.logger.Fatal("Unable to create cell renderer:", err)
	}
	column, err = gtk.TreeViewColumnNewWithAttribute("Name", renderer, "text", NAME_COLUMN)
	if err != nil {
		mw.logger.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	mainvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		mw.logger.Fatal("Unable to create box:", err)
	}
	menuBar, err := gtk.MenuBarNew()
	if err != nil {
		mw.logger.Fatal("Unable to create menu bar:", err)
	}
	toolsSubMenu, err := gtk.MenuNew()
	if err != nil {
		mw.logger.Fatal("Unable to create menu:", err)
	}

	toolsMenu, err := gtk.MenuItemNewWithLabel("Tools")
	if err != nil {
		mw.logger.Fatal("Unable to create menu item:", err)
	}
	decryptContentsMenuItem, err := gtk.MenuItemNewWithLabel("Decrypt contents")
	if err != nil {
		mw.logger.Fatal("Unable to create menu item:", err)
	}
	decryptContentsMenuItem.Connect("activate", func() {
		mw.progressWindow, err = createProgressWindow(mw.window)
		if err != nil {
			return
		}
		selectedPath, err := dialog.Directory().Title("Select the game path").Browse()
		if err != nil {
			glib.IdleAdd(func() {
				if mw.progressWindow.Window.IsVisible() {
					mw.progressWindow.Window.Close()
				}
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
		mw.logger.Fatal("Unable to create menu item:", err)
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

		var contentCount uint16
		if err := binary.Read(bytes.NewReader(tmdData[478:480]), binary.BigEndian, &contentCount); err != nil {
			return
		}

		var titleVersion uint16
		if err := binary.Read(bytes.NewReader(tmdData[476:478]), binary.BigEndian, &titleVersion); err != nil {
			return
		}

		var titleID uint64
		if err := binary.Read(bytes.NewReader(tmdData[0x018C:0x0194]), binary.BigEndian, &titleID); err != nil {
			return
		}

		titleKey, err := wiiudownloader.GenerateKey(fmt.Sprintf("%016x", titleID))
		if err != nil {
			return
		}

		wiiudownloader.GenerateTicket(filepath.Join(parentDir, "title.tik"), titleID, titleKey, titleVersion)

		cert, err := wiiudownloader.GenerateCert(tmdData, contentCount, mw.progressWindow, http.DefaultClient, context.Background(), make([]byte, 0))
		if err != nil {
			return
		}

		certPath := filepath.Join(parentDir, "title.cert")
		certFile, err := os.Create(certPath)
		if err != nil {
			return
		}
		if err := binary.Write(certFile, binary.BigEndian, cert.Bytes()); err != nil {
			return
		}
		defer certFile.Close()
	})
	toolsSubMenu.Append(generateFakeTicketCert)

	toolsMenu.SetSubmenu(toolsSubMenu)
	menuBar.Append(toolsMenu)
	mainvBox.PackStart(menuBar, false, false, 0)
	tophBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		mw.logger.Fatal("Unable to create box:", err)
	}

	mw.categoryButtons = make([]*gtk.ToggleButton, 0)
	for _, cat := range []string{"Game", "Update", "DLC", "Demo", "All"} {
		button, err := gtk.ToggleButtonNewWithLabel(cat)
		if err != nil {
			mw.logger.Fatal("Unable to create toggle button:", err)
		}
		tophBox.PackStart(button, false, false, 0)
		button.Connect("pressed", mw.onCategoryToggled)
		buttonLabel, err := button.GetLabel()
		if err != nil {
			mw.logger.Fatal("Unable to get label:", err)
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
		mw.logger.Fatal("Unable to create scrolled window:", err)
	}
	scrollable.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	selection.Connect("changed", mw.onSelectionChanged)
	scrollable.Add(mw.treeView)

	mainvBox.PackStart(scrollable, true, true, 0)

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		mw.logger.Fatal("Unable to create box:", err)
	}

	mw.addToQueueButton, err = gtk.ButtonNewWithLabel("Add to queue")
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}

	downloadQueueButton, err := gtk.ButtonNewWithLabel("Download queue")
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}

	decryptContentsCheckbox, err := gtk.CheckButtonNewWithLabel("Decrypt contents")
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}

	mw.deleteEncryptedContentsCheckbox, err = gtk.CheckButtonNewWithLabel("Delete encrypted contents after decryption")
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}
	mw.deleteEncryptedContentsCheckbox.SetSensitive(false)

	mw.addToQueueButton.Connect("clicked", mw.onAddToQueueClicked)
	downloadQueueButton.Connect("clicked", func() {
		if len(mw.titleQueue) == 0 {
			return
		}
		mw.progressWindow, err = createProgressWindow(mw.window)
		if err != nil {
			return
		}
		selectedPath, err := dialog.Directory().Title("Select a path to save the games to").Browse()
		if err != nil {
			glib.IdleAdd(func() {
				if mw.progressWindow.Window.IsVisible() {
					mw.progressWindow.Window.Close()
				}
			})
			return
		}
		mw.progressWindow.Window.ShowAll()

		go func() {
			if err := mw.onDownloadQueueClicked(selectedPath); err != nil {
				glib.IdleAdd(func() {
					mw.showError(err)
				})
			}
		}()
	})
	decryptContentsCheckbox.Connect("clicked", mw.onDecryptContentsClicked)
	bottomhBox.PackStart(mw.addToQueueButton, false, false, 0)
	bottomhBox.PackStart(downloadQueueButton, false, false, 0)

	checkboxvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		mw.logger.Fatal("Unable to create box:", err)
	}
	checkboxvBox.PackStart(decryptContentsCheckbox, false, false, 0)
	checkboxvBox.PackEnd(mw.deleteEncryptedContentsCheckbox, false, false, 0)

	bottomhBox.PackStart(checkboxvBox, false, false, 0)

	japanButton, err := gtk.CheckButtonNewWithLabel("Japan")
	japanButton.SetActive(true)
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}
	japanButton.Connect("clicked", func() {
		mw.onRegionChange(japanButton, wiiudownloader.MCP_REGION_JAPAN)
	})
	bottomhBox.PackEnd(japanButton, false, false, 0)

	usaButton, err := gtk.CheckButtonNewWithLabel("USA")
	usaButton.SetActive(true)
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}
	usaButton.Connect("clicked", func() {
		mw.onRegionChange(usaButton, wiiudownloader.MCP_REGION_USA)
	})
	bottomhBox.PackEnd(usaButton, false, false, 0)

	europeButton, err := gtk.CheckButtonNewWithLabel("Europe")
	europeButton.SetActive(true)
	if err != nil {
		mw.logger.Fatal("Unable to create button:", err)
	}
	europeButton.Connect("clicked", func() {
		mw.onRegionChange(europeButton, wiiudownloader.MCP_REGION_EUROPE)
	})
	bottomhBox.PackEnd(europeButton, false, false, 0)

	mainvBox.PackEnd(bottomhBox, false, false, 0)

	mainvBox.SetMarginBottom(2)
	mainvBox.SetMarginEnd(2)
	mainvBox.SetMarginStart(2)
	mainvBox.SetMarginTop(2)

	mw.window.Add(mainvBox)

	mainvBox.ShowAll()
}

func (mw *MainWindow) onRegionChange(button *gtk.CheckButton, region uint8) {
	if button.GetActive() {
		mw.currentRegion = region | mw.currentRegion
	} else {
		mw.currentRegion = region ^ mw.currentRegion
	}
	mw.updateTitles(mw.titles)
	mw.filterTitles(mw.lastSearchText)
}

func (mw *MainWindow) onSearchEntryChanged() {
	text, err := mw.searchEntry.GetText()
	if err != nil {
		mw.logger.Fatal("Unable to get text:", err)
	}
	mw.lastSearchText = text
	mw.filterTitles(text)
}

func (mw *MainWindow) filterTitles(filterText string) {
	store, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get tree view model:", err)
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
				[]interface{}{mw.isTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
			); err != nil {
				mw.logger.Fatal("Unable to set values:", err)
			}
		}
	}
}

func (mw *MainWindow) onCategoryToggled(button *gtk.ToggleButton) {
	category, err := button.GetLabel()
	if err != nil {
		mw.logger.Fatal("Unable to get label:", err)
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
		if mw.progressWindow.Window.IsVisible() {
			mw.progressWindow.Window.Close()
		}
	})
	return err
}

func (mw *MainWindow) isSelectionInQueue() bool {
	selection, err := mw.treeView.GetSelection()
	if err != nil {
		mw.logger.Fatal("Unable to get selection:", err)
	}

	store, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get model:", err)
	}

	storeRef := store.(*gtk.ListStore)
	treeModel := storeRef.ToTreeModel()
	if treeModel == nil {
		return false
	}

	selectionSelected := selection.GetSelectedRows(treeModel)
	if selectionSelected == nil {
		return false
	}

	for i := uint(0); i < selectionSelected.Length(); i++ {
		path, ok := selectionSelected.Nth(i).Data().(*gtk.TreePath)
		if !ok {
			continue
		}

		iter, err := treeModel.GetIter(path)
		if err != nil {
			continue
		}

		inQueueVal, err := treeModel.GetValue(iter, IN_QUEUE_COLUMN)
		if err != nil {
			continue
		}
		isInQueue, err := inQueueVal.GoValue()
		if err != nil {
			continue
		}

		if !isInQueue.(bool) {
			return false
		}
	}

	return true
}

func (mw *MainWindow) onSelectionChanged() {
	if mw.isSelectionInQueue() {
		mw.addToQueueButton.SetLabel("Remove from queue")
	} else {
		mw.addToQueueButton.SetLabel("Add to queue")
	}
}

func (mw *MainWindow) onDecryptContentsClicked() {
	if mw.decryptContents {
		mw.decryptContents = false
		mw.deleteEncryptedContentsCheckbox.SetSensitive(false)
	} else {
		mw.decryptContents = true
		mw.deleteEncryptedContentsCheckbox.SetSensitive(true)
	}
}

func (mw *MainWindow) getDeleteEncryptedContents() bool {
	if mw.deleteEncryptedContentsCheckbox.GetSensitive() {
		if mw.deleteEncryptedContentsCheckbox.GetActive() {
			return true
		}
	}
	return false
}

func (mw *MainWindow) isTitleInQueue(title wiiudownloader.TitleEntry) bool {
	for _, entry := range mw.titleQueue {
		if entry.TitleID == title.TitleID {
			return true
		}
	}
	return false
}

func (mw *MainWindow) addToQueue(tid, name string) {
	titleID, err := strconv.ParseUint(tid, 16, 64)
	if err != nil {
		mw.logger.Fatal("Unable to parse title ID:", err)
	}
	mw.titleQueue = append(mw.titleQueue, wiiudownloader.TitleEntry{TitleID: titleID, Name: name})
}

func (mw *MainWindow) removeFromQueue(tid string) {
	for i, entry := range mw.titleQueue {
		if fmt.Sprintf("%016x", entry.TitleID) == tid {
			mw.titleQueue = append(mw.titleQueue[:i], mw.titleQueue[i+1:]...)
			return
		}
	}
}

func (mw *MainWindow) onAddToQueueClicked() {
	selection, err := mw.treeView.GetSelection()
	if err != nil {
		mw.logger.Fatal("Unable to get selection:", err)
	}

	store, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get model:", err)
	}

	storeRef := store.(*gtk.ListStore)
	treeModel := storeRef.ToTreeModel()
	if treeModel == nil {
		return
	}

	addToQueue := !mw.isSelectionInQueue()

	selectionSelected := selection.GetSelectedRows(treeModel)
	if selectionSelected == nil || selectionSelected.Length() == 0 {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			mw.logger.Fatal("Error updating model:", r)
		}
	}()

	iter, _ := treeModel.GetIterFirst()
	for iter != nil {
		isSelected := selection.IterIsSelected(iter)
		if isSelected {
			inQueueVal, err := treeModel.GetValue(iter, IN_QUEUE_COLUMN)
			if err != nil {
				continue
			}
			isInQueue, err := inQueueVal.GoValue()
			if err != nil {
				continue
			}

			if addToQueue != isInQueue.(bool) {
				inQueueVal.SetBool(addToQueue)

				tid, err := treeModel.GetValue(iter, TITLE_ID_COLUMN)
				if err != nil {
					continue
				}
				tidStr, err := tid.GetString()
				if err != nil {
					continue
				}

				if addToQueue {
					name, err := treeModel.GetValue(iter, NAME_COLUMN)
					if err != nil {
						continue
					}
					nameStr, err := name.GetString()
					if err != nil {
						continue
					}
					mw.addToQueue(tidStr, nameStr)
					name.Unset()
				} else {
					mw.removeFromQueue(tidStr)
				}

				storeRef.SetValue(iter, IN_QUEUE_COLUMN, addToQueue)
				tid.Unset()
			}
		}

		if !storeRef.IterNext(iter) {
			break
		}
	}

	if addToQueue {
		mw.addToQueueButton.SetLabel("Remove from queue")
	} else {
		mw.addToQueueButton.SetLabel("Add to queue")
	}
}

func (mw *MainWindow) updateTitlesInQueue() {
	store, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get tree view model:", err)
	}

	storeRef := store.(*gtk.ListStore)

	iter, ok := storeRef.GetIterFirst()
	if !ok {
		mw.logger.Fatal("Unable to get first iter:", err)
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
				isInQueue := mw.isTitleInQueue(wiiudownloader.TitleEntry{TitleID: tidNum})
				storeRef.SetValue(iter, IN_QUEUE_COLUMN, isInQueue)
				tid.Unset()
			}
		}
		if !storeRef.IterNext(iter) {
			break
		}
	}
}

func (mw *MainWindow) showError(err error) {
	glib.IdleAdd(func() {
		if mw.progressWindow.Window.IsVisible() {
			mw.progressWindow.Window.Close()
		}
	})
	errorDialog := gtk.MessageDialogNew(mw.window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, err.Error())
	errorDialog.Run()
	errorDialog.Destroy()
}

func (mw *MainWindow) onDownloadQueueClicked(selectedPath string) error {
	if len(mw.titleQueue) == 0 {
		return nil
	}

	var err error = nil

	queueStatusChan := make(chan bool, 1)
	defer close(queueStatusChan)
	errGroup := errgroup.Group{}

	queueCtx, cancel := context.WithCancel(context.Background())
	mw.progressWindow.cancelFunc = cancel
	defer mw.progressWindow.cancelFunc()

	for _, title := range mw.titleQueue {
		errGroup.Go(func() error {
			select {
			case <-queueCtx.Done():
				queueStatusChan <- true
				return nil
			default:
			}
			tidStr := fmt.Sprintf("%016x", title.TitleID)
			titlePath := filepath.Join(selectedPath, fmt.Sprintf("%s [%s] [%s]", normalizeFilename(title.Name), wiiudownloader.GetFormattedKind(title.TitleID), tidStr))
			if err := wiiudownloader.DownloadTitle(queueCtx, tidStr, titlePath, mw.decryptContents, mw.progressWindow, mw.getDeleteEncryptedContents(), mw.logger, mw.client); err != nil && err != context.Canceled {
				return err
			}

			queueStatusChan <- true
			return nil
		})

		if err = errGroup.Wait(); err != nil {
			queueStatusChan <- false
		}

		if !<-queueStatusChan {
			break
		}
	}

	mw.titleQueue = []wiiudownloader.TitleEntry{} // Clear the queue
	glib.IdleAdd(func() {
		if mw.progressWindow.Window.IsVisible() {
			mw.progressWindow.Window.Close()
		}
	})
	mw.updateTitlesInQueue()
	mw.onSelectionChanged()

	return err
}
