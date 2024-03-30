package wiiudownloader

import (
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
	FSTFile      *os.File
	TotalEntries uint32
	NamesOffset  uint32
	FSTEntries   []FEntry
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

		//h0Hash := hashes[0x14*blockNumber : 0x14*blockNumber+sha1.Size]
		iv = hashes[0x14*blockNumber : 0x14*blockNumber+aes.BlockSize]

		cipher.NewCBCDecrypter(cipherHashTree, iv).CryptBlocks(decryptedContent, encryptedContent[HASHES_SIZE:])

		hash := sha1.Sum(decryptedContent[:HASH_BLOCK_SIZE])

		if blockNumber == 0 {
			hash[1] ^= byte(contentId)
		}

		// TODO: FIX THIS
		/*if !reflect.DeepEqual(hash[:], h0Hash) {
			fmt.Printf("\rBlock %v: hash (this is what we calculated): %x\n", blockNumber, hash)
			fmt.Printf("\rBlock %v: h0 (this is what the file has): %x\n", blockNumber, h0Hash)
			fmt.Printf("\rPath: %s\n", path)
			return errors.New("h0 hash mismatch")
		}*/

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

func extractFile(src *os.File, part_data_offset uint64, file_offset uint64, size uint64, path string, content_id uint16, cipherHashTree cipher.Block) error {
	enc := make([]byte, BLOCK_SIZE)
	dec := make([]byte, BLOCK_SIZE)
	iv := make([]byte, 16)

	roffset := file_offset / BLOCK_SIZE * BLOCK_SIZE
	soffset := file_offset - (file_offset / BLOCK_SIZE * BLOCK_SIZE)

	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create '%s': %w", path, err)
	}
	defer dst.Close()

	iv[1] = byte(content_id)

	write_size := BLOCK_SIZE
	if soffset+size > uint64(write_size) {
		write_size = write_size - int(soffset)
	}

	_, err = src.Seek(int64(part_data_offset+roffset), io.SeekStart)
	if err != nil {
		return err
	}

	for size > 0 {
		if uint64(write_size) > size {
			write_size = int(size)
		}

		if _, err := io.ReadFull(src, enc); err != nil {
			return fmt.Errorf("could not read %d bytes from '%s': %w", BLOCK_SIZE, path, err)
		}

		mode := cipher.NewCBCDecrypter(cipherHashTree, iv)
		mode.CryptBlocks(dec, enc)

		size -= uint64(write_size)

		_, err = dst.Write(dec[soffset : soffset+uint64(write_size)])
		if err != nil {
			return err
		}

		if soffset != 0 {
			write_size = BLOCK_SIZE
			soffset = 0
		}
	}

	return nil
}

func parseFSTEntry(fst *FSTData) error {
	for i := uint32(0); i < fst.TotalEntries-1; i++ {
		entry := FEntry{}
		entry.Type = readByte(fst.FSTFile)
		entry.NameOffset = uint32(read3BytesBE(fst.FSTFile))
		entry.Offset = readInt(fst.FSTFile, 4)
		entry.Length = readInt(fst.FSTFile, 4)
		entry.Flags = readInt16(fst.FSTFile, 2)
		entry.ContentID = readInt16(fst.FSTFile, 2)
		fst.FSTEntries = append(fst.FSTEntries, entry)
	}
	return nil
}

func parseFST(fst *FSTData) {
	fst.FSTFile.Seek(4, io.SeekStart)
	_ = readInt(fst.FSTFile, 4)
	exhCount := readInt(fst.FSTFile, 4)
	fst.FSTFile.Seek(int64(0x14+(32*exhCount)), io.SeekCurrent)
	fileEntriesOffset, _ := fst.FSTFile.Seek(0, io.SeekCurrent)
	fst.FSTFile.Seek(8, io.SeekCurrent)
	fst.TotalEntries = readInt(fst.FSTFile, 4)
	fst.FSTFile.Seek(4, io.SeekCurrent)
	fst.NamesOffset = uint32(fileEntriesOffset) + fst.TotalEntries*0x10
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

func readString(f *os.File) string {
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

func decryptContentToFile(encryptedFile *os.File, decryptedFile *os.File, cipherHashTree cipher.Block, content Content) error {
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
			fmt.Println("H3 Hash mismatch!")
			fmt.Println(" > TMD:    " + hex.EncodeToString(content.Hash))
			fmt.Println(" > Result: " + hex.EncodeToString(h3BytesSHASum[:]))
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
				fmt.Printf("\rData block hash invalid in chunk %v\n", chunkNum)
				return errors.New("data block hash invalid")
			}

			decryptedFile.Write(hashes)
			decryptedFile.Write(decryptedData)

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
			_, err = decryptedFile.Write(decryptedContent)
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
			print("Content Hash mismatch!")
			return errors.New("content hash mismatch")
		}
	}
	return nil
}

// TODO: Implement proper Logging
func DecryptContents(path string, progressReporter ProgressReporter, deleteEncryptedContents bool) error {
	tmdPath := filepath.Join(path, "title.tmd")
	if _, err := os.Stat(tmdPath); os.IsNotExist(err) {
		fmt.Println("No TMD (title.tmd) was found.")
		return err
	}

	// find title id and content id
	var titleID []byte
	var contentCount uint16
	tmd, err := os.Open(tmdPath)
	if err != nil {
		fmt.Println("Failed to open TMD:", err)
		return err
	}
	defer tmd.Close()

	tmd.Seek(0x18C, io.SeekStart)
	titleID = make([]byte, 8)
	if _, err := io.ReadFull(tmd, titleID); err != nil {
		fmt.Println("Failed to read title ID:", err)
		return err
	}

	tmd.Seek(0x1DE, io.SeekStart)
	if err := binary.Read(tmd, binary.BigEndian, &contentCount); err != nil {
		fmt.Println("Failed to read content count:", err)
		return err
	}

	tmd.Seek(0x204, io.SeekStart)
	tmdIndex := make([]byte, 2)
	if _, err := io.ReadFull(tmd, tmdIndex); err != nil {
		fmt.Println("Failed to read TMD index:", err)
		return err
	}

	contents := make([]Content, contentCount)

	for c := uint16(0); c < contentCount; c++ {
		offset := 2820 + (48 * c)
		tmd.Seek(int64(offset), io.SeekStart)
		if err := binary.Read(tmd, binary.BigEndian, &contents[c].ID); err != nil {
			return err
		}

		tmd.Seek(0xB08+(0x30*int64(c)), io.SeekStart)
		contents[c].Index = make([]byte, 2)
		if _, err := io.ReadFull(tmd, contents[c].Index); err != nil {
			fmt.Println("Failed to read content index:", err)
			return err
		}

		tmd.Seek(0xB0A+(0x30*int64(c)), io.SeekStart)
		if err := binary.Read(tmd, binary.BigEndian, &contents[c].Type); err != nil {
			fmt.Println("Failed to read content type:", err)
			return err
		}

		tmd.Seek(0xB0C+(0x30*int64(c)), io.SeekStart)
		if err := binary.Read(tmd, binary.BigEndian, &contents[c].Size); err != nil {
			fmt.Println("Failed to read content size:", err)
			return err
		}

		tmd.Seek(0xB14+(0x30*int64(c)), io.SeekStart)
		contents[c].Hash = make([]byte, 0x14)
		if _, err := io.ReadFull(tmd, contents[c].Hash); err != nil {
			fmt.Println("Failed to read content hash:", err)
			return err
		}

		contents[c].CIDStr = fmt.Sprintf("%08X", contents[c].ID)

		_, err := os.Stat(filepath.Join(path, contents[c].CIDStr+".app"))
		if err != nil {
			contents[c].CIDStr = fmt.Sprintf("%08x", contents[c].ID)
			_, err = os.Stat(filepath.Join(path, contents[c].CIDStr+".app"))
			if err != nil {
				fmt.Println("Failed to find encrypted content:", err)
				return err
			}
		}
	}
	fmt.Printf("Title ID: %s\n", hex.EncodeToString(titleID))

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
	fmt.Printf("Encrypted Titlekey: %s\n", hex.EncodeToString(encryptedTitleKey))
	c, err := aes.NewCipher(commonKey)
	if err != nil {
		return err
	}

	cbc := cipher.NewCBCDecrypter(c, append(titleID, make([]byte, 8)...))

	decryptedTitleKey := make([]byte, len(encryptedTitleKey))
	cbc.CryptBlocks(decryptedTitleKey, encryptedTitleKey)

	cipherHashTree, err := aes.NewCipher(decryptedTitleKey)
	if err != nil {
		return fmt.Errorf("failed to create AES cipher: %w", err)
	}

	fmt.Printf("Decrypted Titlekey: %x\n", decryptedTitleKey)

	fstEncFile, err := os.Open(filepath.Join(path, contents[0].CIDStr+".app"))
	if err != nil {
		fmt.Println("Failed to open encrypted FST file:", err)
		return err
	}
	defer fstEncFile.Close()

	fstDecFile, err := os.Create(filepath.Join(path, contents[0].CIDStr+".app.dec"))
	if err != nil {
		fmt.Println("Failed to create decrypted content file:", err)
		return err
	}
	defer fstDecFile.Close()

	if err := decryptContentToFile(fstEncFile, fstDecFile, cipherHashTree, contents[0]); err != nil {
		fmt.Println("Failed to decrypt content file:", err)
		return err
	}

	fstDecFile.Seek(0, io.SeekStart)
	fst := FSTData{FSTFile: fstDecFile, FSTEntries: make([]FEntry, 0), TotalEntries: 0, NamesOffset: 0}
	parseFST(&fst)
	fmt.Printf("FST Entry count: %v\n", fst.TotalEntries)

	outputPath := path
	entry := make([]uint32, 0x10)
	lEntry := make([]uint32, 0x10)
	level := uint32(0)

	for i := uint32(1); i < fst.TotalEntries; i++ {
		if level > 0 {
			for (level >= 1) && (lEntry[level-1] == i) {
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
				fst.FSTFile.Seek(int64(fst.NamesOffset+pathOffset), io.SeekStart)
				outputPath = filepath.Join(outputPath, readString(fst.FSTFile))
				os.MkdirAll(outputPath, 0755)
			}
			pathOffset = fst.FSTEntries[i].NameOffset & 0x00FFFFFF
			fst.FSTFile.Seek(int64(fst.NamesOffset+pathOffset), io.SeekStart)
			outputPath = filepath.Join(outputPath, readString(fst.FSTFile))
			contentOffset := uint64(fst.FSTEntries[i].Offset)
			if fst.FSTEntries[i].Flags&4 == 0 {
				contentOffset <<= 5
			}
			if fst.FSTEntries[i].Type&0x80 == 0 {
				matchingContent := contents[fst.FSTEntries[i].ContentID]
				tmdFlags := matchingContent.Type
				srcFile, err := os.Open(filepath.Join(path, matchingContent.CIDStr+".app"))
				if err != nil {
					return err
				}
				defer srcFile.Close()
				if tmdFlags&2 != 0 {
					err = extractFileHash(srcFile, contentOffset, uint64(fst.FSTEntries[i].Offset), uint64(fst.FSTEntries[i].Length), outputPath, fst.FSTEntries[i].ContentID, cipherHashTree)
				} else {
					err = extractFile(srcFile, contentOffset, uint64(fst.FSTEntries[i].Offset), uint64(fst.FSTEntries[i].Length), outputPath, fst.FSTEntries[i].ContentID, cipherHashTree)
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
