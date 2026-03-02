package wiiudownloader

import (
	"io"
	"time"
)

const WRITER_PROGRESS_FLUSH_INTERVAL = 100 * time.Millisecond

type WriterProgress struct {
	writer               io.Writer
	progressReporter     ProgressReporter
	updateProgressTicker *time.Ticker
	downloadToReport     int64
	filename             string
}

type pauseWaiter interface {
	WaitIfPaused() bool
}

func newWriterProgress(writer io.Writer, progressReporter ProgressReporter, filename string) *WriterProgress {
	return &WriterProgress{writer: writer, progressReporter: progressReporter, updateProgressTicker: time.NewTicker(WRITER_PROGRESS_FLUSH_INTERVAL), downloadToReport: 0, filename: filename}
}

func (r *WriterProgress) Write(p []byte) (n int, err error) {
	if waiter, ok := r.progressReporter.(pauseWaiter); ok {
		if !waiter.WaitIfPaused() {
			return 0, errCancel
		}
	}
	if r.progressReporter != nil && r.progressReporter.Cancelled() {
		return 0, errCancel
	}
	n, err = r.writer.Write(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	r.downloadToReport += int64(n)

	select {
	case <-r.updateProgressTicker.C:
		r.flushPending()
	default:
	}
	return n, err
}

func (r *WriterProgress) Close() error {
	r.updateProgressTicker.Stop()
	r.flushPending()
	return nil
}

func (r *WriterProgress) flushPending() {
	if r.downloadToReport > 0 {
		if r.progressReporter != nil {
			r.progressReporter.UpdateDownloadProgress(r.downloadToReport, r.filename)
		}
		r.downloadToReport = 0
	}
}
