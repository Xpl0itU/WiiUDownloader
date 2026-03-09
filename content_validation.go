package wiiudownloader

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"os"
)

func expectedContentDownloadSize(content Content) int64 {
	return int64(content.Size)
}

func expectedH3DownloadSize(content Content) int64 {
	if content.Type&CONTENT_TYPE_HASHED != CONTENT_TYPE_HASHED {
		return 0
	}
	chunkCount := (content.Size + HASH_BLOCK_SIZE - 1) / HASH_BLOCK_SIZE
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
