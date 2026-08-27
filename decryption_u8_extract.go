package wiiudownloader

import (
	"bytes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	u8fmt "github.com/Xpl0itU/WiiUDownloader/internal/formats/u8"
)

const (
	// Wii U U8 archives are aligned; probe aligned positions for U8 magic.
	U8_HEADER_PROBE_SIZE = 32
	U8_ALIGNMENT_STEP    = 16
	U8_MAGIC_OFFSET_SIZE = 4
)

var decryptedContentBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const maxPooledDecryptedContentSize = 256 << 20

func getDecryptedContentBuffer() *bytes.Buffer {
	return decryptedContentBufPool.Get().(*bytes.Buffer)
}

func putDecryptedContentBuffer(b *bytes.Buffer) {
	b.Reset()
	if b.Cap() <= maxPooledDecryptedContentSize {
		decryptedContentBufPool.Put(b)
	}
}

func extractWiiContents(srcPath string, destPath string, tmd *TMD, cipherHashTree cipher.Block, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	if destPath == "" {
		destPath = srcPath
	}
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	for i, content := range tmd.Contents {
		if progressReporter != nil && len(tmd.Contents) > 0 {
			progressReporter.UpdateDecryptionProgress(float64(i) / float64(len(tmd.Contents)))
		}
		if err := extractWiiContent(srcPath, destPath, i, content, cipherHashTree, deleteEncryptedContents); err != nil {
			return err
		}
	}
	return nil
}

func extractWiiContent(srcPath string, destPath string, contentIndex int, content Content, cipherHashTree cipher.Block, deleteEncryptedContents bool) error {
	srcFile, err := os.Open(filepath.Join(srcPath, content.CIDStr+".app"))
	if err != nil {
		return err
	}

	decryptedBuffer := getDecryptedContentBuffer()
	defer putDecryptedContentBuffer(decryptedBuffer)

	if err := decryptContentToBuffer(srcFile, decryptedBuffer, cipherHashTree, content); err != nil {
		srcFile.Close()
		return err
	}
	if err := srcFile.Close(); err != nil {
		return err
	}

	decData := decryptedBuffer.Bytes()
	foundU8 := false
	extractCount := 0

	for pos := 0; pos < len(decData)-U8_HEADER_PROBE_SIZE; pos += U8_ALIGNMENT_STEP {
		if binary.BigEndian.Uint32(decData[pos:pos+U8_MAGIC_OFFSET_SIZE]) != u8fmt.Magic {
			continue
		}
		if _, err := u8fmt.Parse(decData[pos:]); err != nil {
			continue
		}

		foundU8 = true
		var outPath string
		switch {
		case extractCount == 0 && contentIndex == 0:
			outPath = destPath
		case extractCount == 0:
			outPath = filepath.Join(destPath, content.CIDStr)
		default:
			outPath = filepath.Join(destPath, content.CIDStr, fmt.Sprintf("u8_%X", pos))
		}

		if err := u8fmt.Extract(decData[pos:], outPath); err == nil {
			extractCount++
		}
	}

	if !foundU8 {
		outputPath := decryptedWiiContentPath(destPath, content.CIDStr, deleteEncryptedContents)
		if err := os.WriteFile(outputPath, decData, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func decryptedWiiContentPath(path string, cid string, deleteEncryptedContents bool) string {
	if deleteEncryptedContents {
		return filepath.Join(path, cid+".app")
	}
	return filepath.Join(path, cid+".dec.app")
}
