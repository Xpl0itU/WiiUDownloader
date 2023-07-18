package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/sqweek/dialog"
)

const (
	NAME_COLUMN     = 0
	KIND_COLUMN     = 1
	TITLE_ID_COLUMN = 2
	REGION_COLUMN   = 3
)

type MainWindow struct {
	window          *gtk.Window
	treeView        *gtk.TreeView
	titles          []wiiudownloader.TitleEntry
	searchEntry     *gtk.Entry
	categoryButtons []*gtk.ToggleButton
	titleQueue      []wiiudownloader.TitleEntry
	progressWindow  wiiudownloader.ProgressWindow
}

func NewMainWindow(entries []wiiudownloader.TitleEntry) *MainWindow {
	gtk.Init(nil)

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}

	win.SetTitle("WiiUDownloaderGo")
	win.SetDefaultSize(400, 300)
	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	searchEntry, err := gtk.EntryNew()
	if err != nil {
		log.Fatal("Unable to create search entry:", err)
	}

	mainWindow := MainWindow{
		window:      win,
		titles:      entries,
		searchEntry: searchEntry,
	}

	searchEntry.Connect("changed", mainWindow.onSearchEntryChanged)

	return &mainWindow
}

func (mw *MainWindow) updateTitles(titles []wiiudownloader.TitleEntry) {
	store, err := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		log.Fatal("Unable to create list store:", err)
	}

	for _, entry := range titles {
		iter := store.Append()
		err = store.Set(iter,
			[]int{NAME_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN},
			[]interface{}{entry.Name, wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region)},
		)
		if err != nil {
			log.Fatal("Unable to set values:", err)
		}
	}
	mw.treeView.SetModel(store)
}

func (mw *MainWindow) ShowAll() {
	store, err := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		log.Fatal("Unable to create list store:", err)
	}

	for _, entry := range mw.titles {
		iter := store.Append()
		err = store.Set(iter,
			[]int{NAME_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN},
			[]interface{}{entry.Name, wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region)},
		)
		if err != nil {
			log.Fatal("Unable to set values:", err)
		}
	}

	mw.treeView, err = gtk.TreeViewNew()
	if err != nil {
		log.Fatal("Unable to create tree view:", err)
	}

	mw.treeView.SetModel(store)

	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		log.Fatal("Unable to create cell renderer:", err)
	}
	column, err := gtk.TreeViewColumnNewWithAttribute("Name", renderer, "text", NAME_COLUMN)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	renderer, err = gtk.CellRendererTextNew()
	if err != nil {
		log.Fatal("Unable to create cell renderer:", err)
	}
	column, err = gtk.TreeViewColumnNewWithAttribute("Kind", renderer, "text", KIND_COLUMN)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	renderer, err = gtk.CellRendererTextNew()
	if err != nil {
		log.Fatal("Unable to create cell renderer:", err)
	}
	column, err = gtk.TreeViewColumnNewWithAttribute("Title ID", renderer, "text", TITLE_ID_COLUMN)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Region", renderer, "text", REGION_COLUMN)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	mw.treeView.AppendColumn(column)

	mainvBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		log.Fatal("Unable to create box:", err)
	}
	tophBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		log.Fatal("Unable to create box:", err)
	}

	mw.categoryButtons = make([]*gtk.ToggleButton, 0)
	for _, cat := range []string{"Game", "Update", "DLC", "Demo", "All"} {
		button, err := gtk.ToggleButtonNewWithLabel(cat)
		if err != nil {
			log.Fatal("Unable to create toggle button:", err)
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
		log.Fatal("Unable to create scrolled window:", err)
	}
	scrollable.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrollable.Add(mw.treeView)

	mainvBox.PackStart(scrollable, true, true, 0)

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		log.Fatal("Unable to create box:", err)
	}

	addToQueueButton, err := gtk.ButtonNewWithLabel("Add to queue")
	if err != nil {
		log.Fatal("Unable to create button:", err)
	}

	downloadQueueButton, err := gtk.ButtonNewWithLabel("Download queue")
	if err != nil {
		log.Fatal("Unable to create button:", err)
	}

	addToQueueButton.Connect("clicked", mw.onAddToQueueClicked)
	downloadQueueButton.Connect("clicked", func() {
		mw.progressWindow, err = wiiudownloader.CreateProgressWindow(mw.window)
		if err != nil {
			return
		}
		mw.progressWindow.Window.ShowAll()
		go mw.onDownloadQueueClicked()
	})
	bottomhBox.PackStart(addToQueueButton, false, false, 0)
	bottomhBox.PackStart(downloadQueueButton, false, false, 0)

	mainvBox.PackEnd(bottomhBox, false, false, 0)

	mw.window.Add(mainvBox)

	mw.window.ShowAll()
}

func (mw *MainWindow) onSearchEntryChanged() {
	text, _ := mw.searchEntry.GetText()
	mw.filterTitles(text)
}

func (mw *MainWindow) filterTitles(filterText string) {
	store, err := mw.treeView.GetModel()
	if err != nil {
		log.Fatal("Unable to get tree view model:", err)
	}

	storeRef := store.(*gtk.ListStore)
	storeRef.Clear()

	for _, entry := range mw.titles {
		if strings.Contains(strings.ToLower(entry.Name), strings.ToLower(filterText)) ||
			strings.Contains(strings.ToLower(fmt.Sprintf("%016x", entry.TitleID)), strings.ToLower(filterText)) {
			iter := storeRef.Append()
			err := storeRef.Set(iter,
				[]int{NAME_COLUMN, KIND_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN},
				[]interface{}{entry.Name, wiiudownloader.GetFormattedKind(entry.TitleID), fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region)},
			)
			if err != nil {
				log.Fatal("Unable to set values:", err)
			}
		}
	}
}

func (mw *MainWindow) onCategoryToggled(button *gtk.ToggleButton) {
	category, _ := button.GetLabel()
	mw.updateTitles(wiiudownloader.GetTitleEntries(wiiudownloader.GetCategoryFromFormattedCategory(category)))
	for _, catButton := range mw.categoryButtons {
		catButton.SetActive(false)
	}
	button.Activate()
}

func (mw *MainWindow) addToQueue(tid string, name string) {
	titleID, err := strconv.ParseUint(tid, 16, 64)
	if err != nil {
		log.Fatal("Unable to parse title ID:", err)
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
		log.Fatal("Unable to get selection:", err)
	}
	model, iter, _ := selection.GetSelected()
	if iter != nil {
		tid, _ := model.ToTreeModel().GetValue(iter, TITLE_ID_COLUMN)
		name, _ := model.ToTreeModel().GetValue(iter, NAME_COLUMN)
		if tid != nil {
			if tidStr, err := tid.GetString(); err == nil {
				nameStr, _ := name.GetString()
				mw.addToQueue(tidStr, nameStr)
			}
		}
	}
}

func (mw *MainWindow) onDownloadQueueClicked() {
	var wg sync.WaitGroup

	selectedPath, err := dialog.Directory().Title("Select a path to save the games to").Browse()
	if err != nil {
		return
	}

	for _, title := range mw.titleQueue {
		wg.Add(1)

		go func(title wiiudownloader.TitleEntry, selectedPath string, progressWindow *wiiudownloader.ProgressWindow) {
			defer wg.Done()

			tidStr := fmt.Sprintf("%016x", title.TitleID)
			wiiudownloader.DownloadTitle(tidStr, fmt.Sprintf("%s/%s [%s]", selectedPath, title.Name, tidStr), true, progressWindow)
			mw.removeFromQueue(tidStr)
		}(title, selectedPath, &mw.progressWindow)

		wg.Wait()
	}
	mw.progressWindow.Window.Close()
}

func Main() {
	gtk.Main()
}
