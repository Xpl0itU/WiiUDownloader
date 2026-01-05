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
	"golang.org/x/sync/semaphore"
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

type WatchdogReader struct {
	io.Reader
	timer *time.Timer
}

func (r *WatchdogReader) Read(p []byte) (int, error) {
	if !r.timer.Stop() {
		// If the timer already fired, we might be too late, but we entered Read, so we are alive.
		// However, the context might be cancelling. We rely on the timer reset.
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

func downloadFileWithSemaphore(ctx context.Context, progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool, sem *semaphore.Weighted) error {
	if err := sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer sem.Release(1)

	basePath := filepath.Base(dstPath)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// specific retry context
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			if doRetries && attempt < maxRetries && !progressReporter.Cancelled() {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if doRetries && attempt < maxRetries && !progressReporter.Cancelled() {
				time.Sleep(retryDelay)
				continue
			}
			return fmt.Errorf("download error after %d attempts, status code: %d", attempt, resp.StatusCode)
		}

		file, err := os.Create(dstPath)
		if err != nil {
			resp.Body.Close()
			return err
		}

		timer := time.AfterFunc(downloadTimeout, func() {
			cancel()
		})

		progressReporter.SetTotalDownloadedForFile(basePath, 0)
		writerProgress := newWriterProgress(file, progressReporter, basePath)
		writerProgressWithContext := ctxio.NewWriter(ctx, writerProgress)

		watchdog := &WatchdogReader{
			Reader: resp.Body,
			timer:  timer,
		}

		_, err = io.Copy(writerProgressWithContext, watchdog)
		timer.Stop()

		if err != nil {
			file.Close()
			resp.Body.Close()
			writerProgress.Close()
			if doRetries && attempt < maxRetries && !progressReporter.Cancelled() {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}
		file.Close()
		resp.Body.Close()
		writerProgress.Close()
		progressReporter.MarkFileAsDone(basePath)
		break
	}

	return nil
}

func downloadFile(progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
		if err != nil {
			return err
		}

		req.Header.Set("User-Agent", "WiiUDownloader")

		resp, err := client.Do(req)
		if err != nil {
			if doRetries && attempt < maxRetries && !progressReporter.Cancelled() {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if doRetries && attempt < maxRetries && !progressReporter.Cancelled() {
				time.Sleep(retryDelay)
				continue
			}
			return fmt.Errorf("download error after %d attempts, status code: %d", attempt, resp.StatusCode)
		}

		file, err := os.Create(dstPath)
		if err != nil {
			resp.Body.Close()
			return err
		}

		timer := time.AfterFunc(downloadTimeout, func() {
			cancel()
		})

		writerProgress := newWriterProgress(file, progressReporter, filepath.Base(dstPath))

		watchdog := &WatchdogReader{
			Reader: resp.Body,
			timer:  timer,
		}

		_, err = io.Copy(writerProgress, watchdog)
		timer.Stop()

		if err != nil {
			file.Close()
			resp.Body.Close()
			if doRetries && attempt < maxRetries && !progressReporter.Cancelled() {
				time.Sleep(retryDelay)
				continue
			}
			return err
		}
		file.Close()
		resp.Body.Close()
		break
	}

	return nil
}

func DownloadTitle(titleID, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, client *http.Client) error {
	tid, err := strconv.ParseUint(titleID, 16, 64)
	if err != nil {
		return err
	}
	tEntry := GetTitleEntryFromTid(tid)

	progressReporter.ResetTotals()
	progressReporter.SetGameTitle(tEntry.Name)

	outputDir := strings.TrimRight(outputDirectory, "/\\")
	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", titleID)

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}

	tmdPath := filepath.Join(outputDir, "title.tmd")
	if err := downloadFile(progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "tmd"), tmdPath, true); err != nil {
		if progressReporter.Cancelled() {
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
	if err := downloadFile(progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "cetk"), tikPath, false); err != nil {
		if progressReporter.Cancelled() {
			return nil
		}
		titleKey, err := GenerateKey(titleID)
		if err != nil {
			return err
		}
		if err := GenerateTicket(tikPath, tEntry.TitleID, titleKey, tmd.TitleVersion); err != nil {
			return err
		}
	}

	var titleSize uint64

	for i := 0; i < int(tmd.ContentCount); i++ {
		titleSize += tmd.Contents[i].Size
	}

	progressReporter.SetDownloadSize(int64(titleSize))

	if err := GenerateCert(tmd, filepath.Join(outputDir, "title.cert"), progressReporter, client); err != nil {
		if progressReporter.Cancelled() {
			return nil
		}
		return err
	}

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrentDownloads)
	sem := semaphore.NewWeighted(maxConcurrentDownloads)
	progressReporter.SetStartTime(time.Now())

	for i := 0; i < int(tmd.ContentCount); i++ {
		i := i
		g.Go(func() error {
			filePath := filepath.Join(outputDir, fmt.Sprintf("%08X.app", tmd.Contents[i].ID))
			if err := downloadFileWithSemaphore(ctx, progressReporter, client, fmt.Sprintf("%s/%08X", baseURL, tmd.Contents[i].ID), filePath, true, sem); err != nil {
				if progressReporter.Cancelled() {
					return errCancel
				}
				return err
			}

			if tmd.Contents[i].Type&0x2 == 2 { // has a hash
				filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", tmd.Contents[i].ID))
				if err := downloadFileWithSemaphore(ctx, progressReporter, client, fmt.Sprintf("%s/%08X.h3", baseURL, tmd.Contents[i].ID), filePath, true, sem); err != nil {
					if progressReporter.Cancelled() {
						return errCancel
					}
					return err
				}
			}
			if progressReporter.Cancelled() {
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

	if doDecryption && !progressReporter.Cancelled() {
		if err := DecryptContents(outputDir, progressReporter, deleteEncryptedContents); err != nil {
			return err
		}
	}

	return nil
}
