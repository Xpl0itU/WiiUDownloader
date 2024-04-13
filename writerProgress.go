package wiiudownloader

import (
	"io"
	"time"
)

type WriterProgress struct {
	writer               io.Writer
	progressReporter     ProgressReporter
	updateProgressTicker *time.Ticker
	downloadToReport     int64 // Number of bytes to report to the progressReporter since the last update
	filename             string
}

func newWriterProgress(writer io.Writer, progressReporter ProgressReporter, filename string) *WriterProgress {
	return &WriterProgress{writer: writer, progressReporter: progressReporter, updateProgressTicker: time.NewTicker(25 * time.Millisecond), downloadToReport: 0, filename: filename}
}

func (r *WriterProgress) Write(p []byte) (n int, err error) {
	select {
	case <-r.updateProgressTicker.C:
		r.progressReporter.UpdateDownloadProgress(r.downloadToReport, r.filename)
		r.downloadToReport = 0
	default:
	}
	if r.progressReporter.Cancelled() {
		return 0, nil
	}
	n, err = r.writer.Write(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	r.downloadToReport += int64(n)
	return n, err
}

func (r *WriterProgress) Close() error {
	r.updateProgressTicker.Stop()
	return nil
}
