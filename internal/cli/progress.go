package cli

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

// TerminalProgressReporter implements wiiudownloader.ProgressReporter with
// real-time terminal output, animated progress bars, speed, ETA, and signal handling.
type TerminalProgressReporter struct {
	mu sync.Mutex

	title           string
	totalSize       int64
	totalDownloaded int64
	progressPerFile map[string]int64

	startTime       time.Time
	lastSampleTime  time.Time
	lastSampleBytes int64
	currentSpeed    float64
	speeds          []float64

	isDecrypting bool
	decProgress  float64

	isTTY bool
	quiet bool

	cancelled     bool
	cancelledChan chan struct{}
	cancelOnce    sync.Once

	stopTicker chan struct{}
	tickerDone chan struct{}

	sigChan chan os.Signal
}

// NewTerminalProgressReporter creates a new terminal progress reporter.
func NewTerminalProgressReporter(quiet bool) *TerminalProgressReporter {
	isTTY := checkIsTTY()
	reporter := &TerminalProgressReporter{
		progressPerFile: make(map[string]int64),
		speeds:          make([]float64, 0, 16),
		cancelledChan:   make(chan struct{}),
		stopTicker:      make(chan struct{}),
		tickerDone:      make(chan struct{}),
		isTTY:           isTTY,
		quiet:           quiet,
		sigChan:         make(chan os.Signal, 2),
	}

	// Intercept Ctrl+C
	signal.Notify(reporter.sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range reporter.sigChan {
			reporter.mu.Lock()
			if !reporter.cancelled {
				reporter.cancelled = true
				reporter.cancelOnce.Do(func() {
					close(reporter.cancelledChan)
				})
				reporter.mu.Unlock()
				fmt.Fprintln(os.Stderr, "\n\rDownload cancelled. Cleaning up... (press Ctrl+C again to force exit)")
			} else {
				reporter.mu.Unlock()
				fmt.Fprintln(os.Stderr, "\n\rForce exiting...")
				os.Exit(130)
			}
		}
	}()

	if !quiet && isTTY {
		go reporter.renderLoop()
	} else {
		close(reporter.tickerDone)
	}

	return reporter
}

func checkIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (r *TerminalProgressReporter) renderLoop() {
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	defer close(r.tickerDone)

	for {
		select {
		case <-r.stopTicker:
			r.renderLine(true)
			return
		case <-ticker.C:
			r.renderLine(false)
		}
	}
}

func (r *TerminalProgressReporter) renderLine(final bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.quiet || !r.isTTY {
		return
	}

	now := time.Now()
	total := r.totalDownloaded
	for _, v := range r.progressPerFile {
		total += v
	}

	// Update speed calculation
	if !r.lastSampleTime.IsZero() {
		elapsed := now.Sub(r.lastSampleTime)
		if elapsed >= 100*time.Millisecond {
			bytesDiff := total - r.lastSampleBytes
			if bytesDiff >= 0 {
				instantSpeed := float64(bytesDiff) / elapsed.Seconds()
				r.addSpeedSample(instantSpeed)
			}
			r.lastSampleBytes = total
			r.lastSampleTime = now
		}
	} else {
		r.lastSampleBytes = total
		r.lastSampleTime = now
	}

	var pct float64
	if r.isDecrypting {
		pct = r.decProgress * 100
	} else if r.totalSize > 0 {
		pct = (float64(total) / float64(r.totalSize)) * 100
		if pct > 100 {
			pct = 100
		}
	}

	barWidth := 25
	filled := int((pct / 100.0) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}
	empty := barWidth - filled

	bar := strings.Repeat("=", filled)
	if filled < barWidth && empty > 0 {
		bar += ">"
		empty--
	}
	bar += strings.Repeat(" ", empty)

	var line string
	if r.isDecrypting {
		line = fmt.Sprintf("\r  [Decrypting] [%s] %5.1f%%", bar, pct)
	} else {
		speedStr := ""
		if r.currentSpeed > 0 {
			speedStr = fmt.Sprintf(" %s/s", wiiudownloader.FormatBytes(uint64(int64(r.currentSpeed))))
		}

		etaStr := ""
		if r.currentSpeed > 0 && r.totalSize > total {
			remainingSec := float64(r.totalSize-total) / r.currentSpeed
			etaStr = fmt.Sprintf(" ETA %s", wiiudownloader.FormatDuration(time.Duration(remainingSec)*time.Second))
		}

		sizeStr := ""
		if r.totalSize > 0 {
			sizeStr = fmt.Sprintf(" (%s / %s)", wiiudownloader.FormatBytes(uint64(total)), wiiudownloader.FormatBytes(uint64(r.totalSize)))
		} else if total > 0 {
			sizeStr = fmt.Sprintf(" (%s)", wiiudownloader.FormatBytes(uint64(total)))
		}

		line = fmt.Sprintf("\r  [%s] %5.1f%%%s%s%s", bar, pct, sizeStr, speedStr, etaStr)
	}

	// Pad with spaces to clear trailing text
	if len(line) < 79 {
		line += strings.Repeat(" ", 79-len(line))
	}
	fmt.Print(line)

	if final {
		fmt.Println()
	}
}

func (r *TerminalProgressReporter) addSpeedSample(speed float64) {
	if math.IsNaN(speed) || speed < 0 {
		return
	}
	if len(r.speeds) >= 16 {
		r.speeds = r.speeds[1:]
	}
	r.speeds = append(r.speeds, speed)

	var sum float64
	for _, s := range r.speeds {
		sum += s
	}
	avg := sum / float64(len(r.speeds))
	r.currentSpeed = 0.3*speed + 0.7*avg
}

// ClearLine wipes the current progress line from the terminal.
func (r *TerminalProgressReporter) ClearLine() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isTTY && !r.quiet {
		fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")
	}
}

// Close stops the render loop and removes signal handlers.
func (r *TerminalProgressReporter) Close() {
	signal.Stop(r.sigChan)
	if r.isTTY && !r.quiet {
		select {
		case r.stopTicker <- struct{}{}:
			<-r.tickerDone
		default:
		}
		r.ClearLine()
	}
}

// SetGameTitle is called by downloader when starting a title download.
func (r *TerminalProgressReporter) SetGameTitle(title string) {
	r.mu.Lock()
	r.title = title
	r.mu.Unlock()

	if !r.quiet {
		if r.isTTY {
			fmt.Printf("\n▶ Downloading: %s\n", title)
		} else {
			fmt.Printf("Downloading: %s\n", title)
		}
	}
}

// UpdateDownloadProgress reports incremental downloaded bytes for a content file.
func (r *TerminalProgressReporter) UpdateDownloadProgress(downloaded int64, filename string) {
	if downloaded <= 0 {
		return
	}
	r.mu.Lock()
	r.progressPerFile[filename] += downloaded
	r.mu.Unlock()
}

// UpdateDecryptionProgress reports extraction/decryption progress (0.0 to 1.0).
func (r *TerminalProgressReporter) UpdateDecryptionProgress(progress float64) {
	r.mu.Lock()
	r.isDecrypting = true
	r.decProgress = progress
	r.mu.Unlock()

	if !r.isTTY && !r.quiet {
		pct := int(progress * 100)
		if pct%25 == 0 {
			fmt.Printf("Decryption progress: %d%%\n", pct)
		}
	}
}

// Cancelled returns whether the user has requested cancellation.
func (r *TerminalProgressReporter) Cancelled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelled
}

// SetCancelled marks the operation as cancelled.
func (r *TerminalProgressReporter) SetCancelled() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelled = true
	r.cancelOnce.Do(func() {
		close(r.cancelledChan)
	})
}

// Done returns a channel that is closed upon cancellation.
func (r *TerminalProgressReporter) Done() <-chan struct{} {
	return r.cancelledChan
}

// SetDownloadSize sets the total payload size for all contents.
func (r *TerminalProgressReporter) SetDownloadSize(size int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalSize = size
}

// ResetTotals resets per-title counters.
func (r *TerminalProgressReporter) ResetTotals() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalDownloaded = 0
	r.progressPerFile = make(map[string]int64)
	r.speeds = r.speeds[:0]
	r.currentSpeed = 0
	r.isDecrypting = false
	r.decProgress = 0
	r.lastSampleTime = time.Time{}
	r.lastSampleBytes = 0
}

// MarkFileAsDone commits a content file's progress into the total.
func (r *TerminalProgressReporter) MarkFileAsDone(filename string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalDownloaded += r.progressPerFile[filename]
	delete(r.progressPerFile, filename)
}

// SetTotalDownloadedForFile sets the downloaded bytes for already-validated files.
func (r *TerminalProgressReporter) SetTotalDownloadedForFile(filename string, downloaded int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progressPerFile[filename] = downloaded
}

// SetStartTime records the download start time.
func (r *TerminalProgressReporter) SetStartTime(startTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startTime = startTime
	r.lastSampleTime = startTime
	r.lastSampleBytes = 0
}
