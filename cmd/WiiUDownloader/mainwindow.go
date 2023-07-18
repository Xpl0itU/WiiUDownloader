package main

import (
	"fmt"
	"log"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	NAME_COLUMN     = 0
	TITLE_ID_COLUMN = 1
	REGION_COLUMN   = 2
)

type MainWindow struct {
	window   *gtk.Window
	treeView *gtk.TreeView
	titles   []wiiudownloader.TitleEntry
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

	return &MainWindow{
		window: win,
		titles: entries,
	}
}

func (mw *MainWindow) ShowAll() {
	store, err := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		log.Fatal("Unable to create list store:", err)
	}

	for _, entry := range mw.titles {
		iter := store.Append()
		err = store.Set(iter,
			[]int{NAME_COLUMN, TITLE_ID_COLUMN, REGION_COLUMN},
			[]interface{}{entry.Name, fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region)},
		)
		if err != nil {
			log.Fatal("Unable to set values:", err)
		}
	}

	mw.treeView, err = gtk.TreeViewNewWithModel(store)
	if err != nil {
		log.Fatal("Unable to create tree view:", err)
	}

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

	mw.treeView.Connect("row-activated", mw.onRowActivated)

	scrollable, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		log.Fatal("Unable to create scrolled window:", err)
	}
	scrollable.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrollable.Add(mw.treeView)

	mw.window.Add(scrollable)

	mw.window.ShowAll()
}

func (mw *MainWindow) onRowActivated() {
	selection, err := mw.treeView.GetSelection()
	if err != nil {
		log.Fatal("Unable to get selection:", err)
	}

	model, iter, _ := selection.GetSelected()
	if iter != nil {
		tid, _ := model.ToTreeModel().GetValue(iter, TITLE_ID_COLUMN)
		if tid != nil {
			if tidStr, err := tid.GetString(); err == nil {
				fmt.Println("Cell Value:", tidStr)
				progressWindow, err := wiiudownloader.CreateProgressWindow(mw.window)
				if err != nil {
					return
				}
				progressWindow.Window.ShowAll()
				go wiiudownloader.DownloadTitle(tidStr, "output", true, &progressWindow)
			}
		}
	}
}

func Main() {
	gtk.Main()
}
