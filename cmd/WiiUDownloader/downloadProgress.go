package main

import (
	"fmt"
	"math"
	"sync"
	"time"

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

// DownloadProgress is the app's only progress surface: it owns a run's state
// (byte counters, rate, pause/cancel, error list) for downloads, decryption and
// ticket/cert generation alike, and paints the queue pane's run bar. Worker
// goroutines report into it, so widget writes marshal onto the main loop and the
// counters are mutex-guarded.
type DownloadProgress struct {
	pane   *QueuePane
	window *gtk.Window

	title string

	cancelled     bool
	paused        bool
	cancelledChan chan struct{}
	cancelOnce    sync.Once
	controlMutex  sync.Mutex
	controlCond   *sync.Cond

	totalToDownload int64
	totalDownloaded int64
	progressPerFile map[string]int64
	progressMutex   sync.Mutex
	updatePending   bool
	decPending      bool
	decProgress     float64
	speedAverager   *SpeedAverager
	startTime       time.Time

	queueDone  int
	queueTotal int

	errors      []DownloadError
	errorsMutex sync.Mutex
}

func newDownloadProgress(pane *QueuePane, window *gtk.Window) *DownloadProgress {
	dp := &DownloadProgress{
		pane:            pane,
		window:          window,
		cancelledChan:   make(chan struct{}),
		progressPerFile: make(map[string]int64),
		speedAverager:   newSpeedAverager(),
		errors:          make([]DownloadError, 0),
	}
	dp.controlCond = sync.NewCond(&dp.controlMutex)
	pane.SetRunCallbacks(dp.TogglePaused, dp.SetCancelled)
	return dp
}

// Start shows the run bar and names the run.
func (dp *DownloadProgress) Start(title string) {
	dp.SetGameTitle(title)
	dp.pane.BeginRun()
}

// Finish hides the run bar and puts the window title back.
func (dp *DownloadProgress) Finish() {
	dp.pane.EndRun()
	uiIdleAdd(func() {
		if dp.window != nil {
			dp.window.SetTitle(APP_NAME)
		}
	})
}

// transferDetail is the "done / total, rate, time left" line.
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

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

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

// ProgressFraction reads the byte counters rather than the progress bar, which a
// worker goroutine must not touch.
func (dp *DownloadProgress) ProgressFraction() float64 {
	dp.progressMutex.Lock()
	defer dp.progressMutex.Unlock()
	if dp.totalToDownload <= 0 {
		return 0
	}
	total := dp.totalDownloaded
	for _, v := range dp.progressPerFile {
		total += v
	}
	return clampFraction(float64(total) / float64(dp.totalToDownload))
}

// Paused reports the transfer pause state; safe from any goroutine.
func (dp *DownloadProgress) Paused() bool {
	dp.controlMutex.Lock()
	defer dp.controlMutex.Unlock()
	return dp.paused
}

// SetQueueProgress reports the run's position in the queue. total <= 0 hides it.
func (dp *DownloadProgress) SetQueueProgress(done, total int) {
	dp.progressMutex.Lock()
	dp.queueDone, dp.queueTotal = done, total
	title := dp.title
	dp.progressMutex.Unlock()

	dp.pane.SetRunQueueProgress(done, total)
	dp.setWindowTitle(title, done, total)
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

func (dp *DownloadProgress) setWindowTitle(title string, done, total int) {
	text := runTitleText(title, done, total)
	uiIdleAdd(func() {
		if dp.window != nil {
			dp.window.SetTitle(text)
		}
	})
}

func runTitleText(title string, done, total int) string {
	if title == "" {
		return APP_NAME
	}
	text := WINDOW_TITLE_PREFIX + title
	if position := queueProgressText(done, total); position != "" {
		text += fmt.Sprintf(" (%s)", position)
	}
	return text
}

func (dp *DownloadProgress) SetGameTitle(title string) {
	dp.progressMutex.Lock()
	dp.title = title
	done, total := dp.queueDone, dp.queueTotal
	dp.progressMutex.Unlock()

	dp.pane.SetRunProgress(title, 0, "")
	dp.setWindowTitle(title, done, total)
}

func (dp *DownloadProgress) UpdateDownloadProgress(downloaded int64, filename string) {
	if downloaded == 0 {
		return
	}

	dp.progressMutex.Lock()
	if _, ok := dp.progressPerFile[filename]; !ok {
		dp.progressMutex.Unlock()
		return
	}
	dp.progressPerFile[filename] += downloaded

	if dp.updatePending {
		dp.progressMutex.Unlock()
		return
	}
	dp.updatePending = true
	dp.progressMutex.Unlock()

	// Sample the rate on the main thread; the flag makes updates land one at a time.
	uiIdleAddBool(func() bool {
		dp.progressMutex.Lock()
		dp.updatePending = false
		total := dp.totalDownloaded
		for _, v := range dp.progressPerFile {
			total += v
		}
		toDownload := dp.totalToDownload
		title := dp.title
		dp.progressMutex.Unlock()

		now := time.Now()
		dp.speedAverager.Sample(total, now)
		speed := dp.speedAverager.GetAverageSpeed()
		detail := transferDetail(total, toDownload, speed)

		dp.pane.SetRunProgress(title, clampFraction(float64(total)/float64(toDownload)), detail)
		return false
	})
}

func (dp *DownloadProgress) UpdateDecryptionProgress(progress float64) {
	dp.progressMutex.Lock()
	dp.decProgress = progress
	if dp.decPending {
		dp.progressMutex.Unlock()
		return
	}
	dp.decPending = true
	dp.progressMutex.Unlock()

	uiIdleAddBool(func() bool {
		dp.pane.SetRunControlsSensitive(false)
		dp.progressMutex.Lock()
		prog := dp.decProgress
		title := dp.title
		dp.decPending = false
		dp.progressMutex.Unlock()

		dp.pane.SetRunProgress(title, prog, fmt.Sprintf("Decrypting (%d%%)", int(math.Round(prog*PERCENT_SCALE))))
		return false
	})
}

func (dp *DownloadProgress) Cancelled() bool {
	if dp == nil {
		return false
	}
	dp.controlMutex.Lock()
	defer dp.controlMutex.Unlock()
	return dp.cancelled
}

func (dp *DownloadProgress) SetCancelled() {
	dp.controlMutex.Lock()
	dp.cancelled = true
	dp.paused = false
	if dp.controlCond != nil {
		dp.controlCond.Broadcast()
	}
	dp.controlMutex.Unlock()

	if dp.cancelledChan != nil {
		dp.cancelOnce.Do(func() {
			close(dp.cancelledChan)
		})
	}

	dp.pane.SetRunControlsSensitive(false)
	dp.pane.SetRunPaused(false)
	dp.pane.SetRunProgress("Cancelling...", dp.ProgressFraction(), "")
}

// Done returns a channel closed once when SetCancelled is called.
func (dp *DownloadProgress) Done() <-chan struct{} {
	if dp == nil {
		return nil
	}
	return dp.cancelledChan
}

func (dp *DownloadProgress) WaitIfPaused() bool {
	dp.controlMutex.Lock()
	defer dp.controlMutex.Unlock()

	for dp.paused && !dp.cancelled {
		if dp.controlCond == nil {
			break
		}
		dp.controlCond.Wait()
	}
	return !dp.cancelled
}

func (dp *DownloadProgress) TogglePaused() {
	dp.controlMutex.Lock()
	if dp.cancelled {
		dp.controlMutex.Unlock()
		return
	}
	dp.paused = !dp.paused
	paused := dp.paused
	if !paused && dp.controlCond != nil {
		dp.controlCond.Broadcast()
	}
	dp.controlMutex.Unlock()

	dp.pane.SetRunPaused(paused)
}

func (dp *DownloadProgress) SetDownloadSize(size int64) {
	dp.progressMutex.Lock()
	defer dp.progressMutex.Unlock()
	dp.totalToDownload = size
}

func (dp *DownloadProgress) resetTransferState() {
	dp.controlMutex.Lock()
	dp.cancelled = false
	dp.paused = false
	dp.controlMutex.Unlock()
}

func (dp *DownloadProgress) ResetTotals() {
	dp.progressMutex.Lock()
	title := dp.title
	done, total := dp.queueDone, dp.queueTotal
	dp.progressMutex.Unlock()

	dp.pane.SetRunControlsSensitive(true)
	dp.pane.SetRunPaused(false)
	dp.pane.SetRunProgress(title, 0, "")
	dp.setWindowTitle(title, done, total)

	dp.resetTransferState()
	dp.progressMutex.Lock()
	defer dp.progressMutex.Unlock()
	dp.progressPerFile = make(map[string]int64)
	dp.totalDownloaded = 0
	dp.totalToDownload = 0
	dp.decPending = false
	dp.decProgress = 0
	dp.speedAverager.Reset()
}

func (dp *DownloadProgress) ResetTotalsAndErrors() {
	dp.ResetTotals()
	dp.ClearErrors()
}

func (dp *DownloadProgress) MarkFileAsDone(filename string) {
	dp.progressMutex.Lock()
	dp.totalDownloaded += dp.progressPerFile[filename]
	delete(dp.progressPerFile, filename)
	dp.progressMutex.Unlock()
}

func (dp *DownloadProgress) SetTotalDownloadedForFile(filename string, downloaded int64) {
	dp.progressMutex.Lock()
	dp.progressPerFile[filename] = downloaded
	dp.progressMutex.Unlock()
}

func (dp *DownloadProgress) SetStartTime(startTime time.Time) {
	dp.progressMutex.Lock()
	defer dp.progressMutex.Unlock()
	dp.startTime = startTime
}

func (dp *DownloadProgress) AddErrorWithType(title, errorMsg, tidStr, errorType string, version int) {
	dp.errorsMutex.Lock()
	defer dp.errorsMutex.Unlock()
	dp.errors = append(dp.errors, DownloadError{
		Title:     title,
		Error:     errorMsg,
		TidStr:    tidStr,
		ErrorType: errorType,
		Version:   version,
	})
}

func (dp *DownloadProgress) GetErrors() []DownloadError {
	dp.errorsMutex.Lock()
	defer dp.errorsMutex.Unlock()
	errors := make([]DownloadError, len(dp.errors))
	copy(errors, dp.errors)
	return errors
}

func (dp *DownloadProgress) ClearErrors() {
	dp.errorsMutex.Lock()
	defer dp.errorsMutex.Unlock()
	dp.errors = nil
}

// SetTitleState repaints one queue row, so the pane shows where each title is.
func (dp *DownloadProgress) SetTitleState(titleID uint64, state queueRowState) {
	dp.pane.SetTitleState(titleID, state)
}
