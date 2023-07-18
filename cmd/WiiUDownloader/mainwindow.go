package main

import (
	"fmt"
	"log"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type MainWindow struct {
	window *gtk.Window
	titles []wiiudownloader.TitleEntry
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
			[]int{0, 1, 2},
			[]interface{}{entry.Name, fmt.Sprintf("%016x", entry.TitleID), wiiudownloader.GetFormattedRegion(entry.Region)},
		)
		if err != nil {
			log.Fatal("Unable to set values:", err)
		}
	}

	treeView, err := gtk.TreeViewNewWithModel(store)
	if err != nil {
		log.Fatal("Unable to create tree view:", err)
	}

	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		log.Fatal("Unable to create cell renderer:", err)
	}
	column, err := gtk.TreeViewColumnNewWithAttribute("Name", renderer, "text", 0)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	treeView.AppendColumn(column)

	renderer, err = gtk.CellRendererTextNew()
	if err != nil {
		log.Fatal("Unable to create cell renderer:", err)
	}
	column, err = gtk.TreeViewColumnNewWithAttribute("Title ID", renderer, "text", 1)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	treeView.AppendColumn(column)

	column, err = gtk.TreeViewColumnNewWithAttribute("Region", renderer, "text", 2)
	if err != nil {
		log.Fatal("Unable to create tree view column:", err)
	}
	treeView.AppendColumn(column)

	scrollable, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		log.Fatal("Unable to create scrolled window:", err)
	}
	scrollable.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrollable.Add(treeView)

	mw.window.Add(scrollable)

	mw.window.ShowAll()
}

func Main() {
	gtk.Main()
}
