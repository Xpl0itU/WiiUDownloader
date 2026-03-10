package wiiudownloader

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

var wiiUCommonKey = []byte{0xD7, 0xB0, 0x04, 0x02, 0x65, 0x9B, 0xA2, 0xAB, 0xD2, 0xCB, 0x0D, 0xB2, 0x7F, 0xA2, 0xB6, 0x56}
var wiiCommonKeys = map[byte][]byte{
	0: {0xEB, 0xE4, 0x2A, 0x22, 0x5E, 0x85, 0x93, 0xE4, 0x48, 0xD9, 0xC5, 0x45, 0x73, 0x81, 0xAA, 0xF7},
	1: {0x63, 0xB8, 0x2B, 0xB4, 0xF4, 0x61, 0x4E, 0x2E, 0x13, 0xF2, 0xFE, 0xFB, 0xBA, 0x4C, 0x9B, 0x7E},
	2: {0x30, 0xBF, 0xC7, 0x6E, 0x7C, 0x19, 0xAF, 0xBB, 0x23, 0x16, 0x33, 0x30, 0xCE, 0xD7, 0xC2, 0x8D},
}

const (
	BLOCK_SIZE        = 0x8000
	BLOCK_SIZE_HASHED = 0x10000
	HASH_BLOCK_SIZE   = 0xFC00
	HASHES_SIZE       = 0x0400
	MAX_LEVELS        = 0x10
)

const (
	READ_SIZE    = 8 * 1024 * 1024
	MAX_FST_SIZE = 200 * 1024 * 1024
)

type Content struct {
	ID     uint32
	Index  []byte
	Type   uint16
	Size   uint64
	Hash   []byte
	CIDStr string
}

func DecryptContents(path string, progressReporter ProgressReporter, deleteEncryptedContents bool) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("decryption error: %w", err)
		}
	}()

	tmdPath := filepath.Join(path, "title.tmd")
	if _, statErr := os.Stat(tmdPath); os.IsNotExist(statErr) {
		return statErr
	}

	tmdData, err := os.ReadFile(tmdPath)
	if err != nil {
		return err
	}

	tmd, err := ParseTMD(tmdData)
	if err != nil {
		return err
	}

	if err := resolveContentFileNames(path, tmd); err != nil {
		return err
	}

	encryptedTitleKey, ticketKeyIndex, err := readTicketData(filepath.Join(path, "title.tik"))
	if err != nil {
		return err
	}

	selectedCommonKey := chooseCommonKey(tmd.Version, ticketKeyIndex)
	cbcCipher, err := aes.NewCipher(selectedCommonKey)
	if err != nil {
		return err
	}

	var titleIDBytes [8]byte
	binary.BigEndian.PutUint64(titleIDBytes[:], tmd.TitleID)
	var ivTitle [aes.BlockSize]byte
	copy(ivTitle[:], titleIDBytes[:])

	cbc := cipher.NewCBCDecrypter(cbcCipher, ivTitle[:])
	decryptedTitleKey := make([]byte, len(encryptedTitleKey))
	cbc.CryptBlocks(decryptedTitleKey, encryptedTitleKey)

	cipherHashTree, err := aes.NewCipher(decryptedTitleKey)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	if tmd.Version == TMD_VERSION_WIIU {
		if err := extractWiiUContents(path, tmd, cipherHashTree, progressReporter, deleteEncryptedContents); err != nil {
			return err
		}
	} else {
		if err := extractWiiContents(path, tmd, cipherHashTree, progressReporter, deleteEncryptedContents); err != nil {
			return err
		}
	}

	if progressReporter != nil {
		progressReporter.UpdateDecryptionProgress(1.0)
	}
	if deleteEncryptedContents {
		if err := doDeleteEncryptedContents(path); err != nil {
			log.Printf("failed to remove encrypted contents in %q: %v", path, err)
		}
	}
	return nil
}

func resolveContentFileNames(path string, tmd *TMD) error {
	for i := range tmd.Contents {
		tmd.Contents[i].CIDStr = fmt.Sprintf("%08X", tmd.Contents[i].ID)
		if _, err := os.Stat(filepath.Join(path, tmd.Contents[i].CIDStr+".app")); err == nil {
			continue
		}

		tmd.Contents[i].CIDStr = fmt.Sprintf("%08x", tmd.Contents[i].ID)
		if _, err := os.Stat(filepath.Join(path, tmd.Contents[i].CIDStr+".app")); err != nil {
			return errors.New("content not found")
		}
	}
	return nil
}

func readTicketData(ticketPath string) ([]byte, byte, error) {
	const TICKET_KEY_INDEX_UNKNOWN = 0xFF

	ticketKeyIndex := byte(TICKET_KEY_INDEX_UNKNOWN)
	if _, err := os.Stat(ticketPath); err != nil {
		if os.IsNotExist(err) {
			return nil, ticketKeyIndex, nil
		}
		return nil, ticketKeyIndex, err
	}

	cetk, err := os.Open(ticketPath)
	if err != nil {
		return nil, ticketKeyIndex, err
	}
	defer cetk.Close()

	if _, err := cetk.Seek(TICKET_ENCRYPTED_KEY_OFFSET, io.SeekStart); err != nil {
		return nil, ticketKeyIndex, err
	}
	encryptedTitleKey := make([]byte, TICKET_ENCRYPTED_KEY_SIZE)
	if _, err := io.ReadFull(cetk, encryptedTitleKey); err != nil {
		return nil, ticketKeyIndex, err
	}
	if _, err := cetk.Seek(TICKET_KEY_INDEX_OFFSET, io.SeekStart); err != nil {
		return nil, ticketKeyIndex, err
	}
	if err := binary.Read(cetk, binary.BigEndian, &ticketKeyIndex); err != nil {
		return nil, ticketKeyIndex, err
	}
	return encryptedTitleKey, ticketKeyIndex, nil
}

func chooseCommonKey(tmdVersion byte, ticketKeyIndex byte) []byte {
	if tmdVersion == TMD_VERSION_WII {
		if key, ok := wiiCommonKeys[ticketKeyIndex]; ok {
			return key
		}
		return wiiCommonKeys[0]
	}
	return wiiUCommonKey
}
