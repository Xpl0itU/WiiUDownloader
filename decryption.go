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
	"strings"
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

type FSTData struct {
	FSTFile      *os.File
	TotalEntries uint32
	NamesOffset  uint32
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

func fileChunkOffset(offset uint32) uint32 {
	chunks := (offset / 0xFC00)
	singleChunkOffset := offset % 0xFC00
	return singleChunkOffset + ((chunks + 1) * 0x400) + (chunks * 0xFC00)
}

func iterateDirectory(f *os.File, iterStart uint32, count uint32, namesOffset int64, depth uint32, topdir int32, contentRecords []Content, tree []string) error {
	i := iterStart
	for i < count {
		fType := make([]byte, 1)
		f.Read(fType)
		isDir := fType[0] & 1

		nameOffset := int64(read3BytesBE(f)) + namesOffset

		origOffset, _ := f.Seek(0, io.SeekCurrent)
		f.Seek(int64(nameOffset), io.SeekStart)
		fName := readString(f)
		f.Seek(origOffset, io.SeekStart)

		fOffset := readInt(f, 4)
		fSize := readInt(f, 4)
		fFlags := readInt16(f, 2)
		if fFlags&4 == 0 {
			fOffset <<= 5
		}

		contentIndex := readInt16(f, 2)

		// this should be based on fFlags, but I'm not sure if there is a reliable way to determine this yet.
		hasHashTree := contentRecords[contentIndex].Type & 2

		fRealOffset := fileChunkOffset(fOffset)
		if hasHashTree == 0 {
			fRealOffset = fOffset
		}

		if isDir != 0 {
			if int32(fOffset) <= topdir {
				return errors.New("invalid directory offset")
			}
			tree = append(tree, fName+"/")
			os.MkdirAll(strings.Join(tree, ""), 0755)
			iterateDirectory(f, i+1, fSize, namesOffset, depth+1, int32(fOffset), contentRecords, tree)
			tree = tree[:len(tree)-1]
			i = fSize - 1
		} else {
			withC, err := os.Open(contentRecords[contentIndex].CIDStr + ".app.dec")
			if err != nil {
				return err
			}
			defer withC.Close()
			withO, err := os.Create(strings.Join(tree, "") + fName)
			if err != nil {
				return err
			}
			defer withO.Close()

			_, err = withC.Seek(int64(fRealOffset), 0)
			if err != nil {
				return err
			}

			buf := []byte{}
			left := fSize

			for left > 0 {
				toRead := min(0x20, int(left))
				readBuf := make([]byte, toRead)
				_, err = withC.Read(readBuf)
				if err != nil && err != io.EOF {
					return err
				}
				buf = append(buf, readBuf...)
				left -= uint32(toRead)

				if len(buf) >= 0x200 {
					_, err = withO.Write(buf)
					if err != nil {
						return err
					}
					buf = []byte{}
				}

				withCOffset, _ := withC.Seek(0, io.SeekCurrent)

				if hasHashTree != 0 && withCOffset%0x10000 < 0x400 {
					_, err = withC.Seek(0x400, 1)
					if err != nil {
						return err
					}
				}
			}

			if len(buf) > 0 {
				_, err = withO.Write(buf)
				if err != nil {
					return err
				}
			}
		}
		i++
	}
	return nil
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

		decryptedContent := make([]byte, 0x400)
		buffer := make([]byte, 0x400)

		for chunkNum := int64(0); chunkNum < chunkCount; chunkNum++ {
			encryptedFile.Read(buffer)
			cipher.NewCBCDecrypter(cipherHashTree, make([]byte, aes.BlockSize)).CryptBlocks(decryptedContent, buffer)

			h0Hashes := decryptedContent[0:0x140]
			h1Hashes := decryptedContent[0x140:0x280]
			h2Hashes := decryptedContent[0x280:0x3c0]

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

			decryptedFile.Write(decryptedContent)
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

// TODO: Implement Logging, add extraction at the same time of decryption
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
	fst := FSTData{nil, 0, 0}
	for i, c := range contents {
		c.CIDStr = fmt.Sprintf("%08X", c.ID)
		fmt.Printf("Decrypting %v...\n", c.CIDStr)

		encryptedFile, err := os.Open(filepath.Join(path, c.CIDStr+".app"))
		if err != nil {
			c.CIDStr = fmt.Sprintf("%08x", c.ID)
			encryptedFile, err = os.Open(filepath.Join(path, c.CIDStr+".app"))
			if err != nil {
				fmt.Println("Failed to find and open encrypted content:", err)
				return err
			}
		}
		defer encryptedFile.Close()

		decryptedFile, err := os.Create(filepath.Join(path, c.CIDStr+".app.dec"))
		if err != nil {
			fmt.Println("Failed to create decrypted content file:", err)
			return err
		}
		defer decryptedFile.Close()

		if err := decryptContentToFile(encryptedFile, decryptedFile, cipherHashTree, c); err != nil {
			fmt.Println("Failed to decrypt content file:", err)
			return err
		}

		if i == 0 {
			fst.FSTFile = decryptedFile
			parseFST(&fst)
		}
	}
	return iterateDirectory(fst.FSTFile, 1, uint32(len(contents)), int64(fst.NamesOffset), 0, -1, contents, []string{})
}
