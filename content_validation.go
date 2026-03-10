package wiiudownloader

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
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
