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
)

const (
	maxRetries = 5
	retryDelay = 5 * time.Second
	bufferSize = 1048576
)

type ProgressReporter interface {
	SetGameTitle(title string)
	UpdateDownloadProgress(downloaded, speed int64, filePath string)
	UpdateDecryptionProgress(progress float64)
	Cancelled() bool
	SetCancelled()
	SetDownloadSize(size int64)
	SetTotalDownloaded(total int64)
	AddToTotalDownloaded(toAdd int64)
}

func calculateDownloadSpeed(downloaded int64, startTime, endTime time.Time) int64 {
	duration := endTime.Sub(startTime).Seconds()
	if duration > 0 {
		return int64(float64(downloaded) / duration)
	}
	return 0
}

func downloadFile(ctx context.Context, progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool, buffer []byte) error {
	filePath := filepath.Base(dstPath)

	var startTime time.Time

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
		if err != nil {
			return err
		}

		req.Header.Set("User-Agent", "WiiUDownloader")
		req.Header.Set("Connection", "Keep-Alive")
		req.Header.Set("Accept-Encoding", "")

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if doRetries && attempt < maxRetries {
				time.Sleep(retryDelay)
				continue
			}
			return fmt.Errorf("download error after %d attempts, status code: %d", attempt, resp.StatusCode)
		}

		file, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer file.Close()

		var downloaded int64

		startTime = time.Now()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		go func() {
			for range ticker.C {
				if progressReporter.Cancelled() {
					break
				}
				progressReporter.UpdateDownloadProgress(downloaded, calculateDownloadSpeed(downloaded, startTime, time.Now()), filePath)
			}
		}()

	Loop:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				n, err := resp.Body.Read(buffer)
				if err != nil && err != io.EOF {
					if doRetries && attempt < maxRetries {
						time.Sleep(retryDelay)
						break
					}
					return fmt.Errorf("download error after %d attempts: %+v", attempt, err)
				}

				if n == 0 {
					break Loop
				}

				_, err = file.Write(buffer[:n])
				if err != nil {
					if doRetries && attempt < maxRetries {
						time.Sleep(retryDelay)
						break
					}
					return fmt.Errorf("write error after %d attempts: %+v", attempt, err)
				}

				downloaded += int64(n)
			}
		}
		break
	}

	return nil
}

func DownloadTitle(cancelCtx context.Context, titleID, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, logger *Logger, client *http.Client) error {
	tEntry := getTitleEntryFromTid(titleID)

	progressReporter.SetTotalDownloaded(0)
	progressReporter.SetGameTitle(tEntry.Name)

	outputDir := strings.TrimRight(outputDirectory, "/\\")
	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", titleID)

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}

	buffer := make([]byte, bufferSize)

	tmdPath := filepath.Join(outputDir, "title.tmd")
	if err := downloadFile(cancelCtx, progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "tmd"), tmdPath, true, buffer); err != nil {
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
	if err := downloadFile(cancelCtx, progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "cetk"), tikPath, false, buffer); err != nil {
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
	var contentSizes []uint64
	for i := 0; i < int(contentCount); i++ {
		contentDataLoc := 0xB04 + (0x30 * i)

		var contentSizeInt uint64
		if err := binary.Read(bytes.NewReader(tmdData[contentDataLoc+8:contentDataLoc+8+8]), binary.BigEndian, &contentSizeInt); err != nil {
			return err
		}

		titleSize += contentSizeInt
		contentSizes = append(contentSizes, contentSizeInt)
	}

	progressReporter.SetDownloadSize(int64(titleSize))

	cert, err := GenerateCert(tmdData, contentCount, progressReporter, client, cancelCtx, buffer)
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
	defer certFile.Close()
	logger.Info("Certificate saved to %v \n", certPath)

	var content Content
	tmdDataReader := bytes.NewReader(tmdData)

	for i := 0; i < int(contentCount); i++ {
		offset := 2820 + (48 * i)
		tmdDataReader.Seek(int64(offset), 0)
		if err := binary.Read(tmdDataReader, binary.BigEndian, &content.ID); err != nil {
			return err
		}
		filePath := filepath.Join(outputDir, fmt.Sprintf("%08X.app", content.ID))
		if err := downloadFile(cancelCtx, progressReporter, client, fmt.Sprintf("%s/%08X", baseURL, content.ID), filePath, true, buffer); err != nil {
			if progressReporter.Cancelled() {
				break
			}
			return err
		}
		progressReporter.AddToTotalDownloaded(int64(contentSizes[i]))

		if tmdData[offset+7]&0x2 == 2 {
			filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", content.ID))
			if err := downloadFile(cancelCtx, progressReporter, client, fmt.Sprintf("%s/%08X.h3", baseURL, content.ID), filePath, true, buffer); err != nil {
				if progressReporter.Cancelled() {
					break
				}
				return err
			}
		}
		if progressReporter.Cancelled() {
			break
		}
	}

	if doDecryption && !progressReporter.Cancelled() {
		if err := DecryptContents(outputDir, progressReporter, deleteEncryptedContents); err != nil {
			return err
		}
	}

	return nil
}
