package wiiudownloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	encryptedContent := make([]byte, BLOCK_SIZE_HASHED)
	decryptedContent := make([]byte, BLOCK_SIZE_HASHED)
	hashes := make([]byte, HASHES_SIZE)

	writeSize := HASH_BLOCK_SIZE
	blockNumber := (fileOffset / HASH_BLOCK_SIZE) & 0x0F

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()

	roffset := fileOffset / HASH_BLOCK_SIZE * BLOCK_SIZE_HASHED
	soffset := fileOffset - (fileOffset / HASH_BLOCK_SIZE * HASH_BLOCK_SIZE)

	if soffset+size > uint64(writeSize) {
		writeSize = writeSize - int(soffset)
	}

	_, err = src.Seek(int64(partDataOffset+roffset), io.SeekStart)
	if err != nil {
		return err
	}

	for size > 0 {
		if uint64(writeSize) > size {
			writeSize = int(size)
		}

		if _, err := io.ReadFull(src, encryptedContent); err != nil {
			return fmt.Errorf("could not read %d bytes from '%s': %w", BLOCK_SIZE_HASHED, path, err)
		}

		iv := make([]byte, aes.BlockSize)
		iv[1] = byte(contentId)
		cipher.NewCBCDecrypter(cipherHashTree, iv).CryptBlocks(hashes, encryptedContent[:HASHES_SIZE])

		h0Hash := hashes[0x14*blockNumber : 0x14*blockNumber+sha1.Size]
		iv = hashes[0x14*blockNumber : 0x14*blockNumber+aes.BlockSize]

		if blockNumber == 0 {
			iv[1] ^= byte(contentId)
		}

		cipher.NewCBCDecrypter(cipherHashTree, iv).CryptBlocks(decryptedContent, encryptedContent[HASHES_SIZE:])

		hash := sha1.Sum(decryptedContent[:HASH_BLOCK_SIZE])

		if !reflect.DeepEqual(hash[:], h0Hash) {
			return errors.New("h0 hash mismatch")
		}

		size -= uint64(writeSize)

		_, err = dst.Write(decryptedContent[soffset : soffset+uint64(writeSize)])
		if err != nil {
			return err
		}

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
	encryptedContent := make([]byte, BLOCK_SIZE)
	decryptedContent := make([]byte, BLOCK_SIZE)

	writeSize := BLOCK_SIZE

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()

	roffset := fileOffset / BLOCK_SIZE * BLOCK_SIZE
	soffset := fileOffset - (fileOffset / BLOCK_SIZE * BLOCK_SIZE)

	if soffset+size > uint64(writeSize) {
		writeSize = writeSize - int(soffset)
	}

	_, err = src.Seek(int64(partDataOffset+roffset), io.SeekStart)
	if err != nil {
		return err
	}

	iv := make([]byte, aes.BlockSize)
	iv[1] = byte(contentId)

	aesCipher := cipher.NewCBCDecrypter(cipherHashTree, iv)

	for size > 0 {
		if uint64(writeSize) > size {
			writeSize = int(size)
		}

		if n, err := io.ReadFull(src, encryptedContent); err != nil && n != BLOCK_SIZE {
			return fmt.Errorf("could not read %d bytes from '%s': %w", BLOCK_SIZE, path, err)
		}

		aesCipher.CryptBlocks(decryptedContent, encryptedContent)

		n, err := dst.Write(decryptedContent[soffset : soffset+uint64(writeSize)])
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

func parseFSTEntry(fst *FSTData) error {
	for i := uint32(0); i < fst.Entries; i++ {
		entry := FEntry{}
		entry.Type = readByte(fst.FSTReader)
		entry.NameOffset = uint32(read3BytesBE(fst.FSTReader))
		entry.Offset = readInt(fst.FSTReader, 4)
		entry.Length = readInt(fst.FSTReader, 4)
		entry.Flags = readInt16(fst.FSTReader, 2)
		entry.ContentID = readInt16(fst.FSTReader, 2)
		fst.FSTEntries = append(fst.FSTEntries, entry)
	}
	return nil
}

func parseFST(fst *FSTData) {
	fst.FSTReader.Seek(0x8, io.SeekStart)
	fst.EntryCount = readInt(fst.FSTReader, 4)
	fst.FSTReader.Seek(int64(0x20+fst.EntryCount*0x20+8), io.SeekStart)
	fst.Entries = readInt(fst.FSTReader, 4)
	fst.NamesOffset = 0x20 + fst.EntryCount*0x20 + fst.Entries*0x10
	fst.FSTReader.Seek(4, io.SeekCurrent)
	parseFSTEntry(fst)
}

func readByte(f io.ReadSeeker) byte {
	buf := make([]byte, 1)
	n, err := f.Read(buf)
	if err != nil {
		panic(err)
	}
	if n < 1 {
		panic(io.ErrUnexpectedEOF)
	}
	return buf[0]
}

func readInt(f io.ReadSeeker, s int) uint32 {
	bufSize := 4 // Buffer size is always 4 for uint32
	buf := make([]byte, bufSize)

	n, err := f.Read(buf[:s])
	if err != nil {
		panic(err)
	}

	if n < s {
		// If we didn't read the expected number of bytes, seek back to the
		// previous position in the file and return an error.
		if _, err := f.Seek(int64(-n), io.SeekCurrent); err != nil {
			panic(err)
		}
		panic(io.ErrUnexpectedEOF)
	}

	return binary.BigEndian.Uint32(buf)
}

func readInt16(f io.ReadSeeker, s int) uint16 {
	bufSize := 2 // Buffer size is always 2 for uint16
	buf := make([]byte, bufSize)

	n, err := f.Read(buf[:s])
	if err != nil {
		panic(err)
	}

	if n < s {
		// If we didn't read the expected number of bytes, seek back to the
		// previous position in the file and return an error.
		if _, err := f.Seek(int64(-n), io.SeekCurrent); err != nil {
			panic(err)
		}
		panic(io.ErrUnexpectedEOF)
	}

	return binary.BigEndian.Uint16(buf)
}

func readString(f io.ReadSeeker) string {
	buf := []byte{}
	for {
		char := make([]byte, 1)
		f.Read(char)
		if char[0] == byte(0) || len(char) == 0 {
			return string(buf)
		}
		buf = append(buf, char[0])
	}
}

func read3BytesBE(f io.ReadSeeker) int {
	b := make([]byte, 3)
	f.Read(b)
	return int(uint(b[2]) | uint(b[1])<<8 | uint(b[0])<<16)
}

func decryptContentToBuffer(encryptedFile *os.File, decryptedBuffer *bytes.Buffer, cipherHashTree cipher.Block, content Content) error {
	hasHashTree := content.Type&2 != 0
	encryptedStat, err := encryptedFile.Stat()
	if err != nil {
		return err
	}
	encryptedSize := encryptedStat.Size()
	path := filepath.Dir(encryptedFile.Name())

	if hasHashTree { // if has a hash tree
		chunkCount := encryptedSize / 0x10000
		h3Data, err := os.ReadFile(filepath.Join(path, fmt.Sprintf("%s.h3", content.CIDStr)))
		if err != nil {
			return err
		}
		h3BytesSHASum := sha1.Sum(h3Data)
		if hex.EncodeToString(h3BytesSHASum[:]) != hex.EncodeToString(content.Hash) {
			return errors.New("H3 Hash mismatch")
		}

		h0HashNum := int64(0)
		h1HashNum := int64(0)
		h2HashNum := int64(0)
		h3HashNum := int64(0)

		hashes := make([]byte, 0x400)
		buffer := make([]byte, 0x400)

		for chunkNum := int64(0); chunkNum < chunkCount; chunkNum++ {
			encryptedFile.Read(buffer)
			cipher.NewCBCDecrypter(cipherHashTree, make([]byte, aes.BlockSize)).CryptBlocks(hashes, buffer)

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

			if !reflect.DeepEqual(h0HashesHash[:], h1Hash) {
				return errors.New("h0 Hashes Hash mismatch")
			}
			if !reflect.DeepEqual(h1HashesHash[:], h2Hash) {
				return errors.New("h1 Hashes Hash mismatch")
			}
			if !reflect.DeepEqual(h2HashesHash[:], h3Hash) {
				return errors.New("h2 Hashes Hash mismatch")
			}

			decryptedData := make([]byte, 0xFC00)
			encryptedFile.Read(decryptedData)

			cipher.NewCBCDecrypter(cipherHashTree, h0Hash[:16]).CryptBlocks(decryptedData, decryptedData)
			decryptedDataHash := sha1.Sum(decryptedData)

			if !reflect.DeepEqual(decryptedDataHash[:], h0Hash) {
				return errors.New("data block hash invalid")
			}

			_, err = decryptedBuffer.Write(hashes)
			if err != nil {
				return err
			}
			_, err = decryptedBuffer.Write(decryptedData)
			if err != nil {
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
		cipherContent := cipher.NewCBCDecrypter(cipherHashTree, append(content.Index, make([]byte, 14)...))
		contentHash := sha1.New()
		left := content.Size
		leftHash := content.Size

		for i := 0; i <= int(content.Size/READ_SIZE)+1; i++ {
			toRead := min(READ_SIZE, left)
			toReadHash := min(READ_SIZE, leftHash)

			encryptedContent := make([]byte, toRead)
			_, err = io.ReadFull(encryptedFile, encryptedContent)
			if err != nil {
				return err
			}

			decryptedContent := make([]byte, len(encryptedContent))
			cipherContent.CryptBlocks(decryptedContent, encryptedContent)
			contentHash.Write(decryptedContent[:toReadHash])
			_, err = decryptedBuffer.Write(decryptedContent)
			if err != nil {
				return err
			}

			left -= toRead
			leftHash -= toRead

			if left == 0 {
				break
			}
		}
		if !reflect.DeepEqual(content.Hash, contentHash.Sum(nil)) {
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
	tmd, err := parseTMD(tmdData)
	if err != nil {
		return err
	}

	// Check if all contents are present and how they are named
	for i := 0; i < len(tmd.Contents); i++ {
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
			cetk.Seek(0x1BF, 0)
			encryptedTitleKey = make([]byte, 0x10)
			cetk.Read(encryptedTitleKey)
			cetk.Close()
		}
	}
	c, err := aes.NewCipher(commonKey)
	if err != nil {
		return err
	}

	titleIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(titleIDBytes, tmd.TitleID)
	cbc := cipher.NewCBCDecrypter(c, append(titleIDBytes, make([]byte, 8)...))

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
	defer fstEncFile.Close()

	decryptedBuffer := bytes.Buffer{}
	if err := decryptContentToBuffer(fstEncFile, &decryptedBuffer, cipherHashTree, tmd.Contents[0]); err != nil {
		return err
	}
	fst := FSTData{FSTReader: bytes.NewReader(bytes.Clone(decryptedBuffer.Bytes())), FSTEntries: make([]FEntry, 0), EntryCount: 0, Entries: 0, NamesOffset: 0}
	parseFST(&fst)

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
				outputPath = filepath.Join(outputPath, readString(fst.FSTReader))
				os.MkdirAll(outputPath, 0755)
			}
			pathOffset = fst.FSTEntries[i].NameOffset & 0x00FFFFFF
			fst.FSTReader.Seek(int64(fst.NamesOffset+pathOffset), io.SeekStart)
			outputPath = filepath.Join(outputPath, readString(fst.FSTReader))
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
				defer srcFile.Close()
				if tmdFlags&0x02 != 0 {
					err = extractFileHash(srcFile, 0, contentOffset, uint64(fst.FSTEntries[i].Length), outputPath, fst.FSTEntries[i].ContentID, cipherHashTree)
				} else {
					err = extractFile(srcFile, 0, contentOffset, uint64(fst.FSTEntries[i].Length), outputPath, fst.FSTEntries[i].ContentID, cipherHashTree)
				}
				if err != nil {
					return err
				}
			}
		}
	}
	if deleteEncryptedContents {
		doDeleteEncryptedContents(path)
	}
	return nil
}
