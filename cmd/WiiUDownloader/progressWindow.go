package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type DownloadError struct {
	Title     string
	Error     string
	TidStr    string
	ErrorType string
}

const (
	MAX_SPEEDS         = 32
	SMOOTHING_FACTOR   = 0.2
	PERCENT_SCALE      = 100
	PROGRESS_MIN_WIDTH = 350
)

type SpeedAverager struct {
	speeds       []int64
	averageSpeed int64
}

func newSpeedAverager() *SpeedAverager {
	return &SpeedAverager{
		speeds:       make([]int64, 0, MAX_SPEEDS),
		averageSpeed: 0,
	}
}

func (sa *SpeedAverager) AddSpeed(speed int64) {
	if len(sa.speeds) >= MAX_SPEEDS {
		copy(sa.speeds[:MAX_SPEEDS/2], sa.speeds[MAX_SPEEDS/2:])
		sa.speeds = sa.speeds[:MAX_SPEEDS/2]
	}
	sa.speeds = append(sa.speeds, speed)
}

func calculateDownloadSpeed(downloaded int64, startTime, endTime time.Time) int64 {
	duration := endTime.Sub(startTime).Seconds()
	if duration > 0 {
		return int64(float64(downloaded) / duration)
	}
	return 0
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

func formatDownloadSize(bytes uint64) string {
	const unit = 1000

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	value := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	value = math.Round(value*100) / 100
	return fmt.Sprintf("%.2f %s", value, units[unitIndex])
}

type ProgressWindow struct {
	Window          *gtk.Window
	box             *gtk.Box
	gameLabel       *gtk.Label
	bar             *gtk.ProgressBar
	pauseButton     *gtk.Button
	cancelButton    *gtk.Button
	cancelled       bool
	paused          bool
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
}

func (pw *ProgressWindow) SetGameTitle(title string) {
	glib.IdleAdd(func() {
		pw.gameLabel.SetText(title)
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

	glib.IdleAdd(func() bool {
		pw.setTransferControlsSensitive(true)
		pw.progressMutex.Lock()
		pw.updatePending = false
		total := pw.totalDownloaded
		for _, v := range pw.progressPerFile {
			total += v
		}
		pw.progressMutex.Unlock()
		
		pw.bar.SetFraction(float64(total) / float64(pw.totalToDownload))
		pw.speedAverager.AddSpeed(calculateDownloadSpeed(total, pw.startTime, time.Now()))
		pw.bar.SetText(fmt.Sprintf(
			"Downloading... (%s/%s) (%s/s)",
			formatDownloadSize(uint64(total)),
			formatDownloadSize(uint64(pw.totalToDownload)),
			formatDownloadSize(uint64(int64(pw.speedAverager.GetAverageSpeed()))),
		))
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

	glib.IdleAdd(func() bool {
		pw.setTransferControlsSensitive(false)
		pw.progressMutex.Lock()
		prog := pw.decProgress
		pw.decPending = false
		pw.progressMutex.Unlock()
		
		pw.bar.SetFraction(prog)
		pw.bar.SetText(fmt.Sprintf("Decrypting (%.2f%%)", prog*PERCENT_SCALE))
		return false
	})
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

	glib.IdleAdd(func() bool {
		pw.setTransferControlsSensitive(false)
		if pw.pauseButton != nil {
			pw.pauseButton.SetLabel("Pause")
		}
		pw.gameLabel.SetText("Cancelling...")
		return false
	})
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

	glib.IdleAdd(func() bool {
		if pw.pauseButton != nil {
			if paused {
				pw.pauseButton.SetLabel("Resume")
			} else {
				pw.pauseButton.SetLabel("Pause")
			}
		}
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
	glib.IdleAdd(func() {
		pw.setTransferControlsSensitive(true)
		if pw.pauseButton != nil {
			pw.pauseButton.SetLabel("Pause")
		}
		pw.bar.SetFraction(0)
		pw.bar.SetText("Preparing...")
	})
	pw.resetTransferState()
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	pw.progressPerFile = make(map[string]int64)
	pw.totalDownloaded = 0
	pw.totalToDownload = 0
}

func (pw *ProgressWindow) ResetTotalsAndErrors() {
	glib.IdleAdd(func() {
		pw.setTransferControlsSensitive(true)
		if pw.pauseButton != nil {
			pw.pauseButton.SetLabel("Pause")
		}
		pw.bar.SetFraction(0)
		pw.bar.SetText("Preparing...")
	})
	pw.resetTransferState()
	pw.progressMutex.Lock()
	defer pw.progressMutex.Unlock()
	pw.progressPerFile = make(map[string]int64)
	pw.totalDownloaded = 0
	pw.totalToDownload = 0
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

func (pw *ProgressWindow) AddError(title, errorMsg, tidStr string) {
	pw.errorsMutex.Lock()
	defer pw.errorsMutex.Unlock()
	pw.errors = append(pw.errors, DownloadError{
		Title:  title,
		Error:  errorMsg,
		TidStr: tidStr,
	})
}

func (pw *ProgressWindow) AddErrorWithType(title, errorMsg, tidStr, errorType string) {
	pw.errorsMutex.Lock()
	defer pw.errorsMutex.Unlock()
	pw.errors = append(pw.errors, DownloadError{
		Title:     title,
		Error:     errorMsg,
		TidStr:    tidStr,
		ErrorType: errorType,
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
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("WiiUDownloader - Downloading")
	win.SetTypeHint(gdk.WINDOW_TYPE_HINT_DIALOG)
	win.SetModal(true)
	if parent != nil {
		win.SetTransientFor(parent)
		win.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	} else {
		win.SetPosition(gtk.WIN_POS_CENTER)
	}
	SetupWindowAccessibility(win, "Download Progress")
	win.SetDeletable(false)

	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	if err != nil {
		return nil, err
	}
	win.Add(box)

	gameLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	SetupLabelAccessibility(gameLabel, "Game title label")
	box.PackStart(gameLabel, false, false, 0)

	progressBar, err := gtk.ProgressBarNew()
	if err != nil {
		return nil, err
	}
	progressBar.SetShowText(true)
	progressBar.ToWidget().SetProperty("tooltip-text", "Download progress bar - Shows current download status, speed, and bytes downloaded")
	box.PackStart(progressBar, false, false, 0)

	cancelButton, err := gtk.ButtonNewWithLabel("Cancel")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(cancelButton, "Stop the current download operation")
	pauseButton, err := gtk.ButtonNewWithLabel("Pause")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(pauseButton, "Temporarily pause or resume downloads")

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	if err != nil {
		return nil, err
	}
	bottomhBox.SetSizeRequest(PROGRESS_MIN_WIDTH, -1)
	bottomhBox.PackEnd(cancelButton, false, false, 0)
	bottomhBox.PackEnd(pauseButton, false, false, 0)
	box.SetMarginBottom(5)
	box.SetMarginEnd(5)
	box.SetMarginStart(5)
	box.SetMarginTop(5)
	box.PackEnd(bottomhBox, false, false, 0)

	progressWindow := ProgressWindow{
		Window:        win,
		box:           box,
		gameLabel:     gameLabel,
		bar:           progressBar,
		pauseButton:   pauseButton,
		cancelButton:  cancelButton,
		cancelled:     false,
		paused:        false,
		speedAverager: newSpeedAverager(),
		errors:        make([]DownloadError, 0),
	}
	progressWindow.controlCond = sync.NewCond(&progressWindow.controlMutex)

	progressWindow.pauseButton.Connect("clicked", func() {
		progressWindow.TogglePaused()
	})

	progressWindow.cancelButton.Connect("clicked", func() {
		progressWindow.SetCancelled()
	})

	progressWindow.cancelButton.Connect("key-press-event", func(button *gtk.Button, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		if keyEvent.KeyVal() == gdk.KEY_Return || keyEvent.KeyVal() == gdk.KEY_KP_Enter {
			return true
		}
		return false
	})

	return &progressWindow, nil
}
