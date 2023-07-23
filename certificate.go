package wiiudownloader

import (
	"bytes"
	"fmt"
	"os"
	"path"

	"github.com/cavaliergopher/grab/v3"
)

var cetkData []byte

func getCert(tmdData []byte, id int, numContents uint16) ([]byte, error) {
	var certSlice []byte
	if len(tmdData) == int((0x0B04+0x30*numContents+0xA00)-0x300) {
		certSlice = tmdData[0x0B04+0x30*numContents : 0x0B04+0x30*numContents+0xA00-0x300]
	} else {
		certSlice = tmdData[0x0B04+0x30*numContents : 0x0B04+0x30*numContents+0xA00]
	}
	switch id {
	case 0:
		return certSlice[:0x400], nil
	case 1:
		return certSlice[0x400 : 0x400+0x300], nil
	default:
		return nil, fmt.Errorf("invalid id: %d", id)
	}
}

func getDefaultCert(progressWindow *ProgressWindow, client *grab.Client) ([]byte, error) {
	if len(cetkData) >= 0x350+0x300 {
		return cetkData[0x350 : 0x350+0x300], nil
	}
	cetkDir := path.Join(os.TempDir(), "cetk")
	if err := downloadFile(progressWindow, client, "http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/000500101000400a/cetk", cetkDir, true); err != nil {
		return nil, err
	}
	cetkData, err := os.ReadFile(cetkDir)
	if err != nil {
		return nil, err
	}

	if err := os.Remove(cetkDir); err != nil {
		return nil, err
	}

	if len(cetkData) >= 0x350+0x300 {
		return cetkData[0x350 : 0x350+0x300], nil
	}
	return nil, fmt.Errorf("failed to download OSv10 cetk, length: %d", len(cetkData))
}

func GenerateCert(tmdData []byte, contentCount uint16, progressWindow *ProgressWindow, client *grab.Client) (bytes.Buffer, error) {
	cert := bytes.Buffer{}

	cert0, err := getCert(tmdData, 0, contentCount)
	if err != nil {
		return bytes.Buffer{}, err
	}
	cert.Write(cert0)

	cert1, err := getCert(tmdData, 1, contentCount)
	if err != nil {
		return bytes.Buffer{}, err
	}
	cert.Write(cert1)

	defaultCert, err := getDefaultCert(progressWindow, client)
	if err != nil {
		return bytes.Buffer{}, err
	}
	cert.Write(defaultCert)
	return cert, nil
}
