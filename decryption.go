package wiiudownloader

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

const READ_SIZE = 8 * 1024 * 1024

type Content struct {
	ID    uint32
	Index []byte
	Type  uint16
	Size  uint64
	Hash  []byte
}

// TODO: Implement Logging, add extraction at the same time of decryption
func DecryptContents(path string, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	tmdPath := filepath.Join(path, "title.tmd")
	if _, err := os.Stat(tmdPath); os.IsNotExist(err) {
		fmt.Println("No TMD (title.tmd) was found.")
		return err
	}

	// find title id and content id
	var titleID []byte
	var contentCount uint16
	tmd, err := os.Open(tmdPath)
	if err != nil {
		fmt.Println("Failed to open TMD:", err)
		return err
	}
	defer tmd.Close()

	tmd.Seek(0x18C, io.SeekStart)
	titleID = make([]byte, 8)
	if _, err := io.ReadFull(tmd, titleID); err != nil {
		fmt.Println("Failed to read title ID:", err)
		return err
	}

	tmd.Seek(0x1DE, io.SeekStart)
	if err := binary.Read(tmd, binary.BigEndian, &contentCount); err != nil {
		fmt.Println("Failed to read content count:", err)
		return err
	}

	tmd.Seek(0x204, io.SeekStart)
	tmdIndex := make([]byte, 2)
	if _, err := io.ReadFull(tmd, tmdIndex); err != nil {
		fmt.Println("Failed to read TMD index:", err)
		return err
	}

	contents := make([]Content, contentCount)

	for c := uint16(0); c < contentCount; c++ {
		offset := 2820 + (48 * c)
		tmd.Seek(int64(offset), io.SeekStart)
		if err := binary.Read(tmd, binary.BigEndian, &contents[c].ID); err != nil {
			return err
		}

		tmd.Seek(0xB08+(0x30*int64(c)), io.SeekStart)
		contents[c].Index = make([]byte, 2)
		if _, err := io.ReadFull(tmd, contents[c].Index); err != nil {
			fmt.Println("Failed to read content index:", err)
			return err
		}

		tmd.Seek(0xB0A+(0x30*int64(c)), io.SeekStart)
		if err := binary.Read(tmd, binary.BigEndian, &contents[c].Type); err != nil {
			fmt.Println("Failed to read content type:", err)
			return err
		}

		tmd.Seek(0xB0C+(0x30*int64(c)), io.SeekStart)
		if err := binary.Read(tmd, binary.BigEndian, &contents[c].Size); err != nil {
			fmt.Println("Failed to read content size:", err)
			return err
		}

		tmd.Seek(0xB14+(0x30*int64(c)), io.SeekStart)
		contents[c].Hash = make([]byte, 0x14)
		if _, err := io.ReadFull(tmd, contents[c].Hash); err != nil {
			fmt.Println("Failed to read content hash:", err)
			return err
		}
	}
	fmt.Printf("Title ID: %s\n", hex.EncodeToString(titleID))

	// Find the encrypted titlekey
	var encryptedTitleKey []byte

	ticketPath := filepath.Join(path, "title.tik")

	if _, err := os.Stat(ticketPath); err == nil {
		cetk, err := os.Open(ticketPath)
		if err == nil {
			cetk.Seek(0x1BF, 0)
			encryptedTitleKey = make([]byte, 0x10)
			cetk.Read(encryptedTitleKey)
			cetk.Close()
		}
	}
	fmt.Printf("Encrypted Titlekey: %s\n", hex.EncodeToString(encryptedTitleKey))
	c, err := aes.NewCipher(commonKey)
	if err != nil {
		return err
	}

	cbc := cipher.NewCBCDecrypter(c, append(titleID, make([]byte, 8)...))

	decryptedTitleKey := make([]byte, len(encryptedTitleKey))
	cbc.CryptBlocks(decryptedTitleKey, encryptedTitleKey)

	cipherHashTree, err := aes.NewCipher(decryptedTitleKey)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	fmt.Printf("Decrypted Titlekey: %x\n", decryptedTitleKey)
	for _, c := range contents {
		cidStr := fmt.Sprintf("%08X", c.ID)
		fmt.Printf("Decrypting %v...\n", cidStr)

		left, err := os.Stat(filepath.Join(path, cidStr+".app"))
		if err != nil {
			cidStr = fmt.Sprintf("%08x", c.ID)
			left, err = os.Stat(filepath.Join(path, cidStr+".app"))
			if err != nil {
				fmt.Println("Failed to find encrypted content:", err)
				return err
			}
		}
		leftSize := left.Size()

		if c.Type&2 != 0 { // if has a hash tree
			chunkCount := leftSize / 0x10000
			h3Data, err := os.ReadFile(filepath.Join(path, fmt.Sprintf("%s.h3", cidStr)))
			if err != nil {
				return err
			}
			h3BytesSHASum := sha1.Sum(h3Data)
			if hex.EncodeToString(h3BytesSHASum[:]) != hex.EncodeToString(c.Hash) {
				fmt.Println("H3 Hash mismatch!")
				fmt.Println(" > TMD:    " + hex.EncodeToString(c.Hash))
				fmt.Println(" > Result: " + hex.EncodeToString(h3BytesSHASum[:]))
				return errors.New("H3 Hash mismatch")
			}

			h0HashNum := int64(0)
			h1HashNum := int64(0)
			h2HashNum := int64(0)
			h3HashNum := int64(0)

			encryptedFile, err := os.Open(filepath.Join(path, cidStr+".app"))
			if err != nil {
				return err
			}
			defer encryptedFile.Close()

			decryptedFile, err := os.Create(filepath.Join(path, cidStr+".app.dec"))
			if err != nil {
				return err
			}
			defer decryptedFile.Close()

			decryptedContent := make([]byte, 0x400)
			buffer := make([]byte, 0x400)

			for chunkNum := int64(0); chunkNum < chunkCount; chunkNum++ {
				encryptedFile.Read(buffer)
				cipher.NewCBCDecrypter(cipherHashTree, make([]byte, aes.BlockSize)).CryptBlocks(decryptedContent, buffer)

				h0Hashes := decryptedContent[0:0x140]
				h1Hashes := decryptedContent[0x140:0x280]
				h2Hashes := decryptedContent[0x280:0x3c0]

				h0Hash := h0Hashes[(h0HashNum * 0x14):((h0HashNum + 1) * 0x14)]
				h1Hash := h1Hashes[(h1HashNum * 0x14):((h1HashNum + 1) * 0x14)]
				h2Hash := h2Hashes[(h2HashNum * 0x14):((h2HashNum + 1) * 0x14)]
				h3Hash := h3Data[(h3HashNum * 0x14):((h3HashNum + 1) * 0x14)]

				h0HashesHash := sha1.Sum(h0Hashes)
				h1HashesHash := sha1.Sum(h1Hashes)
				h2HashesHash := sha1.Sum(h2Hashes)

				if !reflect.DeepEqual(h0HashesHash[:], h1Hash) {
					return errors.New("h0 Hashes Hash mismatch")
				}
				if !reflect.DeepEqual(h1HashesHash[:], h2Hash) {
					return errors.New("h1 Hashes Hash mismatch")
				}
				if !reflect.DeepEqual(h2HashesHash[:], h3Hash) {
					return errors.New("h2 Hashes Hash mismatch")
				}

				decryptedData := make([]byte, 0xFC00)
				encryptedFile.Read(decryptedData)

				iv := h0Hash[:16]
				cipher.NewCBCDecrypter(cipherHashTree, iv).CryptBlocks(decryptedData, decryptedData)
				decryptedDataHash := sha1.Sum(decryptedData)

				if !reflect.DeepEqual(decryptedDataHash[:], h0Hash) {
					fmt.Printf("\rData block hash invalid in chunk %v\n", chunkNum)
					return errors.New("data block hash invalid")
				}

				decryptedFile.Write(decryptedContent)
				decryptedFile.Write(decryptedData)

				h0HashNum++
				if h0HashNum >= 16 {
					h0HashNum = 0
					h1HashNum++
				}
				if h1HashNum >= 16 {
					h1HashNum = 0
					h2HashNum++
				}
				if h2HashNum >= 16 {
					h2HashNum = 0
					h3HashNum++
				}
			}
		} else {
			cipherHashTree, err := aes.NewCipher(decryptedTitleKey)
			if err != nil {
				fmt.Println(err)
				return err
			}
			cipherContent := cipher.NewCBCDecrypter(cipherHashTree, append(c.Index, make([]byte, 14)...))
			contentHash := sha1.New()
			left := c.Size
			leftHash := c.Size

			encrypted, err := os.Open(filepath.Join(path, cidStr+".app"))
			if err != nil {
				return err
			}
			defer encrypted.Close()

			decrypted, err := os.Create(filepath.Join(path, cidStr+".app.dec"))
			if err != nil {
				return err
			}
			defer decrypted.Close()

			for i := 0; i <= int(c.Size/READ_SIZE)+1; i++ {
				toRead := min(READ_SIZE, left)
				toReadHash := min(READ_SIZE, leftHash)

				encryptedContent := make([]byte, toRead)
				_, err = io.ReadFull(encrypted, encryptedContent)
				if err != nil {
					return err
				}

				decryptedContent := make([]byte, len(encryptedContent))
				cipherContent.CryptBlocks(decryptedContent, encryptedContent)
				contentHash.Write(decryptedContent[:toReadHash])
				_, err = decrypted.Write(decryptedContent)
				if err != nil {
					return err
				}

				left -= uint64(toRead)
				leftHash -= uint64(toRead)

				if left == 0 {
					break
				}
			}
			if !reflect.DeepEqual(c.Hash, contentHash.Sum(nil)) {
				print("Content Hash mismatch!")
				return errors.New("Content Hash mismatch")
			}
		}
	}
	return nil
}
