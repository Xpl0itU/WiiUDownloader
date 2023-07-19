package wiiudownloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cavaliergopher/grab/v3"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type ProgressWindow struct {
	Window       *gtk.Window
	box          *gtk.Box
	label        *gtk.Label
	bar          *gtk.ProgressBar
	percentLabel *gtk.Label
	cancelButton *gtk.Button
	cancelled    bool
}

func CreateProgressWindow(parent *gtk.Window) (ProgressWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return ProgressWindow{}, err
	}
	win.SetTitle("File Download")

	win.SetTransientFor(parent)

	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	if err != nil {
		return ProgressWindow{}, err
	}
	win.Add(box)

	filenameLabel, err := gtk.LabelNew("File: ")
	if err != nil {
		return ProgressWindow{}, err
	}
	box.PackStart(filenameLabel, false, false, 0)

	progressBar, err := gtk.ProgressBarNew()
	if err != nil {
		return ProgressWindow{}, err
	}
	box.PackStart(progressBar, false, false, 0)

	percentLabel, err := gtk.LabelNew("0%")
	if err != nil {
		return ProgressWindow{}, err
	}
	box.PackStart(percentLabel, false, false, 0)

	cancelButton, err := gtk.ButtonNewWithLabel("Cancel")
	if err != nil {
		return ProgressWindow{}, err
	}

	bottomhBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 5)
	if err != nil {
		return ProgressWindow{}, err
	}
	bottomhBox.PackEnd(cancelButton, false, false, 0)
	box.PackEnd(bottomhBox, false, false, 0)

	return ProgressWindow{
		Window:       win,
		box:          box,
		label:        filenameLabel,
		bar:          progressBar,
		percentLabel: percentLabel,
		cancelButton: cancelButton,
		cancelled:    false,
	}, nil
}

func downloadFile(progressWindow *ProgressWindow, client *grab.Client, url string, outputPath string) error {
	if progressWindow.cancelled {
		return fmt.Errorf("cancelled")
	}
	done := false
	req, err := grab.NewRequest(outputPath, url)
	if err != nil {
		return err
	}

	resp := client.Do(req)

	filePath := path.Base(outputPath)

	go func(err *error) {
		for !resp.IsComplete() {
			glib.IdleAdd(func() {
				progressWindow.label.SetText(filePath)
				progressWindow.bar.SetFraction(resp.Progress())
				progressWindow.percentLabel.SetText(fmt.Sprintf("%.0f%%", 100*resp.Progress()))
			})

			if progressWindow.cancelled {
				resp.Cancel()
				break
			}
		}

		*err = resp.Err()
		done = true
	}(&err)

	for {
		for gtk.EventsPending() {
			gtk.MainIteration()
		}
		if done {
			break
		}
	}

	if err != nil {
		return err
	}

	return nil
}

func DownloadTitle(titleID string, outputDirectory string, doDecryption bool, progressWindow *ProgressWindow, deleteEncryptedContents bool) error {
	progressWindow.cancelButton.Connect("clicked", func() {
		progressWindow.cancelled = true
	})
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
	downloadURL := fmt.Sprintf("%s/%s", baseURL, "tmd")
	tmdPath := filepath.Join(outputDir, "title.tmd")
	if err := downloadFile(progressWindow, client, downloadURL, tmdPath); err != nil {
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
	downloadURL = fmt.Sprintf("%s/%s", baseURL, "cetk")
	if err := downloadFile(progressWindow, client, downloadURL, tikPath); err != nil {
		titleKey, err := generateKey(titleID)
		if err != nil {
			return err
		}
		if err := generateTicket(tikPath, titleID, titleKey, titleVersion); err != nil {
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

	cert := bytes.Buffer{}

	cert0, err := getCert(tmdData, 0, contentCount)
	if err != nil {
		return err
	}
	cert.Write(cert0)

	cert1, err := getCert(tmdData, 1, contentCount)
	if err != nil {
		return err
	}
	cert.Write(cert1)

	defaultCert, err := getDefaultCert(progressWindow, client)
	if err != nil {
		return err
	}
	cert.Write(defaultCert)

	certPath := filepath.Join(outputDir, "title.cert")
	certFile, err := os.Create(certPath)
	if err != nil {
		return err
	}
	if err := binary.Write(certFile, binary.BigEndian, cert.Bytes()); err != nil {
		return err
	}
	defer certFile.Close()
	fmt.Printf("[Info] Certificate saved to ./%v \n", certPath)

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
	tmdDataReader := bytes.NewReader(tmdData)

	for i := 0; i < int(contentCount); i++ {
		offset := 2820 + (48 * i)
		tmdDataReader.Seek(int64(offset), 0)
		if err := binary.Read(tmdDataReader, binary.BigEndian, &id); err != nil {
			return err
		}
		filePath := filepath.Join(outputDir, fmt.Sprintf("%08X.app", id))
		downloadURL = fmt.Sprintf("%s/%08X", baseURL, id)
		if err := downloadFile(progressWindow, client, downloadURL, filePath); err != nil {
			return err
		}

		if tmdData[offset+7]&0x2 == 2 {
			filePath = filepath.Join(outputDir, fmt.Sprintf("%08X.h3", id))
			downloadURL = fmt.Sprintf("%s/%08X.h3", baseURL, id)
			if err := downloadFile(progressWindow, client, downloadURL, filePath); err != nil {
				return err
			}
			var content contentInfo
			content.Hash = tmdData[offset+16 : offset+0x14]
			content.ID = fmt.Sprintf("%08X", id)
			tmdDataReader.Seek(int64(offset+8), 0)
			if err := binary.Read(tmdDataReader, binary.BigEndian, &content.Size); err != nil {
				return err
			}
			if err := checkContentHashes(outputDirectory, content, &cipherHashTree); err != nil {
				fmt.Println(err)
				return err
			}
		}
		if progressWindow.cancelled {
			break
		}
	}

	if doDecryption && !progressWindow.cancelled {
		if err := DecryptContents(outputDir, progressWindow, deleteEncryptedContents); err != nil {
			return err
		}
	}

	return nil
}
