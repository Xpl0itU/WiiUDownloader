package main

import (
	"math"
	"sync"
	"time"
)

const tuiProgressInterval = 80 * time.Millisecond

type tuiProgressMsg struct {
	Current int64
	Total   int64
	Phase   string
	Decrypt float64
}

// tuiProgress is a ProgressReporter whose snapshots are delivered to the
// Bubble Tea event loop through a channel. The channel is per operation, so a
// slow or abandoned UI cannot block the downloader.
type tuiProgress struct {
	ch chan<- tuiProgressMsg

	mu         sync.Mutex
	cancelled  bool
	cancelCh   chan struct{}
	cancelOnce sync.Once

	files     map[string]int64
	completed int64
	total     int64
	phase     string
	decrypt   float64
	lastEmit  time.Time
}

func newTUIProgress(ch chan<- tuiProgressMsg) *tuiProgress {
	return &tuiProgress{
		ch:       ch,
		cancelCh: make(chan struct{}),
		files:    make(map[string]int64),
		phase:    "download",
	}
}

func (p *tuiProgress) emit(force bool) {
	p.mu.Lock()
	if !force && !p.lastEmit.IsZero() && time.Since(p.lastEmit) < tuiProgressInterval {
		p.mu.Unlock()
		return
	}
	p.lastEmit = time.Now()
	current := p.completed
	for _, value := range p.files {
		current += value
	}
	msg := tuiProgressMsg{
		Current: current,
		Total:   p.total,
		Phase:   p.phase,
		Decrypt: p.decrypt,
	}
	p.mu.Unlock()

	select {
	case p.ch <- msg:
	default:
	}
}

func (p *tuiProgress) SetGameTitle(string) {}

func (p *tuiProgress) UpdateDownloadProgress(downloaded int64, filename string) {
	if downloaded <= 0 {
		return
	}
	p.mu.Lock()
	p.files[filename] += downloaded
	p.phase = "download"
	p.mu.Unlock()
	p.emit(false)
}

func (p *tuiProgress) UpdateDecryptionProgress(progress float64) {
	if math.IsNaN(progress) || progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	p.mu.Lock()
	p.phase = "decrypt"
	p.decrypt = progress
	p.mu.Unlock()
	p.emit(progress >= 1)
}

func (p *tuiProgress) Cancelled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancelled
}

func (p *tuiProgress) SetCancelled() {
	p.mu.Lock()
	if p.cancelled {
		p.mu.Unlock()
		return
	}
	p.cancelled = true
	p.mu.Unlock()
	p.cancelOnce.Do(func() { close(p.cancelCh) })
}

func (p *tuiProgress) Done() <-chan struct{} { return p.cancelCh }

func (p *tuiProgress) SetDownloadSize(size int64) {
	p.mu.Lock()
	p.total = size
	p.mu.Unlock()
	p.emit(true)
}

func (p *tuiProgress) ResetTotals() {
	p.mu.Lock()
	p.files = make(map[string]int64)
	p.completed = 0
	p.total = 0
	p.phase = "download"
	p.decrypt = 0
	p.lastEmit = time.Time{}
	p.mu.Unlock()
	p.emit(true)
}

func (p *tuiProgress) MarkFileAsDone(filename string) {
	p.mu.Lock()
	p.completed += p.files[filename]
	delete(p.files, filename)
	p.mu.Unlock()
	p.emit(true)
}

func (p *tuiProgress) SetTotalDownloadedForFile(filename string, downloaded int64) {
	p.mu.Lock()
	p.files[filename] = downloaded
	p.mu.Unlock()
}

func (p *tuiProgress) SetStartTime(time.Time) {}
