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
	container             *gtk.Box
	titleTreeView         *gtk.TreeView
	titleQueue            *Locked[[]wiiudownloader.TitleEntry]
	removeFromQueueButton *gtk.Button
	store                 *gtk.ListStore
	updateFunc            func()
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
		log.Println("Unable to get selection:", err)
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

	removeFromQueueButton, err := gtk.ButtonNewWithLabel("Remove from Queue")
	if err != nil {
		return nil, err
	}
	removeFromQueueButton.SetSizeRequest(-1, 42)

	queuePane := QueuePane{
		container:             queueVBox,
		titleTreeView:         titleTreeView,
		store:                 store,
		titleQueue:            NewLocked(make([]wiiudownloader.TitleEntry, 0)),
		removeFromQueueButton: removeFromQueueButton,
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
				log.Println("Error updating model:", r)
			}
		}()

		titlesToRemove := make([]uint64, 0)

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
				titlesToRemove = append(titlesToRemove, tidParsed)

				tid.Unset()
			}

			if !storeRef.IterNext(iter) {
				break
			}
		}

		// Perform removal in bulk or one by one, using the lock
		queuePane.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
			for _, tidToRemove := range titlesToRemove {
				for i, t := range *queue {
					if t.TitleID == tidToRemove {
						*queue = append((*queue)[:i], (*queue)[i+1:]...)
						break
					}
				}
			}
		})

		queuePane.Update(true)
	})
	queueVBox.PackEnd(removeFromQueueButton, false, false, 0)

	return &queuePane, nil
}

func (qp *QueuePane) AddTitle(title wiiudownloader.TitleEntry) {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		*queue = append(*queue, title)
	})
}

func (qp *QueuePane) RemoveTitle(title wiiudownloader.TitleEntry) {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		for i, t := range *queue {
			if t.TitleID == title.TitleID {
				*queue = append((*queue)[:i], (*queue)[i+1:]...)
				break
			}
		}
	})
}

func (qp *QueuePane) Clear() {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		*queue = make([]wiiudownloader.TitleEntry, 0)
	})
}

func (qp *QueuePane) GetContainer() *gtk.Box {
	return qp.container
}

func (qp *QueuePane) GetTitleQueue() []wiiudownloader.TitleEntry {
	var result []wiiudownloader.TitleEntry
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		result = make([]wiiudownloader.TitleEntry, len(queue))
		copy(result, queue)
	})
	return result
}

func (qp *QueuePane) IsQueueEmpty() bool {
	var empty bool
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		empty = len(queue) == 0
	})
	return empty
}

func (qp *QueuePane) GetTitleQueueSize() int {
	var size int
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		size = len(queue)
	})
	return size
}

func (qp *QueuePane) GetTitleQueueAtIndex(index int) wiiudownloader.TitleEntry {
	var entry wiiudownloader.TitleEntry
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		entry = queue[index]
	})
	return entry
}

func (qp *QueuePane) IsTitleInQueue(title wiiudownloader.TitleEntry) bool {
	var found bool
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		for _, t := range queue {
			if t.TitleID == title.TitleID {
				found = true
				break
			}
		}
	})
	return found
}

func (qp *QueuePane) ForEachRemoving(f func(wiiudownloader.TitleEntry) bool) {
	// Create a copy first to iterate safely without holding the lock during the callback (which might take time or call back)
	var titleQueueCopy []wiiudownloader.TitleEntry
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		titleQueueCopy = make([]wiiudownloader.TitleEntry, len(queue))
		copy(titleQueueCopy, queue)
	})

	for _, title := range titleQueueCopy {
		shouldContinue := f(title)
		if shouldContinue {
			qp.RemoveTitle(title)
		} else {
			break
		}
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

	// Snapshot for UI update
	var queueSnapshot []wiiudownloader.TitleEntry
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		queueSnapshot = make([]wiiudownloader.TitleEntry, len(queue))
		copy(queueSnapshot, queue)
	})

	for _, title := range queueSnapshot {
		iter := qp.store.Append()

		qp.store.Set(iter, []int{0, 1, 2}, []interface{}{title.Name, wiiudownloader.GetFormattedRegion(title.Region), fmt.Sprintf("%016x", title.TitleID)})
	}

	if qp.updateFunc != nil && doUpdateFunc {
		qp.updateFunc()
	}
}
