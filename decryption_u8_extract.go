package wiiudownloader

import (
	"bytes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	u8fmt "github.com/Xpl0itU/WiiUDownloader/internal/formats/u8"
)

const (
	// Wii U U8 archives are aligned; probe aligned positions for U8 magic.
	U8_HEADER_PROBE_SIZE = 32
	U8_ALIGNMENT_STEP    = 16
	U8_MAGIC_OFFSET_SIZE = 4
)

func extractWiiContents(path string, tmd *TMD, cipherHashTree cipher.Block, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	for i, content := range tmd.Contents {
		if progressReporter != nil && len(tmd.Contents) > 0 {
			progressReporter.UpdateDecryptionProgress(float64(i) / float64(len(tmd.Contents)))
		}

		srcFile, err := os.Open(filepath.Join(path, content.CIDStr+".app"))
		if err != nil {
			return err
		}

		decryptedBuffer := bytes.Buffer{}
		if err := decryptContentToBuffer(srcFile, &decryptedBuffer, cipherHashTree, content); err != nil {
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
			case extractCount == 0 && i == 0:
				outPath = path
			case extractCount == 0:
				outPath = filepath.Join(path, content.CIDStr)
			default:
				outPath = filepath.Join(path, content.CIDStr, fmt.Sprintf("u8_%X", pos))
			}

			if err := u8fmt.Extract(decData[pos:], outPath); err == nil {
				extractCount++
			}
		}

		if !foundU8 {
			outputPath := decryptedWiiContentPath(path, content.CIDStr, deleteEncryptedContents)
			if err := os.WriteFile(outputPath, decData, 0o644); err != nil {
				return err
			}
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
