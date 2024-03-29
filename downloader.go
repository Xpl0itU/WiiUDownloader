package wiiudownloader

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
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

func downloadChunk(buffer []byte, resp *http.Response, file *os.File, doRetries bool, attempt int) (int64, error) {
	n, err := resp.Body.Read(buffer)
	if err != nil && err != io.EOF {
		if doRetries && attempt < maxRetries {
			time.Sleep(retryDelay)
			return 0, nil
		}
		return 0, fmt.Errorf("download error after %d attempts: %+v", attempt, err)
	}

	if n == 0 {
		return 0, nil
	}

	_, err = file.Write(buffer[:n])
	if err != nil {
		if doRetries && attempt < maxRetries {
			time.Sleep(retryDelay)
			return 0, nil
		}
		return 0, fmt.Errorf("write error after %d attempts: %+v", attempt, err)
	}

	return int64(n), nil
}

func downloadFile(ctx context.Context, progressReporter ProgressReporter, client *http.Client, downloadURL, dstPath string, doRetries bool, buffer []byte) error {
	filePath := filepath.Base(dstPath)

	var startTime time.Time

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
		if err != nil {
			return err
		}

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

	Loop:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				n, err := downloadChunk(buffer, resp, file, doRetries, attempt)
				if err != nil {
					return err
				}

				if n == 0 {
					break Loop
				}

				downloaded += n
				progressReporter.UpdateDownloadProgress(downloaded, calculateDownloadSpeed(downloaded, startTime, time.Now()), filePath)
			default:
				n, err := downloadChunk(buffer, resp, file, doRetries, attempt)
				if err != nil {
					return err
				}

				if n == 0 {
					break Loop
				}

				downloaded += n
			}
		}
	}

	return nil
}

func DownloadTitle(cancelCtx context.Context, titleID, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, logger *Logger, client *http.Client) error {
	titleEntry := getTitleEntryFromTid(titleID)

	progressReporter.SetTotalDownloaded(0)
	progressReporter.SetGameTitle(titleEntry.Name)

	outputDir := strings.TrimRight(outputDirectory, "/\\")
	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", titleID)
	titleIDBytes, err := hex.DecodeString(titleID)
	if err != nil {
		return err
	}

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
		if err := GenerateTicket(tikPath, titleEntry.TitleID, titleKey, titleVersion); err != nil {
			return err
		}
	}
	tikData, err := os.ReadFile(tikPath)
	if err != nil {
		return err
	}
	encryptedTitleKey := tikData[0x1BF : 0x1BF+0x10]

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

	c, err := aes.NewCipher(commonKey)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	decryptedTitleKey := make([]byte, len(encryptedTitleKey))
	cbc := cipher.NewCBCDecrypter(c, append(titleIDBytes, make([]byte, 8)...))
	cbc.CryptBlocks(decryptedTitleKey, encryptedTitleKey)

	cipherHashTree, err := aes.NewCipher(decryptedTitleKey)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	var id uint32
	var content contentInfo
	tmdDataReader := bytes.NewReader(tmdData)

	for i := 0; i < int(contentCount); i++ {
		offset := 2820 + (48 * i)
		tmdDataReader.Seek(int64(offset), 0)
		if err := binary.Read(tmdDataReader, binary.BigEndian, &id); err != nil {
			return err
		}
		filePath := filepath.Join(outputDir, fmt.Sprintf("%08X.app", id))
		if err := downloadFile(cancelCtx, progressReporter, client, fmt.Sprintf("%s/%08X", baseURL, id), filePath, true, buffer); err != nil {
			if progressReporter.Cancelled() {
				break
			}
			return err
		}
		progressReporter.AddToTotalDownloaded(int64(contentSizes[i]))

		if tmdData[offset+7]&0x2 == 2 {
			filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", id))
			if err := downloadFile(cancelCtx, progressReporter, client, fmt.Sprintf("%s/%08X.h3", baseURL, id), filePath, true, buffer); err != nil {
				if progressReporter.Cancelled() {
					break
				}
				return err
			}
			content.Hash = tmdData[offset+16 : offset+0x14]
			content.ID = fmt.Sprintf("%08X", id)
			content.Size = int64(contentSizes[i])
			if err := checkContentHashes(outputDirectory, content, cipherHashTree); err != nil {
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
