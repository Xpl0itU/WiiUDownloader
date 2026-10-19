package wiiudownloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const tidHighWiiSystemApp = 0x00010002
const tidHighWiiSystem = 0x00010008

func titleIDsMatchTMD(expectedTitleID, actualTitleID uint64, tmdVersion byte) bool {
	if expectedTitleID == 0 || actualTitleID == expectedTitleID {
		return true
	}
	if tmdVersion != TMD_VERSION_WII {
		return false
	}
	expectedHigh := GetTitleIDHigh(expectedTitleID)
	actualHigh := GetTitleIDHigh(actualTitleID)
	switch expectedHigh {
	case TID_HIGH_VWII_SYSTEM_APP:
		if actualHigh != tidHighWiiSystemApp {
			return false
		}
	case TID_HIGH_VWII_SYSTEM:
		if actualHigh != tidHighWiiSystem {
			return false
		}
	default:
		return false
	}
	return GetTitleIDLow(expectedTitleID) == GetTitleIDLow(actualTitleID)
}

func expectedContentDownloadSize(content Content) int64 {
	if content.Type&CONTENT_TYPE_HASHED == CONTENT_TYPE_HASHED {
		return int64(content.Size)
	}
	return int64(alignToAESBlockSize(content.Size))
}

// DownloadSize is the number of bytes the CDN serves for this content,
// including its hash tree when the content is hashed.
func (c Content) DownloadSize() int64 {
	return expectedContentDownloadSize(c) + expectedH3DownloadSize(c)
}

func expectedH3DownloadSize(content Content) int64 {
	if content.Type&CONTENT_TYPE_HASHED != CONTENT_TYPE_HASHED {
		return 0
	}
	chunkCount := (content.Size + BLOCK_SIZE_HASHED - 1) / BLOCK_SIZE_HASHED
	h3Entries := (chunkCount + (HASH_ENTRIES_PER_LEVEL * HASH_ENTRIES_PER_LEVEL * HASH_ENTRIES_PER_LEVEL) - 1) /
		(HASH_ENTRIES_PER_LEVEL * HASH_ENTRIES_PER_LEVEL * HASH_ENTRIES_PER_LEVEL)
	return int64(h3Entries) * HASH_ENTRY_SIZE
}

func verifyH3File(path string, content Content) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha1.Sum(data)
	if len(content.Hash) < sha1.Size || !bytes.Equal(sum[:], content.Hash[:sha1.Size]) {
		return errors.New("H3 Hash mismatch")
	}
	return nil
}

func verifyContentFile(appPath, h3Path string, content Content, cipherHashTree cipher.Block) error {
	if cipherHashTree == nil {
		return errors.New("missing title cipher")
	}
	encryptedFile, err := os.Open(appPath)
	if err != nil {
		return err
	}
	defer encryptedFile.Close()

	if content.Type&CONTENT_TYPE_HASHED == CONTENT_TYPE_HASHED {
		encryptedStat, err := encryptedFile.Stat()
		if err != nil {
			return err
		}
		encryptedSize := encryptedStat.Size()
		if encryptedSize < expectedContentDownloadSize(content) {
			return io.ErrUnexpectedEOF
		}

		h3Data, err := os.ReadFile(h3Path)
		if err != nil {
			return err
		}
		h3BytesSHASum := sha1.Sum(h3Data)
		if len(content.Hash) < sha1.Size || !bytes.Equal(h3BytesSHASum[:], content.Hash[:sha1.Size]) {
			return errors.New("H3 Hash mismatch")
		}

		h0HashNum := int64(0)
		h1HashNum := int64(0)
		h2HashNum := int64(0)
		h3HashNum := int64(0)

		hashes := make([]byte, HASHES_SIZE)
		hashesBuffer := make([]byte, HASHES_SIZE)
		decryptedDataBuffer := make([]byte, HASH_BLOCK_SIZE)

		hashesCBC := newReusableCBC(cipherHashTree)
		dataCBC := newReusableCBC(cipherHashTree)

		remaining := encryptedSize
		for remaining > 0 {
			if remaining < HASHES_SIZE || remaining%aes.BlockSize != 0 {
				return errors.New("invalid hashed content block size")
			}
			if _, err := io.ReadFull(encryptedFile, hashesBuffer); err != nil {
				return err
			}
			remaining -= HASHES_SIZE

			var zeroIV [aes.BlockSize]byte
			hashesCBC.decrypt(hashes, hashesBuffer, zeroIV[:])

			h0Hashes := hashes[HASH_H0_START:HASH_H1_START]
			h1Hashes := hashes[HASH_H1_START:HASH_H2_START]
			h2Hashes := hashes[HASH_H2_START:HASH_H2_END]

			if int((h3HashNum+1)*HASH_ENTRY_SIZE) > len(h3Data) {
				return errors.New("H3 file too small")
			}

			h0Hash := hashEntryAt(h0Hashes, h0HashNum)
			h1Hash := hashEntryAt(h1Hashes, h1HashNum)
			h2Hash := hashEntryAt(h2Hashes, h2HashNum)
			h3Hash := hashEntryAt(h3Data, h3HashNum)

			h0HashesHash := sha1.Sum(h0Hashes)
			h1HashesHash := sha1.Sum(h1Hashes)
			h2HashesHash := sha1.Sum(h2Hashes)

			if !bytes.Equal(h0HashesHash[:], h1Hash) {
				return errors.New("h0 Hashes Hash mismatch")
			}
			if !bytes.Equal(h1HashesHash[:], h2Hash) {
				return errors.New("h1 Hashes Hash mismatch")
			}
			if !bytes.Equal(h2HashesHash[:], h3Hash) {
				return errors.New("h2 Hashes Hash mismatch")
			}

			dataLen := int64(HASH_BLOCK_SIZE)
			if remaining < dataLen {
				dataLen = remaining
			}
			if _, err := io.ReadFull(encryptedFile, decryptedDataBuffer[:dataLen]); err != nil {
				return err
			}
			remaining -= dataLen

			dataCBC.decrypt(decryptedDataBuffer[:dataLen], decryptedDataBuffer[:dataLen], h0Hash[:aes.BlockSize])
			if int(dataLen) < HASH_BLOCK_SIZE {
				clear(decryptedDataBuffer[dataLen:])
			}
			decryptedDataHash := sha1.Sum(decryptedDataBuffer[:HASH_BLOCK_SIZE])
			if !bytes.Equal(decryptedDataHash[:], h0Hash) {
				if int(dataLen) < HASH_BLOCK_SIZE {
					partialHash := sha1.Sum(decryptedDataBuffer[:dataLen])
					if !bytes.Equal(partialHash[:], h0Hash) {
						return errors.New("data block hash invalid")
					}
				} else {
					return errors.New("data block hash invalid")
				}
			}

			h0HashNum, h1HashNum, h2HashNum, h3HashNum = advanceHashIndices(h0HashNum, h1HashNum, h2HashNum, h3HashNum)
		}
		return nil
	}

	if len(content.Hash) < sha1.Size {
		return errors.New("invalid content hash length")
	}

	h := sha1.New()
	if _, err := io.Copy(h, encryptedFile); err == nil {
		if bytes.Equal(content.Hash[:sha1.Size], h.Sum(nil)) {
			return nil
		}
	}

	if _, err := encryptedFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var ivContent [aes.BlockSize]byte
	copy(ivContent[:], content.Index)
	cipherContent := cipher.NewCBCDecrypter(cipherHashTree, ivContent[:])
	contentHash := sha1.New()
	left := content.Size

	decBuf := make([]byte, READ_SIZE)
	readSizedBuffer := make([]byte, READ_SIZE)

	for left > 0 {
		toRead := min(uint64(READ_SIZE), left)
		toReadAligned := alignToAESBlockSize(toRead)

		if _, err := io.ReadFull(encryptedFile, readSizedBuffer[:toReadAligned]); err != nil {
			return err
		}

		cipherContent.CryptBlocks(decBuf[:toReadAligned], readSizedBuffer[:toReadAligned])
		if _, err := contentHash.Write(decBuf[:toRead]); err != nil {
			return err
		}

		left -= toRead
	}

	if !bytes.Equal(content.Hash[:sha1.Size], contentHash.Sum(nil)) {
		return errors.New("content hash mismatch")
	}
	return nil
}

func validateTMDFile(path string, expectedTitleID uint64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tmd, err := ParseTMD(data)
	if err != nil {
		return err
	}
	if !titleIDsMatchTMD(expectedTitleID, tmd.TitleID, tmd.Version) {
		return errors.New("title.tmd title ID mismatch")
	}
	return nil
}

func validateTicketFile(path string, expectedTitleID uint64, expectedTitleVersion uint16) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) < TICKET_TITLE_VERSION_OFFSET+TICKET_TITLE_VERSION_SIZE {
		return errors.New("title.tik too small")
	}
	gotTitleID := binary.BigEndian.Uint64(data[TICKET_TITLE_ID_OFFSET : TICKET_TITLE_ID_OFFSET+TICKET_TITLE_ID_SIZE])
	if expectedTitleID != 0 && gotTitleID != expectedTitleID {
		return errors.New("title.tik title ID mismatch")
	}
	gotTitleVersion := binary.BigEndian.Uint16(data[TICKET_TITLE_VERSION_OFFSET : TICKET_TITLE_VERSION_OFFSET+TICKET_TITLE_VERSION_SIZE])
	if expectedTitleVersion != 0 && gotTitleVersion != expectedTitleVersion {
		return errors.New("title.tik title version mismatch")
	}
	return nil
}