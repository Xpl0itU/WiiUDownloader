package main

import (
	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

// DownloadUI is where a download run reports. ProgressWindow (unchanged, and
// still the surface for decryption, tickets and certs) is the standalone
// implementation; inlineDownloadUI mirrors the same run into the queue pane.
// Config.UseInlineDownloadUI picks one, so plugging the window back in for
// downloads is a single flag.
type DownloadUI interface {
	wiiudownloader.ProgressReporter
	Present()
	Hide()
	GetErrors() []DownloadError
	AddErrorWithType(title, errorMsg, tidStr, errorType string, version int)
	ResetTotalsAndErrors()
	SetQueueProgress(done, total int)
	// SetTitleState repaints one queue row; the window has no rows, so it is a
	// no-op there.
	SetTitleState(titleID uint64, state queueRowState)
}

// inlineDownloadUI drives the queue pane while the progress window stays the
// state owner: the window still holds cancellation, pause, totals and the error
// list, it is simply never shown. Embedding it means the whole ProgressReporter
// surface comes along for free, so only the mirrored calls need writing.
//
// Every call into the run arrives on the download goroutine. GTK is not
// thread-safe, so this type never touches a pane widget itself: the window's
// display hook repaints the pane from the main loop, and the pane's own methods
// marshal what they write.
type inlineDownloadUI struct {
	*ProgressWindow
	pane *QueuePane
}

func newInlineDownloadUI(window *ProgressWindow, pane *QueuePane) *inlineDownloadUI {
	ui := &inlineDownloadUI{ProgressWindow: window, pane: pane}
	window.SetDisplayHook(ui.mirrorDisplay)
	return ui
}

// mirrorDisplay copies what the window just painted into the run bar. It runs
// on the main thread, so no marshalling is needed here.
func (ui *inlineDownloadUI) mirrorDisplay() {
	state := ui.ProgressWindow.DisplayState()
	ui.pane.SetInlineProgress(state.Title, state.Fraction, state.Detail)
	ui.pane.SetInlineQueueProgress(state.QueueDone, state.QueueTotal)
}

// TogglePaused keeps the pane's control in step with the run's real state.
func (ui *inlineDownloadUI) TogglePaused() {
	ui.ProgressWindow.TogglePaused()
	ui.pane.SetInlinePaused(ui.ProgressWindow.Paused())
}

func (ui *inlineDownloadUI) ResetTotalsAndErrors() {
	ui.ProgressWindow.ResetTotalsAndErrors()
	ui.pane.BeginInlineRun()
	ui.pane.SetRunCallbacks(ui.TogglePaused, ui.ProgressWindow.SetCancelled)
}

func (ui *inlineDownloadUI) SetTitleState(titleID uint64, state queueRowState) {
	ui.pane.SetTitleState(titleID, state)
}

func (ui *inlineDownloadUI) Present() { ui.pane.BeginInlineRun() }
func (ui *inlineDownloadUI) Hide()    { ui.pane.EndInlineRun() }

func (ui *inlineDownloadUI) GetErrors() []DownloadError {
	return ui.ProgressWindow.GetErrors()
}

// newDownloadUI builds the surface a run should report to. The progress window
// is created either way: it is the state owner behind the inline UI, and the
// fallback surface when the experiment is switched off.
func (mw *MainWindow) newDownloadUI(config *Config) DownloadUI {
	progressWindow, err := createProgressWindow(mw.window)
	if err != nil {
		return nil
	}
	mw.progressWindow = progressWindow

	// The compact layout hides the queue pane, and with it the run bar. Fall
	// back to the window rather than reporting progress to a hidden widget.
	if config == nil || !config.UseInlineDownloadUI || !mw.queuePaneVisible() {
		return progressWindow
	}
	return newInlineDownloadUI(progressWindow, mw.queuePane)
}

func (mw *MainWindow) queuePaneVisible() bool {
	container := mw.queuePane.GetContainer()
	return container != nil && container.Visible()
}
