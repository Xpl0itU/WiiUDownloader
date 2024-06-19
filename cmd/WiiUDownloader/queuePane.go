package main

import (
	"fmt"
	"log"
	"strconv"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type QueuePane struct {
	container     *gtk.Box
	titleTreeView *gtk.TreeView
	titleQueue    []wiiudownloader.TitleEntry
	store         *gtk.ListStore
	updateFunc    func()
}

func createColumn(renderer *gtk.CellRendererText, title string, id int) *gtk.TreeViewColumn {
	column, _ := gtk.TreeViewColumnNewWithAttribute(title, renderer, "text", id)
	return column
}

func NewQueuePane() (*QueuePane, error) {
	scrolledWindow, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return nil, err
	}
	scrolledWindow.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)

	store, err := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		return nil, err
	}

	titleTreeView, err := gtk.TreeViewNew()
	if err != nil {
		return nil, err
	}
	selection, err := titleTreeView.GetSelection()
	if err != nil {
		log.Fatalln("Unable to get selection:", err)
	}
	selection.SetMode(gtk.SELECTION_MULTIPLE)

	titleTreeView.SetModel(store)

	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		return nil, err
	}

	nameColumn := createColumn(renderer, "Name", 0)
	nameColumn.SetMaxWidth(200)
	nameColumn.SetResizable(true)
	titleTreeView.AppendColumn(nameColumn)
	regionColumn := createColumn(renderer, "Region", 1)
	regionColumn.SetMaxWidth(70)
	titleTreeView.AppendColumn(regionColumn)
	titleIDColumn := createColumn(renderer, "Title ID", 2)
	titleIDColumn.SetMaxWidth(125)
	titleTreeView.AppendColumn(titleIDColumn)
	titleTreeView.SetExpanderColumn(nameColumn)

	scrolledWindow.Add(titleTreeView)

	queueVBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	queueVBox.PackStart(scrolledWindow, true, true, 0)

	queuePane := QueuePane{
		container:     queueVBox,
		titleTreeView: titleTreeView,
		store:         store,
		titleQueue:    make([]wiiudownloader.TitleEntry, 0),
	}

	removeFromQueueButton, err := gtk.ButtonNewWithLabel("Remove from Queue")
	if err != nil {
		return nil, err
	}
	removeFromQueueButton.Connect("clicked", func() {
		selection, err := titleTreeView.GetSelection()
		if err != nil {
			return
		}

		store, err := titleTreeView.GetModel()
		if err != nil {
			return
		}

		storeRef := store.(*gtk.ListStore)
		treeModel := storeRef.ToTreeModel()
		if treeModel == nil {
			return
		}

		selectionSelected := selection.GetSelectedRows(treeModel)
		if selectionSelected == nil || selectionSelected.Length() == 0 {
			return
		}

		defer func() {
			if r := recover(); r != nil {
				log.Fatalln("Error updating model:", r)
			}
		}()

		iter, _ := treeModel.GetIterFirst()
		for iter != nil {
			isSelected := selection.IterIsSelected(iter)
			if isSelected {
				tid, err := treeModel.GetValue(iter, 2)
				if err != nil {
					continue
				}
				tidStr, err := tid.GetString()
				if err != nil {
					continue
				}
				tidParsed, err := strconv.ParseUint(tidStr, 16, 64)
				if err != nil {
					continue
				}
				queuePane.RemoveTitle(wiiudownloader.TitleEntry{TitleID: tidParsed})

				tid.Unset()
			}

			if !storeRef.IterNext(iter) {
				break
			}
		}
		queuePane.Update(true)
	})
	queueVBox.PackEnd(removeFromQueueButton, false, false, 0)

	return &queuePane, nil
}

func (qp *QueuePane) AddTitle(title wiiudownloader.TitleEntry) {
	qp.titleQueue = append(qp.titleQueue, title)
}

func (qp *QueuePane) RemoveTitle(title wiiudownloader.TitleEntry) {
	for i, t := range qp.titleQueue {
		if t.TitleID == title.TitleID {
			qp.titleQueue = append(qp.titleQueue[:i], qp.titleQueue[i+1:]...)
			break
		}
	}
}

func (qp *QueuePane) Clear() {
	qp.titleQueue = make([]wiiudownloader.TitleEntry, 0)
}

func (qp *QueuePane) GetContainer() *gtk.Box {
	return qp.container
}

func (qp *QueuePane) GetTitleQueue() []wiiudownloader.TitleEntry {
	return qp.titleQueue
}

func (qp *QueuePane) IsQueueEmpty() bool {
	return len(qp.titleQueue) == 0
}

func (qp *QueuePane) GetTitleQueueSize() int {
	return len(qp.titleQueue)
}

func (qp *QueuePane) GetTitleQueueAtIndex(index int) wiiudownloader.TitleEntry {
	return qp.titleQueue[index]
}

func (qp *QueuePane) IsTitleInQueue(title wiiudownloader.TitleEntry) bool {
	for _, t := range qp.titleQueue {
		if t.TitleID == title.TitleID {
			return true
		}
	}

	return false
}

func (qp *QueuePane) ForEachRemoving(f func(wiiudownloader.TitleEntry)) {
	titleQueueCopy := make([]wiiudownloader.TitleEntry, len(qp.titleQueue))
	copy(titleQueueCopy, qp.titleQueue)
	for _, title := range titleQueueCopy {
		f(title)
		qp.RemoveTitle(title)
	}
}

func (qp *QueuePane) GetTitleTreeView() *gtk.TreeView {
	return qp.titleTreeView
}

func (qp *QueuePane) SetTitleTreeView(titleTreeView *gtk.TreeView) {
	qp.titleTreeView = titleTreeView
}

func (qp *QueuePane) Update(doUpdateFunc bool) {
	qp.store.Clear()

	for _, title := range qp.titleQueue {
		iter := qp.store.Append()

		qp.store.Set(iter, []int{0, 1, 2}, []interface{}{title.Name, wiiudownloader.GetFormattedRegion(title.Region), fmt.Sprintf("%016x", title.TitleID)})
	}

	if qp.updateFunc != nil && doUpdateFunc {
		qp.updateFunc()
	}
}
