package main

import (
	"fmt"
	"log"
	"strconv"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	QUEUE_NAME_COLUMN_MAX_WIDTH   = 200
	QUEUE_REGION_COLUMN_MAX_WIDTH = 70
	QUEUE_KIND_COLUMN_MAX_WIDTH   = 90
	QUEUE_SIZE_COLUMN_MAX_WIDTH   = 100
	QUEUE_BUTTON_HEIGHT           = 42
	TID_BASE_16                   = 16
	TID_BITS_64                   = 64
)

type QueuePane struct {
	container             *gtk.Box
	titleTreeView         *gtk.TreeView
	titleQueue            *Locked[[]wiiudownloader.TitleEntry]
	removeFromQueueButton *gtk.Button
	downloadButton        *gtk.Button
	totalSizeLabel        *gtk.Label
	store                 *gtk.ListStore
	titleSizes            map[uint64]string
	titleBytes            map[uint64]uint64
	updateFunc            func()
}




func createColumn(renderer *gtk.CellRendererText, title string, id int) (*gtk.TreeViewColumn, error) {
	return gtk.TreeViewColumnNewWithAttribute(title, renderer, "text", id)
}

func NewQueuePane() (*QueuePane, error) {
	scrolledWindow, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return nil, err
	}
	scrolledWindow.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)

	store, err := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		return nil, err
	}


	titleTreeView, err := gtk.TreeViewNew()
	if err != nil {
		return nil, err
	}
	selection, err := titleTreeView.GetSelection()
	if err != nil {
		return nil, err
	}
	selection.SetMode(gtk.SELECTION_MULTIPLE)
	SetupTreeViewAccessibility(titleTreeView)
	titleTreeView.ToWidget().SetProperty("tooltip-text", "Download queue - Shows games queued for download. Use arrow keys to navigate, space to select/deselect, or click Remove from Queue button to remove selected titles")

	titleTreeView.SetModel(store)

	renderer, err := gtk.CellRendererTextNew()
	if err != nil {
		return nil, err
	}

	nameColumn, err := createColumn(renderer, "Name", 0)
	if err != nil {
		return nil, err
	}
	nameColumn.SetExpand(true)
	nameColumn.SetResizable(true)
	titleTreeView.AppendColumn(nameColumn)
	regionColumn, err := createColumn(renderer, "Region", 1)
	if err != nil {
		return nil, err
	}
	regionColumn.SetMaxWidth(QUEUE_REGION_COLUMN_MAX_WIDTH)
	titleTreeView.AppendColumn(regionColumn)
	kindColumn, err := createColumn(renderer, "Kind", 2)
	if err != nil {
		return nil, err
	}
	kindColumn.SetMaxWidth(QUEUE_KIND_COLUMN_MAX_WIDTH)
	titleTreeView.AppendColumn(kindColumn)

	sizeColumn, err := createColumn(renderer, "Size", 4)
	if err != nil {
		return nil, err
	}
	sizeColumn.SetMaxWidth(QUEUE_SIZE_COLUMN_MAX_WIDTH)
	titleTreeView.AppendColumn(sizeColumn)

	titleTreeView.SetExpanderColumn(nameColumn)


	scrolledWindow.Add(titleTreeView)

	queueVBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	queueVBox.PackStart(scrolledWindow, true, true, 0)

	removeFromQueueButton, err := gtk.ButtonNew()
	if err != nil {
		return nil, err
	}
	removeLabel, err := gtk.LabelNew("Remove Selected\nfrom Queue")
	if err == nil {
		removeLabel.SetJustify(gtk.JUSTIFY_CENTER)
		removeFromQueueButton.Add(removeLabel)
	}
	removeFromQueueButton.SetSizeRequest(-1, QUEUE_BUTTON_HEIGHT)

	SetupButtonAccessibility(removeFromQueueButton, "Remove selected titles from the download queue")
	addStyleClass(removeFromQueueButton.GetStyleContext, "remove-from-queue-button")
	addStyleClass(removeFromQueueButton.GetStyleContext, "destructive-action")


	downloadButton, err := gtk.ButtonNewWithLabel("Download Queue")
	if err != nil {
		return nil, err
	}
	downloadButton.SetSizeRequest(-1, QUEUE_BUTTON_HEIGHT)
	SetupButtonAccessibility(downloadButton, "Start downloading all titles in your queue")
	addStyleClass(downloadButton.GetStyleContext, "download-queue-button")
	addStyleClass(downloadButton.GetStyleContext, "suggested-action")


	totalSizeLabel, err := gtk.LabelNew("Total Size: 0 B")
	if err != nil {
		return nil, err
	}
	totalSizeLabel.SetHAlign(gtk.ALIGN_START)
	totalSizeLabel.SetMarginStart(12)
	totalSizeLabel.SetMarginEnd(12)
	totalSizeLabel.SetMarginTop(6)
	totalSizeLabel.SetMarginBottom(6)
	addStyleClass(totalSizeLabel.GetStyleContext, "total-size-label")

	queuePane := QueuePane{
		container:             queueVBox,
		titleTreeView:         titleTreeView,
		store:                 store,
		titleQueue:            NewLocked(make([]wiiudownloader.TitleEntry, 0)),
		removeFromQueueButton: removeFromQueueButton,
		downloadButton:        downloadButton,
		totalSizeLabel:        totalSizeLabel,
		titleSizes:            make(map[uint64]string),
		titleBytes:            make(map[uint64]uint64),
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

		iter, ok := treeModel.GetIterFirst()
		if !ok {
			return
		}
		for iter != nil {
			isSelected := selection.IterIsSelected(iter)
			if isSelected {
				tid, err := treeModel.GetValue(iter, 3)
				if err != nil {
					continue
				}
				tidStr, err := tid.GetString()
				if err != nil {
					continue
				}
				tidParsed, err := strconv.ParseUint(tidStr, TID_BASE_16, TID_BITS_64)
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

	buttonBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	if err != nil {
		return nil, err
	}
	buttonBox.PackStart(removeFromQueueButton, true, true, 0)
	buttonBox.PackStart(downloadButton, true, true, 0)

	queueVBox.PackEnd(buttonBox, false, false, 0)
	queueVBox.PackEnd(totalSizeLabel, false, false, 0)

	addStyleClass(queueVBox.GetStyleContext, "queue-pane-vbox")


	return &queuePane, nil
}


func (qp *QueuePane) AddTitle(title wiiudownloader.TitleEntry) {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		*queue = append(*queue, title)
	})
	qp.Update(true)
}

func (qp *QueuePane) AddTitles(titles []wiiudownloader.TitleEntry) {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		*queue = append(*queue, titles...)
	})
	qp.Update(true)
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
	qp.Update(true)
}

func (qp *QueuePane) RemoveTitles(titlesToRemove []uint64) {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		for _, rid := range titlesToRemove {
			for i, t := range *queue {
				if t.TitleID == rid {
					*queue = append((*queue)[:i], (*queue)[i+1:]...)
					break
				}
			}
		}
	})
	qp.Update(true)
}


func (qp *QueuePane) Clear() {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		*queue = make([]wiiudownloader.TitleEntry, 0)
	})
}

func (qp *QueuePane) SetDownloadCallback(f func()) {
	qp.downloadButton.Connect("clicked", f)
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

func (qp *QueuePane) SetTitleSize(titleID uint64, bytes uint64) {
	qp.titleBytes[titleID] = bytes
	qp.titleSizes[titleID] = formatBytes(bytes)
	qp.updateSizeInStore(titleID, formatBytes(bytes))
}

func (qp *QueuePane) SetTitleError(titleID uint64) {
	qp.titleSizes[titleID] = "error"
	qp.updateSizeInStore(titleID, "error")
}

func (qp *QueuePane) SetTitleLoading(titleID uint64) {
	qp.titleSizes[titleID] = "loading..."
	qp.updateSizeInStore(titleID, "loading...")
}

func (qp *QueuePane) updateSizeInStore(titleID uint64, size string) {
	iter, ok := qp.store.GetIterFirst()
	if !ok {
		return
	}
	targetTidStr := fmt.Sprintf("%016x", titleID)
	for {
		tidVal, err := qp.store.GetValue(iter, 3)
		if err == nil {
			if tidStr, _ := tidVal.GetString(); tidStr == targetTidStr {
				qp.store.SetValue(iter, 4, size)
				break
			}
		}
		if !qp.store.IterNext(iter) {
			break
		}
	}
	qp.updateTotalSizeLabel()
}

func (qp *QueuePane) SetTitleLoadingNoUpdate(titleID uint64) {
	qp.titleSizes[titleID] = "loading..."
}


func (qp *QueuePane) Update(doUpdateFunc bool) {
	qp.store.Clear()

	var (
		queueSnapshot []wiiudownloader.TitleEntry
	)
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		queueSnapshot = make([]wiiudownloader.TitleEntry, len(queue))
		copy(queueSnapshot, queue)
	})

	for _, title := range queueSnapshot {
		iter := qp.store.Append()

		sizeStr, ok := qp.titleSizes[title.TitleID]
		if !ok {
			sizeStr = ""
		}

		qp.store.Set(
			iter,
			[]int{0, 1, 2, 3, 4},
			[]interface{}{
				title.Name,
				wiiudownloader.GetFormattedRegion(title.Region),
				wiiudownloader.GetFormattedKind(title.TitleID),
				fmt.Sprintf("%016x", title.TitleID),
				sizeStr,
			},
		)
	}

	qp.updateTotalSizeLabel()


	if qp.updateFunc != nil && doUpdateFunc {
		qp.updateFunc()
	}
	if qp.titleTreeView != nil {
		qp.titleTreeView.ColumnsAutosize()
	}
}

func (qp *QueuePane) updateTotalSizeLabel() {
	var total uint64
	var hasLoading bool
	var hasError bool

	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		for _, title := range queue {
			sizeStr, ok := qp.titleSizes[title.TitleID]
			if !ok {
				continue
			}
			if sizeStr == "loading..." {
				hasLoading = true
				continue
			}
			if sizeStr == "error" {
				hasError = true
				continue
			}
			total += qp.titleBytes[title.TitleID]
		}
	})

	text := fmt.Sprintf("Total Size: %s", formatBytes(total))
	if hasLoading {
		text += " (calculating...)"
	}
	if hasError {
		text += " (some sizes unavailable)"
	}
	qp.totalSizeLabel.SetMarkup(fmt.Sprintf("<b>%s</b>", text))
}


