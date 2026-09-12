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
	// Clearance under the buttons so rounded window corners cannot clip them,
	// and around them so the row is not pressed against its neighbours.
	QUEUE_CORNER_CLEARANCE          = 8
	QUEUE_BUTTON_ROW_CLEARANCE      = 4
	QUEUE_BUTTON_ROW_SIDE_CLEARANCE = 12
	TID_BASE_16                     = 16
	TID_BITS_64                     = 64
	LIST_POSITION_INVALID           = uint(0xFFFFFFFF)
)

// queueRowState is what a queued title reads while a download run is going.
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

// QueuePane is the download queue table, built on GtkColumnView so every cell is
// a real widget.
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
	// Inline download UI (Config.UseInlineDownloadUI): a run bar in the pane and
	// a live state per row.
	runBar          *gtk.Box
	runBarBar       *gtk.ProgressBar
	runBarLabel     *gtk.Label
	runBarCount     *gtk.Label
	runBarDetail    *gtk.Label
	runPauseButton  *gtk.Button
	runCancelButton *gtk.Button
	// statusColumn only earns its width while a run is going, so it is hidden
	// the rest of the time.
	statusColumn  *gtk.ColumnViewColumn
	onTogglePause func()
	onCancelRun   func()
	// The run bar is rendered from these, so its two sources of progress (the
	// current title and the queue position) cannot fight over one widget.
	runTitle     string
	runFraction  float64
	runDetail    string
	runQueueText string
	// controlsSensitive is the app-level gate (a download in flight disables the
	// pane); the remove button is derived from it, never set directly.
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

	nameColumn := textColumn("Name", 0, func(row *queueRow) string { return row.entry.Name })
	nameColumn.SetExpand(true)
	nameColumn.SetFixedWidth(-1)
	nameColumn.SetResizable(true)
	queuePane.columnView.AppendColumn(nameColumn)
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
	// How the title is doing while a run is going, so the queue pane itself can
	// be the download UI. Hidden while nothing is running: the pane is narrow
	// enough already without 110px of empty column.
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

	// Only offer the destructive button while something is actually selected.
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
	// Keeps the row off the run bar above it as well as the window corner below,
	// and lines the buttons up with the rows above instead of the pane edges.
	buttonBox.SetMarginTop(QUEUE_BUTTON_ROW_CLEARANCE)
	buttonBox.SetMarginBottom(QUEUE_CORNER_CLEARANCE)
	buttonBox.SetMarginStart(QUEUE_BUTTON_ROW_SIDE_CLEARANCE)
	buttonBox.SetMarginEnd(QUEUE_BUTTON_ROW_SIDE_CLEARANCE)
	buttonBox.Append(removeFromQueueButton)
	buttonBox.Append(downloadButton)

	queueVBox.Append(totalSizeLabel)
	queueVBox.Append(queuePane.newRunBar())
	queueVBox.Append(buttonBox)

	queueVBox.AddCSSClass("queue-pane-vbox")
	queueVBox.AddCSSClass("sidebar")
	// The inline run bar needs this much room to keep both controls on screen.
	queueVBox.SetSizeRequest(QUEUE_PANE_MIN_WIDTH, -1)

	queuePane.container = queueVBox

	return queuePane, nil
}

// newRunBar builds the inline download UI: the title on screen right now with
// the queue position and transfer controls, the bar for that title, and a detail
// line with bytes, rate and time left. It stays hidden until a run starts.
//
// The controls are icon-only because the pane can be dragged as narrow as
// QUEUE_PANE_MIN_WIDTH, where two icon+label buttons leave the bar no room.
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
		// One press is enough: the run is winding down, and the pause control no
		// longer has anything to pause.
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
	// Breathing room before the pane's button row.
	runBar.SetMarginBottom(12)
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

// refreshRows re-splices the whole model so every row picks up its new state.
func (qp *QueuePane) refreshRows() {
	keys := make([]string, 0, qp.rows.NItems())
	for i := uint(0); i < qp.rows.NItems(); i++ {
		keys = append(keys, qp.rows.String(i))
	}
	qp.spliceRows(0, qp.rows.NItems(), keys)
}

// spliceRows replaces model items and puts the selection back afterwards.
//
// Repainting a row works by swapping its item for an equal one, but that drops
// the old item from the selection — so a size landing, or a title's run state
// changing, silently cleared whatever the user had selected (and with it the
// Remove Selected button). Re-selecting by key avoids that without touching the
// view.
func (qp *QueuePane) spliceRows(position, removed uint, keys []string) {
	selected := qp.selectedKeys()
	qp.rows.Splice(position, removed, keys)
	for _, key := range selected {
		if at := qp.rows.Find(key); at != LIST_POSITION_INVALID {
			qp.selection.SelectItem(uint(at), false)
		}
	}
}

// selectedKeys returns the row keys currently selected.
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

// SetRunCallbacks wires the inline run bar's pause/cancel buttons to the run.
// Marshalled like the rest of the inline API: it is called from the download
// goroutine, while the buttons read it from the main loop.
func (qp *QueuePane) SetRunCallbacks(onTogglePause, onCancel func()) {
	uiIdleAdd(func() {
		qp.onTogglePause = onTogglePause
		qp.onCancelRun = onCancel
	})
}

// BeginInlineRun shows the run bar and resets every row to "Queued".
func (qp *QueuePane) BeginInlineRun() {
	uiIdleAdd(func() {
		if qp.runBar == nil {
			return
		}
		qp.runBar.SetVisible(true)
		qp.setRunControlsSensitive(true)
		qp.runTitle, qp.runFraction, qp.runDetail = "", 0, ""
		// The queue position is refreshed by the run itself; keep it for the
		// next title rather than blanking the count between titles.
		qp.renderRunBar()
		qp.setInlinePaused(false)
		if qp.statusColumn != nil {
			qp.statusColumn.SetVisible(true)
		}

		for _, row := range qp.rowData {
			row.state = queueStateQueued
		}
		qp.refreshRows()
	})
}

// EndInlineRun hides the run bar, leaving the final per-row states visible.
func (qp *QueuePane) EndInlineRun() {
	uiIdleAdd(func() {
		if qp.runBar == nil {
			return
		}
		qp.runBar.SetVisible(false)
		if qp.statusColumn != nil {
			qp.statusColumn.SetVisible(false)
		}
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

// SetInlineQueueProgress reports the queue position. It only touches the count,
// so the running title's own progress is never disturbed.
func (qp *QueuePane) SetInlineQueueProgress(done, total int) {
	count, full := queueCountText(done, total)
	uiIdleAdd(func() {
		qp.runQueueText = count
		if qp.runBarCount != nil {
			qp.runBarCount.SetTooltipText(full)
		}
		qp.renderRunBar()
	})
}

// queueCountText is the run bar's short queue position, plus the long form for
// its tooltip. The pane is narrow, so "2/5" beats "Title 2/5".
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

// SetInlineProgress reports the title on screen right now, how far through it is
// and the bytes/rate/ETA behind that fraction.
func (qp *QueuePane) SetInlineProgress(title string, fraction float64, detail string) {
	uiIdleAdd(func() {
		qp.runTitle, qp.runFraction, qp.runDetail = title, clampFraction(fraction), detail
		qp.renderRunBar()

	})
}

// renderRunBar paints the run bar from the stored run state. Main thread only.
func (qp *QueuePane) renderRunBar() {
	if qp.runBarLabel == nil || qp.runBarBar == nil {
		return
	}

	title := qp.runTitle
	if title == "" {
		title = "Preparing..."
	}
	qp.runBarLabel.SetText(title)
	qp.runBarCount.SetText(qp.runQueueText)
	qp.runBarCount.SetVisible(qp.runQueueText != "")

	qp.runBarBar.SetFraction(qp.runFraction)
	qp.runBarBar.SetText(fmt.Sprintf("%d%%", int(math.Round(qp.runFraction*PERCENT_SCALE))))

	// The detail line is bytes/rate/ETA: the queue position already has its own
	// slot, and this keeps the speed and time left on screen at any pane width.
	qp.runBarDetail.SetText(qp.runDetail)
	qp.runBarDetail.SetVisible(qp.runDetail != "")
}

// SetInlinePaused flips the inline pause control's icon.
func (qp *QueuePane) SetInlinePaused(paused bool) {
	uiIdleAdd(func() {
		qp.setInlinePaused(paused)
	})
}

func (qp *QueuePane) setInlinePaused(paused bool) {
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

// SetTitleState records how one queued title is doing and repaints its row.
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

// newVersionColumn builds the Version column as a flat button per row.
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

// onVersionClicked selects the clicked row and asks for the version picker.
func (qp *QueuePane) onVersionClicked(item *gtk.ListItem) {
	if item == nil || qp.setVersionRequested == nil {
		return
	}
	key := listItemKey(item)
	row := qp.rowData[key]
	if row == nil {
		return
	}

	// Highlight the row being edited so dialog and table agree.
	position := item.Position()
	if position != LIST_POSITION_INVALID {
		qp.selection.UnselectAll()
		qp.selection.SelectItem(position, false)
	}

	qp.setVersionRequested([]wiiudownloader.TitleEntry{row.entry})
}

// selectedTitleIDs maps the current selection back to title IDs.
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

// Clear empties the queue and refreshes the table and persisted queue.
func (qp *QueuePane) Clear() {
	qp.titleQueue.WithLock(func(queue *[]wiiudownloader.TitleEntry) {
		*queue = make([]wiiudownloader.TitleEntry, 0)
	})
	qp.Update(true)
}

// refreshRemoveButton derives the button's state from the app gate and the live
// selection. Re-enabling the pane used to set the button sensitive outright,
// which left "Remove Selected" clickable with nothing selected after a download.
func (qp *QueuePane) refreshRemoveButton() {
	if qp.removeFromQueueButton == nil || qp.selection == nil {
		return
	}
	selected := qp.selection.Selection()
	hasSelection := selected != nil && selected.Size() > 0
	qp.removeFromQueueButton.SetSensitive(qp.controlsSensitive && hasSelection)
}

// SetControlsSensitive gates the pane while a download is running.
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

// updateSizeInStore rewrites one row so the view re-binds just that cell.
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
		// Build the rows as the update lands, not when it was queued: a size
		// that arrived in between must not be replaced by a stale "loading...".
		rows := make(map[string]*queueRow, len(queueSnapshot))
		for _, title := range queueSnapshot {
			key := rowKeyForTitleID(title.TitleID)
			versionStr := "Latest"
			if title.Version >= 0 {
				versionStr = fmt.Sprintf("v%d", title.Version)
			}
			// Carry the run state across: Update rebuilds every row, and dropping
			// it mid-run made finished titles read as untouched again.
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
		// Titles that are still queued keep their selection; only the ones that
		// really went away are dropped.
		qp.spliceRows(0, qp.rows.NItems(), keys)
		qp.refreshRemoveButton()
		qp.updateTotalSizeLabel()

		if qp.updateFunc != nil && doUpdateFunc {
			qp.updateFunc()
		}
	})
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
