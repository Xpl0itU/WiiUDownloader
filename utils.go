package wiiudownloader

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var removableEncryptedExtensions = []string{".app", ".h3"}

const (
	networkDialTimeout         = 30 * time.Second
	networkDialKeepAlive       = 30 * time.Second
	fallbackDNSDialTimeout     = 10 * time.Second
	fallbackDNSResolverEndpoint = "1.1.1.1:53"
	httpMaxIdleConns           = 100
	httpMaxIdleConnsPerHost    = 100
	httpMaxConnsPerHost        = 100
	httpIdleConnTimeout        = 90 * time.Second
	httpTLSHandshakeTimeout    = 10 * time.Second
	httpResponseHeaderTimeout  = 10 * time.Second
	httpExpectContinueTimeout  = 1 * time.Second
)

// BuildHTTPClient creates an HTTP client with fallback DNS dialer (to 1.1.1.1).
func BuildHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: networkDialTimeout, KeepAlive: networkDialKeepAlive}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err == nil {
					return conn, nil
				}
				if !strings.Contains(err.Error(), "no such host") && !strings.Contains(err.Error(), "lookup") {
					return nil, err
				}
				log.Printf("DNS lookup failed for %s, retrying with 1.1.1.1...", addr)
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
						d := net.Dialer{Timeout: fallbackDNSDialTimeout}
						return d.DialContext(ctx, "udp", fallbackDNSResolverEndpoint)
					},
				}
				host, port, splitErr := net.SplitHostPort(addr)
				if splitErr != nil {
					return nil, err
				}
				ips, lookupErr := resolver.LookupIPAddr(ctx, host)
				if lookupErr != nil {
					log.Printf("fallback DNS lookup failed: %v", lookupErr)
					return nil, err
				}
				if len(ips) == 0 {
					return nil, err
				}
				targetAddr := net.JoinHostPort(ips[0].String(), port)
				return dialer.DialContext(ctx, network, targetAddr)
			},
			MaxIdleConns:          httpMaxIdleConns,
			MaxIdleConnsPerHost:   httpMaxIdleConnsPerHost,
			MaxConnsPerHost:       httpMaxConnsPerHost,
			IdleConnTimeout:       httpIdleConnTimeout,
			TLSHandshakeTimeout:   httpTLSHandshakeTimeout,
			ResponseHeaderTimeout: httpResponseHeaderTimeout,
			ExpectContinueTimeout: httpExpectContinueTimeout,
		},
	}
}

// NormalizeFilename cleans title names for filesystem use.
func NormalizeFilename(filename string) string {
	var out strings.Builder
	shouldAppend := true
	firstChar := true

	for _, c := range filename {
		switch {
		case c == '_':
			if shouldAppend {
				out.WriteRune('_')
				shouldAppend = false
			}
			firstChar = false
		case c == ' ':
			if shouldAppend && !firstChar {
				out.WriteRune(' ')
				shouldAppend = false
			}
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
			out.WriteRune(c)
			shouldAppend = true
			firstChar = false
		}
	}

	result := out.String()
	if len(result) > 0 && result[len(result)-1] == '_' {
		result = result[:len(result)-1]
	}

	return result
}

// FormatBytes formats byte count to human-readable string (e.g. 1.25 GB).
func FormatBytes(bytes uint64) string {
	const unit = 1000

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	value := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	value = math.Round(value*100) / 100
	return fmt.Sprintf("%.2f %s", value, units[unitIndex])
}

// FormatDuration formats duration in human-readable time (e.g. 1m 05s).
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// FormatTitleFolderName creates the canonical folder name matching the GUI:
// "<Name> [<Kind>] [<TitleID>] [v<Version>]"
func FormatTitleFolderName(name string, titleID uint64, version int) string {
	cleanName := NormalizeFilename(name)
	if cleanName == "" {
		cleanName = fmt.Sprintf("%016x", titleID)
	}
	folder := fmt.Sprintf("%s [%s] [%016x]", cleanName, GetFormattedKind(titleID), titleID)
	if version >= 0 {
		folder = fmt.Sprintf("%s [v%d]", folder, version)
	}
	return folder
}

// ParseTitleID parses a 16-hex or 8-hex string into a uint64 TitleID.
func ParseTitleID(s string) (uint64, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(s), "0x")
	clean = strings.TrimPrefix(clean, "0X")
	if len(clean) == 16 {
		return strconv.ParseUint(clean, 16, 64)
	}
	if len(clean) == 8 {
		// Low ID: search in TitleDatabase if present
		low, err := strconv.ParseUint(clean, 16, 32)
		if err != nil {
			return 0, err
		}
		for _, entry := range TitleDatabase {
			if GetTitleIDLow(entry.TitleID) == uint32(low) && entry.Category == TITLE_CATEGORY_GAME {
				return entry.TitleID, nil
			}
		}
		// Default to game high ID
		return (uint64(TID_HIGH_GAME) << 32) | low, nil
	}
	return strconv.ParseUint(clean, 16, 64)
}

func doDeleteEncryptedContents(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if hasRemovableEncryptedExtension(name) ||
			name == "title.tmd" ||
			name == "title.tik" ||
			name == "title.cert" {
			if err := os.Remove(filepath.Join(path, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasRemovableEncryptedExtension(name string) bool {
	for _, ext := range removableEncryptedExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// FetchTitleTMD downloads and parses a title's TMD, which lists the contents a
// download would fetch. version < 0 selects the latest version.
func FetchTitleTMD(titleID uint64, version int, client *http.Client) (*TMD, error) {
	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%016x", titleID)
	tmdURL := fmt.Sprintf("%s/tmd", baseURL)
	if version >= 0 {
		tmdURL = fmt.Sprintf("%s/tmd.%d", baseURL, version)
	}

	resp, err := client.Get(tmdURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch TMD: status %d", resp.StatusCode)
	}

	tmdData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read TMD data: %w", err)
	}

	tmd, err := ParseTMD(tmdData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TMD: %w", err)
	}

	return tmd, nil
}

func FetchTMDSize(titleID uint64, version int, client *http.Client) (uint64, error) {
	tmd, err := FetchTitleTMD(titleID, version, client)
	if err != nil {
		return 0, err
	}
	return tmd.CalculateTotalSize(), nil
}
