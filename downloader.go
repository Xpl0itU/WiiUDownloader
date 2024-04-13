package wiiudownloader

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const (
	maxRetries             = 5
	retryDelay             = 5 * time.Second
	maxConcurrentDownloads = 4
)

var (
	errCancel = fmt.Errorf("cancelled download")
)

type ProgressReporter interface {
	SetGameTitle(title string)
	UpdateDownloadProgress(downloaded int64)
	UpdateDecryptionProgress(progress float64)
	Cancelled() bool
	SetCancelled()
	SetDownloadSize(size int64)
	SetTotalDownloaded(total int64)
	AddToTotalDownloaded(toAdd int64)
	SetStartTime(startTime time.Time)
}

func downloadFileWithSemaphore(ctx context.Context, progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool, sem *semaphore.Weighted) error {
	if err := sem.Acquire(ctx, 1); err != nil {
		return nil
	}
	defer sem.Release(1)

	for attempt := 1; attempt <= maxRetries; attempt++ {
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
			fmt.Printf("download error after %d attempts, status code: %d, url: %s\n", attempt, resp.StatusCode, downloadURL)
			return fmt.Errorf("download error after %d attempts, status code: %d", attempt, resp.StatusCode)
		}

		file, err := os.Create(dstPath)
		if err != nil {
			resp.Body.Close()
			return err
		}

		writerProgress := newWriterProgress(file, progressReporter)
		_, err = io.Copy(writerProgress, resp.Body)
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
		break
	}

	return nil
}

func downloadFile(progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", downloadURL, nil)
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

		writerProgress := newWriterProgress(file, progressReporter)
		_, err = io.Copy(writerProgress, resp.Body)
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

func DownloadTitle(titleID, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, logger *Logger, client *http.Client) error {
	tEntry := getTitleEntryFromTid(titleID)

	progressReporter.SetTotalDownloaded(0)
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

	var titleVersion uint16
	if err := binary.Read(bytes.NewReader(tmdData[476:478]), binary.BigEndian, &titleVersion); err != nil {
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
		if err := GenerateTicket(tikPath, tEntry.TitleID, titleKey, titleVersion); err != nil {
			return err
		}
	}

	var contentCount uint16
	if err := binary.Read(bytes.NewReader(tmdData[478:480]), binary.BigEndian, &contentCount); err != nil {
		return err
	}

	var titleSize uint64
	contents := make([]Content, contentCount)
	tmdDataReader := bytes.NewReader(tmdData)

	for i := 0; i < int(contentCount); i++ {
		offset := 0xB04 + (0x30 * i)

		tmdDataReader.Seek(int64(offset), io.SeekStart)
		if err := binary.Read(tmdDataReader, binary.BigEndian, &contents[i].ID); err != nil {
			return err
		}

		tmdDataReader.Seek(2, io.SeekCurrent)

		if err := binary.Read(tmdDataReader, binary.BigEndian, &contents[i].Type); err != nil {
			return err
		}

		if err := binary.Read(tmdDataReader, binary.BigEndian, &contents[i].Size); err != nil {
			return err
		}

		titleSize += contents[i].Size
	}

	progressReporter.SetDownloadSize(int64(titleSize))

	cert, err := GenerateCert(tmdData, contentCount, progressReporter, client)
	if err != nil {
		if progressReporter.Cancelled() {
			return nil
		}
		return err
	}

	certPath := filepath.Join(outputDir, "title.cert")
	certFile, err := os.Create(certPath)
	if err != nil {
		return err
	}
	if err := binary.Write(certFile, binary.BigEndian, cert.Bytes()); err != nil {
		return err
	}
	certFile.Close()
	logger.Info("Certificate saved to %v \n", certPath)

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrentDownloads)
	sem := semaphore.NewWeighted(maxConcurrentDownloads)
	progressReporter.SetStartTime(time.Now())

	for i := 0; i < int(contentCount); i++ {
		i := i
		g.Go(func() error {
			filePath := filepath.Join(outputDir, fmt.Sprintf("%08X.app", contents[i].ID))
			if err := downloadFileWithSemaphore(ctx, progressReporter, client, fmt.Sprintf("%s/%08X", baseURL, contents[i].ID), filePath, true, sem); err != nil {
				if progressReporter.Cancelled() {
					return errCancel
				}
				return err
			}

			if contents[i].Type&0x2 == 2 { // has a hash
				filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", contents[i].ID))
				if err := downloadFileWithSemaphore(ctx, progressReporter, client, fmt.Sprintf("%s/%08X.h3", baseURL, contents[i].ID), filePath, true, sem); err != nil {
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
