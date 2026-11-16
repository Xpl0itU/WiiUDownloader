package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	progressRenderInterval = 100 * time.Millisecond
	plainRenderInterval    = 5 * time.Second
	progressBarWidth       = 22
)

// terminalProgress implements wiiudownloader.ProgressReporter for the CLI.
type terminalProgress struct {
	mu          sync.Mutex
	out         io.Writer
	interactive bool
	quiet       bool
	finished    bool

	total  int64
	files  map[string]int64
	done   int64
	start  time.Time
	mode   string
	decPct float64

	cancelled  bool
	cancelOnce sync.Once
	cancelCh   chan struct{}

	lastRender time.Time
	lastPlain  time.Time
	lastBytes  int64
	lastSample time.Time
	speed      float64
}

func newTerminalProgress(out io.Writer, interactive, quiet bool) *terminalProgress {
	return &terminalProgress{
		out:         out,
		interactive: interactive && !quiet,
		quiet:       quiet,
		files:       make(map[string]int64),
		cancelCh:    make(chan struct{}),
		start:       time.Now(),
	}
}

func (p *terminalProgress) SetGameTitle(string) {}

func (p *terminalProgress) UpdateDownloadProgress(downloaded int64, filename string) {
	if downloaded <= 0 || p.quiet {
		return
	}
	p.mu.Lock()
	current, exists := p.files[filename]
	if !exists {
		current = 0
	}
	p.files[filename] = current + downloaded
	p.mode = "download"
	p.mu.Unlock()
	p.render(false)
}

func (p *terminalProgress) UpdateDecryptionProgress(progress float64) {
	if p.quiet {
		return
	}
	if math.IsNaN(progress) || progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	p.mu.Lock()
	p.decPct = progress
	p.mode = "decrypt"
	p.mu.Unlock()
	p.render(p.interactive && progress >= 1)
}

func (p *terminalProgress) Cancelled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancelled
}

func (p *terminalProgress) SetCancelled() {
	p.mu.Lock()
	if p.cancelled {
		p.mu.Unlock()
		return
	}
	p.cancelled = true
	p.mu.Unlock()
	p.cancelOnce.Do(func() { close(p.cancelCh) })
}

func (p *terminalProgress) Done() <-chan struct{} {
	return p.cancelCh
}

func (p *terminalProgress) SetDownloadSize(size int64) {
	p.mu.Lock()
	p.total = size
	p.mu.Unlock()
}

func (p *terminalProgress) ResetTotals() {
	p.mu.Lock()
	p.files = make(map[string]int64)
	p.done = 0
	p.total = 0
	p.mode = "download"
	p.decPct = 0
	p.start = time.Now()
	p.speed = 0
	p.lastBytes = 0
	p.lastSample = time.Time{}
	p.lastRender = time.Time{}
	p.lastPlain = time.Time{}
	p.finished = false
	p.mu.Unlock()
}

func (p *terminalProgress) MarkFileAsDone(filename string) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	p.done += p.files[filename]
	delete(p.files, filename)
	p.mu.Unlock()
	p.render(p.interactive)
}

func (p *terminalProgress) SetTotalDownloadedForFile(filename string, downloaded int64) {
	p.mu.Lock()
	p.files[filename] = downloaded
	p.mu.Unlock()
}

func (p *terminalProgress) SetStartTime(startTime time.Time) {
	p.mu.Lock()
	p.start = startTime
	p.mu.Unlock()
}

// Finish clears the transient progress line on a terminal and prints the final
// line when output is redirected (so logs and pipes still end with a result).
func (p *terminalProgress) Finish() {
	if p.quiet {
		return
	}
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		return
	}
	interactive := p.interactive
	p.mu.Unlock()

	if interactive {
		p.mu.Lock()
		p.finished = true
		fmt.Fprint(p.out, "\r\033[K")
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	rendered := !p.lastRender.IsZero() || p.total > 0 || p.done > 0 || len(p.files) > 0
	p.mu.Unlock()
	if rendered {
		p.render(true)
	}
	p.mu.Lock()
	p.finished = true
	p.mu.Unlock()
}

func (p *terminalProgress) downloaded() (int64, int64) {
	total := p.done
	for _, value := range p.files {
		total += value
	}
	return total, p.total
}

func (p *terminalProgress) render(force bool) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	// Metadata (TMD/ticket/certificate) completes before the content total is
	// known; showing those few KiB as "progress" is just noise.
	if p.mode == "download" && p.total <= 0 && !force {
		return
	}

	now := time.Now()
	if !force && !p.lastRender.IsZero() && now.Sub(p.lastRender) < progressRenderInterval {
		return
	}
	if !p.interactive && !force && !p.lastPlain.IsZero() && now.Sub(p.lastPlain) < plainRenderInterval {
		return
	}

	current, total := p.downloaded()
	if !p.lastSample.IsZero() {
		elapsed := now.Sub(p.lastSample)
		if elapsed > 0 {
			instant := float64(current-p.lastBytes) / elapsed.Seconds()
			if instant < 0 {
				instant = 0
			}
			if p.speed == 0 {
				p.speed = instant
			} else {
				p.speed = 0.3*instant + 0.7*p.speed
			}
		}
	}
	p.lastSample = now
	p.lastBytes = current

	var line string
	if p.mode == "decrypt" {
		line = fmt.Sprintf("Decrypting %s %5.1f%%", progressBar(p.decPct), p.decPct*100)
	} else {
		line = transferLine(current, total, p.speed, p.start, now)
	}

	if p.interactive {
		fmt.Fprintf(p.out, "\r\033[K%s", line)
	} else {
		fmt.Fprintln(p.out, line)
		p.lastPlain = now
	}
	p.lastRender = now
}

func progressBar(fraction float64) string {
	if math.IsNaN(fraction) || fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(math.Round(fraction * float64(progressBarWidth)))
	if filled > progressBarWidth {
		filled = progressBarWidth
	}
	if filled == 0 {
		return "[" + strings.Repeat(" ", progressBarWidth) + "]"
	}
	if filled >= progressBarWidth {
		return "[" + strings.Repeat("=", progressBarWidth) + "]"
	}
	return "[" + strings.Repeat("=", filled-1) + ">" + strings.Repeat(" ", progressBarWidth-filled) + "]"
}

func transferLine(current, total int64, speed float64, start, now time.Time) string {
	if total <= 0 {
		line := formatBytes(current)
		if speed > 0 {
			line += "  " + formatRate(speed)
		}
		return line
	}

	fraction := float64(current) / float64(total)
	if fraction > 1 {
		fraction = 1
	}
	line := fmt.Sprintf("%s %5.1f%%  %s / %s", progressBar(fraction), fraction*100, formatBytes(current), formatBytes(total))
	if speed > 0 {
		line += "  " + formatRate(speed)
		if current < total {
			remaining := time.Duration(float64(total-current)/speed) * time.Second
			line += "  ETA " + formatDuration(remaining)
		}
	} else if !start.IsZero() && now.Sub(start) > 2*time.Second && current > 0 {
		average := float64(current) / now.Sub(start).Seconds()
		if average > 0 && current < total {
			remaining := time.Duration(float64(total-current)/average) * time.Second
			line += "  ETA " + formatDuration(remaining)
		}
	}
	return line
}

// interruptHandler turns the first Ctrl+C/SIGTERM into a graceful cancellation
// of the current transfer and the second into an immediate exit.
type interruptHandler struct {
	mu      sync.Mutex
	stderr  io.Writer
	current *terminalProgress
	count   int
	signals chan os.Signal
	done    chan struct{}
	once    sync.Once
}

func newInterruptHandler(stderr io.Writer) *interruptHandler {
	handler := &interruptHandler{
		stderr:  stderr,
		signals: make(chan os.Signal, 1),
		done:    make(chan struct{}),
	}
	signal.Notify(handler.signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			select {
			case <-handler.done:
				return
			case <-handler.signals:
				handler.handle()
			}
		}
	}()
	return handler
}

func (h *interruptHandler) handle() {
	h.mu.Lock()
	if h.current == nil {
		h.mu.Unlock()
		fmt.Fprintln(h.stderr)
		os.Exit(130)
	}
	h.count++
	if h.count > 1 {
		h.mu.Unlock()
		fmt.Fprintln(h.stderr, "\nForce quitting.")
		os.Exit(130)
	}
	progress := h.current
	h.mu.Unlock()

	fmt.Fprintln(h.stderr, "\nCancelling; press Ctrl+C again to force quit.")
	progress.SetCancelled()
}

func (h *interruptHandler) set(progress *terminalProgress) {
	h.mu.Lock()
	h.current = progress
	if progress == nil {
		h.count = 0
	}
	h.mu.Unlock()
}

func (h *interruptHandler) stop() {
	h.once.Do(func() {
		signal.Stop(h.signals)
		close(h.done)
	})
}
