package wiiudownloader

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// Content type flag indicating hash-tree/H3 protected content.
	CONTENT_TYPE_HASHED = 0x02
	// Each SHA1 entry in H0/H1/H2/H3 tables is 0x14 bytes.
	HASH_ENTRY_SIZE        = 0x14
	HASH_ENTRIES_PER_LEVEL = 0x10
	// Decrypted 0x400-byte hash header layout: H0[0x000:0x140], H1[0x140:0x280], H2[0x280:0x3C0].
	HASH_H0_START = 0x000
	HASH_H1_START = 0x140
	HASH_H2_START = 0x280
	HASH_H2_END   = 0x3c0
)

func extractFileHash(src *os.File, partDataOffset uint64, fileOffset uint64, size uint64, path string, contentID uint16, cipherHashTree cipher.Block) error {
	writeSize := HASH_BLOCK_SIZE
	blockNumber := (fileOffset / HASH_BLOCK_SIZE) & (HASH_ENTRIES_PER_LEVEL - 1)

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()

	bw := bufio.NewWriterSize(dst, BLOCK_SIZE_HASHED)
	defer bw.Flush()

	readOffset := fileOffset / HASH_BLOCK_SIZE * BLOCK_SIZE_HASHED
	subOffset := fileOffset - (fileOffset / HASH_BLOCK_SIZE * HASH_BLOCK_SIZE)
	if subOffset+size > uint64(writeSize) {
		writeSize -= int(subOffset)
	}

	if _, err := src.Seek(int64(partDataOffset+readOffset), io.SeekStart); err != nil {
		return err
	}

	encryptedHashedContentBuffer := make([]byte, BLOCK_SIZE_HASHED)
	decryptedHashedContentBuffer := make([]byte, HASH_BLOCK_SIZE)
	hashes := make([]byte, HASHES_SIZE)

	fileInfo, err := src.Stat()
	if err != nil {
		return err
	}
	fileSize := fileInfo.Size()

	for size > 0 {
		if uint64(writeSize) > size {
			writeSize = int(size)
		}

		currentPos := int64(partDataOffset + readOffset)
		remainingFile := fileSize - currentPos
		if remainingFile <= 0 {
			return io.ErrUnexpectedEOF
		}

		readLen := BLOCK_SIZE_HASHED
		if int64(readLen) > remainingFile {
			readLen = int(remainingFile)
		}
		if readLen%aes.BlockSize != 0 {
			return fmt.Errorf("read length %d is not a multiple of AES block size", readLen)
		}
		if readLen < HASHES_SIZE {
			return io.ErrUnexpectedEOF
		}

		if _, err := io.ReadFull(src, encryptedHashedContentBuffer[:readLen]); err != nil {
			return fmt.Errorf("failed to read encrypted content block at offset %d: %w", partDataOffset+readOffset, err)
		}

		var iv [aes.BlockSize]byte
		iv[1] = byte(contentID)
		cipher.NewCBCDecrypter(cipherHashTree, iv[:]).CryptBlocks(hashes, encryptedHashedContentBuffer[:HASHES_SIZE])

		h0RangeStart := int(HASH_ENTRY_SIZE * blockNumber)
		h0RangeEnd := h0RangeStart + sha1.Size
		h0Hash := hashes[h0RangeStart:h0RangeEnd]
		var ivBlock [aes.BlockSize]byte
		copy(ivBlock[:], hashes[h0RangeStart:h0RangeStart+aes.BlockSize])
		if blockNumber == 0 {
			ivBlock[1] ^= byte(contentID)
		}

		cipher.NewCBCDecrypter(cipherHashTree, ivBlock[:]).CryptBlocks(decryptedHashedContentBuffer, encryptedHashedContentBuffer[HASHES_SIZE:readLen])
		hash := sha1.Sum(decryptedHashedContentBuffer[:HASH_BLOCK_SIZE])
		if blockNumber == 0 {
			hash[1] ^= byte(contentID)
		}
		if !bytes.Equal(hash[:], h0Hash) {
			return errors.New("h0 hash mismatch")
		}

		n, err := bw.Write(decryptedHashedContentBuffer[subOffset : subOffset+uint64(writeSize)])
		if err != nil {
			return err
		}
		size -= uint64(n)

		blockNumber++
		if blockNumber >= HASH_ENTRIES_PER_LEVEL {
			blockNumber = 0
		}
		if subOffset != 0 {
			writeSize = HASH_BLOCK_SIZE
			subOffset = 0
		}
		readOffset += uint64(BLOCK_SIZE_HASHED)
	}
	return nil
}

func extractFile(src *os.File, partDataOffset uint64, fileOffset uint64, size uint64, path string, contentID uint16, cipherHashTree cipher.Block) error {
	writeSize := BLOCK_SIZE

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()

	bw := bufio.NewWriterSize(dst, BLOCK_SIZE)
	defer bw.Flush()

	readOffset := fileOffset / BLOCK_SIZE * BLOCK_SIZE
	subOffset := fileOffset - (fileOffset / BLOCK_SIZE * BLOCK_SIZE)
	if subOffset+size > uint64(writeSize) {
		writeSize -= int(subOffset)
	}

	if _, err := src.Seek(int64(partDataOffset+readOffset), io.SeekStart); err != nil {
		return err
	}

	var ivLocal [aes.BlockSize]byte
	ivLocal[1] = byte(contentID)
	aesCipher := cipher.NewCBCDecrypter(cipherHashTree, ivLocal[:])

	encryptedContentBuffer := make([]byte, BLOCK_SIZE)
	decryptedContentBuffer := make([]byte, BLOCK_SIZE)

	fileInfo, err := src.Stat()
	if err != nil {
		return err
	}
	fileSize := fileInfo.Size()

	for size > 0 {
		if uint64(writeSize) > size {
			writeSize = int(size)
		}

		currentPos := int64(partDataOffset + readOffset)
		remainingFile := fileSize - currentPos
		if remainingFile <= 0 {
			return io.ErrUnexpectedEOF
		}

		readLen := BLOCK_SIZE
		if int64(readLen) > remainingFile {
			readLen = int(remainingFile)
		}
		if readLen%aes.BlockSize != 0 {
			return fmt.Errorf("read length %d is not a multiple of AES block size", readLen)
		}

		if _, err := io.ReadFull(src, encryptedContentBuffer[:readLen]); err != nil {
			return fmt.Errorf("failed to read encrypted content block at offset %d (expected %d bytes): %w", partDataOffset+readOffset, readLen, err)
		}

		aesCipher.CryptBlocks(decryptedContentBuffer[:readLen], encryptedContentBuffer[:readLen])

		n, err := bw.Write(decryptedContentBuffer[subOffset : subOffset+uint64(writeSize)])
		if err != nil {
			return err
		}
		size -= uint64(n)

		if subOffset != 0 {
			writeSize = BLOCK_SIZE
			subOffset = 0
		}
		readOffset += uint64(BLOCK_SIZE)
	}
	return nil
}

func decryptContentToBuffer(encryptedFile *os.File, decryptedBuffer *bytes.Buffer, cipherHashTree cipher.Block, content Content) error {
	hasHashTree := content.Type&CONTENT_TYPE_HASHED != 0
	encryptedStat, err := encryptedFile.Stat()
	if err != nil {
		return err
	}
	encryptedSize := encryptedStat.Size()
	path := filepath.Dir(encryptedFile.Name())

	growSize := int64(content.Size)
	if hasHashTree {
		growSize = encryptedSize
	}
	if growSize > MAX_FST_SIZE {
		return fmt.Errorf("FST size %d exceeds maximum limit of %d", growSize, MAX_FST_SIZE)
	}
	decryptedBuffer.Grow(int(growSize))

	if hasHashTree {
		chunkCount := encryptedSize / BLOCK_SIZE_HASHED
		h3Data, err := os.ReadFile(filepath.Join(path, fmt.Sprintf("%s.h3", content.CIDStr)))
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

		for chunkNum := int64(0); chunkNum < chunkCount; chunkNum++ {
			if _, err := io.ReadFull(encryptedFile, hashesBuffer); err != nil {
				return err
			}
			var zeroIV [aes.BlockSize]byte
			cipher.NewCBCDecrypter(cipherHashTree, zeroIV[:]).CryptBlocks(hashes, hashesBuffer)

			h0Hashes := hashes[HASH_H0_START:HASH_H1_START]
			h1Hashes := hashes[HASH_H1_START:HASH_H2_START]
			h2Hashes := hashes[HASH_H2_START:HASH_H2_END]

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

			if _, err := io.ReadFull(encryptedFile, decryptedDataBuffer); err != nil {
				return err
			}

			cipher.NewCBCDecrypter(cipherHashTree, h0Hash[:16]).CryptBlocks(decryptedDataBuffer, decryptedDataBuffer)
			decryptedDataHash := sha1.Sum(decryptedDataBuffer)
			if !bytes.Equal(decryptedDataHash[:], h0Hash) {
				return errors.New("data block hash invalid")
			}

			if _, err = decryptedBuffer.Write(hashes); err != nil {
				return err
			}
			if _, err = decryptedBuffer.Write(decryptedDataBuffer); err != nil {
				return err
			}

			h0HashNum, h1HashNum, h2HashNum, h3HashNum = advanceHashIndices(h0HashNum, h1HashNum, h2HashNum, h3HashNum)
		}
		return nil
	}

	if len(content.Hash) >= sha1.Size {
		if _, err := encryptedFile.Seek(0, io.SeekStart); err != nil {
			return err
		}
		h := sha1.New()
		if _, err := io.Copy(h, encryptedFile); err == nil {
			if bytes.Equal(content.Hash[:sha1.Size], h.Sum(nil)) {
				if _, err := encryptedFile.Seek(0, io.SeekStart); err != nil {
					return err
				}
				decryptedBuffer.Reset()
				if _, err := io.Copy(decryptedBuffer, encryptedFile); err != nil {
					return err
				}
				return nil
			}
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
	leftHash := content.Size

	decBuf := make([]byte, READ_SIZE)
	readSizedBuffer := make([]byte, READ_SIZE)

	for i := 0; i <= int(content.Size/READ_SIZE)+1; i++ {
		toRead := min(READ_SIZE, left)
		toReadHash := min(READ_SIZE, leftHash)
		toReadAligned := alignToAESBlockSize(toRead)

		if _, err := io.ReadFull(encryptedFile, readSizedBuffer[:toReadAligned]); err != nil {
			return err
		}

		cipherContent.CryptBlocks(decBuf[:toReadAligned], readSizedBuffer[:toReadAligned])
		if _, err := contentHash.Write(decBuf[:toReadHash]); err != nil {
			return err
		}
		if _, err = decryptedBuffer.Write(decBuf[:toRead]); err != nil {
			return err
		}

		left -= toRead
		leftHash -= toRead
		if left == 0 {
			break
		}
	}

	if len(content.Hash) >= sha1.Size && !bytes.Equal(content.Hash[:sha1.Size], contentHash.Sum(nil)) {
		return errors.New("content hash mismatch")
	}
	return nil
}

func hashEntryAt(data []byte, index int64) []byte {
	start := int(index * HASH_ENTRY_SIZE)
	end := start + HASH_ENTRY_SIZE
	return data[start:end]
}

func advanceHashIndices(h0, h1, h2, h3 int64) (int64, int64, int64, int64) {
	h0++
	if h0 >= HASH_ENTRIES_PER_LEVEL {
		h0 = 0
		h1++
	}
	if h1 >= HASH_ENTRIES_PER_LEVEL {
		h1 = 0
		h2++
	}
	if h2 >= HASH_ENTRIES_PER_LEVEL {
		h2 = 0
		h3++
	}
	return h0, h1, h2, h3
}

func alignToAESBlockSize(size uint64) uint64 {
	mask := uint64(aes.BlockSize - 1)
	return (size + mask) &^ mask
}
