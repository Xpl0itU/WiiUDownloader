package wiiudownloader

import (
	"os"
	"path/filepath"
	"strings"
)

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func isThisDecryptedFile(path string) bool {
	return strings.Contains(path, "code") || strings.Contains(path, "content") || strings.Contains(path, "meta")
}

func doDeleteEncryptedContents(path string) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && !isThisDecryptedFile(filePath) {
			if err := os.Remove(filePath); err != nil {
				return err
			}
		}
		return nil
	})
}

type BufferedWriter struct {
	file       *os.File
	downloaded *int64
	buffer     []byte
}

func NewFileWriterWithProgress(file *os.File, downloaded *int64) (*BufferedWriter, error) {
	return &BufferedWriter{
		file:       file,
		downloaded: downloaded,
		buffer:     make([]byte, bufferSize),
	}, nil
}

func (bw *BufferedWriter) Write(data []byte) (int, error) {
	written := 0
	for written < len(data) {
		remaining := len(data) - written
		toWrite := min(bufferSize, uint64(remaining))
		copy(bw.buffer, data[written:written+int(toWrite)])
		n, err := bw.file.Write(bw.buffer[:toWrite])
		if err != nil {
			return written, err
		}
		written += n
		*bw.downloaded += int64(n)
	}
	return written, nil
}
