package main

import (
	"sort"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

const TITLE_QUEUE_COLUMN_WIDTH = 60

// titleRow is the Go-side data behind one row; the row key is the title ID in hex.
type titleRow struct {
	entry   wiiudownloader.TitleEntry
	kind    string
	region  string
	tidHex  string
	inQueue bool
}

// buildTitleList builds GtkStringList -> filter -> sort -> multi selection ->
// GtkColumnView. Cell callbacks resolve their row from the row key, never from
// the selection.
func (mw *MainWindow) buildTitleList() {
	allTitles := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL)
	mw.titleRows = make(map[string]*titleRow, len(allTitles))
	mw.boundChecks = make(map[string]*gtk.CheckButton)
	mw.checkRowKeys = make(map[uintptr]string)

	queuedTIDs := make(map[uint64]struct{})
	for _, queued := range mw.queuePane.GetTitleQueue() {
		queuedTIDs[queued.TitleID] = struct{}{}
	}

	keys := make([]string, 0, len(allTitles))
	for _, entry := range allTitles {
		key := rowKeyForTitleID(entry.TitleID)
		if _, exists := mw.titleRows[key]; exists {
			continue
		}
		_, inQueue := queuedTIDs[entry.TitleID]
		mw.titleRows[key] = &titleRow{
			entry:   entry,
			kind:    wiiudownloader.GetFormattedKind(entry.TitleID),
			region:  wiiudownloader.GetFormattedRegion(entry.Region),
			tidHex:  key,
			inQueue: inQueue,
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return mw.titleRows[keys[i]].entry.Name < mw.titleRows[keys[j]].entry.Name
	})

	mw.rows = newRowStore(keys)

	mw.titleFilter = gtk.NewCustomFilter(func(item *coreglib.Object) bool {
		row := mw.titleRows[rowKey(item)]
		if row == nil {
			return false
		}
		return mw.titleMatchesFilter(row)
	})

	filtered := gtk.NewFilterListModel(mw.rows, &mw.titleFilter.Filter)
	mw.titleSortModel = gtk.NewSortListModel(filtered, nil)
	mw.titleSelection = gtk.NewMultiSelection(mw.titleSortModel)
	mw.titleView = gtk.NewColumnView(mw.titleSelection)

	mw.titleView.SetTooltipText("Game titles list. Use arrow keys to navigate, space or enter to toggle queue status for selected titles, or click checkboxes to add/remove titles.")

	// The column view owns the user-visible sorter; hand it to the sort model.
	mw.titleSortModel.SetSorter(mw.titleView.Sorter())

	textColumn := func(title string, width int, text func(*titleRow) string, less func(a, b *titleRow) int) *gtk.ColumnViewColumn {
		factory := gtk.NewSignalListItemFactory()
		factory.ConnectSetup(func(obj *coreglib.Object) {
			label := gtk.NewLabel("")
			label.SetXAlign(0)
			label.SetEllipsize(pango.EllipsizeEnd)
			listItem(obj).SetChild(label)
		})
		factory.ConnectBind(func(obj *coreglib.Object) {
			item := listItem(obj)
			label, ok := item.Child().(*gtk.Label)
			if !ok {
				return
			}
			row := mw.titleRows[listItemKey(item)]
			if row == nil {
				label.SetText("")
				return
			}
			label.SetText(text(row))
		})

		column := gtk.NewColumnViewColumn(title, &factory.ListItemFactory)
		column.SetResizable(true)
		if width > 0 {
			column.SetFixedWidth(width)
		}
		column.SetSorter(mw.titleColumnSorter(less))
		return column
	}

	mw.titleView.AppendColumn(mw.newQueueColumn())
	mw.titleView.AppendColumn(textColumn("Kind", 90,
		func(row *titleRow) string { return row.kind },
		func(a, b *titleRow) int { return compareWithNameTieBreak(a, b, a.kind, b.kind) }))
	mw.titleView.AppendColumn(textColumn("Title ID", 130,
		func(row *titleRow) string { return row.tidHex },
		func(a, b *titleRow) int { return compareWithNameTieBreak(a, b, a.tidHex, b.tidHex) }))
	mw.titleView.AppendColumn(textColumn("Region", 80,
		func(row *titleRow) string { return row.region },
		func(a, b *titleRow) int { return compareWithNameTieBreak(a, b, a.region, b.region) }))

	nameColumn := textColumn("Name", 0,
		func(row *titleRow) string { return row.entry.Name },
		func(a, b *titleRow) int { return strings.Compare(a.entry.Name, b.entry.Name) })
	nameColumn.SetExpand(true)
	nameColumn.SetFixedWidth(-1)
	mw.titleView.AppendColumn(nameColumn)

	mw.titleView.SortByColumn(nameColumn, gtk.SortAscending)

	SetupListViewAccessibility(mw.titleView)

	// Claim Space/Enter in the capture phase; GtkTreeView's key handling is gone.
	treeKeyController := gtk.NewEventControllerKey()
	treeKeyController.SetPropagationPhase(gtk.PhaseCapture)
	treeKeyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if !isKeyboardActivationKey(keyval) {
			return false
		}
		return mw.toggleQueueFromKeyboard()
	})
	mw.titleView.AddController(treeKeyController)
}

func compareWithNameTieBreak(a, b *titleRow, av, bv string) int {
	if diff := strings.Compare(av, bv); diff != 0 {
		return diff
	}
	return strings.Compare(a.entry.Name, b.entry.Name)
}

// titleColumnSorter adapts a row comparator to glib.NewObjectComparer.
func (mw *MainWindow) titleColumnSorter(less func(a, b *titleRow) int) *gtk.Sorter {
	sorter := gtk.NewCustomSorter(glib.NewObjectComparer(func(a, b *gtk.StringObject) int {
		var ra, rb *titleRow
		if a != nil {
			ra = mw.titleRows[a.String()]
		}
		if b != nil {
			rb = mw.titleRows[b.String()]
		}
		if ra == nil || rb == nil {
			return 0
		}
		return less(ra, rb)
	}))
	return &sorter.Sorter
}

// newQueueColumn is the queue checkbox column; each checkbox binds its own row key.
func (mw *MainWindow) newQueueColumn() *gtk.ColumnViewColumn {
	factory := gtk.NewSignalListItemFactory()

	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := listItem(obj)
		check := gtk.NewCheckButton()
		check.SetHAlign(gtk.AlignCenter)
		// Keep focus on the row; Space is handled by the view.
		check.SetFocusable(false)
		item.SetChild(check)

		check.ConnectToggled(func() {
			key := listItemKey(item)
			row := mw.titleRows[key]
			if row == nil {
				return
			}
			if check.Active() == row.inQueue {
				// A programmatic sync, not a user click.
				return
			}
			mw.setQueueMembership([]string{key}, check.Active())
		})
	})

	factory.ConnectBind(func(obj *coreglib.Object) {
		item := listItem(obj)
		check, ok := item.Child().(*gtk.CheckButton)
		if !ok {
			return
		}
		key := listItemKey(item)
		row := mw.titleRows[key]
		if row == nil {
			check.SetActive(false)
			return
		}

		// Row widgets get recycled between rows.
		if previous, ok := mw.checkRowKeys[item.Native()]; ok && previous != key {
			delete(mw.boundChecks, previous)
		}
		mw.checkRowKeys[item.Native()] = key
		mw.boundChecks[key] = check

		check.SetActive(row.inQueue)
	})

	factory.ConnectUnbind(func(obj *coreglib.Object) {
		item := listItem(obj)
		key, ok := mw.checkRowKeys[item.Native()]
		if !ok {
			return
		}
		delete(mw.boundChecks, key)
		delete(mw.checkRowKeys, item.Native())
	})

	column := gtk.NewColumnViewColumn("Queue", &factory.ListItemFactory)
	column.SetFixedWidth(TITLE_QUEUE_COLUMN_WIDTH)
	column.SetResizable(false)
	return column
}

// titleMatchesFilter is the current category/region/search predicate.
func (mw *MainWindow) titleMatchesFilter(row *titleRow) bool {
	if mw.currentCategory != wiiudownloader.TITLE_CATEGORY_ALL {
		if row.kind != wiiudownloader.GetFormattedKind(row.entry.TitleID) {
			return false
		}
		if wiiudownloader.GetCategoryFromFormattedCategory(row.kind) != mw.currentCategory {
			return false
		}
	}

	if (mw.currentRegion & row.entry.Region) == 0 {
		return false
	}

	if mw.lastSearchText != "" && !titleMatchesSearch(mw.lastSearchText, row.entry.Name, row.tidHex) {
		return false
	}

	return true
}

// refreshTitleFilter re-runs the predicate after category/region/search changes.
func (mw *MainWindow) refreshTitleFilter() {
	if mw.titleFilter == nil {
		return
	}
	mw.titleFilter.Filter.Changed(gtk.FilterChangeDifferent)
	// The filter model keeps the old scroll anchor, so a shorter list leaves the
	// viewport past its end and looking empty until the user scrolls. Scroll back
	// to the top once the model has settled.
	uiIdleAdd(func() {
		if mw.titleSortModel != nil && mw.titleSortModel.NItems() > 0 {
			mw.titleView.ScrollTo(0, nil, gtk.ListScrollNone, nil)
		}
		if mw.titleStatusPage != nil {
			mw.titleStatusPage.SetVisible(mw.titleSortModel.NItems() == 0)
		}
	})
}

// viewRowKey maps a position in the view's model back to a row key.
func (mw *MainWindow) viewRowKey(position uint) string {
	if mw.titleSortModel == nil {
		return ""
	}
	return rowKey(mw.titleSortModel.Item(position))
}

// selectedRowKeys returns the keys of every selected row.
func (mw *MainWindow) selectedRowKeys() []string {
	if mw.titleSelection == nil {
		return nil
	}
	bits := mw.titleSelection.Selection()
	if bits == nil {
		return nil
	}
	keys := make([]string, 0, int(bits.Size()))
	for i := uint64(0); i < bits.Size(); i++ {
		if key := mw.viewRowKey(bits.Nth(uint(i))); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// focusTitleList selects and reveals the first row for keyboard toggling.
func (mw *MainWindow) focusTitleList() bool {
	if mw.titleView == nil || mw.titleSelection == nil || mw.titleSortModel == nil {
		return false
	}
	if mw.titleSelection.Selection().Size() > 0 {
		return true
	}
	if mw.titleSortModel.NItems() == 0 {
		return false
	}
	mw.titleSelection.SelectItem(0, false)
	mw.titleView.ScrollTo(0, nil, gtk.ListScrollFocus, nil)
	return true
}

func (mw *MainWindow) toggleQueueFromKeyboard() bool {
	if mw.titleSelection == nil || mw.titleSelection.Selection().Size() == 0 {
		if !mw.focusTitleList() {
			return false
		}
	}
	keys := mw.selectedRowKeys()
	if len(keys) == 0 {
		return false
	}

	// Fully queued selections are removed, otherwise every row gets queued.
	queued := true
	for _, key := range keys {
		if row := mw.titleRows[key]; row == nil || !row.inQueue {
			queued = false
			break
		}
	}
	mw.setQueueMembership(keys, !queued)
	return true
}

// setQueueMembership adds or removes rows and syncs the visible checkboxes.
func (mw *MainWindow) setQueueMembership(keys []string, queued bool) {
	entries := make([]wiiudownloader.TitleEntry, 0, len(keys))
	seen := make(map[uint64]struct{}, len(keys))
	for _, key := range keys {
		row := mw.titleRows[key]
		if row == nil {
			continue
		}
		if _, dup := seen[row.entry.TitleID]; dup {
			continue
		}
		seen[row.entry.TitleID] = struct{}{}
		entries = append(entries, row.entry)
	}
	if len(entries) == 0 {
		return
	}

	if !queued {
		mw.queuePane.RemoveTitles(mw.collectTIDs(entries))
		mw.updateTitlesInQueue()
		return
	}

	mw.addTitlesToQueue(entries)
	// Sync before the related-content dialog so row state matches the checkbox.
	mw.updateTitlesInQueue()

	if !mw.suggestRelatedContent {
		return
	}

	candidates := mw.collectRelatedCandidates(entries)
	if len(candidates) == 0 {
		return
	}
	mw.showRelatedTitlesDialog(entries, candidates, func(chosen []wiiudownloader.TitleEntry) {
		if len(chosen) > 0 {
			mw.addTitlesToQueue(chosen)
		}
		mw.updateTitlesInQueue()
	})
}

// updateTitlesInQueue re-syncs row state and the on-screen checkboxes.
func (mw *MainWindow) updateTitlesInQueue() {
	if mw.titleRows == nil {
		return
	}

	queuedTIDs := make(map[uint64]struct{})
	for _, queued := range mw.queuePane.GetTitleQueue() {
		queuedTIDs[queued.TitleID] = struct{}{}
	}

	for key, row := range mw.titleRows {
		_, isInQueue := queuedTIDs[row.entry.TitleID]
		if row.inQueue == isInQueue {
			continue
		}
		row.inQueue = isInQueue
		if check, ok := mw.boundChecks[key]; ok {
			check.SetActive(isInQueue)
		}
	}

	mw.queuePane.Update(false)
}
