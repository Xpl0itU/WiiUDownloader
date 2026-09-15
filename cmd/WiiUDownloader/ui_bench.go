package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
)

// ui_bench.go is the file picker's benchmark harness. It drives the real dialog
// over a BotW-sized tree so picker changes are measured instead of guessed.
// Run with WIIU_UI_BENCH=1.
//
// Breath of the Wild (00050000101C9500), measured from the real CDN:
// 18,719 files, 661 directories, max depth 4, 10.35 GB, FST fetch ~1.5 s.

const (
	benchLeafDirs       = 25
	benchFilesPerLeaf   = 30
	benchContentFolders = 25
)

// benchTitleFiles builds a deterministic tree with BotW's shape and scale.
func benchTitleFiles() []wiiudownloader.TitleFile {
	files := make([]wiiudownloader.TitleFile, 0, 70+benchContentFolders*benchLeafDirs*benchFilesPerLeaf)
	var size uint64 = 4096
	add := func(path string) {
		files = append(files, wiiudownloader.TitleFile{Path: path, Size: size, Length: size})
		if size < 1<<20 {
			size += 1024
		}
	}
	for i := 0; i < 50; i++ {
		add(fmt.Sprintf("code/Module-%02d.rpx", i))
	}
	for i := 0; i < 20; i++ {
		add(fmt.Sprintf("meta/data%03d.bin", i))
	}
	for a := 0; a < benchContentFolders; a++ {
		for b := 0; b < benchLeafDirs; b++ {
			for c := 0; c < benchFilesPerLeaf; c++ {
				add(fmt.Sprintf("content/Area%02d/Pack%02d/Asset%03d.bin", a, b, c))
			}
		}
	}
	return files
}

// benchVisibleNodes mirrors the picker's on-screen rule so both the old and the
// new implementation can be measured with the same yardstick.
func benchVisibleNodes(query string) int {
	count := 0
	for _, node := range lastTitleFileNodes {
		if query != "" {
			if node.match {
				count++
			}
			continue
		}
		visible := true
		for parent := node.parent; parent != nil; parent = parent.parent {
			if !parent.expanded {
				visible = false
				break
			}
		}
		if visible {
			count++
		}
	}
	return count
}

// uiBenchWaitUntil pumps the loop until pred holds, returning how long it took.
func uiBenchWaitUntil(pred func() bool, timeout time.Duration) (time.Duration, bool) {
	start := time.Now()
	for time.Since(start) < timeout {
		if pred() {
			return time.Since(start), true
		}
		uiSmokePump()
		time.Sleep(2 * time.Millisecond)
	}
	return time.Since(start), pred()
}

func uiBenchReport(label string, d time.Duration, extra string) {
	if extra == "" {
		fmt.Printf("  %-32s %9s\n", label, d.Round(time.Millisecond))
		return
	}
	fmt.Printf("  %-32s %9s  %s\n", label, d.Round(time.Millisecond), extra)
}

// benchTitleList measures the per-row work behind the title list. The filter
// predicate runs for every row on every pass, and a pass happens at startup and
// again on each search keystroke or category change, so this is the cost the user
// actually feels. The two predicates below are the cached-category form and the
// string-deriving form it replaced, so the difference is measured, not assumed.
func benchTitleList(mw *MainWindow) {
	fmt.Println()
	fmt.Println("Title list per-row cost")

	rows := make([]*titleRow, 0, len(mw.titleRows))
	for _, row := range mw.titleRows {
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		fmt.Println("  no rows built; skipping")
		return
	}

	const passes = 20

	// The startup call this change removed: the entries were fetched and thrown
	// away because the field they were stored in was never read.
	entriesStart := time.Now()
	gameEntries := 0
	for i := 0; i < passes; i++ {
		gameEntries = len(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME))
	}
	uiBenchMicro(fmt.Sprintf("GetTitleEntries(GAME) x%d", passes),
		time.Since(entriesStart), fmt.Sprintf("%d entries per call", gameEntries))

	cached := func() int {
		n := 0
		for _, row := range rows {
			if row.category == wiiudownloader.TITLE_CATEGORY_GAME {
				n++
			}
		}
		return n
	}
	derived := func() int {
		n := 0
		for _, row := range rows {
			if row.kind != wiiudownloader.GetFormattedKind(row.entry.TitleID) {
				continue
			}
			if wiiudownloader.GetCategoryFromFormattedCategory(row.kind) == wiiudownloader.TITLE_CATEGORY_GAME {
				n++
			}
		}
		return n
	}

	start := time.Now()
	var cachedCount, derivedCount int
	for i := 0; i < passes; i++ {
		cachedCount = cached()
	}
	cachedDur := time.Since(start)

	start = time.Now()
	for i := 0; i < passes; i++ {
		derivedCount = derived()
	}
	derivedDur := time.Since(start)

	uiBenchMicro(fmt.Sprintf("filter pass x%d (cached)", passes), cachedDur, fmt.Sprintf("%d rows per pass", len(rows)))
	uiBenchMicro(fmt.Sprintf("filter pass x%d (derived)", passes), derivedDur, fmt.Sprintf("%d rows per pass", len(rows)))
	if cachedDur > 0 {
		fmt.Printf("  speedup: %.2fx; both forms select %d rows (%v)\n",
			float64(derivedDur)/float64(cachedDur), cachedCount, cachedCount == derivedCount)
	}
}

func uiBenchMicro(label string, d time.Duration, extra string) {
	fmt.Printf("  %-32s %9s  %s\n", label, d.Round(time.Microsecond), extra)
}

func runUIBench() int {
	fmt.Println("File picker benchmark (BotW-sized tree)")

	config := getDefaultConfig()
	config.ShowDonationBar = false
	config.SuggestRelatedContent = false
	mw := NewMainWindow(buildHTTPClient(), config)
	mw.BuildUI()
	mw.window.Present()
	uiSmokeSettle()

	benchTitleList(mw)
	fmt.Println()

	files := benchTitleFiles()
	// WIIU_E2E=1 benchmarks the real Breath of the Wild list instead of the
	// synthetic one, which is the number that matters.
	if os.Getenv("WIIU_E2E") != "" {
		fetchStart := time.Now()
		if real, err := wiiudownloader.FetchTitleFileTree(0x00050000101C9500, wiiudownloader.VersionLatest, mw.client); err == nil {
			uiBenchReport("real BotW FST fetch", time.Since(fetchStart), fmt.Sprintf("%d files", len(real.Files)))
			files = real.Files
		} else {
			fmt.Printf("  real BotW fetch failed (%v); falling back to the synthetic tree\n", err)
		}
	}
	start := time.Now()
	roots, nodes := buildTitleFileNodes(files)
	buildDur := time.Since(start)

	dirs, fileNodes := 0, 0
	for _, node := range nodes {
		if node.dir {
			dirs++
		} else {
			fileNodes++
		}
	}
	fmt.Printf("  tree: %d files, %d dirs, %d roots\n", fileNodes, dirs, len(roots))
	uiBenchReport("buildTitleFileNodes", buildDur, "")

	// Realised rows: the virtualized view only builds widgets for rows on
	// screen, so this is both the memory and the construction cost.
	rowCount := func() int {
		n := 0
		for _, node := range lastTitleFileNodes {
			if node.row != nil {
				n++
			}
		}
		return n
	}
	// Ticked files from the model, which is the source of truth; a row off
	// screen has no checkbox to read.
	checkedCount := func() int {
		n := 0
		for _, node := range lastTitleFileNodes {
			if !node.dir && node.selected {
				n++
			}
		}
		return n
	}

	tree := &wiiudownloader.TitleFileTree{TitleID: 0x00050000101C9500, Name: "The Legend of Zelda Breath of the Wild", Files: files}
	originalFetch := fetchTitleFileTree
	fetchTitleFileTree = func(uint64, int, *http.Client) (*wiiudownloader.TitleFileTree, error) {
		return tree, nil
	}
	defer func() { fetchTitleFileTree = originalFetch }()

	lastTitleFileNodes = nil
	start = time.Now()
	mw.showSpecificFilesDialogFor(wiiudownloader.TitleEntry{TitleID: 0x00050000101C9500, Name: "Breath of the Wild"})
	waitDur, ok := uiBenchWaitUntil(func() bool { return len(lastTitleFileNodes) == len(nodes) }, 10*time.Minute)
	if !ok {
		fmt.Printf("  picker never became ready: %d of %d nodes\n", len(lastTitleFileNodes), len(nodes))
		return 1
	}
	uiBenchReport("picker ready (present -> rows)", waitDur, fmt.Sprintf("row widgets=%d", rowCount()))

	active := checkedCount()
	fmt.Printf("  initial checked files: %d of %d\n", active, fileNodes)
	if lastTitleFilePicker.status != nil {
		fmt.Printf("  initial status: %q\n", lastTitleFilePicker.status.Text())
	}

	// A single click is the interactive hot path: the checkbox handler walks the
	// node's ancestors and refreshes the status on every toggle.
	for _, node := range lastTitleFileNodes {
		if node.dir || node.check == nil {
			continue
		}
		start = time.Now()
		node.check.SetActive(!node.check.Active())
		uiSmokePump()
		uiBenchReport("toggle one file", time.Since(start), fmt.Sprintf("selected=%d", checkedCount()))
		break
	}

	// Wait on the picker's own effective query: the search entry debounces, so
	// "the text was set" is not "the filter ran".
	pickerQuery := func() string {
		if lastTitleFilePicker.query == nil {
			return ""
		}
		return lastTitleFilePicker.query()
	}

	if lastTitleFilePicker.search != nil && len(files) > 0 {
		// Derive the term from the real list so it always matches something.
		middle := files[len(files)/2].Path
		base := middle[strings.LastIndexByte(middle, '/')+1:]
		if dot := strings.LastIndexByte(base, '.'); dot > 0 {
			base = base[:dot]
		}
		term := strings.ToLower(base)

		lastTitleFilePicker.search.SetText(term)
		searchDur, found := uiBenchWaitUntil(func() bool { return pickerQuery() == term }, time.Minute)
		uiBenchReport("search (debounce + filter)", searchDur,
			fmt.Sprintf("term=%q on screen=%d ok=%v", term, benchVisibleNodes(term), found))

		lastTitleFilePicker.search.SetText("")
		uiBenchWaitUntil(func() bool { return pickerQuery() == "" }, time.Minute)
	}

	if lastTitleFilePicker.collapseAll != nil {
		start = time.Now()
		coreglib.InternObject(lastTitleFilePicker.collapseAll).Emit("clicked")
		uiBenchWaitUntil(func() bool { return benchVisibleNodes("") < len(nodes) }, time.Minute)
		uiBenchReport("collapse all", time.Since(start), fmt.Sprintf("on screen=%d", benchVisibleNodes("")))

		start = time.Now()
		coreglib.InternObject(lastTitleFilePicker.selectAll).Emit("clicked")
		uiSmokePump()
		uiBenchReport("select all (collapsed)", time.Since(start), fmt.Sprintf("checked=%d/%d", checkedCount(), fileNodes))

		start = time.Now()
		coreglib.InternObject(lastTitleFilePicker.selectNone).Emit("clicked")
		uiSmokePump()
		uiBenchReport("select none (collapsed)", time.Since(start), fmt.Sprintf("checked=%d/%d", checkedCount(), fileNodes))
	}

	if lastTitleFilePicker.expandAll != nil {
		start = time.Now()
		coreglib.InternObject(lastTitleFilePicker.expandAll).Emit("clicked")
		uiBenchWaitUntil(func() bool { return benchVisibleNodes("") == len(nodes) }, time.Minute)
		uiBenchReport("expand all", time.Since(start), fmt.Sprintf("on screen=%d", benchVisibleNodes("")))
	}
	return 0
}
