package wiiudownloader

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"

	"golang.org/x/crypto/pbkdf2"
)

const (
	KEYGEN_SECRET = "fd040105060b111c2d49"
)

var (
	keygen_pw = []byte{0x6d, 0x79, 0x70, 0x61, 0x73, 0x73}
)

func encryptAES(data []byte, key []byte, iv []byte) ([]byte, error) {
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

func generateKey(tid string) ([]byte, error) {
	tmp := []byte(tid)
	for tmp[0] == '0' && tmp[1] == '0' {
		tmp = tmp[2:]
	}

	h := []byte(KEYGEN_SECRET + string(tmp))

	bhl := len(h) >> 1
	bh := make([]byte, bhl)
	for i, j := 0, 0; j < bhl; i += 2 {
		bh[j] = byte((h[i]%32+9)%25*16 + (h[i+1]%32+9)%25)
		j++
	}

	md5sum := md5.Sum(bh)

	key := pbkdf2WithSHA1(keygen_pw, md5sum[:], 20, 16)

	iv := make([]byte, 16)
	for i, j := 0, 0; j < 8; i += 2 {
		iv[j] = byte((tid[i]%32+9)%25*16 + (tid[i+1]%32+9)%25)
		j++
	}
	copy(iv[8:], make([]byte, 8))

	encrypted, err := encryptAES(key, commonKey, iv)
	if err != nil {
		return []byte{}, err
	}

	return encrypted, nil
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
