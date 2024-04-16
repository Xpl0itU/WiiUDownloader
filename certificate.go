package wiiudownloader

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path"
)

var cetkData []byte

func getDefaultCert(progressReporter ProgressReporter, client *http.Client) ([]byte, error) {
	if len(cetkData) >= 0x350+0x300 {
		return cetkData[0x350 : 0x350+0x300], nil
	}
	cetkDir := path.Join(os.TempDir(), "cetk")
	if err := downloadFile(progressReporter, client, "http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/000500101000400a/cetk", cetkDir, true); err != nil {
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

func GenerateCert(tmd *TMD, progressReporter ProgressReporter, client *http.Client) (bytes.Buffer, error) {
	cert := bytes.Buffer{}

	if _, err := cert.Write(tmd.Certificate1); err != nil {
		return bytes.Buffer{}, err
	}

	if _, err := cert.Write(tmd.Certificate2); err != nil {
		return bytes.Buffer{}, err
	}

	defaultCert, err := getDefaultCert(progressReporter, client)
	if err != nil {
		return bytes.Buffer{}, err
	}
	cert.Write(defaultCert)
	return cert, nil
}
