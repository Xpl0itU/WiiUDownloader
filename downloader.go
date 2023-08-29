package wiiudownloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

const (
	maxRetries = 5
	retryDelay = 5 * time.Second
	bufferSize = 1048576
)

type ProgressReporter interface {
	SetGameTitle(title string)
	UpdateDownloadProgress(resp *grab.Response, filePath string)
	UpdateDecryptionProgress(progress float64)
	Cancelled() bool
}

func downloadFile(progressReporter ProgressReporter, client *grab.Client, downloadURL string, dstPath string, doRetries bool) error {
	filePath := filepath.Base(dstPath)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := grab.NewRequest(dstPath, downloadURL)
		if err != nil {
			return err
		}
		req.BufferSize = bufferSize

		resp := client.Do(req)
		progressReporter.UpdateDownloadProgress(resp, filePath)

		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()

	Loop:
		for {
			select {
			case <-t.C:
				progressReporter.UpdateDownloadProgress(resp, filePath)
				if progressReporter.Cancelled() {
					resp.Cancel()
					break Loop
				}
			case <-resp.Done:
				if err := resp.Err(); err != nil {
					if doRetries && attempt < maxRetries {
						time.Sleep(retryDelay)
						break Loop
					}
					return fmt.Errorf("download error after %d attempts: %+v", attempt, err)
				}
				break Loop
			}
		}
	}

	return nil
}

func DownloadTitle(titleID string, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, logger *Logger) error {
	titleEntry := getTitleEntryFromTid(titleID)

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

	client := grab.NewClient()
	client.BufferSize = bufferSize
	tmdPath := filepath.Join(outputDir, "title.tmd")
	if err := downloadFile(progressReporter, client, fmt.Sprintf("%s/%s", baseURL, "tmd"), tmdPath, true); err != nil {
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

	cert, err := GenerateCert(tmdData, contentCount, progressReporter, client)
	if err != nil {
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
		if err := downloadFile(progressReporter, client, fmt.Sprintf("%s/%08X", baseURL, id), filePath, true); err != nil {
			return err
		}

		if tmdData[offset+7]&0x2 == 2 {
			filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", id))
			if err := downloadFile(progressReporter, client, fmt.Sprintf("%s/%08X.h3", baseURL, id), filePath, true); err != nil {
				return err
			}
			content.Hash = tmdData[offset+16 : offset+0x14]
			content.ID = fmt.Sprintf("%08X", id)
			tmdDataReader.Seek(int64(offset+8), 0)
			if err := binary.Read(tmdDataReader, binary.BigEndian, &content.Size); err != nil {
				return err
			}
			if err := checkContentHashes(outputDirectory, content, &cipherHashTree); err != nil {
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
