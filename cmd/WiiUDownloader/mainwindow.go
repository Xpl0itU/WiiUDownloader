package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
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
	treeView                        *gtk.TreeView
	titles                          []wiiudownloader.TitleEntry
	searchEntry                     *gtk.Entry
	categoryButtons                 []*gtk.ToggleButton
	titleQueue                      []wiiudownloader.TitleEntry
	progressWindow                  wiiudownloader.ProgressWindow
	addToQueueButton                *gtk.Button
	decryptContents                 bool
	currentRegion                   uint8
	deleteEncryptedContentsCheckbox *gtk.CheckButton
	logger                          *wiiudownloader.Logger
	lastSearchText                  string
}

func NewMainWindow(entries []wiiudownloader.TitleEntry, logger *wiiudownloader.Logger) *MainWindow {
	gtk.Init(nil)

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
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
		err = store.Set(iter,
			[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
			[]interface{}{mw.isTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
		)
		if err != nil {
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
	menuBar, _ := gtk.MenuBarNew()
	toolsSubMenu, _ := gtk.MenuNew()

	toolsMenu, _ := gtk.MenuItemNewWithLabel("Tools")
	decryptContentsMenuItem, _ := gtk.MenuItemNewWithLabel("Decrypt contents")
	decryptContentsMenuItem.Connect("activate", func() {
		mw.progressWindow, err = wiiudownloader.CreateProgressWindow(mw.window)
		if err != nil {
			return
		}
		dialog, err := gtk.FileChooserNativeDialogNew("Select the path to decrypt", mw.window, gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER, "Select", "Cancel")
		if err != nil {
			mw.logger.Fatal("Unable to create dialog:", err)
		}
		res := dialog.Run()
		if res != int(gtk.RESPONSE_ACCEPT) {
			mw.progressWindow.Window.Close()
			return
		}

		selectedPath := dialog.FileChooser.GetFilename()
		mw.progressWindow.Window.ShowAll()
		go func() {
			err := mw.onDecryptContentsMenuItemClicked(selectedPath)
			if err != nil {
				glib.IdleAdd(func() {
					mw.showError(err)
				})
			}
		}()
	})
	toolsSubMenu.Append(decryptContentsMenuItem)

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
			continue
		}
		tophBox.PackStart(button, false, false, 0)
		button.Connect("pressed", mw.onCategoryToggled)
		buttonLabel, _ := button.GetLabel()
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
		mw.progressWindow, err = wiiudownloader.CreateProgressWindow(mw.window)
		if err != nil {
			return
		}
		dialog, err := gtk.FileChooserNativeDialogNew("Select a path to save the games to", mw.window, gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER, "Select", "Cancel")
		if err != nil {
			mw.logger.Fatal("Unable to create dialog:", err)
		}
		res := dialog.Run()
		if res != int(gtk.RESPONSE_ACCEPT) {
			mw.progressWindow.Window.Close()
			return
		}

		selectedPath := dialog.FileChooser.GetFilename()
		mw.progressWindow.Window.ShowAll()

		go func() {
			err := mw.onDownloadQueueClicked(selectedPath)
			if err != nil {
				glib.IdleAdd(func() {
					mw.showError(err)
				})
			}
		}()
	})
	decryptContentsCheckbox.Connect("clicked", mw.onDecryptContentsClicked)
	bottomhBox.PackStart(mw.addToQueueButton, false, false, 0)
	bottomhBox.PackStart(downloadQueueButton, false, false, 0)

	checkboxvBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
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

	mw.window.ShowAll()
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
	text, _ := mw.searchEntry.GetText()
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
			err = storeRef.Set(iter,
				[]int{IN_QUEUE_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN, NAME_COLUMN},
				[]interface{}{mw.isTitleInQueue(entry), wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region), entry.Name},
			)
			if err != nil {
				mw.logger.Fatal("Unable to set values:", err)
			}
		}
	}
}

func (mw *MainWindow) onCategoryToggled(button *gtk.ToggleButton) {
	category, err := button.GetLabel()
	if err != nil {
		mw.logger.Fatal("Unable to get label:", err)
		return
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
	err := wiiudownloader.DecryptContents(selectedPath, &mw.progressWindow, false)

	mw.progressWindow.Window.Close()
	return err
}

func (mw *MainWindow) isSelectionInQueue() bool {
	selection, err := mw.treeView.GetSelection()
	if err != nil {
		mw.logger.Fatal("Unable to get selection:", err)
	}
	treeModel, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get model:", err)
	}
	allTitlesInQueue := true
	pathlist := selection.GetSelectedRows(treeModel)
	for l := pathlist; l != nil; l = l.Next() {
		path, ok := l.Data().(*gtk.TreePath)
		if !ok {
			continue
		}
		row, err := treeModel.ToTreeModel().GetIter(path)
		if err != nil {
			continue
		}
		if row != nil {
			inQueue, err := treeModel.ToTreeModel().GetValue(row, IN_QUEUE_COLUMN)
			if err != nil {
				continue
			}
			isInQueue, err := inQueue.GoValue()
			if err != nil {
				continue
			}
			if !isInQueue.(bool) {
				allTitlesInQueue = false
			}
			inQueue.Unset()
		}
	}
	return allTitlesInQueue
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

func (mw *MainWindow) addToQueue(tid string, name string) {
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
	treeModel, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get model:", err)
	}
	addToQueue := !mw.isSelectionInQueue()
	pathlist := selection.GetSelectedRows(treeModel)
	for l := pathlist; l != nil; l = l.Next() {
		path, ok := l.Data().(*gtk.TreePath)
		if !ok {
			continue
		}
		row, err := treeModel.ToTreeModel().GetIter(path)
		if err != nil {
			continue
		}
		if row != nil {
			inQueue, err := treeModel.ToTreeModel().GetValue(row, IN_QUEUE_COLUMN)
			if err != nil {
				continue
			}
			isInQueue, err := inQueue.GoValue()
			if err != nil {
				continue
			}
			if addToQueue == isInQueue.(bool) {
				continue
			}
			inQueue.SetBool(addToQueue)
			tid, err := treeModel.ToTreeModel().GetValue(row, TITLE_ID_COLUMN)
			if err != nil {
				continue
			}
			tidStr, err := tid.GetString()
			if err != nil {
				continue
			}
			if addToQueue {
				name, err := treeModel.ToTreeModel().GetValue(row, NAME_COLUMN)
				if err != nil {
					continue
				}
				nameStr, err := name.GetString()
				if err != nil {
					continue
				}
				mw.addToQueue(tidStr, nameStr)
				mw.addToQueueButton.SetLabel("Remove from queue")
				name.Unset()
			} else {
				mw.removeFromQueue(tidStr)
				mw.addToQueueButton.SetLabel("Add to queue")
			}
			inQueue.SetBool(addToQueue)
			queueModel, err := mw.treeView.GetModel()
			if err != nil {
				continue
			}
			queueModel.(*gtk.ListStore).SetValue(row, IN_QUEUE_COLUMN, addToQueue)
			inQueue.Unset()
			tid.Unset()
		}
	}
}

func (mw *MainWindow) updateTitlesInQueue() {
	store, err := mw.treeView.GetModel()
	if err != nil {
		mw.logger.Fatal("Unable to get tree view model:", err)
	}

	storeRef := store.(*gtk.ListStore)

	iter, _ := storeRef.GetIterFirst()
	for iter != nil {
		tid, _ := storeRef.GetValue(iter, TITLE_ID_COLUMN)
		if tid != nil {
			if tidStr, err := tid.GetString(); err == nil {
				tidNum, _ := strconv.ParseUint(tidStr, 16, 64)
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
	mw.progressWindow.Window.Close()
	errorDialog := gtk.MessageDialogNew(mw.window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, err.Error())
	errorDialog.Run()
	errorDialog.Destroy()
}

func (mw *MainWindow) onDownloadQueueClicked(selectedPath string) error {
	if len(mw.titleQueue) == 0 {
		return nil
	}

	queueStatusChan := make(chan bool, 1)
	errorChan := make(chan error, 1)

	var wg sync.WaitGroup

queueProcessingLoop:
	for _, title := range mw.titleQueue {
		wg.Add(1)

		go func(title wiiudownloader.TitleEntry, selectedPath string, progressWindow *wiiudownloader.ProgressWindow) {
			defer wg.Done()

			tidStr := fmt.Sprintf("%016x", title.TitleID)
			titlePath := fmt.Sprintf("%s/%s [%s] [%s]", selectedPath, normalizeFilename(title.Name), wiiudownloader.GetFormattedKind(title.TitleID), tidStr)
			if err := wiiudownloader.DownloadTitle(tidStr, titlePath, mw.decryptContents, progressWindow, mw.getDeleteEncryptedContents(), mw.logger); err != nil {
				errorChan <- err
				return
			}

			queueStatusChan <- true
		}(title, selectedPath, &mw.progressWindow)

		select {
		case err := <-errorChan:
			queueStatusChan <- false
			wg.Wait()
			mw.titleQueue = []wiiudownloader.TitleEntry{}
			mw.progressWindow.Window.Close()
			mw.updateTitlesInQueue()
			mw.onSelectionChanged()
			close(queueStatusChan)
			close(errorChan)
			return err

		case queueStatus := <-queueStatusChan:
			if !queueStatus {
				wg.Wait()
				break queueProcessingLoop
			}
		}
	}

	mw.titleQueue = []wiiudownloader.TitleEntry{} // Clear the queue
	mw.progressWindow.Window.Close()
	mw.updateTitlesInQueue()
	mw.onSelectionChanged()

	close(queueStatusChan)
	close(errorChan)

	return nil
}

func Main() {
	gtk.Main()
}
