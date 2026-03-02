package wiiudownloader

import (
	"fmt"
	"net/http"
	"os"
	"path"
)

var cetkData []byte

const (
	// Embedded certificate chain starts at this range inside the downloaded cetk.
	CETK_CERT_START_OFFSET = 0x350
	CETK_CERT_SIZE         = 0x300
)

func getDefaultCert(progressReporter ProgressReporter, client *http.Client) ([]byte, error) {
	if hasCetkCertData(cetkData) {
		return cetkData[CETK_CERT_START_OFFSET : CETK_CERT_START_OFFSET+CETK_CERT_SIZE], nil
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

	if hasCetkCertData(cetkData) {
		return cetkData[CETK_CERT_START_OFFSET : CETK_CERT_START_OFFSET+CETK_CERT_SIZE], nil
	}
	return nil, fmt.Errorf("failed to download OSv10 cetk, length: %d", len(cetkData))
}

func GenerateCert(tmd *TMD, outputPath string, progressReporter ProgressReporter, client *http.Client) error {
	cert, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer cert.Close()

	if _, err := cert.Write(tmd.Certificate1); err != nil {
		return err
	}

	if _, err := cert.Write(tmd.Certificate2); err != nil {
		return err
	}

	defaultCert, err := getDefaultCert(progressReporter, client)
	if err != nil {
		return err
	}

	if _, err := cert.Write(defaultCert); err != nil {
		return err
	}
	return nil
}

func hasCetkCertData(data []byte) bool {
	return len(data) >= CETK_CERT_START_OFFSET+CETK_CERT_SIZE
}
