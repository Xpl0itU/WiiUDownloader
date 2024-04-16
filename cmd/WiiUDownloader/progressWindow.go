package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	MAX_SPEEDS       = 32
	SMOOTHING_FACTOR = 0.2
)

type SpeedAverager struct {
	speeds       []int64
	averageSpeed int64
}

func newSpeedAverager() *SpeedAverager {
	return &SpeedAverager{
		speeds:       make([]int64, MAX_SPEEDS),
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
	var total int64
	for _, speed := range sa.speeds {
		total += speed
	}
	sa.averageSpeed = total / int64(len(sa.speeds))
}

func (sa *SpeedAverager) GetAverageSpeed() float64 {
	sa.calculateAverageOfSpeeds()
	return SMOOTHING_FACTOR*float64(sa.speeds[len(sa.speeds)-1]) + (1-SMOOTHING_FACTOR)*float64(sa.averageSpeed)
}

type ProgressWindow struct {
	Window          *gtk.Window
	box             *gtk.Box
	gameLabel       *gtk.Label
	bar             *gtk.ProgressBar
	cancelButton    *gtk.Button
	cancelled       bool
	totalToDownload int64
	totalDownloaded int64
	progressPerFile map[string]int64 // map of filename to downloaded bytes
	progressMutex   sync.Mutex
	speedAverager   *SpeedAverager
	startTime       time.Time
}

func (pw *ProgressWindow) SetGameTitle(title string) {
	glib.IdleAdd(func() {
		pw.gameLabel.SetText(title)
	})
	for gtk.EventsPending() {
		gtk.MainIteration()
	}
}

func (pw *ProgressWindow) UpdateDownloadProgress(downloaded int64, filename string) {
	glib.IdleAdd(func() {
		pw.cancelButton.SetSensitive(true)
		pw.progressMutex.Lock()
		pw.progressPerFile[filename] += downloaded
		total := pw.totalDownloaded
		for _, v := range pw.progressPerFile {
			total += v
		}
		pw.progressMutex.Unlock()
		pw.bar.SetFraction(float64(total) / float64(pw.totalToDownload))
		pw.speedAverager.AddSpeed(calculateDownloadSpeed(total, pw.startTime, time.Now()))
		pw.bar.SetText(fmt.Sprintf("Downloading... (%s/%s) (%s/s)", humanize.Bytes(uint64(total)), humanize.Bytes(uint64(pw.totalToDownload)), humanize.Bytes(uint64(int64(pw.speedAverager.GetAverageSpeed())))))
	})
	for gtk.EventsPending() {
		gtk.MainIteration()
	}
}

func (pw *ProgressWindow) UpdateDecryptionProgress(progress float64) {
	glib.IdleAdd(func() {
		pw.cancelButton.SetSensitive(false)
		pw.bar.SetFraction(progress)
		pw.bar.SetText(fmt.Sprintf("Decrypting (%.2f%%)", progress*100))
	})
	for gtk.EventsPending() {
		gtk.MainIteration()
	}
}

func (pw *ProgressWindow) Cancelled() bool {
	return pw.cancelled
}

func (pw *ProgressWindow) SetCancelled() {
	glib.IdleAdd(func() {
		pw.cancelButton.SetSensitive(false)
		pw.SetGameTitle("Cancelling...")
	})
	for gtk.EventsPending() {
		gtk.MainIteration()
	}
}

func (pw *ProgressWindow) SetDownloadSize(size int64) {
	pw.totalToDownload = size
}

func (pw *ProgressWindow) ResetTotals() {
	pw.progressPerFile = make(map[string]int64)
	pw.totalDownloaded = 0
	pw.totalToDownload = 0
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
	pw.startTime = startTime
}

func createProgressWindow(parent *gtk.ApplicationWindow) (*ProgressWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("WiiUDownloader - Downloading")

	win.SetTransientFor(parent)
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
	box.PackStart(gameLabel, false, false, 0)

	progressBar, err := gtk.ProgressBarNew()
	if err != nil {
		return nil, err
	}
	progressBar.SetShowText(true)
	box.PackStart(progressBar, false, false, 0)

	cancelButton, err := gtk.ButtonNewWithLabel("Cancel")
	if err != nil {
		return nil, err
	}

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	if err != nil {
		return nil, err
	}
	bottomhBox.PackEnd(cancelButton, false, false, 0)
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
		cancelButton:  cancelButton,
		cancelled:     false,
		speedAverager: newSpeedAverager(),
	}

	progressWindow.cancelButton.Connect("clicked", func() {
		progressWindow.cancelled = true
		progressWindow.SetCancelled()
	})

	return &progressWindow, nil
}
