package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	gio "github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/graphene"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type uiSmoke struct {
	pass int
	fail int
}

func (s *uiSmoke) check(ok bool, format string, args ...interface{}) {
	label := " ok "
	if !ok {
		label = "FAIL"
		s.fail++
	} else {
		s.pass++
	}
	fmt.Printf("  [%s] %s\n", label, fmt.Sprintf(format, args...))
}

// uiSmokeSettle drains the loop and lets the frame clock realise rows.
func uiSmokeSettle() {
	for i := 0; i < 6; i++ {
		uiSmokePump()
		time.Sleep(15 * time.Millisecond)
	}
}

// uiSmokePump drains pending main-loop work so uiIdleAdd callbacks run.
func uiSmokePump() {
	ctx := glib.MainContextDefault()
	for i := 0; i < 300; i++ {
		ctx.Iteration(false)
	}
}

func uiSmokeToplevelCount() int {
	return int(gtk.WindowGetToplevels().NItems())
}

// uiSmokeCheckboxSweep checks the invariant that every realised checkbox agrees
// with the row it is bound to, and reports how many were bound.
func uiSmokeCheckboxSweep(mw *MainWindow) (int, bool) {
	consistent := true
	for key, check := range mw.boundChecks {
		row := mw.titleRows[key]
		if row == nil || check.Active() != row.inQueue {
			consistent = false
		}
	}
	return len(mw.boundChecks), consistent
}

// uiSmokeStylesheet parses style.css with the live theme: the libadwaita colour
// names it relies on only resolve once libadwaita has been initialized.
func uiSmokeStylesheet(s *uiSmoke) {
	provider := gtk.NewCSSProvider()
	errors := 0
	provider.ConnectParsingError(func(_ *gtk.CSSSection, err error) {
		errors++
		if errors <= 3 {
			fmt.Printf("  [css] %v\n", err)
		}
	})
	provider.LoadFromData(styleCSS)
	s.check(errors == 0, "style.css parses cleanly under libadwaita (%d errors)", errors)

	// A bare rule for a libadwaita-owned class silently restyles its widgets:
	// "title" is on every preferences-row title, so a global .title rule made the
	// whole settings window oversized.
	var clobbered []string
	for _, class := range libadwaitaOwnedClasses {
		if strings.Contains(styleCSS, "\n."+class+" {") || strings.Contains(styleCSS, "."+class+",") {
			clobbered = append(clobbered, class)
		}
	}
	s.check(len(clobbered) == 0, "style.css does not restyle libadwaita's own classes (%v)", clobbered)
}

// libadwaitaOwnedClasses are class names libadwaita sets itself; styling one
// from the app's own sheet reaches far beyond the intended widget.
var libadwaitaOwnedClasses = []string{
	"title", "subtitle", "heading", "body", "caption", "dimmed",
	"header", "prefixes", "suffixes", "editable-area", "navigation-sidebar",
	"boxed-list", "activatable", "property", "card", "toolbar",
}

// uiSmokeFindByClass returns the first descendant of root carrying a CSS class.
func uiSmokeFindByClass(root gtk.Widgetter, class string) gtk.Widgetter {
	for _, child := range uiSmokeChildren(root) {
		if uiSmokeHasClass(child, class) {
			return child
		}
		if found := uiSmokeFindByClass(child, class); found != nil {
			return found
		}
	}
	return nil
}

func uiSmokeHasClass(w gtk.Widgetter, class string) bool {
	if w == nil {
		return false
	}
	for _, c := range gtk.BaseWidget(w).CSSClasses() {
		if c == class {
			return true
		}
	}
	return false
}

func uiSmokeLabelCount(box gtk.Widgetter) int {
	n := 0
	for _, child := range uiSmokeChildren(box) {
		if gtk.BaseWidget(child).CSSName() == "label" {
			n++
		}
	}
	return n
}

// uiSmokeScan reports the tallest list row height and every label text found in
// a widget subtree.
func uiSmokeScan(w gtk.Widgetter) (int, []string) {
	maxRow := 0
	var texts []string
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		b := gtk.BaseWidget(cur)
		switch b.CSSName() {
		case "row":
			// Natural measure works before the row has an allocation.
			if _, natural, _, _ := b.Measure(gtk.OrientationVertical, -1); natural > maxRow {
				maxRow = natural
			}
		case "label":
			if label, ok := cur.(*gtk.Label); ok {
				texts = append(texts, label.Text())
			}
		}
		for child := b.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
			walk(child)
		}
	}
	walk(w)
	return maxRow, texts
}

func uiSmokeHasText(texts []string, want string) bool {
	for _, text := range texts {
		if text == want {
			return true
		}
	}
	return false
}

// uiSmokeWidgetPoint translates w's origin into target's coordinates.
func uiSmokeWidgetPoint(w, target gtk.Widgetter) (float32, float32, bool) {
	point, ok := gtk.BaseWidget(w).ComputePoint(target, graphene.PointZero())
	if !ok || point == nil {
		return 0, 0, false
	}
	return point.X(), point.Y(), true
}

// uiSmokeWindowTitles reads the title of every toplevel GTK currently has.
func uiSmokeWindowTitles() []string {
	model := gtk.WindowGetToplevels()
	var titles []string
	for i := uint(0); i < model.NItems(); i++ {
		obj := model.Item(i)
		if obj == nil {
			continue
		}
		title, ok := obj.ObjectProperty("title").(string)
		if ok {
			titles = append(titles, title)
		}
	}
	return titles
}

// uiSmokeCheckLayout asserts the layout invariants that must hold at every
// window size: one toolbar row with the search to the right of the category
// pills, the queue pane left of the title list, the donation bar above the
// action bar, and no control squeezed out of its allocation.
func uiSmokeCheckLayout(s *uiSmoke, mw *MainWindow, size string) {
	pillsX, _, okPills := uiSmokeWidgetPoint(mw.categoryBox, mw.toolbar)
	searchX, _, okSearch := uiSmokeWidgetPoint(mw.searchEntry, mw.toolbar)
	s.check(okPills && okSearch, "%s: toolbar children have toolbar-relative coordinates", size)
	if okPills && okSearch {
		s.check(searchX-pillsX >= float32(mw.categoryBox.Width()),
			"%s: search entry starts right of the category pills (%.0f >= %d)", size, searchX-pillsX, mw.categoryBox.Width())
	}
	// Both live inside the one toolbar box, so a GTK box can only lay them out
	// side by side; the geometry check below proves they really are on one row.
	s.check(gtk.BaseWidget(mw.categoryBox).IsAncestor(mw.toolbar) && gtk.BaseWidget(mw.searchEntry).IsAncestor(mw.toolbar),
		"%s: pills and search both live in the toolbar (no wrapping possible)", size)

	pillsY, searchY, pillsH, searchH := float32(0), float32(0), mw.categoryBox.Height(), mw.searchEntry.Height()
	_, pillsY, _ = uiSmokeWidgetPoint(mw.categoryBox, mw.window)
	_, searchY, _ = uiSmokeWidgetPoint(mw.searchEntry, mw.window)
	s.check(pillsY < searchY+float32(searchH) && searchY < pillsY+float32(pillsH),
		"%s: pills and search share the same row vertically (%.0f/%.0f)", size, pillsY, searchY)

	_, toolbarNatural, _, _ := mw.toolbar.Measure(gtk.OrientationHorizontal, -1)
	s.check(mw.toolbar.Width() >= toolbarNatural, "%s: toolbar fits without squeezing (%d >= %d)", size, mw.toolbar.Width(), toolbarNatural)

	visible := gtk.BaseWidget(mw.categoryBox).Mapped() && mw.categoryBox.Width() > 0 &&
		gtk.BaseWidget(mw.searchEntry).Mapped() && mw.searchEntry.Width() > 0 &&
		gtk.BaseWidget(mw.menuButton).Mapped() && mw.menuButton.Width() > 0
	s.check(visible, "%s: pills, search and menu button are all on screen", size)

	if mw.donationBar != nil && mw.bottomBar != nil {
		_, barY, okBar := uiSmokeWidgetPoint(mw.donationBar, mw.window)
		_, actionY, okAction := uiSmokeWidgetPoint(mw.bottomBar, mw.window)
		s.check(okBar && okAction && barY < actionY, "%s: donation bar stays above the action bar (%.0f < %.0f)", size, barY, actionY)
	}

	titleX, _, okTitle := uiSmokeWidgetPoint(mw.titleView, mw.window)
	if gtk.BaseWidget(mw.queuePane.container).Visible() {
		queueX, _, okQueue := uiSmokeWidgetPoint(mw.queuePane.container, mw.window)
		s.check(okQueue && okTitle && queueX+float32(mw.queuePane.container.Width()) <= titleX+1,
			"%s: queue pane stays left of the title list", size)
	} else {
		// Below the breakpoint the pane is dropped so nothing gets clipped. The
		// list must stay usable, and the pane must really be gone: the old check
		// only looked at the list, so a breakpoint that silently never fired
		// still passed.
		s.check(okTitle && gtk.BaseWidget(mw.titleView).Mapped() && mw.titleView.Width() > 0,
			"%s: compact layout keeps the title list usable", size)
		s.check(mw.window.Width() > COMPACT_WINDOW_BREAKPOINT || !gtk.BaseWidget(mw.queuePane.container).Visible(),
			"%s: compact layout really drops the queue pane", size)
	}
}

// uiSmokeCheckContentFits asserts AdwToolbarView's content minimum still fits
// inside the window. Every "exceeds AdwWindow" line in the log comes from this
// one condition, so keeping it false is what actually stops the resize spam.
func uiSmokeCheckContentFits(s *uiSmoke, mw *MainWindow, size string) {
	view := gtk.BaseWidget(mw.toolbarView)
	minW, _, _, _ := view.Measure(gtk.OrientationHorizontal, -1)
	_, minH, _, _ := view.Measure(gtk.OrientationVertical, -1)
	s.check(minW <= mw.window.Width() && minH <= mw.window.Height(),
		"%s: content minimum %dx%d fits inside the window (no resize spam)", size, minW, minH)

	// The pane-visible minimum has to fit the *smallest* window that still shows
	// the pane, which is the breakpoint, not this size. Without this the
	// breakpoint silently drifts below the layout and the warnings come back.
	if gtk.BaseWidget(mw.queuePane.container).Visible() {
		s.check(minW <= COMPACT_WINDOW_BREAKPOINT+1,
			"%s: the pane-visible minimum %d fits the %d px breakpoint", size, minW, COMPACT_WINDOW_BREAKPOINT)
	}
}

// uiSmokeTitlePrefixes checks every open window is titled "WiiUDownloader - ...".
func uiSmokeTitlePrefixes(s *uiSmoke, when string) {
	var bad []string
	titles := uiSmokeWindowTitles()
	for _, title := range titles {
		if title != APP_NAME && !strings.HasPrefix(title, WINDOW_TITLE_PREFIX) {
			bad = append(bad, title)
		}
	}
	s.check(len(bad) == 0, "%s: every window title uses the app prefix (%d titles, bad: %v)", when, len(titles), bad)
}

// uiSmokeWait keeps the loop alive for at least d so timers (search debounce,
// async size fetches) can fire.
func uiSmokeWait(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		uiSmokePump()
		time.Sleep(10 * time.Millisecond)
	}
}

// uiSmokeWaitForWindowTitle pumps the loop until a window with that title shows
// up, so background work can be tested without racing it.
func uiSmokeWaitForWindowTitle(title string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, open := range uiSmokeWindowTitles() {
			if open == title {
				return true
			}
		}
		uiSmokePump()
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func uiSmokeHasWindowTitle(title string) bool {
	for _, open := range uiSmokeWindowTitles() {
		if open == title {
			return true
		}
	}
	return false
}

func uiSmokeChildren(container gtk.Widgetter) []gtk.Widgetter {
	var out []gtk.Widgetter
	for child := gtk.BaseWidget(container).FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
		out = append(out, child)
	}
	return out
}

// uiSmokeSetupChecks returns every check button inside w in tree order.
func uiSmokeSetupChecks(w gtk.Widgetter) []*gtk.CheckButton {
	var out []*gtk.CheckButton
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		if check, ok := cur.(*gtk.CheckButton); ok {
			out = append(out, check)
		}
		for _, child := range uiSmokeChildren(cur) {
			walk(child)
		}
	}
	walk(w)
	return out
}

func runUISmoke() int {
	s := &uiSmoke{}
	fmt.Println("UI smoke run")

	cfg := getDefaultConfig()
	cfg.ShowDonationBar = true
	cfg.SuggestRelatedContent = false
	cfg.GetSizeOnQueue = false

	mw := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL), buildHTTPClient(), cfg)
	mw.BuildUI()
	// Present so rows, cells and allocations exist for real; offscreen widgets
	// have no allocation and cannot be measured.
	mw.window.Present()
	uiSmokePump()
	s.check(mw.window != nil && mw.titleView != nil, "main window builds")
	width, height := mw.window.DefaultSize()
	s.check(width > 0 && height > 0, "main window has a default size (%dx%d)", width, height)
	uiSmokeStylesheet(s)

	// --- category switcher: toggle buttons, mutually exclusive, filtering ---
	s.check(len(mw.categoryButtons) == 5, "5 category buttons (got %d)", len(mw.categoryButtons))
	active, classOK := 0, true
	for _, b := range mw.categoryButtons {
		if b.Active() {
			active++
		}
		if !b.HasCSSClass("category-toggle") {
			classOK = false
		}
	}
	s.check(active == 1, "exactly one category active (got %d)", active)
	s.check(classOK, "every category button carries the category-toggle class")

	byLabel := func(label string) *gtk.ToggleButton {
		for _, b := range mw.categoryButtons {
			if b.Label() == label {
				return b
			}
		}
		return nil
	}
	game, all := byLabel("Game"), byLabel("All")
	if game == nil || all == nil {
		s.check(false, "Game/All category buttons exist")
		fmt.Printf("UI smoke: %d passed, %d failed\n", s.pass, s.fail)
		return 1
	}

	gameRows := mw.titleSortModel.NItems()
	all.SetActive(true)
	uiSmokePump()
	s.check(all.Active() && !game.Active(), "activating All deactivates Game")
	allRows := mw.titleSortModel.NItems()
	s.check(allRows > gameRows, "All shows more rows than Game (%d > %d)", allRows, gameRows)

	// Switching category from the bottom of a long list must repaint
	// immediately; a viewport left past the end of the shorter list is the
	// reported "blank until scrolling" failure.
	blankCategory := ""
	for _, b := range mw.categoryButtons {
		if adj := mw.titleScroll.VAdjustment(); adj != nil && adj.Upper() > adj.PageSize() {
			adj.SetValue(adj.Upper() - adj.PageSize())
			uiSmokePump()
		}
		b.SetActive(true)
		uiSmokeSettle()
		if len(mw.boundChecks) == 0 {
			blankCategory = b.Label()
			break
		}
		if adj := mw.titleScroll.VAdjustment(); adj != nil && adj.Value() != 0 {
			blankCategory = b.Label() + " (scrolled to " + fmt.Sprint(adj.Value()) + ")"
			break
		}
	}
	s.check(blankCategory == "", "every category repaints from the top without scrolling (blank: %q)", blankCategory)
	all.SetActive(true)
	uiSmokePump()

	// --- the main window now carries the same nav bar as every other window ---
	headerName := "none"
	if mw.headerBar != nil {
		headerName = gtk.BaseWidget(mw.headerBar).CSSName()
	}
	s.check(headerName == "headerbar" && gtk.BaseWidget(mw.headerBar).IsAncestor(mw.window),
		"main window uses the libadwaita nav bar (got %q)", headerName)
	s.check(mw.headerBar != nil && gtk.BaseWidget(mw.menuButton).IsAncestor(mw.headerBar), "primary menu button lives in the nav bar")
	s.check(mw.titleStatusPage != nil && !mw.titleStatusPage.Visible(), "empty-state page is hidden while titles are listed")
	uiSmokeCheckLayout(s, mw, "default")
	if raw := mw.menuButton.MenuModel(); raw != nil {
		model := gio.BaseMenuModel(raw)
		s.check(model.NItems() == 2, "menu model has Tools and Settings sections (got %d)", model.NItems())
		section := model.ItemLink(1, "section")
		s.check(section != nil && gio.BaseMenuModel(section).NItems() == 1, "settings section holds a single flat item")
	} else {
		s.check(false, "menu button carries a menu model")
	}

	// --- search entry is the modern GtkSearchEntry ---
	s.check(uiSmokeHasClass(mw.searchEntry, "search"), "search entry uses the GtkSearchEntry style")

	// --- category switcher is a linked segmented group ---
	s.check(mw.categoryButtons[0].Parent() != nil && uiSmokeHasClass(mw.categoryButtons[0].Parent(), "linked"), "category buttons form a linked group")

	// --- donation bar sits above the region/action bar, as in GTK3 ---
	donationIdx, actionIdx := -1, -1
	paned, _ := mw.toolbarView.Content().(*gtk.Paned)
	s.check(paned != nil, "the nav bar's content is the paned layout")
	if paned != nil {
		if mainBox, ok := paned.EndChild().(*gtk.Box); ok {
			for i, child := range uiSmokeChildren(mainBox) {
				if w, ok := child.(*gtk.Box); ok {
					switch {
					case w.HasCSSClass("bottom-bar"):
						actionIdx = i
					case w.HasCSSClass("gratitude-footer"):
						donationIdx = i
					}
				}
			}
		}
	}
	s.check(donationIdx >= 0 && actionIdx >= 0 && donationIdx < actionIdx,
		"donation bar (%d) sits above the region/action bar (%d)", donationIdx, actionIdx)

	// The bottom bar must span the content area rather than reading as a floating
	// card whose fill stops where its children stop.
	s.check(mw.bottomBar != nil && uiSmokeHasClass(mw.bottomBar, "bottom-bar"),
		"bottom bar is a plain box we style ourselves, not a GtkActionBar")
	// Both are direct children of the same box, so they must fill the same width;
	// a bar narrower than its sibling is the "background cuts off" symptom.
	if mw.bottomBar != nil && mw.donationBar != nil {
		barWidth, footerWidth := mw.bottomBar.Width(), mw.donationBar.Width()
		s.check(barWidth > 0 && barWidth >= footerWidth-1,
			"bottom bar background spans the content area (%d vs footer %d)", barWidth, footerWidth)
	}

	// --- footer copy is two stacked labels, not one wrapped blob ---
	s.check(mw.donationBar.Spacing() == DONATION_BAR_SPACING, "footer gives the copy room beside the button (%d)", mw.donationBar.Spacing())
	textBox, ok := uiSmokeChildren(mw.donationBar)[0].(*gtk.Box)
	s.check(ok, "footer starts with a copy block")
	s.check(ok && uiSmokeLabelCount(textBox) == 2, "footer splits headline and subline into separate labels")
	s.check(mw.donationSubLabel != nil && mw.donationSubLabel.Text() != "", "footer subline carries the call to action")
	s.check(strings.Contains(mw.donationLabel.Text(), "Games worth $40+ are free here"), "footer headline keeps its copy (%q)", mw.donationLabel.Text())
	s.check(strings.Contains(mw.donationSubLabel.Text(), "A coffee keeps them coming"), "footer subline keeps its copy (%q)", mw.donationSubLabel.Text())

	mw.setDonationBarVisible(false)
	s.check(!mw.donationBar.Visible(), "donation bar hides")
	mw.setDonationBarVisible(true)
	s.check(mw.donationBar.Visible(), "donation bar shows")

	// --- the same invariants must hold at every window size ---
	for _, size := range []struct{ w, h int }{{1024, 600}, {1600, 900}, {1280, 720}, {900, 500}, {880, 600}, {700, 480}, {MIN_WINDOW_WIDTH, MIN_WINDOW_HEIGHT}} {
		mw.window.SetDefaultSize(size.w, size.h)
		uiSmokeSettle()
		actualW, actualH := mw.window.Width(), mw.window.Height()
		label := fmt.Sprintf("%dx%d (got %dx%d)", size.w, size.h, actualW, actualH)
		s.check(actualW > 0 && actualH > 0, "%s: window has a real allocation", label)
		uiSmokeCheckLayout(s, mw, label)
		uiSmokeCheckContentFits(s, mw, label)
	}
	mw.window.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
	uiSmokeSettle()

	// --- region and search filters ---
	mw.currentCategory = wiiudownloader.TITLE_CATEGORY_ALL
	mw.currentRegion = wiiudownloader.MCP_REGION_USA
	mw.refreshTitleFilter()
	uiSmokePump()
	usaRows := mw.titleSortModel.NItems()
	s.check(usaRows > 0 && usaRows < allRows, "region filter narrows the list (%d of %d)", usaRows, allRows)

	mw.currentRegion = wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA
	mw.lastSearchText = "Mario"
	mw.refreshTitleFilter()
	uiSmokePump()
	searchRows := mw.titleSortModel.NItems()
	s.check(searchRows > 0 && searchRows < allRows, "search filter narrows the list (%d of %d)", searchRows, allRows)
	mw.lastSearchText = ""
	mw.refreshTitleFilter()
	uiSmokePump()
	s.check(mw.titleSortModel.NItems() == allRows, "clearing the search restores every row")

	// --- queue round trip driven from the title list ---
	games := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME)
	first, second := games[0], games[1]
	firstKey := rowKeyForTitleID(first.TitleID)

	mw.setQueueMembership([]string{firstKey}, true)
	uiSmokePump()
	s.check(mw.queuePane.GetTitleQueueSize() == 1, "title list queues a row (queue=%d)", mw.queuePane.GetTitleQueueSize())
	s.check(mw.titleRows[firstKey].inQueue, "row state follows the queue")
	bound, consistent := uiSmokeCheckboxSweep(mw)
	s.check(bound > 0, "title list realises checkboxes for visible rows (%d bound)", bound)
	s.check(consistent, "every on-screen checkbox matches the row it is bound to")

	rowHeight, _ := uiSmokeScan(mw.queuePane.columnView)
	s.check(rowHeight > 0 && rowHeight <= QUEUE_ROW_MAX_HEIGHT, "queue rows keep a sane height (%d px, max %d)", rowHeight, QUEUE_ROW_MAX_HEIGHT)

	mw.setQueueMembership([]string{firstKey}, false)
	uiSmokePump()
	s.check(mw.queuePane.GetTitleQueueSize() == 0, "unqueueing from the title list empties the queue")

	// --- keyboard toggle ---
	mw.focusTitleList()
	uiSmokePump()
	toggled := mw.toggleQueueFromKeyboard()
	uiSmokePump()
	s.check(toggled && mw.queuePane.GetTitleQueueSize() == 1, "keyboard toggle queues the selected row")

	// --- Clear empties the queue and the table together ---
	mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{first, second})
	uiSmokePump()
	before := mw.queuePane.rows.NItems()
	mw.queuePane.Clear()
	uiSmokePump()
	s.check(before > 0 && mw.queuePane.GetTitleQueueSize() == 0 && mw.queuePane.rows.NItems() == 0,
		"Clear empties queue and table (%d rows -> %d)", before, mw.queuePane.rows.NItems())

	// --- "Remove Selected" sensitivity follows the gate *and* the selection ---
	mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{first, second})
	uiSmokeSettle()
	removeButton := mw.queuePane.removeFromQueueButton
	mw.queuePane.selection.UnselectAll()
	uiSmokeSettle()
	s.check(!removeButton.Sensitive(), "Remove Selected starts disabled with nothing selected")

	mw.queuePane.selection.SelectItem(0, false)
	uiSmokeSettle()
	s.check(removeButton.Sensitive(), "Remove Selected enables once a row is selected")

	// Repainting one cell (a size landing, or a run state changing) must not
	// drop the user's selection: that would disable Remove Selected under them
	// mid-download, and it is what made the row-removal check below flaky.
	mw.queuePane.SetTitleSize(first.TitleID, QUEUE_SMOKE_SIZE_BYTES)
	uiSmokeSettle()
	s.check(mw.queuePane.selection.Selection().Size() == 1,
		"repainting a row keeps the selection (%d selected)", mw.queuePane.selection.Selection().Size())

	mw.setDownloadControlsSensitive(false)
	uiSmokeSettle()
	s.check(!removeButton.Sensitive(), "Remove Selected disables while a download runs")

	// The reported bug: finishing a download re-enabled the button outright, even
	// with an empty selection.
	mw.queuePane.selection.UnselectAll()
	mw.setDownloadControlsSensitive(true)
	uiSmokeSettle()
	s.check(!removeButton.Sensitive(), "re-enabling the pane keeps Remove Selected disabled with no selection")

	mw.queuePane.selection.SelectItem(0, false)
	uiSmokeSettle()
	mw.queuePane.RemoveTitles(mw.queuePane.selectedTitleIDs())
	uiSmokeSettle()
	s.check(mw.queuePane.GetTitleQueueSize() == 1, "Remove Selected drops exactly the selected row (queue=%d)", mw.queuePane.GetTitleQueueSize())
	s.check(!removeButton.Sensitive(), "removing the selection disables Remove Selected again")
	mw.queuePane.Clear()
	uiSmokeSettle()

	// --- the queue buttons clear the window's rounded bottom corner ---
	if parent := gtk.BaseWidget(removeButton).Parent(); parent != nil {
		_, buttonY, okButton := uiSmokeWidgetPoint(parent, mw.window)
		buttonBottom := buttonY + float32(gtk.BaseWidget(parent).Height())
		windowBottom := float32(mw.window.Height())
		s.check(okButton && buttonBottom <= windowBottom-float32(QUEUE_CORNER_CLEARANCE)+1,
			"queue buttons clear the rounded window corner (bottom %.0f, window %.0f)", buttonBottom, windowBottom)
	}

	// --- a restored queue must show every size as it arrives, row by row ---
	restored := []wiiudownloader.TitleEntry{games[20], games[21], games[22]}
	sizes := []uint64{QUEUE_SMOKE_SIZE_BYTES, QUEUE_SMOKE_SIZE_BYTES * 4, QUEUE_SMOKE_SIZE_BYTES * 9}
	for _, entry := range restored {
		mw.queuePane.SetTitleLoadingNoUpdate(entry.TitleID)
	}
	mw.queuePane.AddTitles(restored)
	mw.updateTitlesInQueue() // mirrors restorePersistedQueue()
	uiSmokeSettle()

	missing := 0
	for _, entry := range restored {
		row := mw.queuePane.rowData[rowKeyForTitleID(entry.TitleID)]
		if row == nil || row.size != "loading..." {
			missing++
		}
	}
	s.check(missing == 0, "all %d restored rows wait on loading... until sizes are known (%d wrong)", len(restored), missing)

	for i, entry := range restored {
		mw.queuePane.SetTitleSize(entry.TitleID, sizes[i])
	}
	uiSmokeSettle()
	_, texts := uiSmokeScan(mw.queuePane.columnView)
	for i := range restored {
		want := formatBytes(sizes[i])
		s.check(uiSmokeHasText(texts, want), "restored row %d shows the size once fetched (%s)", i, want)
	}
	s.check(!uiSmokeHasText(texts, "loading..."), "no queue row stays stuck on loading... (%v)", texts)

	// --- a size landing while an Update is still queued must survive it ---
	mw.queuePane.Clear()
	uiSmokePump()
	late := games[30]
	mw.queuePane.SetTitleLoadingNoUpdate(late.TitleID)
	mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{late})       // queues an Update holding "loading..."
	mw.queuePane.SetTitleSize(late.TitleID, QUEUE_SMOKE_SIZE_BYTES) // lands while that update is still queued
	uiSmokeSettle()
	_, lateTexts := uiSmokeScan(mw.queuePane.columnView)
	s.check(uiSmokeHasText(lateTexts, formatBytes(QUEUE_SMOKE_SIZE_BYTES)), "a size that lands mid-update is not clobbered by it (%v)", lateTexts)
	mw.queuePane.Clear()
	uiSmokePump()

	// --- typing in the search entry filters after the debounce ---
	mw.searchEntry.SetText("Mario")
	uiSmokeWait(SEARCH_DEBOUNCE_DELAY + 200*time.Millisecond)
	s.check(mw.lastSearchText == "Mario", "search entry text reaches the filter (%q)", mw.lastSearchText)
	s.check(mw.titleSortModel.NItems() > 0 && mw.titleSortModel.NItems() < allRows, "search entry narrows the list (%d of %d)", mw.titleSortModel.NItems(), allRows)
	mw.searchEntry.SetText("")
	uiSmokeWait(SEARCH_DEBOUNCE_DELAY + 200*time.Millisecond)
	s.check(mw.titleSortModel.NItems() == allRows, "clearing the search entry restores every row")

	// --- region checkboxes drive the filter and the config ---
	mw.europeRegionCheckbox.SetActive(false)
	uiSmokePump()
	s.check(mw.currentRegion&wiiudownloader.MCP_REGION_EUROPE == 0, "unchecking Europe drops it from the region mask")
	s.check(mw.europeRegionCheckbox.Active() == false, "the Europe checkbox stays unchecked")
	mw.europeRegionCheckbox.SetActive(true)
	uiSmokePump()
	s.check(mw.currentRegion&wiiudownloader.MCP_REGION_EUROPE != 0, "re-checking Europe restores it")

	// --- control sensitivity ---
	mw.setDownloadControlsSensitive(false)
	mw.setDownloadControlsSensitive(true)
	s.check(true, "download controls toggle without criticals")

	// --- dialogs ---
	base := uiSmokeToplevelCount()

	alert := showAlert(mw.window, WINDOW_TITLE_PREFIX+"Smoke Alert", "hello")
	uiSmokePump()
	// An AdwAlertDialog renders inside its parent window, so it adds no toplevel.
	s.check(alert != nil && gtk.BaseWidget(alert).Visible(), "alert dialog presents")

	mw.showAddByTitleIDDialog()
	uiSmokePump()
	s.check(uiSmokeToplevelCount() > base, "add-by-title-id dialog presents")

	showVersionSelectionDialog(mw.window, first, func(int) {})
	uiSmokePump()
	s.check(uiSmokeToplevelCount() > base, "version picker presents")

	mw.showErrorsDialog([]DownloadError{{
		Title:     first.Name,
		Error:     "boom",
		TidStr:    firstKey,
		ErrorType: "Content Download",
	}})
	uiSmokePump()
	s.check(uiSmokeToplevelCount() > base, "download-errors dialog presents")

	mw.showRelatedTitlesDialog([]wiiudownloader.TitleEntry{first}, []wiiudownloader.TitleEntry{second}, func([]wiiudownloader.TitleEntry) {})
	uiSmokePump()
	s.check(uiSmokeToplevelCount() > base, "related-content dialog presents")

	mw.showDecryptErrorsDialog([]DownloadError{{Title: "Some Game", Error: "not decrypted"}})
	uiSmokePump()
	s.check(uiSmokeToplevelCount() > base, "decryption-errors overview presents")

	// --- manual decryption runs as a batch and reports failures once ---
	if decryptRoot, err := os.MkdirTemp("", "wiiu-decrypt-smoke"); err == nil {
		mw.runDecryptContents([]string{
			filepath.Join(decryptRoot, "missing-a"),
			filepath.Join(decryptRoot, "missing-b"),
		})
		s.check(uiSmokeWaitForWindowTitle(WINDOW_TITLE_PREFIX+"Decryption Errors", 10*time.Second),
			"a decryption batch reports every failed folder in one overview")
		s.check(!uiSmokeHasWindowTitle(WINDOW_TITLE_PREFIX+"Download Complete"),
			"a decryption batch never raises the Download Complete dialog")
		if mw.progressWindow != nil {
			mw.progressWindow.Window.Destroy()
		}
		os.RemoveAll(decryptRoot)
		uiSmokePump()
	}

	// --- download progress window ---
	if pw, err := createProgressWindow(mw.window); err != nil {
		s.check(false, "progress window builds: %v", err)
	} else {
		uiSmokePump()
		s.check(pw.bar != nil && pw.pauseButton != nil && pw.cancelButton != nil, "progress window has progress bar, pause and cancel controls")
		controls := gtk.BaseWidget(gtk.BaseWidget(pw.pauseButton).Parent())
		s.check(controls != nil && controls.HAlign() == gtk.AlignEnd, "progress controls are right aligned (got %v)", controls.HAlign())
		pw.setFraction(math.NaN())
		s.check(pw.bar.Fraction() == 0, "a NaN fraction is clamped to 0 (got %v)", pw.bar.Fraction())
		pw.setFraction(1.5)
		s.check(pw.bar.Fraction() == 1, "a fraction above 1 is clamped (got %v)", pw.bar.Fraction())
		pw.setFraction(0.5)
		s.check(pw.bar.Fraction() == 0.5, "a valid fraction passes through (got %v)", pw.bar.Fraction())
		pw.Window.Destroy()
		uiSmokePump()
	}

	// --- inline download UI (the experiment) and its pluggable fallback ---
	inlineUI := mw.newDownloadUI(&Config{UseInlineDownloadUI: true})
	inline, isInline := inlineUI.(*inlineDownloadUI)
	s.check(isInline, "the inline download UI is selected when configured")
	if isInline {
		s.check(mw.progressWindow != nil, "the progress window is still built as the state owner")
		mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{first, second})
		uiSmokeSettle()

		s.check(!mw.queuePane.statusColumn.Visible(),
			"the Status column stays out of the narrow pane until a run starts")

		mw.queuePane.BeginInlineRun()
		uiSmokeSettle()
		s.check(mw.queuePane.runBar.Visible(), "the inline run bar shows while a run is going")
		s.check(mw.queuePane.statusColumn.Visible(), "the Status column appears while a run is going")

		s.check(!mw.progressWindow.Window.Visible(), "the progress window stays hidden in inline mode")

		// The download core reports from a worker goroutine and GTK is not
		// thread-safe, so every run-bar write has to wait for the main loop
		// rather than reaching the widget in place.
		mw.queuePane.SetInlineProgress("not yet", 0, "")
		s.check(mw.queuePane.runBarLabel.Text() != "not yet",
			"an inline run-bar write does not touch GTK off the main loop")
		uiSmokeSettle()
		s.check(mw.queuePane.runBarLabel.Text() == "not yet",
			"the marshalled inline write lands once the loop runs")

		// The real calling pattern, all of it off-thread.
		reported := make(chan struct{})
		go func() {
			defer close(reported)
			inline.ResetTotalsAndErrors()
			inline.SetQueueProgress(1, 2)
			inline.SetTitleState(first.TitleID, queueStateDownloading)
			inline.SetGameTitle("Mario Kart 8")
			inline.SetDownloadSize(2000)
			inline.SetTotalDownloadedForFile("f.bin", 200)
			inline.UpdateDownloadProgress(300, "f.bin")
		}()
		<-reported
		uiSmokeSettle()

		// 200 + 300 of 2000 bytes for the title on screen, and the second of two
		// queued titles: the bar follows the title, the count follows the queue.
		s.check(mw.queuePane.runBarBar.Fraction() == 0.25,
			"the inline bar tracks the title being downloaded (got %v)", mw.queuePane.runBarBar.Fraction())
		s.check(mw.queuePane.runBarBar.Text() == "25%",
			"the inline bar is labelled with that title's percentage (got %q)", mw.queuePane.runBarBar.Text())
		s.check(mw.queuePane.runBarCount.Text() == "2/2",
			"the run bar shows the queue position (got %q)", mw.queuePane.runBarCount.Text())
		s.check(mw.queuePane.rowData[rowKeyForTitleID(first.TitleID)].state == queueStateDownloading,
			"the queue pane shows each title's state")
		s.check(mw.queuePane.runBarLabel.Text() == "Mario Kart 8",
			"the run bar names the title on screen now (got %q)", mw.queuePane.runBarLabel.Text())

		// The sidebar can be dragged down to QUEUE_PANE_MIN_WIDTH, so the fully
		// populated run bar has to fit there without a control being squeezed out.
		minimum, _, _, _ := gtk.BaseWidget(mw.queuePane.runBar).Measure(gtk.OrientationHorizontal, -1)
		s.check(minimum > 0 && minimum <= QUEUE_PANE_MIN_WIDTH,
			"the populated run bar fits the narrowest queue pane (%d <= %d px)", minimum, QUEUE_PANE_MIN_WIDTH)
		s.check(strings.Contains(mw.queuePane.runBarDetail.Text(), "500 B"),
			"the run bar reports the bytes fetched so far (got %q)", mw.queuePane.runBarDetail.Text())

		// Rate and ETA need two samples an interval apart, which is why the
		// window only publishes them once the average is known.
		time.Sleep(MIN_SAMPLE_INTERVAL + 50*time.Millisecond)
		go inline.UpdateDownloadProgress(300, "f.bin")
		uiSmokeSettle()
		s.check(strings.Contains(mw.queuePane.runBarDetail.Text(), "/s") &&
			strings.Contains(mw.queuePane.runBarDetail.Text(), "left"),
			"the run bar reports speed and time left (got %q)", mw.queuePane.runBarDetail.Text())

		// GTK logs an invalid "valuenow" for NaN, so the inline bar needs the
		// same clamp the progress window got. A 0/0 queue only blanks the count.
		mw.queuePane.SetInlineProgress("Zero", math.NaN(), "")
		mw.queuePane.SetInlineQueueProgress(0, 0)
		uiSmokeSettle()
		s.check(mw.queuePane.runBarBar.Fraction() == 0 && mw.queuePane.runBarBar.Text() == "0%",
			"a NaN inline fraction is clamped (got %v %q)", mw.queuePane.runBarBar.Fraction(), mw.queuePane.runBarBar.Text())
		s.check(mw.queuePane.runBarCount.Text() == "",
			"a 0/0 queue position blanks the count (got %q)", mw.queuePane.runBarCount.Text())

		inline.TogglePaused()
		uiSmokeSettle()
		s.check(mw.progressWindow.paused, "the inline pause control pauses the run")
		s.check(mw.queuePane.runPauseButton.IconName() == "media-playback-start-symbolic",
			"the inline pause control flips to Resume (%q)", mw.queuePane.runPauseButton.IconName())
		inline.TogglePaused()
		uiSmokeSettle()
		s.check(!mw.progressWindow.paused, "the inline pause control resumes the run")

		inline.SetTitleState(first.TitleID, queueStateDone)
		inline.SetTitleState(second.TitleID, queueStateFailed)
		uiSmokeSettle()
		_, statusTexts := uiSmokeScan(mw.queuePane.columnView)
		s.check(uiSmokeHasText(statusTexts, string(queueStateDone)) && uiSmokeHasText(statusTexts, string(queueStateFailed)),
			"finished and failed titles both report their state (%v)", statusTexts)

		// A download removes its title from the queue, and that rebuilds every
		// row: the run states have to survive it.
		mw.queuePane.Update(true)
		uiSmokeSettle()
		s.check(mw.queuePane.rowData[rowKeyForTitleID(first.TitleID)].state == queueStateDone,
			"a queue rebuild keeps each row's run state")

		// Cancelling once disables the run controls, exactly like the window.
		mw.queuePane.runCancelButton.Emit("clicked")
		uiSmokeSettle()
		s.check(mw.progressWindow.Cancelled(), "the inline cancel control cancels the run")
		s.check(!mw.queuePane.runCancelButton.Sensitive() && !mw.queuePane.runPauseButton.Sensitive(),
			"the inline cancel control disables the run controls once pressed")

		inline.Hide()
		uiSmokeSettle()
		s.check(!mw.queuePane.runBar.Visible(), "the inline run bar hides when the run ends")
		s.check(!mw.queuePane.statusColumn.Visible(), "the Status column goes away with the run")
		mw.queuePane.Clear()
		uiSmokeSettle()
	}

	// The compact layout hides the queue pane, so there would be nothing to
	// report to: the run has to fall back to the window.
	mw.queuePane.GetContainer().SetVisible(false)
	narrowUI := mw.newDownloadUI(&Config{UseInlineDownloadUI: true})
	_, narrowInline := narrowUI.(*inlineDownloadUI)
	s.check(!narrowInline && narrowUI != nil, "a hidden queue pane falls back to the progress window")
	if narrowUI != nil {
		narrowUI.Hide()
	}
	mw.queuePane.GetContainer().SetVisible(true)
	uiSmokePump()

	// Flipping the flag must hand the run back to the progress window, which is
	// still whole.
	windowUI := mw.newDownloadUI(&Config{UseInlineDownloadUI: false})
	_, stillInline := windowUI.(*inlineDownloadUI)
	s.check(!stillInline && windowUI != nil, "switching the flag off uses the progress window again")
	if windowUI != nil {
		windowUI.Present()
		uiSmokePump()
		s.check(mw.progressWindow.Window.Visible(), "the progress window still presents for a run")
		windowUI.Hide()
		uiSmokePump()
	}

	uiSmokeTitlePrefixes(s, "with every dialog open")

	// --- settings window ---
	if cw, err := NewConfigWindow(cfg); err != nil {
		s.check(false, "settings window builds: %v", err)
	} else {
		s.check(cw != nil && cw.Window != nil, "settings window builds")
		// libadwaita really does put its own "title" class on every
		// preferences-row title, which is why the stylesheet guard matters. The
		// measured height is reported so a size regression is visible in the run.
		cw.Window.Present()
		uiSmokeSettle()
		if titleLabel := uiSmokeFindByClass(cw.Window, "title"); titleLabel != nil {
			_, titleHeight, _, _ := gtk.BaseWidget(titleLabel).Measure(gtk.OrientationVertical, -1)
			s.check(titleHeight > 0, "settings row titles are libadwaita's own .title widgets (%d px tall, base size)", titleHeight)
		} else {
			s.check(false, "settings row titles carry libadwaita's .title class")
		}
		cw.Window.Destroy()
	}

	// --- setup assistant ---
	if assistant, err := NewInitialSetupAssistantWindow(cfg); err != nil {
		s.check(false, "setup assistant builds: %v", err)
	} else {
		s.check(assistant != nil, "setup assistant builds")
		assistant.window.Present()
		uiSmokeSettle()
		// Observed before anything calls setPage: the wizard has to open already
		// standing on step 1. It used to show the right page but leave the sidebar
		// unselected and the buttons in their stock state until the first Back
		// click.
		openedOnFirst := assistant.stepList.SelectedRow() != nil && assistant.stepList.SelectedRow().Index() == 0
		s.check(openedOnFirst && !assistant.backButton.Sensitive(), "wizard opens initialised on step 1 (not only after a Back click)")
		pages := len(assistant.pageTitles)
		s.check(pages > 0, "setup assistant has pages (%d)", pages)
		headerName := gtk.BaseWidget(assistant.headerBar).CSSName()
		s.check(headerName == "headerbar", "setup assistant uses a libadwaita nav bar (got %q)", headerName)
		badTitles := 0
		buttonIssue := ""
		for page := 0; page < pages; page++ {
			assistant.setPage(page)
			uiSmokeSettle()
			title := assistant.window.Title()
			if !strings.HasPrefix(title, WINDOW_TITLE_PREFIX) {
				badTitles++
			}
			// The button row must be identical on every step.
			wantPrimary := "Next"
			if page == pages-1 {
				wantPrimary = "Finish"
			}
			switch {
			case !assistant.skipButton.Visible() || !assistant.backButton.Visible() || !assistant.nextButton.Visible():
				buttonIssue = fmt.Sprintf("step %d hides a button", page+1)
			case assistant.nextButton.Label() != wantPrimary:
				buttonIssue = fmt.Sprintf("step %d primary reads %q, want %q", page+1, assistant.nextButton.Label(), wantPrimary)
			case page == 0 && assistant.backButton.Sensitive():
				buttonIssue = "Back is enabled on the first step"
			case page > 0 && !assistant.backButton.Sensitive():
				buttonIssue = fmt.Sprintf("Back is disabled on step %d", page+1)
			}
		}
		s.check(badTitles == 0, "every setup assistant step keeps the app title prefix (%d bad of %d)", badTitles, pages)
		s.check(buttonIssue == "", "every setup step keeps the same button row (%s)", buttonIssue)

		// Regions page: Next follows the regions, and only while it is visible.
		assistant.setPage(1)
		uiSmokeSettle()
		regionChecks := uiSmokeSetupChecks(assistant.stack.VisibleChild())
		s.check(len(regionChecks) == 3, "regions step exposes three checkboxes (%d)", len(regionChecks))
		if len(regionChecks) == 3 {
			for _, check := range regionChecks {
				check.SetActive(false)
			}
			uiSmokeSettle()
			s.check(!assistant.nextButton.Sensitive(), "regions step blocks Next with no region selected")
			for _, check := range regionChecks {
				check.SetActive(true)
			}
			uiSmokeSettle()
			s.check(assistant.nextButton.Sensitive(), "reselected regions re-enable Next")
		}

		// Platforms page: a toggle here must not reach across and disable Next.
		assistant.setPage(2)
		uiSmokeSettle()
		platforms := uiSmokeSetupChecks(assistant.stack.VisibleChild())
		s.check(len(platforms) == 2, "platforms step exposes two checkboxes (%d)", len(platforms))
		if len(platforms) == 2 {
			platforms[0].SetActive(false)
			uiSmokeSettle()
			s.check(assistant.nextButton.Sensitive(), "deselecting a platform keeps Next usable while one remains")
			platforms[1].SetActive(false)
			uiSmokeSettle()
			s.check(!assistant.nextButton.Sensitive(), "deselecting every platform disables Next")
			platforms[0].SetActive(true)
			platforms[1].SetActive(true)
			uiSmokeSettle()
			s.check(assistant.nextButton.Sensitive(), "reselected platforms re-enable Next")
		}

		// The regression itself: with the platforms page on screen, toggling a
		// region used to disable the Next button of the page being looked at.
		if len(regionChecks) == 3 {
			regionChecks[0].SetActive(false)
			uiSmokeSettle()
			s.check(assistant.nextButton.Sensitive(), "a region toggle off-screen leaves this page's Next alone")
			regionChecks[0].SetActive(true)
			uiSmokeSettle()
		}

		// Storage never blocks: an unset path is asked for at the first download.
		assistant.setPage(3)
		uiSmokeSettle()
		s.check(assistant.nextButton.Sensitive(), "storage step continues without a chosen path")

		uiSmokeTitlePrefixes(s, "with the assistant open")
		assistant.window.Destroy()
	}
	uiSmokePump()
	uiSmokePump()

	// --- dark mode round trip ---
	setDarkTheme(true)
	setDarkTheme(false)
	s.check(true, "dark mode toggles without criticals")

	fmt.Printf("UI smoke: %d passed, %d failed\n", s.pass, s.fail)
	if s.fail > 0 {
		return 1
	}
	return 0
}
