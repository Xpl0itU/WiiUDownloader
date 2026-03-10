package wiiudownloader

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	KEYGEN_SECRET = "fd040105060b111c2d49"
)

var (
	keygen_pw = []byte{0x6d, 0x79, 0x70, 0x61, 0x73, 0x73}
)

var titleKeyPasswords = map[uint8][]byte{
	TITLE_KEY_mypass:     []byte("mypass"),
	TITLE_KEY_nintendo:   []byte("nintendo"),
	TITLE_KEY_test:       []byte("test"),
	TITLE_KEY_1234567890: []byte("1234567890"),
	TITLE_KEY_Lucy131211: []byte("Lucy131211"),
	TITLE_KEY_fbf10:      []byte("fbf10"),
	TITLE_KEY_5678:       []byte("5678"),
	TITLE_KEY_1234:       []byte("1234"),
	TITLE_KEY_:           []byte(""),
	TITLE_KEY_MAGIC:      []byte("MAGIC"),
}

func encryptAES(data, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	paddedData := PKCS7Padding(data, aes.BlockSize)
	encrypted := make([]byte, len(paddedData))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encrypted, paddedData)

	return encrypted, nil
}

func GenerateKey(tid string) ([]byte, error) {
	return GenerateKeyWithType(tid, TITLE_KEY_mypass)
}

func GenerateKeyWithType(tid string, keyType uint8) ([]byte, error) {
	if err := validateTitleIDHex(tid); err != nil {
		return nil, err
	}
	tmp := trimLeadingZeroPairs([]byte(tid))

	h := []byte(KEYGEN_SECRET + string(tmp))

	bhl := len(h) >> 1
	bh := make([]byte, bhl)
	for i, j := 0, 0; j < bhl; i += 2 {
		bh[j] = byte((h[i]%32+9)%25*16 + (h[i+1]%32+9)%25)
		j++
	}

	md5sum := md5.Sum(bh)

	password, ok := titleKeyPasswords[keyType]
	if !ok {
		password = keygen_pw
	}
	key := pbkdf2WithSHA1(password, md5sum[:], 20, 16)

	iv := make([]byte, 16)
	for i, j := 0, 0; j < 8; i += 2 {
		iv[j] = byte((tid[i]%32+9)%25*16 + (tid[i+1]%32+9)%25)
		j++
	}
	copy(iv[8:], make([]byte, 8))

	encrypted, err := encryptAES(key, wiiUCommonKey, iv)
	if err != nil {
		return nil, err
	}

	return encrypted, nil
}

func validateTitleIDHex(tid string) error {
	if len(tid) != 16 {
		return fmt.Errorf("invalid title ID length: got %d, want 16", len(tid))
	}
	decoded := make([]byte, 8)
	if _, err := hex.Decode(decoded, []byte(tid)); err != nil {
		return fmt.Errorf("invalid title ID hex: %w", err)
	}
	if len(decoded) != 8 {
		return errors.New("invalid title ID bytes")
	}
	return nil
}

func trimLeadingZeroPairs(data []byte) []byte {
	for len(data) >= 2 && data[0] == '0' && data[1] == '0' {
		data = data[2:]
	}
	return data
}

func pbkdf2WithSHA1(password, salt []byte, iterations, keyLength int) []byte {
	return pbkdf2.Key(password, salt, iterations, keyLength, sha1.New)
}

func PKCS7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	paddedData := make([]byte, len(data)+padding)
	copy(paddedData, data)
	for i := len(data); i < len(paddedData); i++ {
		paddedData[i] = byte(padding)
	}
	return paddedData
}
