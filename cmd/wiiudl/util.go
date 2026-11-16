package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

const (
	cliName    = "wiiudl"
	cliVersion = "1.0.0"

	userAgent = "WiiUDownloader"

	networkDialTimeout         = 30 * time.Second
	networkDialKeepAlive       = 30 * time.Second
	fallbackDNSDialTimeout     = 10 * time.Second
	httpMaxIdleConns           = 100
	httpMaxIdleConnsPerHost    = 100
	httpMaxConnsPerHost        = 100
	httpIdleConnTimeout        = 90 * time.Second
	httpTLSHandshakeTimeout    = 10 * time.Second
	httpResponseHeaderTimeout  = 10 * time.Second
	httpExpectContinueTimeout  = 1 * time.Second
	fallbackDNSResolverAddress = "1.1.1.1:53"

	// Same default visibility as the GUI: Japan, USA and Europe. `--region all`
	// additionally includes the China/Korea/Taiwan bits.
	defaultRegionMask = wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_EUROPE
	allRegionMask     = wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_EUROPE |
		wiiudownloader.MCP_REGION_CHINA | wiiudownloader.MCP_REGION_KOREA | wiiudownloader.MCP_REGION_TAIWAN
)

// buildHTTPClient mirrors the GUI's transport: a long-lived pool plus a
// public-DNS fallback for machines whose resolver cannot resolve the CDN.
func buildHTTPClient() *http.Client {
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
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
						d := net.Dialer{Timeout: fallbackDNSDialTimeout}
						return d.DialContext(ctx, "udp", fallbackDNSResolverAddress)
					},
				}
				host, port, splitErr := net.SplitHostPort(addr)
				if splitErr != nil {
					return nil, err
				}
				ips, lookupErr := resolver.LookupIPAddr(ctx, host)
				if lookupErr != nil || len(ips) == 0 {
					return nil, err
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
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

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, units[index])
	}
	return fmt.Sprintf("%.2f %s", value, units[index])
}

func formatRate(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return ""
	}
	return formatBytes(int64(math.Round(bytesPerSecond))) + "/s"
}

func formatDuration(d time.Duration) string {
	if d < 0 || math.IsInf(float64(d), 0) || math.IsNaN(float64(d)) {
		return ""
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// normalizeFilename matches the GUI's output folder naming.
func normalizeFilename(filename string) string {
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
	if strings.HasSuffix(result, "_") {
		result = result[:len(result)-1]
	}
	return result
}

func titleDirName(entry wiiudownloader.TitleEntry, version int) string {
	tid := fmt.Sprintf("%016x", entry.TitleID)
	parts := make([]string, 0, 3)
	if name := normalizeFilename(entry.Name); name != "" {
		parts = append(parts, name)
	}
	parts = append(parts, "["+wiiudownloader.GetFormattedKind(entry.TitleID)+"]", "["+tid+"]")
	name := strings.Join(parts, " ")
	if version >= 0 {
		name += fmt.Sprintf(" [v%d]", version)
	}
	return name
}

func formatRegion(region uint8) string {
	regions := make([]string, 0, 3)
	if region&wiiudownloader.MCP_REGION_USA != 0 {
		regions = append(regions, "USA")
	}
	if region&wiiudownloader.MCP_REGION_EUROPE != 0 {
		regions = append(regions, "Europe")
	}
	if region&wiiudownloader.MCP_REGION_JAPAN != 0 {
		regions = append(regions, "Japan")
	}
	if region&wiiudownloader.MCP_REGION_CHINA != 0 {
		regions = append(regions, "China")
	}
	if region&wiiudownloader.MCP_REGION_KOREA != 0 {
		regions = append(regions, "Korea")
	}
	if region&wiiudownloader.MCP_REGION_TAIWAN != 0 {
		regions = append(regions, "Taiwan")
	}
	if len(regions) == 0 {
		return "Unknown"
	}
	return strings.Join(regions, "/")
}

// categoryOf derives the content category from the title ID, falling back to
// the database's category for IDs the core library does not classify.
func categoryOf(entry wiiudownloader.TitleEntry) uint8 {
	switch entry.TitleID >> 32 {
	case wiiudownloader.TID_HIGH_GAME:
		return wiiudownloader.TITLE_CATEGORY_GAME
	case wiiudownloader.TID_HIGH_DEMO:
		return wiiudownloader.TITLE_CATEGORY_DEMO
	case wiiudownloader.TID_HIGH_UPDATE:
		return wiiudownloader.TITLE_CATEGORY_UPDATE
	case wiiudownloader.TID_HIGH_DLC:
		return wiiudownloader.TITLE_CATEGORY_DLC
	default:
		return entry.Category
	}
}

func categoryName(category uint8) string {
	switch category {
	case wiiudownloader.TITLE_CATEGORY_GAME:
		return "Game"
	case wiiudownloader.TITLE_CATEGORY_UPDATE:
		return "Update"
	case wiiudownloader.TITLE_CATEGORY_DLC:
		return "DLC"
	case wiiudownloader.TITLE_CATEGORY_DEMO:
		return "Demo"
	case wiiudownloader.TITLE_CATEGORY_ALL:
		return "All"
	default:
		return "Unknown"
	}
}

func parseCategory(s string) (uint8, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all", "any":
		return wiiudownloader.TITLE_CATEGORY_ALL, nil
	case "game", "games":
		return wiiudownloader.TITLE_CATEGORY_GAME, nil
	case "update", "updates":
		return wiiudownloader.TITLE_CATEGORY_UPDATE, nil
	case "dlc", "dlcs":
		return wiiudownloader.TITLE_CATEGORY_DLC, nil
	case "demo", "demos":
		return wiiudownloader.TITLE_CATEGORY_DEMO, nil
	default:
		return 0, fmt.Errorf("unknown type %q (want game, update, dlc, demo or all)", s)
	}
}

func parseRegionMask(s string) (uint8, error) {
	var mask uint8
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '|' || r == ' ' || r == '\t' || r == '+' || r == '/'
	})
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty region list")
	}
	for _, field := range fields {
		switch strings.ToLower(field) {
		case "all", "world":
			mask |= allRegionMask
		case "usa", "us", "america", "north-america", "na":
			mask |= wiiudownloader.MCP_REGION_USA
		case "eur", "eu", "europe":
			mask |= wiiudownloader.MCP_REGION_EUROPE
		case "jpn", "jp", "japan":
			mask |= wiiudownloader.MCP_REGION_JAPAN
		case "chn", "cn", "china":
			mask |= wiiudownloader.MCP_REGION_CHINA
		case "kor", "kr", "korea":
			mask |= wiiudownloader.MCP_REGION_KOREA
		case "twn", "tw", "taiwan":
			mask |= wiiudownloader.MCP_REGION_TAIWAN
		default:
			return 0, fmt.Errorf("unknown region %q (want usa, eur, jpn, chn, kor, twn or all)", field)
		}
	}
	if mask == 0 {
		return 0, fmt.Errorf("empty region list")
	}
	return mask, nil
}

// parseTitleID accepts a 16-digit hexadecimal title ID, with or without 0x and
// with optional dashes (e.g. 00050000-101C9500).
func parseTitleID(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 16 {
		return 0, false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return 0, false
		}
	}
	id, err := parseHexUint64(s)
	return id, err == nil
}

func parseHexUint64(s string) (uint64, error) {
	var id uint64
	for _, r := range s {
		var digit uint64
		switch {
		case r >= '0' && r <= '9':
			digit = uint64(r - '0')
		case r >= 'a' && r <= 'f':
			digit = uint64(r-'a') + 10
		case r >= 'A' && r <= 'F':
			digit = uint64(r-'A') + 10
		default:
			return 0, fmt.Errorf("invalid hex digit %q", r)
		}
		id = id<<4 | digit
	}
	return id, nil
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd())) && os.Getenv("TERM") != "dumb"
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func absolutePath(path string) string {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
