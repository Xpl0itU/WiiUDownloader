package wiiudownloader

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jaskaranSM/aria2go"
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

type Aria2gocNotifier struct {
	start    chan string
	complete chan bool
}

func newAria2goNotifier(start chan string, complete chan bool) aria2go.Notifier {
	return Aria2gocNotifier{
		start:    start,
		complete: complete,
	}
}

func (n Aria2gocNotifier) OnStart(gid string) {
	n.start <- gid
}

func (n Aria2gocNotifier) OnPause(gid string) {
	return
}

func (n Aria2gocNotifier) OnStop(gid string) {
	return
}

func (n Aria2gocNotifier) OnComplete(gid string) {
	n.complete <- false
}

func (n Aria2gocNotifier) OnError(gid string) {
	n.complete <- true
}

func calculateDownloadSpeed(downloaded int64, startTime, endTime time.Time) int64 {
	duration := endTime.Sub(startTime).Seconds()
	if duration > 0 {
		return int64(float64(downloaded) / duration)
	}
	return 0
}

func downloadFile(ctx context.Context, progressReporter ProgressReporter, downloadURL, dstPath string, doRetries bool, buffer []byte, ariaSessionPath string) error {
	fileName := filepath.Base(dstPath)

	var startTime time.Time

	for attempt := 1; attempt <= maxRetries; attempt++ {
		client := aria2go.NewAria2(aria2go.Config{
			Options: aria2go.Options{
				"save-session": ariaSessionPath,
			},
		})

		gid, err := client.AddUri(downloadURL, aria2go.Options{
			"dir":      filepath.Dir(dstPath),
			"out":      fileName,
			"continue": "true",
		})
		if err != nil {
			return err
		}

		go func() {
			defer client.Shutdown()
			client.Run()
		}()

		startNotif := make(chan string)
		completeNotif := make(chan bool)
		go func() {
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

			<-quit
			completeNotif <- true
		}()
		client.SetNotifier(newAria2goNotifier(startNotif, completeNotif))

		startTime = time.Now()
		ticker := time.NewTicker(time.Millisecond * 500)
		defer ticker.Stop()
	loop:
		for {
			select {
			case id := <-startNotif:
				gid = id
			case <-ticker.C:
				downloaded := client.GetDownloadInfo(gid).BytesCompleted
				progressReporter.UpdateDownloadProgress(downloaded, calculateDownloadSpeed(downloaded, startTime, time.Now()), fileName)
			case errored := <-completeNotif:
				if errored {
					if doRetries && attempt < maxRetries {
						time.Sleep(retryDelay)
						break loop
					}
					return fmt.Errorf("write error after %d attempts: %+v", attempt, client.GetDownloadInfo(gid).ErrorCode)
				}
				downloaded := client.GetDownloadInfo(gid).BytesCompleted
				progressReporter.UpdateDownloadProgress(downloaded, calculateDownloadSpeed(downloaded, startTime, time.Now()), fileName)
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	}

	return nil
}

func DownloadTitle(cancelCtx context.Context, titleID, outputDirectory string, doDecryption bool, progressReporter ProgressReporter, deleteEncryptedContents bool, logger *Logger, ariaSessionPath string) error {
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
	if err := downloadFile(cancelCtx, progressReporter, fmt.Sprintf("%s/%s", baseURL, "tmd"), tmdPath, true, buffer, ariaSessionPath); err != nil {
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
	if err := downloadFile(cancelCtx, progressReporter, fmt.Sprintf("%s/%s", baseURL, "cetk"), tikPath, false, buffer, ariaSessionPath); err != nil {
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

	cert, err := GenerateCert(tmdData, contentCount, progressReporter, cancelCtx, buffer, ariaSessionPath)
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
		if err := downloadFile(cancelCtx, progressReporter, fmt.Sprintf("%s/%08X", baseURL, id), filePath, true, buffer, ariaSessionPath); err != nil {
			if progressReporter.Cancelled() {
				break
			}
			return err
		}
		progressReporter.AddToTotalDownloaded(int64(contentSizes[i]))

		if tmdData[offset+7]&0x2 == 2 {
			filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", id))
			if err := downloadFile(cancelCtx, progressReporter, fmt.Sprintf("%s/%08X.h3", baseURL, id), filePath, true, buffer, ariaSessionPath); err != nil {
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
