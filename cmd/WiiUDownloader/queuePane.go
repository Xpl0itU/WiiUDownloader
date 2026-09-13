package main

import (
	"fmt"
	"math"
	"strconv"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

const (
	QUEUE_REGION_COLUMN_WIDTH  = 70
	QUEUE_KIND_COLUMN_WIDTH    = 90
	QUEUE_VERSION_COLUMN_WIDTH = 110
	QUEUE_SIZE_COLUMN_WIDTH    = 100
	QUEUE_STATUS_COLUMN_WIDTH  = 110
	// The name column never drops below this, so a narrow pane scrolls instead of
	// squeezing the name away entirely.
	QUEUE_NAME_MIN_COLUMN_WIDTH = 120
	// Automatic growth stops here so a wide pane cannot hand the whole table to
	// the name column. Dragging the header bypasses it.
	QUEUE_NAME_MAX_AUTO_WIDTH = 320
	// Clearance so rounded window corners cannot clip the buttons.
	QUEUE_CORNER_CLEARANCE          = 8
	QUEUE_BUTTON_ROW_CLEARANCE      = 4
	QUEUE_BUTTON_ROW_SIDE_CLEARANCE = 12
	TID_BASE_16                     = 16
	TID_BITS_64                     = 64
	LIST_POSITION_INVALID           = uint(0xFFFFFFFF)
)

type queueRowState string

const (
	queueStateQueued      queueRowState = "Queued"
	queueStateDownloading queueRowState = "Downloading"
	queueStateDone        queueRowState = "Done"
	queueStateFailed      queueRowState = "Failed"
	queueStateCancelled   queueRowState = "Cancelled"
)

// queueRow is the Go-side data behind one row; the row key is the title ID in hex.
type queueRow struct {
	entry   wiiudownloader.TitleEntry
	version string
	size    string
	state   queueRowState
}

// QueuePane is the download queue table. It is a GtkColumnView, so every cell is
// a real widget carrying its own state.
type QueuePane struct {
	container             *gtk.Box
	columnView            *gtk.ColumnView
	rows                  *gtk.StringList
	selection             *gtk.MultiSelection
	rowData               map[string]*queueRow
	titleQueue            *Locked[[]wiiudownloader.TitleEntry]
	removeFromQueueButton *gtk.Button
	downloadButton        *gtk.Button
	totalSizeLabel        *gtk.Label
	titleSizes            map[uint64]string
	titleBytes            map[uint64]uint64
	updateFunc            func()
	setVersionRequested   func([]wiiudownloader.TitleEntry)
	// The run bar is the app's progress surface for every kind of run.
	runBar          *gtk.Box
	runBarBar       *gtk.ProgressBar
	runBarLabel     *gtk.Label
	runBarCount     *gtk.Label
	runBarDetail    *gtk.Label
	runPauseButton  *gtk.Button
	runCancelButton *gtk.Button
	// Only shown while a run is going; it costs 110px otherwise.
	statusColumn *gtk.ColumnViewColumn
	// The name column is the only one that can shrink, so it gets its own
	// bookkeeping. nameAutoWidth is the width the layout last applied, and
	// nameManual records that the user dragged the header and took it over.
	nameColumn    *gtk.ColumnViewColumn
	nameAutoWidth int
	nameManual    bool
	onTogglePause func()
	onCancelRun   func()
	// App-level gate: the remove button is derived from it, never set directly.
	controlsSensitive bool
}

func rowKeyForTitleID(titleID uint64) string {
	return fmt.Sprintf("%016x", titleID)
}

func NewQueuePane() (*QueuePane, error) {
	scrolledWindow := gtk.NewScrolledWindow()
	scrolledWindow.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrolledWindow.SetVExpand(true)

	queuePane := &QueuePane{
		rows:              newRowStore(nil),
		rowData:           make(map[string]*queueRow),
		titleQueue:        NewLocked(make([]wiiudownloader.TitleEntry, 0)),
		titleSizes:        make(map[uint64]string),
		titleBytes:        make(map[uint64]uint64),
		controlsSensitive: true,
	}

	queuePane.selection = gtk.NewMultiSelection(queuePane.rows)
	queuePane.columnView = gtk.NewColumnView(queuePane.selection)
	queuePane.columnView.SetCanFocus(true)
	queuePane.columnView.SetShowRowSeparators(false)
	queuePane.columnView.SetTooltipText("Download queue - Shows games queued for download. Use the Version button to set a title's version, or select rows and use Remove Selected.")

	textColumn := func(title string, width int, text func(*queueRow) string) *gtk.ColumnViewColumn {
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
			row := queuePane.rowData[listItemKey(item)]
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
		return column
	}

	// refreshQueueColumns owns the name column's width, so it must not also ask
	// GTK to expand the column: a fixed width is what lets it stop at the cap.
	nameColumn := textColumn("Name", 0, func(row *queueRow) string { return row.entry.Name })
	nameColumn.SetExpand(false)
	nameColumn.SetResizable(true)
	queuePane.columnView.AppendColumn(nameColumn)
	queuePane.nameColumn = nameColumn
	queuePane.nameAutoWidth = -1
	queuePane.columnView.AppendColumn(textColumn("Region", QUEUE_REGION_COLUMN_WIDTH, func(row *queueRow) string {
		return wiiudownloader.GetFormattedRegion(row.entry.Region)
	}))
	queuePane.columnView.AppendColumn(textColumn("Kind", QUEUE_KIND_COLUMN_WIDTH, func(row *queueRow) string {
		return wiiudownloader.GetFormattedKind(row.entry.TitleID)
	}))
	queuePane.columnView.AppendColumn(queuePane.newVersionColumn())
	queuePane.columnView.AppendColumn(textColumn("Size", QUEUE_SIZE_COLUMN_WIDTH, func(row *queueRow) string {
		return row.size
	}))
	// Per-title state, so the queue pane itself is the download UI.
	statusColumn := textColumn("Status", QUEUE_STATUS_COLUMN_WIDTH, func(row *queueRow) string {
		return string(row.state)
	})
	statusColumn.SetVisible(false)
	queuePane.statusColumn = statusColumn
	queuePane.columnView.AppendColumn(statusColumn)

	scrolledWindow.SetChild(queuePane.columnView)

	queueVBox := gtk.NewBox(gtk.OrientationVertical, 0)
	queueVBox.Append(scrolledWindow)

	removeContent := adw.NewButtonContent()
	removeContent.SetIconName("list-remove-symbolic")
	removeContent.SetLabel("Remove Selected")
	removeFromQueueButton := gtk.NewButton()
	removeFromQueueButton.SetChild(removeContent)

	SetupButtonAccessibility(removeFromQueueButton, "Remove selected titles from the download queue")
	removeFromQueueButton.AddCSSClass("remove-from-queue-button")
	removeFromQueueButton.AddCSSClass("destructive-action")
	removeFromQueueButton.SetSensitive(false)

	downloadContent := adw.NewButtonContent()
	downloadContent.SetIconName(queueDownloadIcon)
	downloadContent.SetLabel("Download Queue")
	downloadButton := gtk.NewButton()
	downloadButton.SetChild(downloadContent)
	SetupButtonAccessibility(downloadButton, "Start downloading all titles in your queue")
	downloadButton.AddCSSClass("download-queue-button")
	downloadButton.AddCSSClass("suggested-action")

	totalSizeLabel := gtk.NewLabel("Total Size: 0 B")
	totalSizeLabel.SetHAlign(gtk.AlignStart)
	totalSizeLabel.SetMarginStart(12)
	totalSizeLabel.SetMarginEnd(12)
	totalSizeLabel.SetMarginTop(6)
	totalSizeLabel.SetMarginBottom(6)
	totalSizeLabel.SetEllipsize(pango.EllipsizeEnd)
	totalSizeLabel.AddCSSClass("total-size-label")

	queuePane.removeFromQueueButton = removeFromQueueButton
	queuePane.downloadButton = downloadButton
	queuePane.totalSizeLabel = totalSizeLabel

	queuePane.selection.ConnectSelectionChanged(func(uint, uint) {
		queuePane.refreshRemoveButton()
	})

	removeFromQueueButton.ConnectClicked(func() {
		titlesToRemove := queuePane.selectedTitleIDs()
		if len(titlesToRemove) == 0 {
			return
		}
		queuePane.RemoveTitles(titlesToRemove)
	})

	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	buttonBox.AddCSSClass("linked")
	// Lines the buttons up with the rows and keeps them off the window corner.
	buttonBox.SetMarginTop(QUEUE_BUTTON_ROW_CLEARANCE)
	buttonBox.SetMarginBottom(QUEUE_CORNER_CLEARANCE)
	buttonBox.SetMarginStart(QUEUE_BUTTON_ROW_SIDE_CLEARANCE)
	buttonBox.SetMarginEnd(QUEUE_BUTTON_ROW_SIDE_CLEARANCE)
	buttonBox.Append(removeFromQueueButton)
	buttonBox.Append(downloadButton)

	queueVBox.Append(queuePane.newRunBar())
	queueVBox.Append(totalSizeLabel)
	queueVBox.Append(buttonBox)

	queueVBox.AddCSSClass("queue-pane-vbox")
	queueVBox.AddCSSClass("sidebar")
	// Enough room to keep both run controls on screen.
	queueVBox.SetSizeRequest(QUEUE_PANE_MIN_WIDTH, -1)

	queuePane.container = queueVBox

	return queuePane, nil
}

// newRunBar builds the run bar: the title on screen with the queue position and
// transfer controls, the bar for that title, and a bytes/rate/ETA line. Hidden
// until a run starts. The controls are icon-only because at QUEUE_PANE_MIN_WIDTH
// two labelled buttons would leave the bar no room.
func (qp *QueuePane) newRunBar() *gtk.Box {
	bar := gtk.NewProgressBar()
	bar.SetHExpand(true)
	bar.SetShowText(true)
	bar.SetTooltipText("Progress of the title being downloaded right now")

	label := gtk.NewLabel("")
	label.SetHAlign(gtk.AlignStart)
	label.SetHExpand(true)
	label.SetEllipsize(pango.EllipsizeMiddle)
	label.AddCSSClass("queue-run-label")

	count := gtk.NewLabel("")
	count.SetHAlign(gtk.AlignEnd)
	count.SetEllipsize(pango.EllipsizeStart)
	count.AddCSSClass("queue-run-count")

	detail := gtk.NewLabel("")
	detail.SetHAlign(gtk.AlignStart)
	detail.SetEllipsize(pango.EllipsizeEnd)
	detail.AddCSSClass("queue-run-detail")

	pauseButton := newIconButton("media-playback-pause-symbolic", "Pause or resume the running download")
	pauseButton.ConnectClicked(func() {
		if qp.onTogglePause != nil {
			qp.onTogglePause()
		}
	})

	cancelButton := newIconButton("process-stop-symbolic", "Cancel the running download")
	cancelButton.AddCSSClass("destructive-action")
	cancelButton.ConnectClicked(func() {
		if qp.onCancelRun != nil {
			qp.onCancelRun()
		}
		// One press is enough; the run is already winding down.
		cancelButton.SetSensitive(false)
		pauseButton.SetSensitive(false)
	})

	controls := gtk.NewBox(gtk.OrientationHorizontal, 0)
	controls.AddCSSClass("linked")
	controls.SetVAlign(gtk.AlignCenter)
	controls.Append(pauseButton)
	controls.Append(cancelButton)

	titleRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	titleRow.Append(label)
	titleRow.Append(count)
	titleRow.Append(controls)

	runBar := gtk.NewBox(gtk.OrientationVertical, 8)
	runBar.AddCSSClass("queue-run-bar")
	runBar.SetMarginStart(12)
	runBar.SetMarginEnd(12)
	runBar.SetMarginTop(8)
	runBar.SetMarginBottom(8)
	runBar.Append(titleRow)
	runBar.Append(bar)
	runBar.Append(detail)
	runBar.SetVisible(false)

	qp.runBar = runBar
	qp.runBarBar = bar
	qp.runBarLabel = label
	qp.runBarCount = count
	qp.runBarDetail = detail
	qp.runPauseButton = pauseButton
	qp.runCancelButton = cancelButton
	return runBar
}

func (qp *QueuePane) refreshRows() {
	keys := make([]string, 0, qp.rows.NItems())
	for i := uint(0); i < qp.rows.NItems(); i++ {
		keys = append(keys, qp.rows.String(i))
	}
	qp.spliceRows(0, qp.rows.NItems(), keys)
}

// spliceRows replaces model items and re-selects the same keys.
//
// Swapping an item for an equal one drops the old item from the selection, so a
// repaint used to clear whatever the user had selected along with it.
func (qp *QueuePane) spliceRows(position, removed uint, keys []string) {
	selected := qp.selectedKeys()
	qp.rows.Splice(position, removed, keys)
	for _, key := range selected {
		if at := qp.rows.Find(key); at != LIST_POSITION_INVALID {
			qp.selection.SelectItem(uint(at), false)
		}
	}
}

func (qp *QueuePane) selectedKeys() []string {
	bits := qp.selection.Selection()
	if bits == nil {
		return nil
	}
	keys := make([]string, 0, int(bits.Size()))
	for i := uint64(0); i < bits.Size(); i++ {
		position := bits.Nth(uint(i))
		if position >= qp.rows.NItems() {
			continue
		}
		keys = append(keys, qp.rows.String(position))
	}
	return keys
}

// SetRunCallbacks wires the run bar's controls; marshalled because the download
// goroutine calls it.
func (qp *QueuePane) SetRunCallbacks(onTogglePause, onCancel func()) {
	uiIdleAdd(func() {
		qp.onTogglePause = onTogglePause
		qp.onCancelRun = onCancel
	})
}

func (qp *QueuePane) BeginRun() {
	uiIdleAdd(func() {
		if qp.runBar == nil {
			return
		}
		qp.runBar.SetVisible(true)
		qp.setRunControlsSensitive(true)
		qp.setRunPaused(false)
		qp.setRunProgress("", 0, "")
		if qp.statusColumn != nil {
			qp.statusColumn.SetVisible(true)
		}
		qp.refreshQueueColumns()

		for _, row := range qp.rowData {
			row.state = queueStateQueued
		}
		qp.refreshRows()
	})
}

// EndRun hides the run bar but leaves the final per-row states on screen.
func (qp *QueuePane) EndRun() {
	uiIdleAdd(func() {
		if qp.runBar == nil {
			return
		}
		qp.runBar.SetVisible(false)
		if qp.statusColumn != nil {
			qp.statusColumn.SetVisible(false)
		}
		qp.refreshQueueColumns()
	})
}

// queueCountText is the short queue position plus its long tooltip form; the pane
// is too narrow for "Title 2/5".
func queueCountText(done, total int) (short, long string) {
	if total <= 0 {
		return "", ""
	}
	if done >= total {
		done = total - 1
	}
	if done < 0 {
		done = 0
	}
	return fmt.Sprintf("%d/%d", done+1, total), queueProgressText(done, total)
}

// SetRunProgress names the running title and paints its fraction and detail.
// Marshalled like every run-bar entry point: the core reports from worker
// goroutines and GTK is not thread-safe.
func (qp *QueuePane) SetRunProgress(title string, fraction float64, detail string) {
	uiIdleAdd(func() {
		qp.setRunProgress(title, fraction, detail)
	})
}

// SetRunQueueProgress paints the queue position; total <= 0 blanks it.
func (qp *QueuePane) SetRunQueueProgress(done, total int) {
	short, long := queueCountText(done, total)
	uiIdleAdd(func() {
		if qp.runBarCount == nil {
			return
		}
		qp.runBarCount.SetText(short)
		qp.runBarCount.SetVisible(short != "")
		qp.runBarCount.SetTooltipText(long)
	})
}

func (qp *QueuePane) SetRunPaused(paused bool) {
	uiIdleAdd(func() {
		qp.setRunPaused(paused)
	})
}

func (qp *QueuePane) setRunProgress(title string, fraction float64, detail string) {
	if qp.runBarLabel == nil || qp.runBarBar == nil {
		return
	}
	if title == "" {
		title = "Preparing..."
	}
	fraction = clampFraction(fraction)

	qp.runBarLabel.SetText(title)
	qp.runBarBar.SetFraction(fraction)
	qp.runBarBar.SetText(fmt.Sprintf("%d%%", int(math.Round(fraction*PERCENT_SCALE))))

	// Bytes/rate/ETA get their own line so the speed survives any pane width.
	qp.runBarDetail.SetText(detail)
	qp.runBarDetail.SetVisible(detail != "")
}

func (qp *QueuePane) setRunPaused(paused bool) {
	if qp.runPauseButton == nil {
		return
	}
	icon, tooltip := "media-playback-pause-symbolic", "Pause or resume the running download"
	if paused {
		icon, tooltip = "media-playback-start-symbolic", "Resume the running download"
	}
	qp.runPauseButton.SetIconName(icon)
	qp.runPauseButton.SetTooltipText(tooltip)
}

// SetRunControlsSensitive gates pause/cancel while a run cannot be interrupted
// (a decryption step, or a cancel already under way).
func (qp *QueuePane) SetRunControlsSensitive(sensitive bool) {
	uiIdleAdd(func() {
		qp.setRunControlsSensitive(sensitive)
	})
}

func (qp *QueuePane) setRunControlsSensitive(sensitive bool) {
	if qp.runPauseButton != nil {
		qp.runPauseButton.SetSensitive(sensitive)
	}
	if qp.runCancelButton != nil {
		qp.runCancelButton.SetSensitive(sensitive)
	}
}

func (qp *QueuePane) SetTitleState(titleID uint64, state queueRowState) {
	uiIdleAdd(func() {
		key := rowKeyForTitleID(titleID)
		row, ok := qp.rowData[key]
		if !ok || row.state == state {
			return
		}
		row.state = state

		position := qp.rows.Find(key)
		if position != LIST_POSITION_INVALID {
			qp.spliceRows(position, 1, []string{key})
		}
	})
}

func (qp *QueuePane) newVersionColumn() *gtk.ColumnViewColumn {
	factory := gtk.NewSignalListItemFactory()
	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := listItem(obj)
		button := newIconLabelButton("document-edit-symbolic", "")
		button.AddCSSClass("flat")
		button.SetTooltipText("Click to change this title's version")
		item.SetChild(button)

		button.ConnectClicked(func() {
			qp.onVersionClicked(item)
		})
	})
	factory.ConnectBind(func(obj *coreglib.Object) {
		item := listItem(obj)
		button, ok := item.Child().(*gtk.Button)
		if !ok {
			return
		}
		content, ok := button.Child().(*adw.ButtonContent)
		if !ok {
			return
		}
		row := qp.rowData[listItemKey(item)]
		if row == nil {
			content.SetLabel("")
			return
		}
		content.SetLabel(row.version)
	})

	column := gtk.NewColumnViewColumn("Version", &factory.ListItemFactory)
	column.SetResizable(true)
	column.SetFixedWidth(QUEUE_VERSION_COLUMN_WIDTH)
	return column
}

func (qp *QueuePane) onVersionClicked(item *gtk.ListItem) {
	if item == nil || qp.setVersionRequested == nil {
		return
	}
	key := listItemKey(item)
	row := qp.rowData[key]
	if row == nil {
		return
	}

	// Select the row being edited so dialog and table agree.
	position := item.Position()
	if position != LIST_POSITION_INVALID {
		qp.selection.UnselectAll()
		qp.selection.SelectItem(position, false)
	}

	qp.setVersionRequested([]wiiudownloader.TitleEntry{row.entry})
}

func (qp *QueuePane) selectedTitleIDs() []uint64 {
	bits := qp.selection.Selection()
	if bits == nil {
		return nil
	}
	ids := make([]uint64, 0, int(bits.Size()))
	for i := uint64(0); i < bits.Size(); i++ {
		position := bits.Nth(uint(i))
		key := qp.rows.String(position)
		tid, err := strconv.ParseUint(key, TID_BASE_16, TID_BITS_64)
		if err == nil {
			ids = append(ids, tid)
		}
	}
	return ids
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
	qp.Update(true)
}

// refreshRemoveButton is the only writer of the button's state: it derives it from
// the app gate and the live selection.
func (qp *QueuePane) refreshRemoveButton() {
	if qp.removeFromQueueButton == nil || qp.selection == nil {
		return
	}
	selected := qp.selection.Selection()
	hasSelection := selected != nil && selected.Size() > 0
	qp.removeFromQueueButton.SetSensitive(qp.controlsSensitive && hasSelection)
}

func (qp *QueuePane) SetControlsSensitive(sensitive bool) {
	qp.controlsSensitive = sensitive
	qp.refreshRemoveButton()
}

func (qp *QueuePane) SetDownloadCallback(f func()) {
	qp.downloadButton.ConnectClicked(f)
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
	key := rowKeyForTitleID(titleID)
	if row, ok := qp.rowData[key]; ok {
		row.size = size
	}

	position := qp.rows.Find(key)
	if position != LIST_POSITION_INVALID {
		qp.spliceRows(position, 1, []string{key})
	}
	qp.updateTotalSizeLabel()
}

func (qp *QueuePane) SetTitleLoadingNoUpdate(titleID uint64) {
	qp.titleSizes[titleID] = "loading..."
}

func (qp *QueuePane) SetSetVersionRequested(f func([]wiiudownloader.TitleEntry)) {
	qp.setVersionRequested = f
}

func (qp *QueuePane) SetTitleVersion(titleID uint64, version int) {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		for i := range *queue {
			if (*queue)[i].TitleID == titleID {
				(*queue)[i].Version = version
				break
			}
		}
	})
	delete(qp.titleSizes, titleID)
	delete(qp.titleBytes, titleID)
	qp.Update(true)
}

func (qp *QueuePane) Update(doUpdateFunc bool) {
	var queueSnapshot []wiiudownloader.TitleEntry
	qp.titleQueue.WithRLock(func(queue []wiiudownloader.TitleEntry) {
		queueSnapshot = make([]wiiudownloader.TitleEntry, len(queue))
		copy(queueSnapshot, queue)
	})

	persistQueue(queueSnapshot)

	keys := make([]string, 0, len(queueSnapshot))
	for _, title := range queueSnapshot {
		keys = append(keys, rowKeyForTitleID(title.TitleID))
	}

	uiIdleAdd(func() {
		// Build rows as the update lands: a size that arrived while it was queued
		// must not be replaced by a stale "loading...".
		rows := make(map[string]*queueRow, len(queueSnapshot))
		for _, title := range queueSnapshot {
			key := rowKeyForTitleID(title.TitleID)
			versionStr := "Latest"
			if title.Version >= 0 {
				versionStr = fmt.Sprintf("v%d", title.Version)
			}
			// Update rebuilds every row, so carry the run state across.
			state := queueRowState("")
			if previous, ok := qp.rowData[key]; ok {
				state = previous.state
			}
			rows[key] = &queueRow{
				entry:   title,
				version: versionStr,
				size:    qp.titleSizes[title.TitleID],
				state:   state,
			}
		}
		qp.rowData = rows
		// spliceRows keeps the selection for titles that are still queued.
		qp.spliceRows(0, qp.rows.NItems(), keys)
		qp.refreshRemoveButton()
		qp.updateTotalSizeLabel()
		qp.refreshQueueColumns()

		if qp.updateFunc != nil && doUpdateFunc {
			qp.updateFunc()
		}
	})
}

// queueFixedColumnsWidth is what the always-visible queue columns cost before
// the name column gets a say.
const queueFixedColumnsWidth = QUEUE_REGION_COLUMN_WIDTH + QUEUE_KIND_COLUMN_WIDTH +
	QUEUE_VERSION_COLUMN_WIDTH + QUEUE_SIZE_COLUMN_WIDTH

// queueNameColumnWidth is the width the name column takes out of a table: what
// the fixed columns leave, never less than nameMin (the table scrolls instead)
// and never more than nameMax.
func queueNameColumnWidth(available, reserved, nameMin, nameMax int) int {
	width := available - reserved
	if width < nameMin {
		width = nameMin
	}
	if width > nameMax {
		width = nameMax
	}
	return width
}

// refreshQueueColumns gives the name column the room the fixed columns leave,
// capped so a wide pane cannot hand it the whole table. Every fixed column stays
// visible; when the pane is too narrow to hold them beside the minimum name
// width the table scrolls rather than squeezing the name away.
//
// GTK writes the column's fixed-width property on an interactive resize, so a
// value that no longer matches the one applied here means the user dragged the
// header. The width is theirs from then on and is never auto-sized again.
func (qp *QueuePane) refreshQueueColumns() {
	if qp.nameColumn == nil || qp.columnView == nil {
		return
	}
	if !qp.nameManual && qp.nameAutoWidth >= 0 && qp.nameColumn.FixedWidth() != qp.nameAutoWidth {
		qp.nameManual = true
	}
	if qp.nameManual {
		return
	}

	available := qp.columnView.Width()
	if available <= 0 {
		return
	}

	reserved := queueFixedColumnsWidth
	if qp.statusColumn != nil && qp.statusColumn.Visible() {
		reserved += QUEUE_STATUS_COLUMN_WIDTH
	}
	width := queueNameColumnWidth(available, reserved, QUEUE_NAME_MIN_COLUMN_WIDTH, QUEUE_NAME_MAX_AUTO_WIDTH)
	if qp.nameAutoWidth == width && qp.nameColumn.FixedWidth() == width {
		return
	}
	// Remember the width before writing it, so the next refresh can tell our own
	// write apart from a user drag.
	qp.nameAutoWidth = width
	qp.nameColumn.SetFixedWidth(width)
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
