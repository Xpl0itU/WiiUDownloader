package wiiudownloader

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var commonKey = []byte{0xD7, 0xB0, 0x04, 0x02, 0x65, 0x9B, 0xA2, 0xAB, 0xD2, 0xCB, 0x0D, 0xB2, 0x7F, 0xA2, 0xB6, 0x56}

const (
	BLOCK_SIZE        = 0x8000
	BLOCK_SIZE_HASHED = 0x10000
	HASH_BLOCK_SIZE   = 0xFC00
	HASHES_SIZE       = 0x0400
	MAX_LEVELS        = 0x10
)

const READ_SIZE = 8 * 1024 * 1024

type Content struct {
	ID     uint32
	Index  []byte
	Type   uint16
	Size   uint64
	Hash   []byte
	CIDStr string
}

type FEntry struct {
	Type       byte   // 0 = file, 1 = directory
	NameOffset uint32 // 3 bytes
	Offset     uint32 // 4 bytes
	Length     uint32 // 4 bytes
	Flags      uint16 // 2 bytes
	ContentID  uint16 // 2 bytes
}

type FSTData struct {
	FSTReader   *bytes.Reader
	EntryCount  uint32
	Entries     uint32
	NamesOffset uint32
	FSTEntries  []FEntry
}

func extractFileHash(src *os.File, partDataOffset uint64, fileOffset uint64, size uint64, path string, contentId uint16, cipherHashTree cipher.Block) error {
	writeSize := HASH_BLOCK_SIZE
	blockNumber := (fileOffset / HASH_BLOCK_SIZE) & 0x0F

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()
	bw := bufio.NewWriterSize(dst, BLOCK_SIZE_HASHED)
	defer bw.Flush()

	roffset := fileOffset / HASH_BLOCK_SIZE * BLOCK_SIZE_HASHED
	soffset := fileOffset - (fileOffset / HASH_BLOCK_SIZE * HASH_BLOCK_SIZE)

	if soffset+size > uint64(writeSize) {
		writeSize = writeSize - int(soffset)
	}

	_, err = src.Seek(int64(partDataOffset+roffset), io.SeekStart)
	if err != nil {
		return err
	}

	encryptedHashedContentBuffer := make([]byte, BLOCK_SIZE_HASHED)
	decryptedHashedContentBuffer := make([]byte, BLOCK_SIZE_HASHED)
	hashes := make([]byte, HASHES_SIZE)

	for size > 0 {
		if uint64(writeSize) > size {
			writeSize = int(size)
		}

		if _, err := io.ReadFull(src, encryptedHashedContentBuffer); err != nil {
			return fmt.Errorf("failed to read encrypted content block at offset %d (read %d of %d bytes): %w",
				partDataOffset+roffset, 0, BLOCK_SIZE_HASHED, err)
		}

		var zeroIV [aes.BlockSize]byte
		cipher.NewCBCDecrypter(cipherHashTree, zeroIV[:]).CryptBlocks(hashes, encryptedHashedContentBuffer[:HASHES_SIZE])

		h0Hash := hashes[0x14*blockNumber : 0x14*blockNumber+sha1.Size]

		var ivBlock [aes.BlockSize]byte
		copy(ivBlock[:], hashes[0x14*blockNumber:0x14*blockNumber+aes.BlockSize])

		cipher.NewCBCDecrypter(cipherHashTree, ivBlock[:]).CryptBlocks(decryptedHashedContentBuffer, encryptedHashedContentBuffer[HASHES_SIZE:])

		hash := sha1.Sum(decryptedHashedContentBuffer[:HASH_BLOCK_SIZE])
		if !bytes.Equal(hash[:], h0Hash) {
			return errors.New("h0 hash mismatch")
		}

		n, err := bw.Write(decryptedHashedContentBuffer[soffset : soffset+uint64(writeSize)])
		if err != nil {
			return err
		}

		size -= uint64(n)

		blockNumber++
		if blockNumber >= 16 {
			blockNumber = 0
		}

		if soffset != 0 {
			writeSize = HASH_BLOCK_SIZE
			soffset = 0
		}
	}

	return nil
}

func extractFile(src *os.File, partDataOffset uint64, fileOffset uint64, size uint64, path string, contentId uint16, cipherHashTree cipher.Block) error {
	writeSize := BLOCK_SIZE

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()
	bw := bufio.NewWriterSize(dst, BLOCK_SIZE)
	defer bw.Flush()

	roffset := fileOffset / BLOCK_SIZE * BLOCK_SIZE
	soffset := fileOffset - (fileOffset / BLOCK_SIZE * BLOCK_SIZE)

	if soffset+size > uint64(writeSize) {
		writeSize = writeSize - int(soffset)
	}

	_, err = src.Seek(int64(partDataOffset+roffset), io.SeekStart)
	if err != nil {
		return err
	}

	var ivLocal [aes.BlockSize]byte
	ivLocal[1] = byte(contentId)
	aesCipher := cipher.NewCBCDecrypter(cipherHashTree, ivLocal[:])

	encryptedContentBuffer := make([]byte, BLOCK_SIZE)
	decryptedContentBuffer := make([]byte, BLOCK_SIZE)

	for size > 0 {
		if uint64(writeSize) > size {
			writeSize = int(size)
		}

		if _, err := io.ReadFull(src, encryptedContentBuffer); err != nil {
			return fmt.Errorf("failed to read encrypted content block at offset %d (expected %d bytes): %w",
				partDataOffset+roffset, BLOCK_SIZE, err)
		}

		aesCipher.CryptBlocks(decryptedContentBuffer, encryptedContentBuffer)

		n, err := bw.Write(decryptedContentBuffer[soffset : soffset+uint64(writeSize)])
		if err != nil {
			return err
		}

		size -= uint64(n)

		if soffset != 0 {
			writeSize = BLOCK_SIZE
			soffset = 0
		}
	}

	return nil
}

func (fst *FSTData) Parse() error {
	var err error // Hack to avoid shadowing

	if _, err := fst.FSTReader.Seek(0x8, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to entry count: %w", err)
	}
	if fst.EntryCount, err = readInt(fst.FSTReader, 4); err != nil {
		return fmt.Errorf("failed to read entry count: %w", err)
	}

	if _, err := fst.FSTReader.Seek(int64(0x20+fst.EntryCount*0x20+8), io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to entries: %w", err)
	}
	if fst.Entries, err = readInt(fst.FSTReader, 4); err != nil {
		return fmt.Errorf("failed to read entries: %w", err)
	}
	fst.NamesOffset = 0x20 + fst.EntryCount*0x20 + fst.Entries*0x10

	if _, err := fst.FSTReader.Seek(4, io.SeekCurrent); err != nil {
		return fmt.Errorf("failed to seek to names offset: %w", err)
	}

	for i := uint32(0); i < fst.Entries; i++ {
		entry := FEntry{}
		if entry.Type, err = readByte(fst.FSTReader); err != nil {
			return fmt.Errorf("failed to read entry type for entry %d: %w", i, err)
		}

		nameOffset, err := read3BytesBE(fst.FSTReader)
		if err != nil {
			return fmt.Errorf("failed to read name offset for entry %d: %w", i, err)
		}
		entry.NameOffset = uint32(nameOffset)

		if entry.Offset, err = readInt(fst.FSTReader, 4); err != nil {
			return fmt.Errorf("failed to read entry offset for entry %d: %w", i, err)
		}
		if entry.Length, err = readInt(fst.FSTReader, 4); err != nil {
			return fmt.Errorf("failed to read entry length for entry %d: %w", i, err)
		}

		flags, err := readInt16(fst.FSTReader, 2)
		if err != nil {
			return fmt.Errorf("failed to read flags for entry %d: %w", i, err)
		}
		entry.Flags = flags

		contentID, err := readInt16(fst.FSTReader, 2)
		if err != nil {
			return fmt.Errorf("failed to read content ID for entry %d: %w", i, err)
		}
		entry.ContentID = contentID

		fst.FSTEntries = append(fst.FSTEntries, entry)
	}

	return nil
}

func readByte(f io.ReadSeeker) (byte, error) {
	buf := make([]byte, 1)
	n, err := f.Read(buf)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	return buf[0], nil
}

func readInt(f io.ReadSeeker, s int) (uint32, error) {
	buf := make([]byte, 4) // Buffer size is always 4 for uint32

	n, err := f.Read(buf[:s])
	if err != nil {
		return 0, err
	}

	if n < s {
		// If we didn't read the expected number of bytes, seek back to the
		// previous position in the file and return an error.
		if _, err := f.Seek(int64(-n), io.SeekCurrent); err != nil {
			return 0, err
		}
		return 0, io.ErrUnexpectedEOF
	}

	return binary.BigEndian.Uint32(buf), nil
}

func readInt16(f io.ReadSeeker, s int) (uint16, error) {
	buf := make([]byte, 2) // Buffer size is always 2 for uint16

	n, err := f.Read(buf[:s])
	if err != nil {
		return 0, err
	}

	if n < s {
		// If we didn't read the expected number of bytes, seek back to the
		// previous position in the file and return an error.
		if _, err := f.Seek(int64(-n), io.SeekCurrent); err != nil {
			return 0, err
		}
		return 0, io.ErrUnexpectedEOF
	}

	return binary.BigEndian.Uint16(buf), nil
}

func readString(f io.ReadSeeker) (string, error) {
	buffer := bytes.NewBuffer(nil)
	chunk := make([]byte, 64) // Read in chunks of 64 bytes

	for {
		n, err := f.Read(chunk)
		if err != nil && err != io.EOF {
			return "", err
		}

		if n == 0 {
			break
		}

		// Look for null terminator in this chunk
		nullIndex := bytes.IndexByte(chunk[:n], 0)
		if nullIndex != -1 {
			// Found the null terminator
			buffer.Write(chunk[:nullIndex])

			// Seek back to position right after the null terminator
			_, err = f.Seek(int64(-(n - nullIndex - 1)), io.SeekCurrent)
			if err != nil {
				return "", err
			}

			return buffer.String(), nil
		}

		// No null terminator found, append the whole chunk
		buffer.Write(chunk[:n])

		if err == io.EOF {
			break
		}
	}

	// If we get here without finding a null terminator
	if buffer.Len() == 0 {
		return "", io.EOF
	}

	return buffer.String(), nil
}

func read3BytesBE(f io.ReadSeeker) (uint32, error) {
	b := make([]byte, 3)
	if _, err := f.Read(b); err != nil {
		return 0, err
	}
	return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16, nil
}

func decryptContentToBuffer(encryptedFile *os.File, decryptedBuffer *bytes.Buffer, cipherHashTree cipher.Block, content Content) error {
	hasHashTree := content.Type&2 != 0
	encryptedStat, err := encryptedFile.Stat()
	if err != nil {
		return err
	}
	encryptedSize := encryptedStat.Size()
	path := filepath.Dir(encryptedFile.Name())

	if hasHashTree {
		decryptedBuffer.Grow(int(encryptedSize))
	} else {
		decryptedBuffer.Grow(int(content.Size))
	}

	if hasHashTree { // if has a hash tree
		chunkCount := encryptedSize / 0x10000
		h3Data, err := os.ReadFile(filepath.Join(path, fmt.Sprintf("%s.h3", content.CIDStr)))
		if err != nil {
			return err
		}
		h3BytesSHASum := sha1.Sum(h3Data)
		if !bytes.Equal(h3BytesSHASum[:], content.Hash[:sha1.Size]) {
			return errors.New("H3 Hash mismatch")
		}

		h0HashNum := int64(0)
		h1HashNum := int64(0)
		h2HashNum := int64(0)
		h3HashNum := int64(0)

		hashes := make([]byte, HASHES_SIZE)
		hashesBuffer := make([]byte, HASHES_SIZE)
		decryptedDataBuffer := make([]byte, 0xFC00)

		for chunkNum := int64(0); chunkNum < chunkCount; chunkNum++ {
			if _, err := io.ReadFull(encryptedFile, hashesBuffer); err != nil {
				return err
			}
			var zeroIV [aes.BlockSize]byte
			cipher.NewCBCDecrypter(cipherHashTree, zeroIV[:]).CryptBlocks(hashes, hashesBuffer)

			h0Hashes := hashes[0:0x140]
			h1Hashes := hashes[0x140:0x280]
			h2Hashes := hashes[0x280:0x3c0]

			h0Hash := h0Hashes[(h0HashNum * 0x14):((h0HashNum + 1) * 0x14)]
			h1Hash := h1Hashes[(h1HashNum * 0x14):((h1HashNum + 1) * 0x14)]
			h2Hash := h2Hashes[(h2HashNum * 0x14):((h2HashNum + 1) * 0x14)]
			h3Hash := h3Data[(h3HashNum * 0x14):((h3HashNum + 1) * 0x14)]

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

			if _, err = io.ReadFull(encryptedFile, readSizedBuffer[:toRead]); err != nil {
				return err
			}

			cipherContent.CryptBlocks(decBuf[:toRead], readSizedBuffer[:toRead])
			contentHash.Write(decBuf[:toReadHash])
			if _, err = decryptedBuffer.Write(decBuf[:toRead]); err != nil {
				return err
			}

			left -= toRead
			leftHash -= toRead

			if left == 0 {
				break
			}
		}
		if !bytes.Equal(content.Hash[:sha1.Size], contentHash.Sum(nil)) {
			return errors.New("content hash mismatch")
		}
	}
	return nil
}

func DecryptContents(path string, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	tmdPath := filepath.Join(path, "title.tmd")
	if _, err := os.Stat(tmdPath); os.IsNotExist(err) {
		return err
	}

	tmdData, err := os.ReadFile(tmdPath)
	if err != nil {
		return err
	}
	tmd, err := ParseTMD(tmdData)
	if err != nil {
		return err
	}

	// Check if all contents are present and how they are named
	for i := range tmd.Contents {
		tmd.Contents[i].CIDStr = fmt.Sprintf("%08X", tmd.Contents[i].ID)
		_, err := os.Stat(filepath.Join(path, tmd.Contents[i].CIDStr+".app"))
		if err != nil {
			tmd.Contents[i].CIDStr = fmt.Sprintf("%08x", tmd.Contents[i].ID)
			_, err = os.Stat(filepath.Join(path, tmd.Contents[i].CIDStr+".app"))
			if err != nil {
				return errors.New("content not found")
			}
		}
	}

	// Find the encrypted titlekey
	var encryptedTitleKey []byte

	ticketPath := filepath.Join(path, "title.tik")

	if _, err := os.Stat(ticketPath); err == nil {
		cetk, err := os.Open(ticketPath)
		if err == nil {
			_, _ = cetk.Seek(0x1BF, 0)
			encryptedTitleKey = make([]byte, 0x10)
			if _, err := io.ReadFull(cetk, encryptedTitleKey); err != nil {
				_ = cetk.Close()
				return err
			}
			if err := cetk.Close(); err != nil {
				return err
			}
		}
	}
	c, err := aes.NewCipher(commonKey)
	if err != nil {
		return err
	}

	var titleIDBytes [8]byte
	binary.BigEndian.PutUint64(titleIDBytes[:], tmd.TitleID)
	var ivTitle [16]byte
	copy(ivTitle[:], titleIDBytes[:])
	cbc := cipher.NewCBCDecrypter(c, ivTitle[:])

	decryptedTitleKey := make([]byte, len(encryptedTitleKey))
	cbc.CryptBlocks(decryptedTitleKey, encryptedTitleKey)

	cipherHashTree, err := aes.NewCipher(decryptedTitleKey)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	fstEncFile, err := os.Open(filepath.Join(path, tmd.Contents[0].CIDStr+".app"))
	if err != nil {
		return err
	}

	decryptedBuffer := bytes.Buffer{}
	if err := decryptContentToBuffer(fstEncFile, &decryptedBuffer, cipherHashTree, tmd.Contents[0]); err != nil {
		if err := fstEncFile.Close(); err != nil {
			return err
		}
		return err
	}
	if err := fstEncFile.Close(); err != nil {
		return err
	}
	fst := FSTData{FSTReader: bytes.NewReader(decryptedBuffer.Bytes()), FSTEntries: make([]FEntry, 0), EntryCount: 0, Entries: 0, NamesOffset: 0}
	if err := fst.Parse(); err != nil {
		return err
	}

	outputPath := path
	entry := make([]uint32, 0x10)
	lEntry := make([]uint32, 0x10)
	level := uint32(0)

	for i := uint32(0); i < fst.Entries-1; i++ {
		progressReporter.UpdateDecryptionProgress(float64(i) / float64(fst.Entries-1))
		if level > 0 {
			for (level >= 1) && (lEntry[level-1] == i+1) {
				level--
			}
		}

		if fst.FSTEntries[i].Type&1 != 0 {
			entry[level] = i
			lEntry[level] = fst.FSTEntries[i].Length
			level++
			if level >= MAX_LEVELS {
				return errors.New("level >= MAX_LEVELS")
			}
		} else {
			pathOffset := uint32(0)
			outputPath = path
			for j := uint32(0); j < level; j++ {
				pathOffset = fst.FSTEntries[entry[j]].NameOffset & 0x00FFFFFF
				fst.FSTReader.Seek(int64(fst.NamesOffset+pathOffset), io.SeekStart)
				directory, err := readString(fst.FSTReader)
				if err != nil {
					return fmt.Errorf("failed to read directory name: %w", err)
				}
				outputPath = filepath.Join(outputPath, directory)
				if err := os.MkdirAll(outputPath, 0755); err != nil {
					return fmt.Errorf("failed to create directory: %w", err)
				}
			}
			pathOffset = fst.FSTEntries[i].NameOffset & 0x00FFFFFF
			fst.FSTReader.Seek(int64(fst.NamesOffset+pathOffset), io.SeekStart)
			fileName, err := readString(fst.FSTReader)
			if err != nil {
				return fmt.Errorf("failed to read file name: %w", err)
			}
			outputPath = filepath.Clean(filepath.Join(outputPath, fileName))
			contentOffset := uint64(fst.FSTEntries[i].Offset)
			if fst.FSTEntries[i].Flags&4 == 0 {
				contentOffset <<= 5
			}
			if fst.FSTEntries[i].Type&0x80 == 0 {
				matchingContent := tmd.Contents[fst.FSTEntries[i].ContentID]
				tmdFlags := matchingContent.Type
				srcFile, err := os.Open(filepath.Join(path, matchingContent.CIDStr+".app"))
				if err != nil {
					return err
				}
				if tmdFlags&0x02 != 0 {
					err = extractFileHash(srcFile, 0, contentOffset, uint64(fst.FSTEntries[i].Length), outputPath, fst.FSTEntries[i].ContentID, cipherHashTree)
				} else {
					err = extractFile(srcFile, 0, contentOffset, uint64(fst.FSTEntries[i].Length), outputPath, fst.FSTEntries[i].ContentID, cipherHashTree)
				}
				srcFile.Close()
				if err != nil {
					return err
				}
			}
		}
	}

	progressReporter.UpdateDecryptionProgress(1.0)

	if deleteEncryptedContents {
		doDeleteEncryptedContents(path)
	}
	return nil
}
