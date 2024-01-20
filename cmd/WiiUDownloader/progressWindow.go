package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const smoothingFactor = 0.1

type ProgressWindow struct {
	Window          *gtk.Window
	box             *gtk.Box
	gameLabel       *gtk.Label
	bar             *gtk.ProgressBar
	cancelButton    *gtk.Button
	cancelled       bool
	cancelFunc      context.CancelFunc
	totalToDownload int64
	totalDownloaded int64
	lastUpdateTime  time.Time
	averageSpeed    float64
}

func (pw *ProgressWindow) SetGameTitle(title string) {
	glib.IdleAdd(func() {
		pw.gameLabel.SetText(title)
	})
	for gtk.EventsPending() {
		gtk.MainIteration()
	}
}

func (pw *ProgressWindow) UpdateDownloadProgress(downloaded, speed int64, filePath string) {
	glib.IdleAdd(func() {
		pw.cancelButton.SetSensitive(true)
		currentDownload := downloaded + pw.totalDownloaded
		pw.bar.SetFraction(float64(currentDownload) / float64(pw.totalToDownload))

		pw.averageSpeed = smoothingFactor*float64(speed) + (1-smoothingFactor)*pw.averageSpeed

		pw.bar.SetText(fmt.Sprintf("Downloading %s (%s/%s) (%s/s)",
			filePath,
			humanize.Bytes(uint64(currentDownload)),
			humanize.Bytes(uint64(pw.totalToDownload)),
			humanize.Bytes(uint64(pw.averageSpeed)),
		))

		pw.lastUpdateTime = time.Now()
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
	pw.cancelFunc()
}

func (pw *ProgressWindow) SetDownloadSize(size int64) {
	pw.totalToDownload = size
}

func (pw *ProgressWindow) SetTotalDownloaded(total int64) {
	pw.totalDownloaded = total
}

func (pw *ProgressWindow) AddToTotalDownloaded(toAdd int64) {
	pw.totalDownloaded += toAdd
}

func createProgressWindow(parent *gtk.ApplicationWindow) (*ProgressWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("WiiUDownloader - Downloading")

	win.SetTransientFor(parent)

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
		Window:       win,
		box:          box,
		gameLabel:    gameLabel,
		bar:          progressBar,
		cancelButton: cancelButton,
		cancelled:    false,
	}

	progressWindow.cancelButton.Connect("clicked", func() {
		progressWindow.cancelled = true
		progressWindow.SetCancelled()
	})

	return &progressWindow, nil
}
