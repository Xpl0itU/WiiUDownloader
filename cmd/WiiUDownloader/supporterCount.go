package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gotk3/gotk3/gtk"
)

const (
	kofiPageURL           = "https://ko-fi.com/dathinkingchair"
	supporterFetchTimeout = 10 * time.Second

	fallbackSupporterCount = 220
)

var supporterCountPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\d[\d,]*)\s*supporters?\b`),
	regexp.MustCompile(`(?i)"supporterCount"\s*:\s*"?(\d[\d,]*)"?`),
	regexp.MustCompile(`(?i)supporterCount[^\d]{0,20}(\d[\d,]*)`),
}

func parseSupporterCount(page string) (int, bool) {
	for _, re := range supporterCountPatterns {
		m := re.FindStringSubmatch(page)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		if err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

func truncateCountToTens(n int) int { return n / 10 * 10 }

func supporterCountText(n int) string {
	return fmt.Sprintf("%d+ supporters chipped in", truncateCountToTens(n))
}

func fetchSupporterCount(client *http.Client) (int, bool) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), supporterFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kofiPageURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; WiiUDownloader)")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Supporter count fetch failed:", err)
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Println("Supporter count fetch returned status", resp.StatusCode)
		return 0, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return 0, false
	}
	return parseSupporterCount(string(body))
}

func (mw *MainWindow) newSupporterLabel() *gtk.Label {
	label, err := gtk.LabelNew(supporterCountText(mw.supporterCount))
	if err != nil {
		return nil
	}
	addStyleClass(label.GetStyleContext, "supporter-count")
	label.SetHAlign(gtk.ALIGN_CENTER)
	mw.supporterLabels = append(mw.supporterLabels, label)
	return label
}

func (mw *MainWindow) refreshSupporterCount() {
	count, ok := fetchSupporterCount(mw.client)
	if !ok {
		return
	}
	uiIdleAdd(func() {
		mw.supporterCount = count
		text := supporterCountText(count)
		for _, label := range mw.supporterLabels {
			label.SetText(text)
		}
	})
}
