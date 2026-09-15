package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

const (
	UI_STRESS_DEFAULT_STEPS  = 200
	UI_STRESS_DIALOG_WIDTH   = 320
	UI_STRESS_DIALOG_HEIGHT  = 240
	UI_STRESS_SEARCH_BATCH   = 7
	UI_STRESS_QUEUE_CHURN_AT = 11
	UI_STRESS_BULK_QUEUE     = 60
)

var uiStressSizes = [][2]int{
	{1, 1}, {200, 150}, {320, 240}, {640, 400}, {839, 479}, {840, 480}, {841, 481},
	{919, 599}, {920, 600}, {1040, 700}, {1600, 900}, {2560, 1440}, {4000, 3000},
	{0, 0}, {-1, -1}, {-500, -300}, {3000, 200},
}

var uiStressPanePositions = []int{-500, -1, 0, 1, 140, 279, 280, 420, 700, 1400, 4000}

var uiStressSearches = []string{
	"", " ", "\t\n", "%", "_", "\\", "'", "\"", "--", "-", "null", "undefined", "0", "-1",
	"🎮", "日本語", "zelda", "ZELDA", "Zelda ", " Zelda", "z e l d a", "..", "../../etc/passwd",
	"<script>alert(1)</script>", "'; DROP TABLE titles;--", "%s%s%n", "\x1b[31mred\x1b[0m",
	strings.Repeat("a", 4096), strings.Repeat("日本", 2048), strings.Repeat(" ", 512),
	string([]byte{0xff, 0xfe, 0xfd}),
}

type uiStress struct {
	smoke      *uiSmoke
	rng        *rand.Rand
	step       int
	lastAction string
	violations map[string]int
	detail     map[string]string
}

type stressAction struct {
	name string
	step func(st *uiStress, mw *MainWindow)
}

func (st *uiStress) audit(family string, mw *MainWindow) {
	var problems []string

	expected := 0
	for _, row := range mw.titleRows {
		if mw.titleMatchesFilter(row) {
			expected++
		}
	}
	if got := int(mw.titleSortModel.NItems()); got != expected {
		problems = append(problems, fmt.Sprintf("the list shows %d rows while the filter selects %d", got, expected))
	}
	unique := make(map[string]struct{})
	for _, entry := range wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL) {
		unique[rowKeyForTitleID(entry.TitleID)] = struct{}{}
	}
	if len(mw.titleRows) != len(unique) {
		problems = append(problems, fmt.Sprintf("the row model holds %d of %d distinct titles", len(mw.titleRows), len(unique)))
	}
	if mw.window.Width() <= 0 || mw.window.Height() <= 0 {
		problems = append(problems, fmt.Sprintf("the window allocation collapsed to %dx%d", mw.window.Width(), mw.window.Height()))
	}
	if !mw.titleView.Visible() {
		problems = append(problems, "the title list stopped being visible")
	}
	if mw.queuePane.container.Width() <= 0 {
		problems = append(problems, "the queue pane collapsed to zero width")
	}

	queue := mw.queuePane.GetTitleQueue()
	seen := make(map[uint64]int, len(queue))
	for _, entry := range queue {
		seen[entry.TitleID]++
	}
	for titleID, count := range seen {
		if count > 1 {
			problems = append(problems, fmt.Sprintf("title %08X is queued %d times", titleID, count))
		}
	}
	if mw.queuePane.IsQueueEmpty() != (len(queue) == 0) {
		problems = append(problems, fmt.Sprintf("the queue reports empty=%v with %d entries", mw.queuePane.IsQueueEmpty(), len(queue)))
	}

	activeCategories := 0
	for _, button := range mw.categoryButtons {
		if button.Active() {
			activeCategories++
		}
	}
	if activeCategories != 1 {
		problems = append(problems, fmt.Sprintf("%d category buttons are active at once", activeCategories))
	}

	if len(problems) == 0 {
		return
	}
	st.violations[family]++
	if _, seen := st.detail[family]; !seen {
		st.detail[family] = fmt.Sprintf("step %d (%s): %s", st.step, st.lastAction, strings.Join(problems, "; "))
	}
}

func (st *uiStress) report(family string, steps int) {
	if st.violations[family] == 0 {
		st.smoke.check(true, "%s: %d abusive steps left every invariant standing", family, steps)
		return
	}
	st.smoke.check(false, "%s: %d of %d steps broke an invariant (%s)", family, st.violations[family], steps, st.detail[family])
}

func (st *uiStress) run(mw *MainWindow, family string, steps int, actions []stressAction) {
	for i := 0; i < steps; i++ {
		st.step = i
		action := actions[st.rng.Intn(len(actions))]
		st.lastAction = action.name
		action.step(st, mw)
		uiSmokePump()
		st.audit(family, mw)
	}
	st.report(family, steps)
}

func (st *uiStress) pickTitle() wiiudownloader.TitleEntry {
	database := wiiudownloader.TitleDatabase
	return database[st.rng.Intn(len(database))]
}

func stressResizeWindow(st *uiStress, mw *MainWindow) {
	size := uiStressSizes[st.rng.Intn(len(uiStressSizes))]
	mw.window.SetDefaultSize(size[0], size[1])
}

func stressResizePane(st *uiStress, mw *MainWindow) {
	mw.splitPane.SetPosition(uiStressPanePositions[st.rng.Intn(len(uiStressPanePositions))])
}

func stressMashFilters(st *uiStress, mw *MainWindow) {
	if len(mw.categoryButtons) > 0 {
		mw.categoryButtons[st.rng.Intn(len(mw.categoryButtons))].SetActive(true)
	}
	mw.japanRegionCheckbox.SetActive(st.rng.Intn(2) == 0)
	mw.usaRegionCheckbox.SetActive(st.rng.Intn(2) == 0)
	mw.europeRegionCheckbox.SetActive(st.rng.Intn(2) == 0)
	text := uiStressSearches[st.rng.Intn(len(uiStressSearches))]
	mw.searchEntry.SetText(text)
	if st.rng.Intn(UI_STRESS_SEARCH_BATCH) == 0 {
		uiSmokeWait(SEARCH_DEBOUNCE_DELAY + 40*time.Millisecond)
	} else {
		mw.lastSearchText = mw.searchEntry.Text()
		mw.refreshTitleFilter()
	}
}

func stressChurnQueue(st *uiStress, mw *MainWindow) {
	switch st.rng.Intn(6) {
	case 0, 1:
		mw.addTitlesToQueue([]wiiudownloader.TitleEntry{st.pickTitle()})
	case 2:
		entry := st.pickTitle()
		mw.addTitlesToQueue([]wiiudownloader.TitleEntry{entry, entry})
	case 3:
		if queue := mw.queuePane.GetTitleQueue(); len(queue) > 0 {
			mw.queuePane.RemoveTitle(queue[st.rng.Intn(len(queue))])
		}
	case 4:
		if selected := mw.queuePane.selectedTitleIDs(); len(selected) > 0 {
			mw.queuePane.RemoveTitles(selected)
		}
	case 5:
		if st.rng.Intn(UI_STRESS_QUEUE_CHURN_AT) == 0 {
			mw.queuePane.Clear()
		}
	}
	mw.updateTitlesInQueue()
}

func stressBulkQueue(st *uiStress, mw *MainWindow) {
	batch := make([]wiiudownloader.TitleEntry, 0, UI_STRESS_BULK_QUEUE)
	for i := 0; i < UI_STRESS_BULK_QUEUE; i++ {
		batch = append(batch, st.pickTitle())
	}
	mw.addTitlesToQueue(batch)
	uiSmokePump()
	mw.queuePane.RemoveTitles(mw.queuePane.selectedTitleIDs())
	mw.queuePane.Clear()
	mw.updateTitlesInQueue()
}

func stressAbuseRun(st *uiStress, mw *MainWindow) {
	run := mw.beginRun("stress")
	switch st.rng.Intn(6) {
	case 0:
		run.SetQueueProgress(1, 2)
		run.SetGameTitle("Zelda")
		run.SetDownloadSize(1 << 30)
		run.UpdateDownloadProgress(1<<20, "f.bin")
	case 1:
		run.TogglePaused()
		mw.queuePane.SetRunPaused(true)
		mw.queuePane.SetRunPaused(false)
		run.TogglePaused()
	case 2:
		run.SetCancelled()
		run.Finish()
	case 3:
		run.Finish()
		run.Finish()
	case 4:
		mw.queuePane.BeginRun()
		mw.queuePane.EndRun()
		mw.queuePane.EndRun()
	case 5:
		go func() {
			run.SetGameTitle("off-thread")
			run.UpdateDecryptionProgress(0.5)
		}()
	}
	mw.queuePane.EndRun()
	run.Finish()
	mw.setDownloadControlsSensitive(true)
}

func stressFlapTheme(st *uiStress, mw *MainWindow) {
	setDarkTheme(st.rng.Intn(2) == 0)
}

func stressStormDialogs(st *uiStress, mw *MainWindow) {
	switch st.rng.Intn(4) {
	case 0:
		dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Stress")
		dialog.SetDefaultSize(UI_STRESS_DIALOG_WIDTH, UI_STRESS_DIALOG_HEIGHT)
		dialog.Present()
		uiSmokePump()
		button := dialog.AddButton("Close", nil)
		button.Emit("clicked")
		uiSmokePump()
	case 1:
		dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Stress")
		dialog.Present()
		uiSmokePump()
		dialog.Destroy()
		dialog.CloseThen(func() {})
	case 2:
		alert := showAlert(mw.window, WINDOW_TITLE_PREFIX+"Stress", "stress")
		if alert != nil {
			uiSmokePump()
			alert.Close()
		}
	case 3:
		mw.showAddByTitleIDDialog()
		uiSmokePump()
		if window := uiSmokeWindowWidget(WINDOW_TITLE_PREFIX + "Add by Title ID"); window != nil {
			if cancel := uiSmokeButtonByLabel(window, "Cancel"); cancel != nil {
				cancel.Emit("clicked")
			}
		}
		uiSmokePump()
	}
}

var (
	uiStressResizeWindow = stressAction{"resize the window", stressResizeWindow}
	uiStressResizePane   = stressAction{"resize the queue pane", stressResizePane}
	uiStressMashFilters  = stressAction{"mash filters and search", stressMashFilters}
	uiStressChurnQueue   = stressAction{"churn the queue", stressChurnQueue}
	uiStressBulkQueue    = stressAction{"bulk queue and clear", stressBulkQueue}
	uiStressAbuseRun     = stressAction{"abuse the run bar", stressAbuseRun}
	uiStressFlapTheme    = stressAction{"flap the theme", stressFlapTheme}
	uiStressStormDialogs = stressAction{"storm dialogs", stressStormDialogs}
)

var uiStressActions = []stressAction{
	uiStressResizeWindow,
	uiStressResizePane,
	uiStressMashFilters,
	uiStressChurnQueue,
	uiStressBulkQueue,
	uiStressAbuseRun,
	uiStressFlapTheme,
	uiStressStormDialogs,
}

func runUIStress() int {
	seed := time.Now().UnixNano()
	if value := os.Getenv("WIIU_UI_STRESS_SEED"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			seed = parsed
		}
	}
	steps := UI_STRESS_DEFAULT_STEPS
	if value := os.Getenv("WIIU_UI_STRESS_STEPS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			steps = parsed
		}
	}

	s := &uiSmoke{}
	st := &uiStress{
		smoke:      s,
		rng:        rand.New(rand.NewSource(seed)),
		violations: make(map[string]int),
		detail:     make(map[string]string),
	}
	fmt.Printf("UI stress run: seed %d, %d steps per family\n", seed, steps)

	cfg := getDefaultConfig()
	cfg.ShowDonationBar = true
	cfg.SuggestRelatedContent = false
	cfg.GetSizeOnQueue = false

	mw := NewMainWindow(buildHTTPClient(), cfg)
	mw.BuildUI()
	mw.window.Present()
	uiSmokeSettle()
	s.check(mw.window != nil && mw.titleView != nil, "the window is up before the abuse starts")

	dialogsBefore := uiSmokeToplevelCount()

	st.run(mw, "window and pane resizing", steps, []stressAction{uiStressResizeWindow, uiStressResizePane})
	st.run(mw, "filters, regions and search", steps, []stressAction{uiStressMashFilters})
	st.run(mw, "queue churn", steps, []stressAction{uiStressChurnQueue, uiStressBulkQueue})
	st.run(mw, "run bar abuse", steps, []stressAction{uiStressAbuseRun})
	st.run(mw, "theme flapping", steps, []stressAction{uiStressFlapTheme})
	st.run(mw, "dialog storm", steps, []stressAction{uiStressStormDialogs})
	st.run(mw, "everything at once", steps*2, uiStressActions)

	mw.queuePane.Clear()
	mw.window.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
	mw.splitPane.SetPosition(280)
	mw.searchEntry.SetText("")
	mw.lastSearchText = ""
	mw.refreshTitleFilter()
	uiSmokeSettle()

	mw.japanRegionCheckbox.SetActive(false)
	mw.usaRegionCheckbox.SetActive(false)
	mw.europeRegionCheckbox.SetActive(false)
	uiSmokePump()
	emptyMaskRows := int(mw.titleSortModel.NItems())
	stillChecked := mw.japanRegionCheckbox.Active() || mw.usaRegionCheckbox.Active() || mw.europeRegionCheckbox.Active()
	s.check(stillChecked && mw.currentRegion != 0,
		"unchecking every region keeps one selected (%d rows, mask %d)", emptyMaskRows, mw.currentRegion)
	mw.applyRegionSelection(wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA)
	uiSmokeSettle()

	leaked := uiSmokeToplevelCount() - dialogsBefore
	s.check(leaked == 0, "the abuse leaves no window behind (%d extra toplevels)", leaked)
	s.check(mw.queuePane.IsQueueEmpty(), "the queue empties when asked")
	s.check(mw.titleSortModel.NItems() > 0, "the list still shows titles (%d rows)", mw.titleSortModel.NItems())
	uiSmokeTitlePrefixes(s, "after the abuse")

	fmt.Printf("UI stress: %d passed, %d failed (seed %d)\n", s.pass, s.fail, seed)
	if s.fail > 0 {
		return 1
	}
	return 0
}
