package main

import (
	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	CONTEXT_MENU_BUTTON              = 3
	CONTEXT_MENU_QUEUE_ADD           = "Add to Queue"
	CONTEXT_MENU_QUEUE_REMOVE        = "Remove from Queue"
	CONTEXT_MENU_SPECIFIC_FILES      = "Download Specific Files…"
	CONTEXT_MENU_SET_VERSION         = "Set Title Version…"
	CONTEXT_MENU_COPY_ID             = "Copy Title ID"
	CONTEXT_MENU_ACTION_TOGGLE_QUEUE = "win.row-toggle-queue"
	CONTEXT_MENU_ACTION_FILES        = "win.row-specific-files"
	CONTEXT_MENU_ACTION_SET_VERSION  = "win.row-set-version"
	CONTEXT_MENU_ACTION_COPY_ID      = "win.row-copy-id"
)

// attachRowContextGesture puts a right-click handler on one cell's child widget.
// The controller lives on the cell, so the clicked row is known directly instead
// of being guessed from coordinates.
func (mw *MainWindow) attachRowContextGesture(widget gtk.Widgetter, item *gtk.ListItem) {
	gesture := gtk.NewGestureClick()
	gesture.SetButton(CONTEXT_MENU_BUTTON)
	gesture.ConnectPressed(func(_ int, x, y float64) {
		key := listItemKey(item)
		if key == "" {
			return
		}
		mw.showTitleRowMenu(widget, x, y, key)
	})
	gtk.BaseWidget(widget).AddController(gesture)
}

// contextRowEntry is the title the open context menu refers to.
func (mw *MainWindow) contextRowEntry() (wiiudownloader.TitleEntry, bool) {
	row := mw.titleRows[mw.contextRowKey]
	if row == nil {
		return wiiudownloader.TitleEntry{}, false
	}
	return row.entry, true
}

func (mw *MainWindow) showTitleRowMenu(anchor gtk.Widgetter, x, y float64, key string) {
	row := mw.titleRows[key]
	if row == nil {
		return
	}
	mw.contextRowKey = key
	mw.selectRowByKey(key)

	menu := mw.buildTitleRowMenu(row)

	if mw.titleRowMenu == nil {
		mw.titleRowMenu = gtk.NewPopoverMenuFromModel(menu)
		mw.titleRowMenu.SetAutohide(true)
		mw.titleRowMenu.SetHasArrow(true)
		mw.titleRowMenu.SetParent(mw.titleView)
	} else {
		mw.titleRowMenu.SetMenuModel(menu)
	}

	point := graphene.NewPointAlloc().Init(float32(x), float32(y))
	if mapped, ok := gtk.BaseWidget(anchor).ComputePoint(gtk.BaseWidget(mw.titleView), point); ok && mapped != nil {
		rect := gdk.NewRectangle(int(mapped.X()), int(mapped.Y()), 1, 1)
		mw.titleRowMenu.SetPointingTo(&rect)
	}
	mw.titleRowMenu.Popup()
}

// buildTitleRowMenu rebuilds the menu per open, so the queue item matches the
// row's current state.
func (mw *MainWindow) buildTitleRowMenu(row *titleRow) *gio.Menu {
	menu := gio.NewMenu()
	if row.inQueue {
		menu.Append(CONTEXT_MENU_QUEUE_REMOVE, CONTEXT_MENU_ACTION_TOGGLE_QUEUE)
	} else {
		menu.Append(CONTEXT_MENU_QUEUE_ADD, CONTEXT_MENU_ACTION_TOGGLE_QUEUE)
	}
	menu.Append(CONTEXT_MENU_SPECIFIC_FILES, CONTEXT_MENU_ACTION_FILES)
	menu.Append(CONTEXT_MENU_SET_VERSION, CONTEXT_MENU_ACTION_SET_VERSION)
	menu.Append(CONTEXT_MENU_COPY_ID, CONTEXT_MENU_ACTION_COPY_ID)
	return menu
}

// selectRowByKey makes the right-clicked row the selection, so the menu never
// acts on a different row than the one highlighted.
func (mw *MainWindow) selectRowByKey(key string) {
	if mw.titleSelection == nil || mw.titleSortModel == nil {
		return
	}
	if bits := mw.titleSelection.Selection(); bits != nil {
		for i := uint64(0); i < bits.Size(); i++ {
			if rowKey(mw.titleSortModel.Item(bits.Nth(uint(i)))) == key {
				return
			}
		}
	}
	for i := uint(0); i < mw.titleSortModel.NItems(); i++ {
		if rowKey(mw.titleSortModel.Item(i)) == key {
			mw.titleSelection.SelectItem(i, true)
			return
		}
	}
}
