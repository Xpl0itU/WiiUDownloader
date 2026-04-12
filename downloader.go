package wiiudownloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	ctxio "github.com/jbenet/go-context/io"
	"golang.org/x/sync/errgroup"
)

const (
	maxRetries             = 5
	retryDelay             = 5 * time.Second
	maxConcurrentDownloads = 4
)

var (
	errCancel       = fmt.Errorf("cancelled download")
	downloadTimeout = 30 * time.Second
)

type watchdogReader struct {
	io.Reader
	timer *time.Timer
}

func (r *watchdogReader) Read(p []byte) (int, error) {
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer.Reset(downloadTimeout)
	return r.Reader.Read(p)
}

type ProgressReporter interface {
	SetGameTitle(title string)
	UpdateDownloadProgress(downloaded int64, filename string)
	UpdateDecryptionProgress(progress float64)
	Cancelled() bool
	SetCancelled()
	SetDownloadSize(size int64)
	ResetTotals()
	MarkFileAsDone(filename string)
	SetTotalDownloadedForFile(filename string, downloaded int64)
	SetStartTime(startTime time.Time)
}

type pauseAwareReporter interface {
	WaitIfPaused() bool
}

func waitUntilResumed(progressReporter ProgressReporter) bool {
	if progressReporter == nil {
		return true
	}
	if waiter, ok := progressReporter.(pauseAwareReporter); ok {
		return waiter.WaitIfPaused()
	}
	return !progressReporter.Cancelled()
}

func isCancelled(progressReporter ProgressReporter) bool {
	return progressReporter != nil && progressReporter.Cancelled()
}

func shouldRetry(progressReporter ProgressReporter, doRetries bool, attempt int) bool {
	return doRetries && attempt < maxRetries && waitUntilResumed(progressReporter)
}

func monitorCancellation(ctx context.Context, cancel context.CancelFunc, progressReporter ProgressReporter) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				if isCancelled(progressReporter) {
					cancel()
					return
				}
			}
		}
	}()
	return func() {
		close(done)
	}
}

func responseExpectedSize(resp *http.Response, existingOffset int64, expectedSize int64) int64 {
	if expectedSize > 0 {
		return expectedSize
	}
	if resp.ContentLength >= 0 {
		if existingOffset > 0 && resp.StatusCode == http.StatusPartialContent {
			return existingOffset + resp.ContentLength
		}
		return resp.ContentLength
	}
	return 0
}

func responseRangeStart(resp *http.Response) (int64, bool) {
	if resp.StatusCode != http.StatusPartialContent {
		return 0, false
	}
	var start, end, total int64
	if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total); err != nil {
		return 0, false
	}
	return start, true
}

func acceptsRangedBodyWithStatusOK(resp *http.Response, existingOffset int64, expectedSize int64) bool {
	if existingOffset <= 0 || resp.StatusCode != http.StatusOK || expectedSize <= 0 || resp.ContentLength < 0 {
		return false
	}
	return expectedSize-existingOffset == resp.ContentLength
}

func validateExistingDownload(dstPath string, opts downloadOptions) error {
	if err := finalFileSizeMatches(dstPath, opts.ExpectedSize); err != nil {
		return err
	}
	if opts.Validate != nil {
		return opts.Validate(dstPath)
	}
	return nil
}

func downloadFileWithOptions(ctx context.Context, progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, opts downloadOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	basePath := filepath.Base(dstPath)

	if _, err := os.Stat(dstPath); err == nil {
		if err := validateExistingDownload(dstPath, opts); err == nil {
			if progressReporter != nil {
				progressReporter.SetTotalDownloadedForFile(basePath, opts.ExpectedSize)
				progressReporter.MarkFileAsDone(basePath)
			}
			return nil
		}
		if err := os.Remove(dstPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if !waitUntilResumed(progressReporter) {
			return errCancel
		}

		state, existingOffset, err := prepareDownloadState(dstPath, downloadURL, opts.ExpectedSize, opts.AllowResume, opts.SegmentSize)
		if err != nil {
			return err
		}

		attemptCtx, cancel := context.WithCancel(ctx)
		stopMonitor := monitorCancellation(attemptCtx, cancel, progressReporter)

		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, downloadURL, nil)
		if err != nil {
			stopMonitor()
			cancel()
			return err
		}
		if opts.UserAgent != "" {
			req.Header.Set("User-Agent", opts.UserAgent)
		}
		if existingOffset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingOffset))
		}

		resp, err := client.Do(req)
		if err != nil {
			stopMonitor()
			cancel()
			if isCancelled(progressReporter) {
				return errCancel
			}
			if shouldRetry(progressReporter, opts.DoRetries, attempt) {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}

		if existingOffset > 0 && resp.StatusCode == http.StatusPartialContent {
			start, ok := responseRangeStart(resp)
			if !ok || start != existingOffset {
				resp.Body.Close()
				stopMonitor()
				cancel()
				if err := cleanupPartialDownload(dstPath); err != nil {
					return err
				}
				if shouldRetry(progressReporter, opts.DoRetries, attempt) {
					time.Sleep(retryDelay)
					continue
				}
				return fmt.Errorf("download resume failed: unexpected content-range")
			}
		}

		if acceptsRangedBodyWithStatusOK(resp, existingOffset, opts.ExpectedSize) {
		} else if existingOffset > 0 && resp.StatusCode == http.StatusOK {
			if err := cleanupPartialDownload(dstPath); err != nil {
				resp.Body.Close()
				stopMonitor()
				cancel()
				return err
			}
			state, existingOffset, err = prepareDownloadState(dstPath, downloadURL, opts.ExpectedSize, opts.AllowResume, opts.SegmentSize)
			if err != nil {
				resp.Body.Close()
				stopMonitor()
				cancel()
				return err
			}
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			stopMonitor()
			cancel()
			if shouldRetry(progressReporter, opts.DoRetries, attempt) {
				time.Sleep(retryDelay)
				continue
			}
			return fmt.Errorf("download error after %d attempts, status code: %d", attempt, resp.StatusCode)
		}

		expectedSize := responseExpectedSize(resp, existingOffset, opts.ExpectedSize)
		if state != nil {
			if state.ExpectedSize > 0 && expectedSize > 0 && state.ExpectedSize != expectedSize {
				resp.Body.Close()
				stopMonitor()
				cancel()
				if err := cleanupPartialDownload(dstPath); err != nil {
					return err
				}
				if shouldRetry(progressReporter, opts.DoRetries, attempt) {
					time.Sleep(retryDelay)
					continue
				}
				return fmt.Errorf("download size mismatch: expected %d, got %d", state.ExpectedSize, expectedSize)
			}
			if expectedSize > 0 {
				state.ExpectedSize = expectedSize
			}
			lastModified := resp.Header.Get("Last-Modified")
			if state.LastModified != "" && lastModified != "" && state.LastModified != lastModified && existingOffset > 0 {
				resp.Body.Close()
				stopMonitor()
				cancel()
				if err := cleanupPartialDownload(dstPath); err != nil {
					return err
				}
				if shouldRetry(progressReporter, opts.DoRetries, attempt) {
					time.Sleep(retryDelay)
					continue
				}
				return fmt.Errorf("download source changed while resuming")
			}
			if lastModified != "" {
				state.LastModified = lastModified
			}
			etag := resp.Header.Get("ETag")
			if state.ETag != "" && etag != "" && state.ETag != etag && existingOffset > 0 {
				resp.Body.Close()
				stopMonitor()
				cancel()
				if err := cleanupPartialDownload(dstPath); err != nil {
					return err
				}
				if shouldRetry(progressReporter, opts.DoRetries, attempt) {
					time.Sleep(retryDelay)
					continue
				}
				return fmt.Errorf("download source changed while resuming")
			}
			if etag != "" {
				state.ETag = etag
			}
			if err := saveDownloadState(statePathFor(dstPath), *state); err != nil {
				resp.Body.Close()
				stopMonitor()
				cancel()
				return err
			}
		}

		partPath := partPathFor(dstPath)
		file, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			resp.Body.Close()
			stopMonitor()
			cancel()
			return err
		}
		if existingOffset == 0 {
			if err := file.Truncate(0); err != nil {
				file.Close()
				resp.Body.Close()
				stopMonitor()
				cancel()
				return err
			}
		}
		if _, err := file.Seek(existingOffset, io.SeekStart); err != nil {
			file.Close()
			resp.Body.Close()
			stopMonitor()
			cancel()
			return err
		}

		timer := time.AfterFunc(downloadTimeout, cancel)
		if progressReporter != nil {
			progressReporter.SetTotalDownloadedForFile(basePath, existingOffset)
		}

		var underlyingWriter io.Writer = file
		var stateWriter *resumeStateWriter
		if state != nil {
			stateWriter, err = newResumeStateWriter(file, state, statePathFor(dstPath))
			if err != nil {
				file.Close()
				resp.Body.Close()
				stopMonitor()
				cancel()
				return err
			}
			underlyingWriter = stateWriter
		}

		writerProgress := newWriterProgress(underlyingWriter, progressReporter, basePath)
		writerProgressWithContext := ctxio.NewWriter(attemptCtx, writerProgress)
		watchdog := &watchdogReader{Reader: resp.Body, timer: timer}

		_, err = io.Copy(writerProgressWithContext, watchdog)
		timer.Stop()
		stopMonitor()
		resp.Body.Close()

		if err != nil {
			writerProgress.Close()
			if stateWriter != nil {
				if finalizeErr := stateWriter.Finalize(); finalizeErr != nil {
					file.Close()
					cancel()
					return finalizeErr
				}
			}
			file.Close()
			cancel()
			if isCancelled(progressReporter) {
				return errCancel
			}
			if shouldRetry(progressReporter, opts.DoRetries, attempt) {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}

		if err := writerProgress.Close(); err != nil {
			file.Close()
			cancel()
			return err
		}
		if stateWriter != nil {
			if err := stateWriter.Finalize(); err != nil {
				file.Close()
				cancel()
				return err
			}
		}
		if err := file.Close(); err != nil {
			cancel()
			return err
		}
		if err := finalFileSizeMatches(partPath, expectedSize); err != nil {
			cancel()
			if shouldRetry(progressReporter, opts.DoRetries, attempt) {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}
		if opts.Validate != nil {
			if err := opts.Validate(partPath); err != nil {
				cancel()
				if cleanupErr := cleanupPartialDownload(dstPath); cleanupErr != nil {
					return cleanupErr
				}
				if shouldRetry(progressReporter, opts.DoRetries, attempt) {
					time.Sleep(retryDelay)
					continue
				}
				return err
			}
		}
		if err := os.Rename(partPath, dstPath); err != nil {
			cancel()
			return err
		}
		if err := os.Remove(statePathFor(dstPath)); err != nil && !os.IsNotExist(err) {
			cancel()
			return err
		}
		if progressReporter != nil {
			progressReporter.MarkFileAsDone(basePath)
		}
		cancel()
		return nil
	}

	return nil
}

func downloadFileWithSemaphoreOptions(ctx context.Context, progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, opts downloadOptions) error {
	return downloadFileWithOptions(ctx, progressReporter, client, downloadURL, dstPath, opts)
}

func downloadFile(progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool) error {
	return downloadFileWithOptions(context.Background(), progressReporter, client, downloadURL, dstPath, downloadOptions{
		DoRetries:   doRetries,
		AllowResume: true,
		UserAgent:   "WiiUDownloader",
	})
}

func DownloadTitle(titleID, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, client *http.Client) error {
	tid, err := strconv.ParseUint(titleID, 16, 64)
	if err != nil {
		return err
	}
	tEntry := GetTitleEntryFromTid(tid)

	if progressReporter != nil {
		progressReporter.ResetTotals()
		progressReporter.SetGameTitle(tEntry.Name)
	}
	if !waitUntilResumed(progressReporter) {
		return nil
	}

	outputDir := strings.TrimRight(outputDirectory, "/\\")
	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", titleID)

	if err := os.MkdirAll(outputDir, downloadStateDirPerm); err != nil {
		return err
	}

	tmdPath := filepath.Join(outputDir, "title.tmd")
	if err := downloadFileWithOptions(context.Background(), progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "tmd"), tmdPath, downloadOptions{
		DoRetries:   true,
		AllowResume: true,
		UserAgent:   "WiiUDownloader",
		Validate: func(path string) error {
			return validateTMDFile(path, tid)
		},
	}); err != nil {
		if isCancelled(progressReporter) || err == errCancel {
			return nil
		}
		return err
	}

	tmdData, err := os.ReadFile(tmdPath)
	if err != nil {
		return err
	}

	tmd, err := ParseTMD(tmdData)
	if err != nil {
		return err
	}

	tikPath := filepath.Join(outputDir, "title.tik")
	if err := downloadFileWithOptions(context.Background(), progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "cetk"), tikPath, downloadOptions{
		DoRetries:   false,
		AllowResume: true,
		UserAgent:   "WiiUDownloader",
		Validate: func(path string) error {
			return validateTicketFile(path, tmd.TitleID, tmd.TitleVersion)
		},
	}); err != nil {
		if isCancelled(progressReporter) || err == errCancel {
			return nil
		}
		titleKeyType := uint8(TITLE_KEY_mypass)
		if tEntry.TitleID == tid {
			titleKeyType = tEntry.Key
		}
		titleKey, err := GenerateKeyWithType(titleID, titleKeyType)
		if err != nil {
			return err
		}
		if err := GenerateTicket(tikPath, tmd.TitleID, titleKey, tmd.TitleVersion); err != nil {
			return err
		}
	}

	titleSize := tmd.CalculateTotalSize()

	if progressReporter != nil {
		progressReporter.SetDownloadSize(int64(titleSize))
	}

	if err := GenerateCert(tmd, filepath.Join(outputDir, "title.cert"), progressReporter, client); err != nil {
		if isCancelled(progressReporter) || err == errCancel {
			return nil
		}
		return err
	}

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrentDownloads)
	if progressReporter != nil {
		progressReporter.SetStartTime(time.Now())
	}

	for i := 0; i < int(tmd.ContentCount); i++ {
		i := i
		g.Go(func() error {
			if !waitUntilResumed(progressReporter) {
				return errCancel
			}
			content := tmd.Contents[i]
			filePath := filepath.Join(outputDir, fmt.Sprintf("%08X.app", content.ID))
			if err := downloadFileWithSemaphoreOptions(ctx, progressReporter, client, fmt.Sprintf("%s/%08X", baseURL, content.ID), filePath, downloadOptions{
				ExpectedSize: expectedContentDownloadSize(content),
				DoRetries:    true,
				AllowResume:  true,
				UserAgent:    "WiiUDownloader",
			}); err != nil {
				if isCancelled(progressReporter) {
					return errCancel
				}
				return err
			}

			if content.Type&CONTENT_TYPE_HASHED == CONTENT_TYPE_HASHED {
				filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", content.ID))
				if err := downloadFileWithSemaphoreOptions(ctx, progressReporter, client, fmt.Sprintf("%s/%08X.h3", baseURL, content.ID), filePath, downloadOptions{
					ExpectedSize: expectedH3DownloadSize(content),
					DoRetries:    true,
					AllowResume:  true,
					UserAgent:    "WiiUDownloader",
					Validate: func(path string) error {
						return verifyH3File(path, content)
					},
				}); err != nil {
					if isCancelled(progressReporter) {
						return errCancel
					}
					return err
				}
			}
			if isCancelled(progressReporter) {
				return errCancel
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if err == errCancel {
			return nil
		}
		return err
	}

	if doDecryption && !isCancelled(progressReporter) {
		if err := DecryptContents(outputDir, progressReporter, deleteEncryptedContents); err != nil {
			return err
		}
	}

	return nil
}
