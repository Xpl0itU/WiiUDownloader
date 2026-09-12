package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type DownloadError struct {
	Title     string
	Error     string
	TidStr    string
	ErrorType string
	Version   int
}

const (
	MAX_SPEEDS          = 32
	SMOOTHING_FACTOR    = 0.2
	PERCENT_SCALE       = 100
	PROGRESS_MIN_WIDTH  = 350
	MIN_SAMPLE_INTERVAL = 100 * time.Millisecond
)

type SpeedAverager struct {
	speeds       []int64
	averageSpeed int64
	lastBytes    int64
	lastTime     time.Time
}

func newSpeedAverager() *SpeedAverager {
	return &SpeedAverager{
		speeds: make([]int64, 0, MAX_SPEEDS),
	}
}

func (sa *SpeedAverager) Reset() {
	sa.speeds = sa.speeds[:0]
	sa.averageSpeed = 0
	sa.lastBytes = 0
	sa.lastTime = time.Time{}
}

func (sa *SpeedAverager) Sample(totalBytes int64, now time.Time) {
	if sa.lastTime.IsZero() || now.Before(sa.lastTime) {
		sa.lastBytes = totalBytes
		sa.lastTime = now
		return
	}
	elapsed := now.Sub(sa.lastTime)
	if elapsed < MIN_SAMPLE_INTERVAL {
		return
	}
	speed := int64(float64(totalBytes-sa.lastBytes) / elapsed.Seconds())
	if speed < 0 {
		speed = 0
	}
	sa.lastBytes = totalBytes
	sa.lastTime = now
	sa.AddSpeed(speed)
}

func (sa *SpeedAverager) AddSpeed(speed int64) {
	if len(sa.speeds) >= MAX_SPEEDS {
		copy(sa.speeds[:MAX_SPEEDS/2], sa.speeds[MAX_SPEEDS/2:])
		sa.speeds = sa.speeds[:MAX_SPEEDS/2]
	}
	sa.speeds = append(sa.speeds, speed)
}

func (sa *SpeedAverager) calculateAverageOfSpeeds() {
	if len(sa.speeds) == 0 {
		sa.averageSpeed = 0
		return
	}
	var total int64
	for _, speed := range sa.speeds {
		total += speed
	}
	sa.averageSpeed = total / int64(len(sa.speeds))
}

func (sa *SpeedAverager) GetAverageSpeed() float64 {
	sa.calculateAverageOfSpeeds()
	if len(sa.speeds) == 0 {
		return 0
	}
	return SMOOTHING_FACTOR*float64(sa.speeds[len(sa.speeds)-1]) + (1-SMOOTHING_FACTOR)*float64(sa.averageSpeed)
}

type ProgressWindow struct {
	Window          *gtk.Window
	gameLabel       *gtk.Label
	queueLabel      *gtk.Label
	bar             *gtk.ProgressBar
	pauseButton     *gtk.Button
	cancelButton    *gtk.Button
	cancelled       bool
	paused          bool
	cancelledChan   chan struct{}
	cancelOnce      sync.Once
	totalToDownload int64
	totalDownloaded int64
	progressPerFile map[string]int64
	progressMutex   sync.Mutex
	controlMutex    sync.Mutex
	controlCond     *sync.Cond
	speedAverager   *SpeedAverager
	startTime       time.Time
	errors          []DownloadError
	errorsMutex     sync.Mutex
	updatePending   bool
	decPending      bool
	decProgress     float64
	queueDone       int
	queueTotal      int
	// display records what the window last painted, and displayHook mirrors it
	// into another surface (the queue pane's run bar). Guarded by progressMutex;
	// the hook only ever runs on the main thread.
	display     DownloadDisplayState
	displayHook func()
}

// DownloadDisplayState is a run's progress in a form another surface can
// mirror. Every field is plain data: reading it never touches a widget, so it
// is safe from the download goroutine.
//
// Fraction is the current title (or decryption step), not the queue: the queue
// count rides along in QueueText so the two can never fight over one bar.
type DownloadDisplayState struct {
	Title      string
	Fraction   float64
	Detail     string // bytes done, speed and ETA
	QueueDone  int
	QueueTotal int
}

// SetDisplayHook installs a callback fired on the main thread whenever the
// window repaints. The inline queue-pane UI uses it to follow the run without
// reading GTK widgets from the download goroutine.
func (pw *ProgressWindow) SetDisplayHook(hook func()) {
	pw.progressMutex.Lock()
	pw.displayHook = hook
	pw.progressMutex.Unlock()
}

// DisplayState returns what the window last painted.
func (pw *ProgressWindow) DisplayState() DownloadDisplayState {
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	state := pw.display
	state.QueueDone, state.QueueTotal = pw.queueDone, pw.queueTotal
	return state
}

// notifyDisplay records what was just painted and mirrors it. Every caller is
// already inside an idle callback, so this is main-thread code.
func (pw *ProgressWindow) notifyDisplay(state DownloadDisplayState) {
	state.Fraction = clampFraction(state.Fraction)
	pw.progressMutex.Lock()
	pw.display = state
	hook := pw.displayHook
	pw.progressMutex.Unlock()
	if hook != nil {
		hook()
	}
}

// transferDetail describes what has been fetched: bytes done, the rate and the
// time left. The window puts it in its bar text and the run bar in its detail
// line, so both surfaces always agree.
func transferDetail(total, toDownload int64, speed float64) string {
	detail := fmt.Sprintf("%s / %s", formatBytes(uint64(total)), formatBytes(uint64(toDownload)))
	if speed <= 0 {
		return detail
	}
	detail += fmt.Sprintf(", %s/s", formatBytes(uint64(int64(speed))))
	if toDownload > total {
		remaining := time.Duration(float64(toDownload-total)/speed) * time.Second
		detail += fmt.Sprintf(", ~%s left", formatDuration(remaining))
	}
	return detail
}

// ProgressFraction reports the overall fraction from the byte counters, rather
// than reading it back off the progress bar.
func (pw *ProgressWindow) ProgressFraction() float64 {
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	if pw.totalToDownload <= 0 {
		return 0
	}
	total := pw.totalDownloaded
	for _, v := range pw.progressPerFile {
		total += v
	}
	return clampFraction(float64(total) / float64(pw.totalToDownload))
}

// Paused reports the transfer pause state; safe from any goroutine.
func (pw *ProgressWindow) Paused() bool {
	pw.controlMutex.Lock()
	defer pw.controlMutex.Unlock()
	return pw.paused
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// SetQueueProgress updates the queue indicator and window title; total <= 0 hides
// the indicator.
func (pw *ProgressWindow) SetQueueProgress(done, total int) {
	pw.progressMutex.Lock()
	pw.queueDone, pw.queueTotal = done, total
	pw.progressMutex.Unlock()

	uiIdleAdd(func() {
		text := queueProgressText(done, total)
		if pw.queueLabel != nil {
			pw.queueLabel.SetText(text)
			pw.queueLabel.SetVisible(text != "")
		}
		if pw.Window != nil {
			title := WINDOW_TITLE_PREFIX + "Downloading"
			if text != "" {
				title += fmt.Sprintf(" (%s)", text)
			}
			pw.Window.SetTitle(title)
		}
		// The queue moving is its own reason to repaint a mirroring surface.
		pw.notifyDisplay(pw.DisplayState())
	})
}

func queueProgressText(done, total int) string {
	if total <= 0 {
		return ""
	}
	if done >= total {
		done = total - 1
	}
	if done < 0 {
		done = 0
	}
	return fmt.Sprintf("Title %d/%d", done+1, total)
}

// SetTitleState satisfies DownloadUI: the window shows per-file progress itself,
// so it has no per-row state to repaint.
func (pw *ProgressWindow) SetTitleState(uint64, queueRowState) {}

// Present and Hide are the DownloadUI names for the window's visibility.
func (pw *ProgressWindow) Present() { pw.Window.Present() }
func (pw *ProgressWindow) Hide()    { pw.Window.SetVisible(false) }

func (pw *ProgressWindow) SetGameTitle(title string) {
	uiIdleAdd(func() {
		pw.gameLabel.SetText(title)
		// A new title: nothing fetched yet for it.
		pw.notifyDisplay(DownloadDisplayState{Title: title})
	})
}

func (pw *ProgressWindow) UpdateDownloadProgress(downloaded int64, filename string) {
	if downloaded == 0 {
		return
	}

	pw.progressMutex.Lock()
	if _, ok := pw.progressPerFile[filename]; !ok {
		pw.progressMutex.Unlock()
		return
	}
	pw.progressPerFile[filename] += downloaded

	if pw.updatePending {
		pw.progressMutex.Unlock()
		return
	}
	pw.updatePending = true
	pw.progressMutex.Unlock()

	uiIdleAddBool(func() bool {
		pw.progressMutex.Lock()
		pw.updatePending = false
		total := pw.totalDownloaded
		for _, v := range pw.progressPerFile {
			total += v
		}
		toDownload := pw.totalToDownload
		pw.progressMutex.Unlock()

		now := time.Now()
		pw.speedAverager.Sample(total, now)
		speed := pw.speedAverager.GetAverageSpeed()
		detail := transferDetail(total, toDownload, speed)

		fraction := clampFraction(float64(total) / float64(toDownload))
		pw.setFraction(fraction)
		pw.bar.SetText(fmt.Sprintf("Downloading... (%s)", detail))
		pw.notifyDisplay(DownloadDisplayState{Title: pw.gameLabel.Text(), Fraction: fraction, Detail: detail})

		return false
	})
}

func (pw *ProgressWindow) UpdateDecryptionProgress(progress float64) {
	pw.progressMutex.Lock()
	pw.decProgress = progress
	if pw.decPending {
		pw.progressMutex.Unlock()
		return
	}
	pw.decPending = true
	pw.progressMutex.Unlock()

	uiIdleAddBool(func() bool {
		pw.setTransferControlsSensitive(false)
		pw.progressMutex.Lock()
		prog := pw.decProgress
		pw.decPending = false
		pw.progressMutex.Unlock()

		pw.setFraction(prog)
		pw.bar.SetText(fmt.Sprintf("Decrypting (%.2f%%)", prog*PERCENT_SCALE))
		pw.notifyDisplay(DownloadDisplayState{
			Title:    pw.gameLabel.Text(),
			Fraction: prog,
			Detail:   fmt.Sprintf("Decrypting (%d%%)", int(math.Round(prog*PERCENT_SCALE))),
		})
		return false
	})
}

// setFraction keeps the bar's accessible valuenow valid: a 0/0 total would
// otherwise hand GTK a NaN fraction and it logs an error on every update.
// clampFraction keeps NaN (a 0/0 total) and out-of-range values away from GTK,
// which otherwise logs an invalid "valuenow" for the progress bar.
func clampFraction(fraction float64) float64 {
	if math.IsNaN(fraction) || fraction < 0 {
		return 0
	}
	if fraction > 1 {
		return 1
	}
	return fraction
}

func (pw *ProgressWindow) setFraction(fraction float64) {
	pw.bar.SetFraction(clampFraction(fraction))
}

func (pw *ProgressWindow) Cancelled() bool {
	if pw == nil {
		return false
	}
	pw.controlMutex.Lock()
	defer pw.controlMutex.Unlock()
	return pw.cancelled
}

func (pw *ProgressWindow) SetCancelled() {
	pw.controlMutex.Lock()
	pw.cancelled = true
	pw.paused = false
	if pw.controlCond != nil {
		pw.controlCond.Broadcast()
	}
	pw.controlMutex.Unlock()

	if pw.cancelledChan != nil {
		pw.cancelOnce.Do(func() {
			close(pw.cancelledChan)
		})
	}

	uiIdleAddBool(func() bool {
		pw.setTransferControlsSensitive(false)
		pw.setPauseState(false)
		pw.gameLabel.SetText("Cancelling...")
		pw.notifyDisplay(DownloadDisplayState{Title: "Cancelling...", Fraction: pw.ProgressFraction()})
		return false
	})
}

// setPauseState updates the pause button's icon and label together: the button
// holds an AdwButtonContent, so GtkButton.SetLabel no longer reaches the text.
func (pw *ProgressWindow) setPauseState(paused bool) {
	if pw.pauseButton == nil {
		return
	}
	content, ok := pw.pauseButton.Child().(*adw.ButtonContent)
	if !ok {
		return
	}
	if paused {
		content.SetIconName("media-playback-start-symbolic")
		content.SetLabel("Resume")
		return
	}
	content.SetIconName("media-playback-pause-symbolic")
	content.SetLabel("Pause")
}

// Done returns a channel closed once when SetCancelled is called.
func (pw *ProgressWindow) Done() <-chan struct{} {
	if pw == nil {
		return nil
	}
	return pw.cancelledChan
}

func (pw *ProgressWindow) WaitIfPaused() bool {
	pw.controlMutex.Lock()
	defer pw.controlMutex.Unlock()

	for pw.paused && !pw.cancelled {
		if pw.controlCond == nil {
			break
		}
		pw.controlCond.Wait()
	}
	return !pw.cancelled
}

func (pw *ProgressWindow) TogglePaused() {
	pw.controlMutex.Lock()
	if pw.cancelled {
		pw.controlMutex.Unlock()
		return
	}
	pw.paused = !pw.paused
	paused := pw.paused
	if !paused && pw.controlCond != nil {
		pw.controlCond.Broadcast()
	}
	pw.controlMutex.Unlock()

	uiIdleAddBool(func() bool {
		pw.setPauseState(paused)
		return false
	})
}

func (pw *ProgressWindow) SetDownloadSize(size int64) {
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	pw.totalToDownload = size
}

func (pw *ProgressWindow) setTransferControlsSensitive(sensitive bool) {
	if pw.cancelButton != nil {
		pw.cancelButton.SetSensitive(sensitive)
	}
	if pw.pauseButton != nil {
		pw.pauseButton.SetSensitive(sensitive)
	}
}

func (pw *ProgressWindow) resetTransferState() {
	pw.controlMutex.Lock()
	pw.cancelled = false
	pw.paused = false
	pw.controlMutex.Unlock()
}

func (pw *ProgressWindow) ResetTotals() {
	uiIdleAdd(func() {
		pw.setTransferControlsSensitive(true)
		pw.setPauseState(false)
		pw.setFraction(0)
		pw.bar.SetText("Preparing...")
		pw.notifyDisplay(DownloadDisplayState{Title: pw.gameLabel.Text()})
	})
	pw.resetTransferState()
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	pw.progressPerFile = make(map[string]int64)
	pw.totalDownloaded = 0
	pw.totalToDownload = 0
	pw.speedAverager.Reset()
}

func (pw *ProgressWindow) ResetTotalsAndErrors() {
	pw.ResetTotals()
	pw.ClearErrors()
}

func (pw *ProgressWindow) MarkFileAsDone(filename string) {
	pw.progressMutex.Lock()
	pw.totalDownloaded += pw.progressPerFile[filename]
	delete(pw.progressPerFile, filename)
	pw.progressMutex.Unlock()
}

func (pw *ProgressWindow) SetTotalDownloadedForFile(filename string, downloaded int64) {
	pw.progressMutex.Lock()
	pw.progressPerFile[filename] = downloaded
	pw.progressMutex.Unlock()
}

func (pw *ProgressWindow) SetStartTime(startTime time.Time) {
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	pw.startTime = startTime
}

func (pw *ProgressWindow) AddErrorWithType(title, errorMsg, tidStr, errorType string, version int) {
	pw.errorsMutex.Lock()
	defer pw.errorsMutex.Unlock()
	pw.errors = append(pw.errors, DownloadError{
		Title:     title,
		Error:     errorMsg,
		TidStr:    tidStr,
		ErrorType: errorType,
		Version:   version,
	})
}

func (pw *ProgressWindow) GetErrors() []DownloadError {
	pw.errorsMutex.Lock()
	defer pw.errorsMutex.Unlock()
	errors := make([]DownloadError, len(pw.errors))
	copy(errors, pw.errors)
	return errors
}

func (pw *ProgressWindow) ClearErrors() {
	pw.errorsMutex.Lock()
	defer pw.errorsMutex.Unlock()
	pw.errors = nil
}

func createProgressWindow(parent *gtk.Window) (*ProgressWindow, error) {
	win := adw.NewWindow()
	win.SetTitle(WINDOW_TITLE_PREFIX + "Downloading")
	win.SetModal(false)
	// The GTK-facing helpers take a *gtk.Window; this is the same object.
	parentWindow := &win.Window
	if parent != nil {
		win.SetTransientFor(parent)
	}
	SetupWindowAccessibility(parentWindow, "Download Progress")
	win.SetDeletable(false)

	header := adw.NewHeaderBar()

	box := gtk.NewBox(gtk.OrientationVertical, 12)
	box.SetMarginBottom(18)
	box.SetMarginEnd(18)
	box.SetMarginStart(18)
	box.SetMarginTop(18)

	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(header)
	toolbar.SetContent(box)
	win.SetContent(toolbar)

	gameLabel := gtk.NewLabel("")
	// Namespaced on purpose: libadwaita gives every preferences-row title the
	// CSS class "title", so a global .title rule would restyle the settings.
	gameLabel.AddCSSClass("progress-title")
	SetupLabelAccessibility(gameLabel, "Game Title")

	queueLabel := gtk.NewLabel("")
	queueLabel.AddCSSClass("queue-position-label")
	queueLabel.SetHAlign(gtk.AlignCenter)
	queueLabel.SetVisible(false)

	titleBox := gtk.NewBox(gtk.OrientationVertical, 2)
	titleBox.Append(gameLabel)
	titleBox.Append(queueLabel)
	box.Append(titleBox)

	progressBar := gtk.NewProgressBar()
	progressBar.SetShowText(true)
	progressBar.SetTooltipText("Download progress bar - Shows current download status, speed, and bytes downloaded")
	box.Append(progressBar)

	cancelContent := adw.NewButtonContent()
	cancelContent.SetIconName("process-stop-symbolic")
	cancelContent.SetLabel("Cancel")
	cancelButton := gtk.NewButton()
	cancelButton.SetChild(cancelContent)
	SetupButtonAccessibility(cancelButton, "Stop the current download operation")
	cancelButton.AddCSSClass("destructive-action")

	pauseContent := adw.NewButtonContent()
	pauseContent.SetIconName("media-playback-pause-symbolic")
	pauseContent.SetLabel("Pause")
	pauseButton := gtk.NewButton()
	pauseButton.SetChild(pauseContent)
	SetupButtonAccessibility(pauseButton, "Temporarily pause or resume downloads")

	bottomhBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	bottomhBox.AddCSSClass("linked")
	bottomhBox.SetHAlign(gtk.AlignEnd)
	bottomhBox.Append(pauseButton)
	bottomhBox.Append(cancelButton)
	box.Append(bottomhBox)

	progressWindow := ProgressWindow{
		Window:        parentWindow,
		gameLabel:     gameLabel,
		queueLabel:    queueLabel,
		bar:           progressBar,
		pauseButton:   pauseButton,
		cancelButton:  cancelButton,
		cancelled:     false,
		paused:        false,
		cancelledChan: make(chan struct{}),
		speedAverager: newSpeedAverager(),
		errors:        make([]DownloadError, 0),
	}
	progressWindow.controlCond = sync.NewCond(&progressWindow.controlMutex)

	progressWindow.pauseButton.ConnectClicked(func() {
		progressWindow.TogglePaused()
	})

	progressWindow.cancelButton.ConnectClicked(func() {
		progressWindow.SetCancelled()
	})

	// Swallow Return/Enter so it cannot activate the button by accident.
	cancelKeyController := gtk.NewEventControllerKey()
	cancelKeyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		return keyval == gdk.KEY_Return || keyval == gdk.KEY_KP_Enter
	})
	progressWindow.cancelButton.AddController(cancelKeyController)

	return &progressWindow, nil
}
