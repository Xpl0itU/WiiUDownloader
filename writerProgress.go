package wiiudownloader

import (
	"io"
	"time"
)

func calculateDownloadSpeed(downloaded int64, startTime, endTime time.Time) int64 {
	duration := endTime.Sub(startTime).Seconds()
	if duration > 0 {
		return int64(float64(downloaded) / duration)
	}
	return 0
}

type WriterProgress struct {
	writer           io.Writer
	progressReporter ProgressReporter
	startTime        time.Time
	filePath         string
	totalDownloaded  int64
}

func newWriterProgress(writer io.Writer, progressReporter ProgressReporter, startTime time.Time, filePath string) *WriterProgress {
	return &WriterProgress{writer: writer, totalDownloaded: 0, progressReporter: progressReporter, startTime: startTime, filePath: filePath}
}

func (r *WriterProgress) Write(p []byte) (n int, err error) {
	if r.progressReporter.Cancelled() {
		return len(p), nil
	}
	n, err = r.writer.Write(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	r.totalDownloaded += int64(n)
	r.progressReporter.UpdateDownloadProgress(r.totalDownloaded, calculateDownloadSpeed(r.totalDownloaded, r.startTime, time.Now()), r.filePath)
	return n, err
}
