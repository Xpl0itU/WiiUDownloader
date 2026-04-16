package wiiudownloader

import (
	"bytes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fstfmt "github.com/Xpl0itU/WiiUDownloader/internal/formats/fst"
)

const (
	// Lower 24 bits are the FST string table offset for an entry name.
	FST_NAME_OFFSET_MASK    = 0x00FFFFFF
	FST_DIRECTORY_TYPE_FLAG = 0x01
	// Shared content flag indicates entry should not be extracted as a normal file payload.
	FST_SHARED_CONTENT_FLAG = 0x80
	// If unset, entry offsets are scaled by FST factor.
	FST_CONTENT_FACTOR_FLAG = 0x04
	FST_HASHED_CONTENT_TYPE = 0x02
)

func extractWiiUContents(path string, tmd *TMD, cipherHashTree cipher.Block, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	fstEncFile, err := os.Open(filepath.Join(path, tmd.Contents[0].CIDStr+".app"))
	if err != nil {
		return err
	}
	defer fstEncFile.Close()

	var decryptedBuffer bytes.Buffer
	if err := decryptContentToBuffer(fstEncFile, &decryptedBuffer, cipherHashTree, tmd.Contents[0]); err != nil {
		return err
	}

	table, err := fstfmt.Parse(decryptedBuffer.Bytes())
	if err != nil {
		return extractRawWiiUContents(path, tmd, cipherHashTree, progressReporter, deleteEncryptedContents)
	}
	if len(table.Entries) == 0 {
		return extractRawWiiUContents(path, tmd, cipherHashTree, progressReporter, deleteEncryptedContents)
	}

	entry := make([]uint32, MAX_LEVELS)
	level := uint32(0)
	entriesLen := uint32(len(table.Entries))

	for i := uint32(1); i < entriesLen; i++ {
		if progressReporter != nil && entriesLen > 1 {
			progressReporter.UpdateDecryptionProgress(float64(i) / float64(entriesLen-1))
		}

		if level > 0 {
			for level >= 1 && table.Entries[entry[level-1]].Length == i {
				level--
			}
		}

		currentEntry := table.Entries[i]
		if currentEntry.Type&FST_DIRECTORY_TYPE_FLAG != 0 {
			entry[level] = i
			level++
			if level >= MAX_LEVELS {
				return errors.New("level >= MAX_LEVELS")
			}

			// Create the directory immediately to support empty folders
			currentOutputPath := path
			for j := uint32(0); j < level; j++ {
				directory, err := table.NameAt(table.Entries[entry[j]].NameOffset & FST_NAME_OFFSET_MASK)
				if err != nil {
					return fmt.Errorf("failed to read directory name: %w", err)
				}
				currentOutputPath, err = safeJoinUnderBase(path, currentOutputPath, directory)
				if err != nil {
					return err
				}
			}
			if err := os.MkdirAll(currentOutputPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			continue
		}

		currentOutputPath := path
		for j := uint32(0); j < level; j++ {
			directory, err := table.NameAt(table.Entries[entry[j]].NameOffset & FST_NAME_OFFSET_MASK)
			if err != nil {
				return fmt.Errorf("failed to read directory name: %w", err)
			}
			currentOutputPath, err = safeJoinUnderBase(path, currentOutputPath, directory)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(currentOutputPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}

		fileName, err := table.NameAt(currentEntry.NameOffset & FST_NAME_OFFSET_MASK)
		if err != nil {
			return fmt.Errorf("failed to read file name: %w", err)
		}
		targetPath, err := safeJoinUnderBase(path, currentOutputPath, fileName)
		if err != nil {
			return err
		}

		contentOffset := uint64(currentEntry.Offset)
		if currentEntry.Flags&FST_CONTENT_FACTOR_FLAG == 0 {
			contentOffset *= uint64(table.Factor)
		}
		if currentEntry.Type&FST_SHARED_CONTENT_FLAG != 0 {
			continue
		}

		if int(currentEntry.ContentID) >= len(tmd.Contents) {
			return fmt.Errorf("invalid content index %d", currentEntry.ContentID)
		}
		matchingContent := tmd.Contents[currentEntry.ContentID]
		srcFile, err := os.Open(filepath.Join(path, matchingContent.CIDStr+".app"))
		if err != nil {
			return err
		}

		if matchingContent.Type&FST_HASHED_CONTENT_TYPE != 0 {
			err = extractFileHash(srcFile, 0, contentOffset, uint64(currentEntry.Length), targetPath, currentEntry.ContentID, cipherHashTree)
		} else {
			err = extractFile(srcFile, 0, contentOffset, uint64(currentEntry.Length), targetPath, currentEntry.ContentID, cipherHashTree)
		}
		closeErr := srcFile.Close()
		if err != nil {
			return fmt.Errorf("failed to extract file %s (ID: %d, offset: %d, size: %d): %w", fileName, matchingContent.ID, contentOffset, currentEntry.Length, err)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractRawWiiUContents(path string, tmd *TMD, cipherHashTree cipher.Block, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	for i, content := range tmd.Contents {
		if progressReporter != nil && len(tmd.Contents) > 0 {
			progressReporter.UpdateDecryptionProgress(float64(i) / float64(len(tmd.Contents)))
		}

		srcFile, err := os.Open(filepath.Join(path, content.CIDStr+".app"))
		if err != nil {
			return err
		}

		targetPath := decryptedWiiContentPath(path, content.CIDStr, deleteEncryptedContents)
		contentIndex := binary.BigEndian.Uint16(content.Index)
		if content.Type&FST_HASHED_CONTENT_TYPE != 0 {
			err = extractFileHash(srcFile, 0, 0, content.Size, targetPath, contentIndex, cipherHashTree)
		} else {
			err = extractFile(srcFile, 0, 0, content.Size, targetPath, contentIndex, cipherHashTree)
		}
		closeErr := srcFile.Close()
		if err != nil {
			return fmt.Errorf("failed to extract raw content %s: %w", content.CIDStr, err)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func safeJoinUnderBase(basePath string, current string, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if cleanName == "." || cleanName == ".." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "../") {
		return "", fmt.Errorf("unsafe path in content metadata: %q", name)
	}
	target := filepath.Join(current, cleanName)
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if absTarget != absBase && !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe extraction target: %s", absTarget)
	}
	return absTarget, nil
}
