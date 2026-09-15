package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
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

func uiSmokeMenuLabel(model *gio.MenuModel, index int) string {
	return uiSmokeMenuAttribute(model, index, gio.MENU_ATTRIBUTE_LABEL)
}

func uiSmokeMenuAction(model *gio.MenuModel, index int) string {
	return uiSmokeMenuAttribute(model, index, gio.MENU_ATTRIBUTE_ACTION)
}

func uiSmokeMenuAttribute(model *gio.MenuModel, index int, attribute string) string {
	value := model.ItemAttributeValue(index, attribute, glib.NewVariantType("s"))
	if value == nil {
		return ""
	}
	return value.String()
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
type uiSmokeForwardHandler struct {
	count *int
}

func (h *uiSmokeForwardHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *uiSmokeForwardHandler) Handle(context.Context, slog.Record) error {
	*h.count++
	return nil
}

func (h *uiSmokeForwardHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *uiSmokeForwardHandler) WithGroup(string) slog.Handler { return h }

func uiSmokeRendererFallback(s *uiSmoke) {
	plans := []struct {
		name  string
		user  string
		state string
		want  rendererPlan
	}{
		{"fresh launch tries GL and leaves a marker", "", "", rendererPlan{persist: RENDERER_STATE_ATTEMPTING, watch: true}},
		{"a healthy previous launch tries GL again", "", RENDERER_STATE_HEALTHY, rendererPlan{persist: RENDERER_STATE_ATTEMPTING, watch: true}},
		{"a launch that never reached a frame falls back for good", "", RENDERER_STATE_ATTEMPTING, rendererPlan{setEnv: RENDERER_CAIRO, persist: RENDERER_STATE_CAIRO}},
		{"an existing fallback is kept", "", RENDERER_STATE_CAIRO, rendererPlan{setEnv: RENDERER_CAIRO}},
		{"the user's cairo is left alone", "  cairo ", RENDERER_STATE_ATTEMPTING, rendererPlan{}},
		{"the user's gl is left alone", "gl", RENDERER_STATE_ATTEMPTING, rendererPlan{}},
	}
	for _, c := range plans {
		got := planRendererLaunch(c.user, c.state)
		s.check(got == c.want, "renderer plan: %s (%+v)", c.name, got)
	}

	dir, err := os.MkdirTemp("", "wiiu-renderer")
	if err != nil {
		s.check(false, "renderer state temp dir (%v)", err)
		return
	}
	defer os.RemoveAll(dir)
	path := rendererStatePathFor(dir)
	s.check(readRendererStateFile(path) == "", "a missing renderer state reads empty")
	s.check(writeRendererStateFile(path, RENDERER_STATE_ATTEMPTING) == nil && readRendererStateFile(path) == RENDERER_STATE_ATTEMPTING,
		"the renderer state round trips")
	s.check(filepath.Base(filepath.Dir(path)) == WIIUDOWNLOADER_CONFIG_DIR,
		"the renderer state sits in the app's config directory (%s)", path)

	s.check(markRendererHealthyFile(path) && readRendererStateFile(path) == RENDERER_STATE_HEALTHY,
		"a launch that drew frames is recorded healthy")
	if writeRendererStateFile(path, RENDERER_STATE_CAIRO) == nil {
		markRendererHealthyFile(path)
		s.check(readRendererStateFile(path) == RENDERER_STATE_CAIRO,
			"a launch already on cairo never rewrites the state (%s)", readRendererStateFile(path))
	}

	failures := []struct {
		name            string
		domain, message string
		want            bool
	}{
		{"the MSYS2 shader failure is caught", "Gsk", "Failed to load shader program: Compilation failure in shader.", true},
		{"the domain match ignores case", "gsk", "Failed to load shader program: x", true},
		{"a Gtk message is not a renderer failure", "Gtk", "Failed to load shader program: x", false},
		{"GTK's own realize fallback is not a renderer failure", "Gsk", "Failed to realize renderer 'GskGLRenderer' for surface", false},
		{"an empty message is not a renderer failure", "Gsk", "", false},
	}
	for _, c := range failures {
		s.check(isGLRenderFailure(c.domain, c.message) == c.want, "GL failure predicate: %s", c.name)
	}

	forwarded, fired, derivedFired := 0, 0, 0
	watcher := &glFailureWatcher{
		next:      &uiSmokeForwardHandler{count: &forwarded},
		onFailure: func() { fired++ },
		state:     &glFailureWatchState{},
	}
	emit := func(h slog.Handler, domain, message string) {
		record := slog.NewRecord(time.Now(), slog.LevelError, message, 0)
		record.AddAttrs(slog.String(GLIB_LOG_DOMAIN_ATTR, domain))
		_ = h.Handle(context.Background(), record)
	}
	emit(watcher, "Gsk", "Failed to load shader program: first")
	emit(watcher, "Gsk", "Failed to load shader program: second")
	emit(watcher, "Gtk", "Failed to load shader program: other domain")
	emit(watcher, "Gsk", "unrelated message")
	s.check(fired == 1, "the first GL failure restarts once (%d)", fired)
	s.check(forwarded == 4, "every record still reaches the wrapped handler (%d)", forwarded)

	derived := watcher.WithAttrs([]slog.Attr{slog.String("k", "v")}).(*glFailureWatcher)
	derived.onFailure = func() { derivedFired++ }
	emit(derived, "Gsk", "Failed to load shader program: third")
	s.check(derivedFired == 0, "a derived handler cannot restart a second time (%d)", derivedFired)

	os.Setenv(RENDERER_ENV_VAR, "gl")
	restartEnv := rendererEnvWithoutOverride()
	os.Unsetenv(RENDERER_ENV_VAR)
	inherited := 0
	for _, entry := range restartEnv {
		if strings.HasPrefix(entry, RENDERER_ENV_VAR+"=") {
			inherited++
		}
	}
	s.check(inherited == 0, "the restart drops an inherited override (%d left)", inherited)
	s.check(uiTestModeActive(), "a harness run is recognised, so it arms nothing and writes no state")

	glibFired := 0
	previous := slog.Default()
	installGLFailureFallback(func() { glibFired++ })
	emitGLib := func(domain, message string, level glib.LogLevelFlags) {
		dict := glib.NewVariantDict(nil)
		dict.InsertValue("MESSAGE", glib.NewVariantString(message))
		glib.LogVariant(domain, level, dict.End())
	}
	emitGLib("Gsk", "Failed to load shader program: first", glib.LogLevelCritical)
	emitGLib("Gsk", "Failed to load shader program: second", glib.LogLevelCritical)
	emitGLib("Gtk", "Failed to load shader program: other domain", glib.LogLevelWarning)
	emitGLib("Gsk", "some other gsk message", glib.LogLevelCritical)
	slog.SetDefault(previous)
	s.check(glibFired == 1, "a real Gsk shader failure triggers one fallback (%d)", glibFired)
}

func uiSmokeStylesheet(s *uiSmoke) {
	provider := gtk.NewCSSProvider()
	errors := 0
	provider.ConnectParsingError(func(_ *gtk.CSSSection, err error) {
		errors++
		if errors <= 3 {
			fmt.Printf("  [css] %v\n", err)
		}
	})
	provider.LoadFromString(styleCSS)
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

func uiSmokeVisibleLinkTitles(root gtk.Widgetter) map[string]string {
	rows := map[string]string{}
	for _, row := range uiSmokeRows(root) {
		if !gtk.BaseWidget(row).Visible() {
			continue
		}
		uri, _ := row.ObjectProperty("uri").(string)
		if uri == "" {
			continue
		}
		title, _ := row.ObjectProperty("title").(string)
		rows[uri] = strings.TrimPrefix(title, "_")
	}
	return rows
}

func uiSmokeIconFile(name string) string {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return ""
	}
	paintable := gtk.IconThemeGetForDisplay(display).LookupIcon(name, nil, 512, 1, gtk.TextDirNone, 0)
	if paintable == nil || paintable.File() == nil {
		return ""
	}
	return paintable.File().Path()
}

func uiSmokeTextContains(texts []string, want string) bool {
	for _, text := range texts {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

func uiSmokeRows(root gtk.Widgetter) []gtk.Widgetter {
	var rows []gtk.Widgetter
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		if gtk.BaseWidget(cur).CSSName() == "row" {
			rows = append(rows, cur)
		}
		for _, child := range uiSmokeChildren(cur) {
			walk(child)
		}
	}
	walk(root)
	return rows
}

func uiSmokeHasText(texts []string, want string) bool {
	for _, text := range texts {
		if text == want {
			return true
		}
	}
	return false
}

func uiSmokeWidgetPoint(w, target gtk.Widgetter) (float32, float32, bool) {
	point, ok := gtk.BaseWidget(w).ComputePoint(target, graphene.PointZero())
	if !ok || point == nil {
		return 0, 0, false
	}
	return point.X(), point.Y(), true
}

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

// FLATPAK_PACKAGING_DIR is relative to cmd/WiiUDownloader, where the smoke runs.
const FLATPAK_PACKAGING_DIR = "../../packaging/flatpak"

// uiSmokePackagingIDs checks the GApplication ID against the Flatpak metadata.
// A sandbox only lets an app own its own ID on the session bus, so a mismatch
// makes the app die with "Failed to register: ... ServiceUnknown" before a
// window ever appears — in Flatpak only, which is why the AppImage is unaffected.
func uiSmokePackagingIDs(s *uiSmoke) {
	entries, err := os.ReadDir(FLATPAK_PACKAGING_DIR)
	if err != nil {
		s.check(false, "flatpak manifests are readable: %v", err)
		return
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(FLATPAK_PACKAGING_DIR, entry.Name()))
		if err != nil {
			s.check(false, "%s is readable: %v", entry.Name(), err)
			continue
		}
		var manifest struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			s.check(false, "%s parses: %v", entry.Name(), err)
			continue
		}
		checked++
		s.check(manifest.ID == APP_ID,
			"%s id matches the GApplication id (want %q, got %q)", entry.Name(), APP_ID, manifest.ID)
	}
	s.check(checked > 0, "at least one flatpak manifest was checked (%d)", checked)
}

// uiSmokeRetiredProgressWindow guards the retirement of the separate progress
// window: its files are gone, and no source may bring the seam back.
func uiSmokeRetiredProgressWindow(s *uiSmoke) {
	for _, retired := range []string{"progressWindow.go", "downloadUI.go"} {
		_, err := os.Stat(retired)
		s.check(os.IsNotExist(err), "%s is retired", retired)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		s.check(false, "the package sources are readable: %v", err)
		return
	}
	var references []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "ui_smoke") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, symbol := range []string{"createProgressWindow", "UseInlineDownloadUI", "inlineDownloadUI"} {
			if strings.Contains(string(data), symbol) {
				references = append(references, name+":"+symbol)
			}
		}
	}
	s.check(len(references) == 0, "nothing still builds a separate progress window (%v)", references)
}

// uiSmokeVisibleWindowTitles lists the toplevels that are actually on screen.
// A closed GtkWindow stays in the toplevel list, so counting titles alone would
// not notice a window that was closed without being taken down.
func uiSmokeVisibleWindowTitles() []string {
	model := gtk.WindowGetToplevels()
	var titles []string
	for i := uint(0); i < model.NItems(); i++ {
		obj := model.Item(i)
		if obj == nil {
			continue
		}
		if visible, ok := obj.ObjectProperty("visible").(bool); !ok || !visible {
			continue
		}
		title, ok := obj.ObjectProperty("title").(string)
		if ok {
			titles = append(titles, title)
		}
	}
	return titles
}

// uiSmokeCheckLayout asserts the layout invariants that must hold at every window
// size.
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

	// The queue pane holds the queue and download controls plus the run bar, so it
	// has to survive every window size instead of being dropped at a breakpoint.
	s.check(gtk.BaseWidget(mw.queuePane.container).Visible() && gtk.BaseWidget(mw.queuePane.container).Mapped(),
		"%s: the queue pane stays visible while resizing", size)
	titleX, _, okTitle := uiSmokeWidgetPoint(mw.titleView, mw.window)
	queueX, _, okQueue := uiSmokeWidgetPoint(mw.queuePane.container, mw.window)
	s.check(okQueue && okTitle && queueX+float32(mw.queuePane.container.Width()) <= titleX+1,
		"%s: queue pane stays left of the title list", size)
	s.check(okTitle && gtk.BaseWidget(mw.titleView).Mapped() && mw.titleView.Width() > 0,
		"%s: the title list stays usable", size)
	for name, button := range map[string]*gtk.Button{
		"download queue":  mw.queuePane.downloadButton,
		"remove selected": mw.queuePane.removeFromQueueButton,
	} {
		s.check(gtk.BaseWidget(button).Mapped() && button.Width() > 0,
			"%s: the %s button stays on screen (%d px wide)", size, name, button.Width())
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

	// What has to fit is the minimum of the layout actually in force: the compact
	// one below the breakpoint, the wide one just above it. Without this the
	// minimum silently drifts past the window floor.
	limit := COMPACT_WINDOW_BREAKPOINT
	if mw.window.Width() <= COMPACT_WINDOW_BREAKPOINT {
		limit = MIN_WINDOW_WIDTH
	}
	s.check(minW <= limit,
		"%s: the layout minimum %d fits the %d px it is allowed", size, minW, limit)
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

// uiSmokeWindowWidget wraps the newest on-screen toplevel with the given title so
// its children can be walked; the list hands back bare GObjects and keeps closed
// windows, so an older one must not be picked up instead.
func uiSmokeWindowWidget(title string) *gtk.Widget {
	model := gtk.WindowGetToplevels()
	var found *gtk.Widget
	for i := uint(0); i < model.NItems(); i++ {
		obj := model.Item(i)
		if obj == nil {
			continue
		}
		if visible, _ := obj.ObjectProperty("visible").(bool); !visible {
			continue
		}
		if got, ok := obj.ObjectProperty("title").(string); ok && got == title {
			found = &gtk.Widget{Object: obj}
		}
	}
	return found
}

func uiSmokeButtonByLabel(root gtk.Widgetter, label string) *gtk.Button {
	var found *gtk.Button
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		if found != nil {
			return
		}
		if button, ok := cur.(*gtk.Button); ok && button.Label() == label {
			found = button
			return
		}
		for _, child := range uiSmokeChildren(cur) {
			walk(child)
		}
	}
	walk(root)
	return found
}

func uiSmokeSpinButton(root gtk.Widgetter) *gtk.SpinButton {
	var found *gtk.SpinButton
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		if found != nil {
			return
		}
		if spin, ok := cur.(*gtk.SpinButton); ok {
			found = spin
			return
		}
		for _, child := range uiSmokeChildren(cur) {
			walk(child)
		}
	}
	walk(root)
	return found
}

// uiSmokeSpinArrow finds one of a spin button's own step buttons. GTK builds
// them from a template as real children, so emitting "clicked" on one takes the
// same path a user's click does. They are matched by their up/down class; the
// icon names moved from pan-*-symbolic to value-*-symbolic in GTK 4.22, so they
// are only a fallback.
func uiSmokeSpinArrow(spin *gtk.SpinButton, class string, icons ...string) *gtk.Button {
	var found *gtk.Button
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		if found != nil {
			return
		}
		if button, ok := cur.(*gtk.Button); ok {
			if button.HasCSSClass(class) {
				found = button
				return
			}
			for _, icon := range icons {
				if button.IconName() == icon {
					found = button
					return
				}
			}
		}
		for _, child := range uiSmokeChildren(cur) {
			walk(child)
		}
	}
	walk(spin)
	return found
}

func uiSmokeCheckByLabel(root gtk.Widgetter, label string) *gtk.CheckButton {
	var found *gtk.CheckButton
	var walk func(gtk.Widgetter)
	walk = func(cur gtk.Widgetter) {
		if found != nil {
			return
		}
		if check, ok := cur.(*gtk.CheckButton); ok && check.Label() == label {
			found = check
			return
		}
		for _, child := range uiSmokeChildren(cur) {
			walk(child)
		}
	}
	walk(root)
	return found
}

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

// errSmokeFetch stands in for a failed file-list fetch.
var errSmokeFetch = errors.New("smoke: file list fetch failed")

// smokeDirCounts reports how many files under a folder are ticked, which is
// what its tri-state checkbox has to agree with.
func smokeDirCounts(node *titleFileNode) (int, int) {
	active, total := 0, 0
	for _, child := range node.children {
		if !child.dir {
			total++
			if child.selected {
				active++
			}
			continue
		}
		childActive, childTotal := smokeDirCounts(child)
		active += childActive
		total += childTotal
	}
	return active, total
}

// smokeSelectedFiles counts the picker's ticked files from the model state, not
// from widgets: only rows on screen carry a checkbox at all.
func smokeSelectedFiles() int {
	n := 0
	for _, node := range lastTitleFileNodes {
		if !node.dir && node.selected {
			n++
		}
	}
	return n
}

func runUISmoke() int {
	s := &uiSmoke{}
	fmt.Println("UI smoke run")

	cfg := getDefaultConfig()
	cfg.ShowDonationBar = true
	cfg.SuggestRelatedContent = false
	cfg.GetSizeOnQueue = false

	mw := NewMainWindow(buildHTTPClient(), cfg)
	mw.BuildUI()
	// Present so rows, cells and allocations exist for real; offscreen widgets
	// have no allocation and cannot be measured.
	mw.window.Present()
	uiSmokePump()
	s.check(mw.window != nil && mw.titleView != nil, "main window builds")
	width, height := mw.window.DefaultSize()
	s.check(width > 0 && height > 0, "main window has a default size (%dx%d)", width, height)
	uiSmokeStylesheet(s)
	uiSmokePackagingIDs(s)

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

	// --- the main window carries the same nav bar as the other windows ---
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
		settingsItems := 0
		if section != nil {
			settingsItems = gio.BaseMenuModel(section).NItems()
		}
		s.check(settingsItems == 2, "the settings section holds Settings and About (got %d items)", settingsItems)
		if settingsItems == 2 {
			settingsModel := gio.BaseMenuModel(section)
			s.check(uiSmokeMenuAction(settingsModel, 1) == "win.about",
				"the About menu item is wired to win.about (got %q)", uiSmokeMenuAction(settingsModel, 1))
		}

		tools := model.ItemLink(0, "section")
		toolsItems := 0
		hasSpecificFiles := false
		if tools != nil {
			toolsModel := gio.BaseMenuModel(tools)
			toolsItems = toolsModel.NItems()
			for i := 0; i < toolsItems; i++ {
				if uiSmokeMenuLabel(toolsModel, i) == CONTEXT_MENU_SPECIFIC_FILES {
					hasSpecificFiles = true
				}
			}
		}
		s.check(toolsItems == 3, "tools section lists three items (got %d)", toolsItems)
		s.check(!hasSpecificFiles, "the app menu does not duplicate the row action %q", CONTEXT_MENU_SPECIFIC_FILES)
	} else {
		s.check(false, "menu button carries a menu model")
	}

	// --- title row context menu ---
	if ctxKey := mw.viewRowKey(0); ctxKey != "" {
		ctxMenu := gio.BaseMenuModel(mw.buildTitleRowMenu(mw.titleRows[ctxKey]))
		s.check(ctxMenu.NItems() == 4, "row context menu has four items (got %d)", ctxMenu.NItems())
		s.check(uiSmokeMenuLabel(ctxMenu, 1) == CONTEXT_MENU_SPECIFIC_FILES,
			"row context menu offers %q (got %q)", CONTEXT_MENU_SPECIFIC_FILES, uiSmokeMenuLabel(ctxMenu, 1))
		queueLabel := uiSmokeMenuLabel(ctxMenu, 0)
		s.check(queueLabel == CONTEXT_MENU_QUEUE_ADD || queueLabel == CONTEXT_MENU_QUEUE_REMOVE,
			"row context menu queue entry matches the row state (got %q)", queueLabel)

		mw.showTitleRowMenu(mw.titleView, 8, 8, ctxKey)
		uiSmokePump()
		s.check(mw.titleRowMenu != nil && gtk.BaseWidget(mw.titleRowMenu).Visible(), "right-click menu pops up")
		if mw.titleRowMenu != nil {
			mw.titleRowMenu.Popdown()
		}
		uiSmokePump()
		if mw.titleSelection != nil {
			mw.titleSelection.UnselectAll()
		}
		uiSmokePump()
	} else {
		s.check(false, "a title row is available for the context menu")
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
	for _, size := range []struct{ w, h int }{{1024, 600}, {1600, 900}, {1280, 720}, {COMPACT_WINDOW_BREAKPOINT, 600}, {COMPACT_WINDOW_BREAKPOINT + 1, 600}, {900, 500}, {880, 600}, {MIN_WINDOW_WIDTH, MIN_WINDOW_HEIGHT}, {MIN_WINDOW_WIDTH, 700}} {
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

	for _, test := range []struct {
		name               string
		defaultW, defaultH int
		availW, availH     int
		wantW, wantH       int
	}{
		{"desktop monitor", 1040, 700, 2560, 1440, 1040, 700},
		{"1366x768 laptop at 125% scaling", 1040, 700, 1093, 614, 1040, 518},
		{"1024x600 netbook", 1040, 700, 1024, 600, 992, 504},
		{"a window that already fits", 900, 500, 1024, 600, 900, 500},
		{"a default under the layout minimum", 700, 400, 2560, 1440, MIN_WINDOW_WIDTH, MIN_WINDOW_HEIGHT},
		{"a screen under the layout minimum", 1040, 700, 640, 400, MIN_WINDOW_WIDTH, MIN_WINDOW_HEIGHT},
		{"a monitor that reports no size", 1040, 700, 0, 0, 1040, 700},
	} {
		gotW, gotH := planWindowSize(test.defaultW, test.defaultH, test.availW, test.availH)
		s.check(gotW == test.wantW && gotH == test.wantH,
			"%s: %dx%d on a %dx%d screen fits as %dx%d", test.name, test.defaultW, test.defaultH, test.availW, test.availH, gotW, gotH)
	}

	if areaW, areaH, scale, ok := windowArea(mw.window); !ok || areaW <= 0 || areaH <= 0 || scale < 1 {
		s.check(false, "the presented window reports the monitor it is on")
	} else {
		s.check(true, "the presented window reports its monitor (%dx%d at scale %d)", areaW, areaH, scale)
	}

	mw.window.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
	uiSmokeSettle()
	beforeH := mw.window.Height()
	fittedW, fittedH, changed := applyWindowSize(mw.window, 1093, 614)
	uiSmokeSettle()
	fitH := mw.window.Height()
	s.check(changed && fittedW == 1040 && fittedH == 518,
		"a monitor smaller than the default plans a shorter window (%dx%d changed=%v)", fittedW, fittedH, changed)
	s.check(fitH < beforeH && fitH <= 614, "the planned size reaches the window (%d -> %d)", beforeH, fitH)
	if _, _, again := applyWindowSize(mw.window, 1093, 614); again {
		s.check(false, "a window that already fits is left alone")
	} else {
		s.check(true, "a window that already fits is left alone")
	}
	mw.window.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
	uiSmokeSettle()
	s.check(mw.window.Height() > fitH, "restoring the default size grows the window back (%d -> %d)", fitH, mw.window.Height())

	earlyWindow := NewMainWindow(buildHTTPClient(), cfg)
	earlyWindow.window.Realize()
	uiSmokePump()
	earlyW, earlyH, _, earlyOK := windowArea(earlyWindow.window)
	s.check(earlyOK && earlyW > 0 && earlyH > 0,
		"a realized but unpresented window knows its monitor (%dx%d, ok=%v)", earlyW, earlyH, earlyOK)
	_, _, earlyChanged := applyWindowSize(earlyWindow.window, 1093, 614)
	s.check(earlyChanged, "the window is sized before its first frame")
	earlyWindow.window.Destroy()
	uiSmokePump()

	shownWindow := NewMainWindow(buildHTTPClient(), cfg)
	fitWindowToMonitorBeforeShow(shownWindow.window)
	uiSmokeSettle()
	shownW, shownH := shownWindow.window.DefaultSize()
	s.check(shownWindow.window.Visible(), "sizing the window before it is shown still presents it")
	s.check(shownW > 0 && shownH > 0, "the pre-show window keeps a usable default size (%dx%d)", shownW, shownH)
	shownAreaW, shownAreaH, _, shownOK := windowArea(shownWindow.window)
	_, _, changedAfterShow := applyWindowSize(shownWindow.window, shownAreaW, shownAreaH)
	s.check(shownOK && !changedAfterShow,
		"the pre-show fit already matches its monitor (%dx%d), so nothing is left to resize", shownAreaW, shownAreaH)
	shownWindow.window.Destroy()
	uiSmokePump()

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

	// --- the per-row category the filter reads is the one its title ID implies ---
	// titleMatchesFilter reads titleRow.category, derived once at build time. If it
	// ever drifts from what the row displays, filtering silently selects the wrong
	// rows, so compare it against the derivation for every row and category.
	drift := 0
	for _, row := range mw.titleRows {
		kind := wiiudownloader.GetFormattedKind(row.entry.TitleID)
		if row.kind != kind || row.category != wiiudownloader.GetCategoryFromFormattedCategory(kind) {
			drift++
		}
	}
	s.check(drift == 0, "every row's cached kind and category match its title ID (%d of %d wrong)", drift, len(mw.titleRows))

	mw.currentRegion = wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA
	mw.lastSearchText = ""
	mismatch := 0
	for _, category := range []uint8{
		wiiudownloader.TITLE_CATEGORY_ALL,
		wiiudownloader.TITLE_CATEGORY_GAME,
		wiiudownloader.TITLE_CATEGORY_UPDATE,
		wiiudownloader.TITLE_CATEGORY_DLC,
		wiiudownloader.TITLE_CATEGORY_DEMO,
	} {
		mw.currentCategory = category
		for _, row := range mw.titleRows {
			// The form the filter used before the category was cached: derive the
			// kind and its category from the title ID on every row.
			derivedCategory := wiiudownloader.GetCategoryFromFormattedCategory(
				wiiudownloader.GetFormattedKind(row.entry.TitleID))
			want := (category == wiiudownloader.TITLE_CATEGORY_ALL || derivedCategory == category) &&
				(mw.currentRegion&row.entry.Region) != 0
			if mw.titleMatchesFilter(row) != want {
				mismatch++
			}
		}
	}
	s.check(mismatch == 0, "the cached-category filter selects the same rows as deriving them per row (%d mismatches)", mismatch)
	mw.currentCategory = wiiudownloader.TITLE_CATEGORY_ALL
	mw.refreshTitleFilter()
	uiSmokePump()

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

	// --- the name column gets the room the fixed columns leave ---
	{
		// The width rule is the whole behaviour, so pin it on its own first.
		s.check(queueNameColumnWidth(313, queueFixedColumnsWidth, QUEUE_NAME_MIN_COLUMN_WIDTH, QUEUE_NAME_MAX_AUTO_WIDTH) == QUEUE_NAME_MIN_COLUMN_WIDTH,
			"a pane too narrow for the fixed columns still keeps the name minimum")
		s.check(queueNameColumnWidth(573, queueFixedColumnsWidth, QUEUE_NAME_MIN_COLUMN_WIDTH, QUEUE_NAME_MAX_AUTO_WIDTH) == 203,
			"a roomy pane gives the name column what the fixed columns leave (got %d)",
			queueNameColumnWidth(573, queueFixedColumnsWidth, QUEUE_NAME_MIN_COLUMN_WIDTH, QUEUE_NAME_MAX_AUTO_WIDTH))
		s.check(queueNameColumnWidth(900, queueFixedColumnsWidth, QUEUE_NAME_MIN_COLUMN_WIDTH, QUEUE_NAME_MAX_AUTO_WIDTH) == QUEUE_NAME_MAX_AUTO_WIDTH,
			"a wide pane stops the name column at the %dpx cap", QUEUE_NAME_MAX_AUTO_WIDTH)

		// And the same rule drives the live column view.
		pane := mw.splitPane
		start := pane.Position()
		mw.queuePane.Clear()
		uiSmokeSettle()
		mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{first, second})
		uiSmokeSettle()

		nameWidth := mw.queuePane.nameColumn.FixedWidth()
		s.check(nameWidth == QUEUE_NAME_MIN_COLUMN_WIDTH,
			"the default pane gives the name column its minimum instead of nothing (%d)", nameWidth)
		// The width has to reach the table's own minimum, otherwise the column
		// would still be laid out at zero and the check above would be vacuous.
		minWidth, _, _, _ := mw.queuePane.columnView.Measure(gtk.OrientationHorizontal, -1)
		s.check(minWidth >= queueFixedColumnsWidth+QUEUE_NAME_MIN_COLUMN_WIDTH,
			"the table asks for the name column on top of the fixed ones (%d >= %d)",
			minWidth, queueFixedColumnsWidth+QUEUE_NAME_MIN_COLUMN_WIDTH)

		// A wider window lets the pane and the table grow with it; the name column
		// follows, up to the cap.
		mw.window.SetDefaultSize(1600, 900)
		uiSmokeSettle()
		pane.SetPosition(900)
		uiSmokeSettle()
		s.check(mw.queuePane.nameColumn.FixedWidth() > nameWidth,
			"widening the pane expands the name column (%d -> %d)", nameWidth, mw.queuePane.nameColumn.FixedWidth())
		s.check(mw.queuePane.nameColumn.FixedWidth() == QUEUE_NAME_MAX_AUTO_WIDTH,
			"the expanded name column stops at the automatic cap (%d)", mw.queuePane.nameColumn.FixedWidth())
		mw.window.SetDefaultSize(MAIN_WINDOW_WIDTH, MAIN_WINDOW_HEIGHT)
		pane.SetPosition(start)
		uiSmokeSettle()

		// A header drag wins. GTK writes fixed-width on an interactive resize, so
		// the same write stands in for the drag; the layout pass after it must
		// notice, and then leave the width alone as the pane changes around it.
		manual := 250
		mw.queuePane.nameColumn.SetFixedWidth(manual)
		mw.queuePane.refreshQueueColumns()
		s.check(mw.queuePane.nameManual, "a header drag marks the name width as the user's")

		pane.SetPosition(start + 100)
		uiSmokeSettle()
		s.check(mw.queuePane.nameColumn.FixedWidth() == manual,
			"a manually sized name column is left alone as the pane changes (got %d, want %d)",
			mw.queuePane.nameColumn.FixedWidth(), manual)

		// Hand the width back to the layout rule for whatever runs next.
		pane.SetPosition(start)
		mw.queuePane.nameManual = false
		mw.queuePane.nameAutoWidth = -1
		uiSmokeSettle()
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

	mw.europeRegionCheckbox.SetActive(false)
	mw.usaRegionCheckbox.SetActive(false)
	mw.japanRegionCheckbox.SetActive(false)
	uiSmokePump()
	s.check(mw.currentRegion != 0 && mw.japanRegionCheckbox.Active(),
		"unchecking the last region keeps it selected (mask %d)", mw.currentRegion)
	s.check(mw.titleSortModel.NItems() > 0, "the list is never left empty by the region boxes (%d rows)", mw.titleSortModel.NItems())
	mw.applyRegionSelection(wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_JAPAN | wiiudownloader.MCP_REGION_USA)
	uiSmokePump()

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

	// The picker fetches the FST over the network; stub it so the tree UI can be
	// exercised without one.
	smokeTree := &wiiudownloader.TitleFileTree{
		TitleID: 0x0005000010143500,
		Name:    "Smoke Title",
		Files: []wiiudownloader.TitleFile{
			{Path: "content/a.bin", Size: 1024},
			{Path: "content/sub/b.bin", Size: 2048},
			{Path: "meta/meta.xml", Size: 512},
		},
	}
	smokeTreeRoots, smokeTreeNodes := buildTitleFileNodes(smokeTree.Files)
	fileNodeCount := 0
	for _, node := range smokeTreeNodes {
		if !node.dir {
			fileNodeCount++
		}
	}
	s.check(len(smokeTreeRoots) == 2 && len(smokeTreeNodes) == 6 && fileNodeCount == 3,
		"the FST tree keeps folders and files (%d roots, %d nodes, %d files)", len(smokeTreeRoots), len(smokeTreeNodes), fileNodeCount)

	// A search narrows the bulk buttons; a collapsed folder must not. Scoping
	// them to hidden rows is what made Select All/None look broken on a big
	// title, where almost every file sits inside a folder.
	smokeFiles := make([]*titleFileNode, 0, fileNodeCount)
	for _, node := range smokeTreeNodes {
		if !node.dir {
			smokeFiles = append(smokeFiles, node)
		}
	}
	allShown := bulkTitleFileNodes(smokeFiles, "")
	for _, node := range smokeTreeNodes {
		if node.dir && node.name == "content" {
			node.expanded = false
		}
	}
	collapsedShown := bulkTitleFileNodes(smokeFiles, "")
	for _, node := range smokeFiles {
		node.match = strings.Contains(node.path, "b.bin")
	}
	searchShown := bulkTitleFileNodes(smokeFiles, "b.bin")
	s.check(len(allShown) == 3 && len(collapsedShown) == 3 && len(searchShown) == 1,
		"select all/none ignore collapsed folders but follow a search (all=%d collapsed=%d search=%d)",
		len(allShown), len(collapsedShown), len(searchShown))
	for _, node := range smokeTreeNodes {
		if node.dir {
			node.expanded = true
		}
	}

	originalFetch := fetchTitleFileTree
	fetchTitleFileTree = func(uint64, int, *http.Client) (*wiiudownloader.TitleFileTree, error) {
		return smokeTree, nil
	}
	pickerBase := uiSmokeToplevelCount()
	mw.showSpecificFilesDialogFor(wiiudownloader.TitleEntry{TitleID: 0x0005000010143500, Name: "Smoke Title"})
	uiSmokeSettle()
	s.check(uiSmokeToplevelCount() > pickerBase, "download-specific-files dialog presents")

	// The model carries every node, and the factory must actually materialise
	// rows: a key that does not resolve hides a row silently, which is how the
	// "files detected but no tree" bug shipped.
	modelItems := -1
	storeItems := -1
	if lastTitleFilePicker.model != nil {
		modelItems = int(lastTitleFilePicker.model.NItems())
	}
	if lastTitleFilePicker.store != nil {
		storeItems = int(lastTitleFilePicker.store.NItems())
	}
	_ = storeItems
	materialised, bound := 0, 0
	for _, node := range lastTitleFileNodes {
		if node.row == nil || node.check == nil {
			continue
		}
		materialised++
		// A file row follows the node's own tick; a folder row follows its
		// subtree's tri-state.
		want := node.selected
		if node.dir {
			active, total := smokeDirCounts(node)
			want = total > 0 && active == total
		}
		if node.check.Active() == want {
			bound++
		}
	}
	s.check(len(lastTitleFileNodes) == 6 && modelItems == 6 && materialised > 0 && bound == materialised,
		"the FST tree renders from the model (%d nodes, %d in store, %d in model, %d rows materialised, %d bound)",
		len(lastTitleFileNodes), storeItems, modelItems, materialised, bound)

	// Wiring, not just the helper: with a search active, Select All must touch
	// only the matching files.
	if lastTitleFilePicker.search != nil && lastTitleFilePicker.selectAll != nil {
		smokeSelectFiles := func(active bool) {
			for _, node := range lastTitleFileNodes {
				if node.dir {
					continue
				}
				node.selected = active
				if node.check != nil {
					node.check.SetActive(active)
				}
			}
		}
		smokeSelectFiles(false)
		lastTitleFilePicker.search.SetText("b.bin")
		// GtkSearchEntry debounces search-changed, so wait past its delay.
		uiSmokeWait(400 * time.Millisecond)
		if lastTitleFilePicker.status != nil {
			s.check(strings.HasPrefix(lastTitleFilePicker.status.Text(), "1 of 3 file(s)"),
				"the search status reports matches against the total (%q)", lastTitleFilePicker.status.Text())
		}
		lastTitleFilePicker.bulkPasses, lastTitleFilePicker.folderPasses, lastTitleFilePicker.statusPasses = 0, 0, 0
		coreglib.BaseObject(lastTitleFilePicker.selectAll).Emit("clicked")
		uiSmokeWait(100 * time.Millisecond)

		active, shownActive, shownTotal := 0, 0, 0
		for _, node := range lastTitleFileNodes {
			if node.dir {
				continue
			}
			if node.selected {
				active++
			}
			if node.match {
				shownTotal++
				if node.selected {
					shownActive++
				}
			}
		}
		s.check(active == 1 && shownTotal == 1 && shownActive == 1,
			"select all with a search touches only shown files (%d active, %d of %d shown)", active, shownActive, shownTotal)
		// Bounded passes, not one tree walk per checkbox.
		s.check(lastTitleFilePicker.bulkPasses == 1 && lastTitleFilePicker.folderPasses == 1 && lastTitleFilePicker.statusPasses == 1,
			"select all is one pass, not one per file (bulk=%d folder=%d status=%d)",
			lastTitleFilePicker.bulkPasses, lastTitleFilePicker.folderPasses, lastTitleFilePicker.statusPasses)
		lastTitleFilePicker.search.SetText("")
		uiSmokeWait(400 * time.Millisecond)
		if lastTitleFilePicker.status != nil {
			s.check(strings.HasPrefix(lastTitleFilePicker.status.Text(), "3 file(s)"),
				"clearing the search restores the plain file total (%q)", lastTitleFilePicker.status.Text())
		}

		// The reported bug: with folders collapsed, Select None and Select All
		// did nothing because every file was hidden behind a folder. Run it
		// through the real buttons, not the helper.
		if lastTitleFilePicker.collapseAll != nil {
			coreglib.BaseObject(lastTitleFilePicker.collapseAll).Emit("clicked")
			uiSmokeWait(100 * time.Millisecond)
			collapsedItems := -1
			if lastTitleFilePicker.model != nil {
				collapsedItems = int(lastTitleFilePicker.model.NItems())
			}
			coreglib.BaseObject(lastTitleFilePicker.selectNone).Emit("clicked")
			uiSmokeWait(100 * time.Millisecond)
			s.check(collapsedItems < 6 && smokeSelectedFiles() == 0,
				"select none clears every file with folders collapsed (%d rows on screen, %d selected)",
				collapsedItems, smokeSelectedFiles())

			coreglib.BaseObject(lastTitleFilePicker.selectAll).Emit("clicked")
			uiSmokeWait(100 * time.Millisecond)
			s.check(smokeSelectedFiles() == 3,
				"select all selects every file with folders collapsed (%d selected)", smokeSelectedFiles())

			coreglib.BaseObject(lastTitleFilePicker.expandAll).Emit("clicked")
			uiSmokeWait(100 * time.Millisecond)
		}
	} else {
		s.check(false, "the file picker exposes its controls to the smoke run")
	}

	// Rows are recycled, so the interactive paths have to be driven through the
	// real widgets: a handler that resolves the wrong node silently ticks another
	// file, which is exactly what recycling can break.
	smokeHasBoundRows := false
	for _, node := range lastTitleFileNodes {
		if node.check != nil {
			smokeHasBoundRows = true
			break
		}
	}
	if smokeHasBoundRows {
		var dirNode, fileNode *titleFileNode
		for _, node := range lastTitleFileNodes {
			if node.check == nil {
				continue
			}
			if node.dir && dirNode == nil && len(node.children) > 0 {
				dirNode = node
			}
			if !node.dir && fileNode == nil {
				fileNode = node
			}
		}

		// Indentation is measured, not assumed: a child row has to sit one step
		// to the right of its parent. The arrow slot is reserved for files too,
		// and dropping it pulled children back beside their parent.
		if lastTitleFilePicker.view != nil {
			aligned, compared := 0, 0
			var bad []string
			for _, node := range lastTitleFileNodes {
				if node.row == nil || node.check == nil || node.parent == nil {
					continue
				}
				parent := node.parent
				if parent.row == nil || parent.check == nil {
					continue
				}
				childX, _, childOK := uiSmokeWidgetPoint(node.check, lastTitleFilePicker.view)
				parentX, _, parentOK := uiSmokeWidgetPoint(parent.check, lastTitleFilePicker.view)
				if !childOK || !parentOK {
					continue
				}
				compared++
				if step := childX - parentX; step > SPECIFIC_FILE_INDENT-2 && step < SPECIFIC_FILE_INDENT+2 {
					aligned++
				} else {
					bad = append(bad, fmt.Sprintf("%s is %.0fpx from %s", node.path, step, parent.path))
				}
			}
			s.check(compared > 0 && aligned == compared,
				"a child row is indented one step past its parent (%d of %d, bad: %v)", aligned, compared, bad)
		}

		if dirNode != nil {
			dirNode.check.SetActive(false)
			uiSmokeWait(50 * time.Millisecond)
			active, total := smokeDirCounts(dirNode)
			s.check(active == 0 && total > 0 && !dirNode.check.Active(),
				"un-ticking a folder clears its subtree (%d of %d selected)", active, total)

			dirNode.check.SetActive(true)
			uiSmokeWait(50 * time.Millisecond)
			active, total = smokeDirCounts(dirNode)
			s.check(active == total && total > 0 && dirNode.check.Active(),
				"re-ticking a folder selects its subtree again (%d of %d)", active, total)
		}

		if fileNode != nil && fileNode.parent != nil && fileNode.parent.check != nil {
			parent := fileNode.parent
			activeBefore, total := smokeDirCounts(parent)
			fileNode.check.SetActive(false)
			uiSmokeWait(50 * time.Millisecond)
			activeAfter, _ := smokeDirCounts(parent)
			s.check(activeAfter == activeBefore-1 && !parent.check.Active() && parent.check.Inconsistent(),
				"un-ticking one file leaves its folder partially selected (%d of %d, inconsistent=%v)",
				activeAfter, total, parent.check.Inconsistent())

			fileNode.check.SetActive(true)
			uiSmokeWait(50 * time.Millisecond)
			s.check(!parent.check.Inconsistent() && parent.check.Active(),
				"re-ticking the file settles the folder back to fully selected")
		}
	}

	// A title that lists no files, and a failed fetch: both are real paths and
	// both must say so rather than leave an empty list on screen.
	emptyTree := &wiiudownloader.TitleFileTree{TitleID: 0x0005000010143500, Name: "Empty Title"}
	fetchTitleFileTree = func(uint64, int, *http.Client) (*wiiudownloader.TitleFileTree, error) {
		return emptyTree, nil
	}
	mw.showSpecificFilesDialogFor(wiiudownloader.TitleEntry{TitleID: 0x0005000010143500, Name: "Empty Title"})
	uiSmokeSettle()
	if lastTitleFilePicker.status != nil {
		s.check(lastTitleFilePicker.status.Text() == "This title lists no files.",
			"a title with no files says so (%q)", lastTitleFilePicker.status.Text())
	}

	fetchTitleFileTree = func(uint64, int, *http.Client) (*wiiudownloader.TitleFileTree, error) {
		return nil, errSmokeFetch
	}
	mw.showSpecificFilesDialogFor(wiiudownloader.TitleEntry{TitleID: 0x0005000010143500, Name: "Broken Title"})
	uiSmokeSettle()
	if lastTitleFilePicker.status != nil {
		s.check(lastTitleFilePicker.status.Text() == "Could not load the file list.",
			"a failed file-list fetch says so (%q)", lastTitleFilePicker.status.Text())
	}
	fetchTitleFileTree = originalFetch

	showVersionSelectionDialog(mw.window, first, func(int) {})
	uiSmokePump()
	s.check(uiSmokeToplevelCount() > base, "version picker presents")

	// Clicking OK is the path that took the app down, so drive it for real
	// instead of only presenting the dialog.
	chosenVersion := -2
	latestEntry := first
	latestEntry.Version = wiiudownloader.VersionLatest
	picker := showVersionSelectionDialog(mw.window, latestEntry, func(v int) { chosenVersion = v })
	uiSmokeSettle()
	if okButton := uiSmokeButtonByLabel(picker.Window, "OK"); okButton != nil {
		okButton.Emit("clicked")
		uiSmokeSettle()
		s.check(chosenVersion == wiiudownloader.VersionLatest,
			"OK on the default choice reports Latest (got %d)", chosenVersion)
	} else {
		s.check(false, "version picker has an OK button")
	}

	chosenVersion = -2
	customEntry := first
	customEntry.Version = 32
	picker = showVersionSelectionDialog(mw.window, customEntry, func(v int) { chosenVersion = v })
	uiSmokeSettle()
	if okButton := uiSmokeButtonByLabel(picker.Window, "OK"); okButton != nil {
		okButton.Emit("clicked")
		uiSmokeSettle()
		s.check(chosenVersion == 32, "OK reports the already-set custom version (got %d)", chosenVersion)
	} else {
		s.check(false, "version picker reopens with an OK button")
	}

	// The reported crash: setting the version through the entry point the row menu
	// uses, so the real handler runs and kicks off the size refetch that follows.
	queued := first
	queued.Version = wiiudownloader.VersionLatest
	mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{queued})
	mw.updateTitlesInQueue()
	uiSmokeSettle()
	mw.onSetVersionRequested([]wiiudownloader.TitleEntry{queued})
	uiSmokeSettle()
	menuPicker := uiSmokeWindowWidget(WINDOW_TITLE_PREFIX + "Select Title Version")
	if menuPicker == nil {
		s.check(false, "the row menu's version picker presents")
	} else {
		if specific := uiSmokeCheckByLabel(menuPicker, "Set version:"); specific != nil {
			specific.SetActive(true)
			uiSmokeWait(50 * time.Millisecond)
		}
		if okButton := uiSmokeButtonByLabel(menuPicker, "OK"); okButton != nil {
			okButton.Emit("clicked")
			uiSmokeSettle()
			queuedVersion := wiiudownloader.VersionLatest
			for _, entry := range mw.queuePane.GetTitleQueue() {
				if entry.TitleID == queued.TitleID {
					queuedVersion = entry.Version
				}
			}
			s.check(queuedVersion >= 0,
				"setting a version from the row menu updates the queue entry (got %d)", queuedVersion)
		} else {
			s.check(false, "the row menu's version picker has an OK button")
		}
	}

	// The same flow driven the way the row menu drives it: the popover is still up
	// when the action fires, which is how a user reaches this dialog.
	mw.showTitleRowMenu(mw.titleView, 8, 8, rowKeyForTitleID(queued.TitleID))
	uiSmokePump()
	mw.window.ActivateAction("row-set-version", nil)
	uiSmokeSettle()
	menuPicker = uiSmokeWindowWidget(WINDOW_TITLE_PREFIX + "Select Title Version")
	if menuPicker == nil {
		s.check(false, "the row action opens the version picker")
	} else {
		if specific := uiSmokeCheckByLabel(menuPicker, "Set version:"); specific != nil {
			specific.SetActive(true)
			uiSmokeWait(50 * time.Millisecond)
		}
		if okButton := uiSmokeButtonByLabel(menuPicker, "OK"); okButton != nil {
			okButton.Emit("clicked")
			uiSmokeSettle()
			s.check(!menuPicker.Visible(), "the picker opened from the row menu closes on OK")
		} else {
			s.check(false, "the row-menu picker has an OK button")
		}
	}
	if mw.titleRowMenu != nil {
		mw.titleRowMenu.Popdown()
	}
	uiSmokeSettle()

	// The field is live only for "Set version:". The user picks that with a click,
	// not with SetActive, so the widget-level activation is driven too.
	clickEntry := first
	clickEntry.Version = wiiudownloader.VersionLatest
	clickPicker := showVersionSelectionDialog(mw.window, clickEntry, func(int) {})
	uiSmokeSettle()
	clickSpecific := uiSmokeCheckByLabel(clickPicker.Window, "Set version:")
	clickSpin := uiSmokeSpinButton(clickPicker.Window)
	if clickSpecific == nil || clickSpin == nil {
		s.check(false, "version picker has a clickable choice and a version field")
	} else {
		s.check(!clickSpin.Sensitive(),
			"the version field is disabled while \"Latest version\" is the choice (usable=%v)", clickSpin.Sensitive())

		clickSpecific.Activate()
		uiSmokeWait(50 * time.Millisecond)
		s.check(clickSpecific.Active() && clickSpin.Sensitive(),
			"activating \"Set version:\" selects it and enables the field (chosen=%v usable=%v)",
			clickSpecific.Active(), clickSpin.Sensitive())

		clickSpecific.SetActive(false)
		uiSmokeWait(50 * time.Millisecond)
		s.check(!clickSpin.Sensitive(),
			"going back to \"Latest version\" disables the field again (usable=%v)", clickSpin.Sensitive())
	}
	clickPicker.Window.Destroy()
	uiSmokeSettle()

	// The reported flow: pick "Set version:" on a title that was on Latest, then
	// hit OK.
	chosenVersion = -2
	customEntry = first
	customEntry.Version = wiiudownloader.VersionLatest
	picker = showVersionSelectionDialog(mw.window, customEntry, func(v int) { chosenVersion = v })
	uiSmokeSettle()
	if spin := uiSmokeSpinButton(picker.Window); spin != nil {
		// The step buttons exist and are live once the choice is made. GTK drives
		// them from gesture controllers, which cannot be delivered from here, so the
		// arithmetic a step performs is what gets asserted: a spin button left with
		// the adjustment it makes for itself has a step of 0, and
		// gtk_spin_button_spin adds count * step_increment, so its +/- buttons could
		// never move the value.
		up := uiSmokeSpinArrow(spin, "up", "pan-up-symbolic", "value-increase-symbolic")
		down := uiSmokeSpinArrow(spin, "down", "pan-down-symbolic", "value-decrease-symbolic")
		s.check(up != nil && down != nil, "the version field exposes its step buttons (up=%v down=%v)", up != nil, down != nil)

		step, page, lower, upper := 0.0, 0.0, 0.0, 0.0
		if adjustment := spin.Adjustment(); adjustment != nil {
			step, page = adjustment.StepIncrement(), adjustment.PageIncrement()
			lower, upper = adjustment.Lower(), adjustment.Upper()
		}
		s.check(step > 0 && page > 0,
			"the version field's step buttons have something to step by (step=%.0f page=%.0f)", step, page)
		s.check(lower == 0 && upper == 65535,
			"the version field carries its range (lower=%.0f upper=%.0f)", lower, upper)

		before := spin.ValueAsInt()
		if adjustment := spin.Adjustment(); adjustment != nil {
			adjustment.SetValue(adjustment.Value() + step)
		}
		uiSmokeWait(50 * time.Millisecond)
		steppedUp := spin.ValueAsInt()
		if adjustment := spin.Adjustment(); adjustment != nil {
			adjustment.SetValue(adjustment.Value() - step)
		}
		uiSmokeWait(50 * time.Millisecond)
		s.check(steppedUp == before+1,
			"one step moves the field by one version (%d -> %d)", before, steppedUp)
		s.check(spin.ValueAsInt() == before,
			"stepping back returns the field to where it was (%d -> %d)", steppedUp, spin.ValueAsInt())
	} else {
		s.check(false, "version picker has a version field")
	}
	if specific := uiSmokeCheckByLabel(picker.Window, "Set version:"); specific != nil {
		specific.SetActive(true)
		uiSmokeWait(50 * time.Millisecond)
	} else {
		s.check(false, "version picker has a \"Set version:\" choice")
	}
	if okButton := uiSmokeButtonByLabel(picker.Window, "OK"); okButton != nil {
		okButton.Emit("clicked")
		uiSmokeSettle()
		s.check(chosenVersion == 0, "choosing a custom version reports it (got %d)", chosenVersion)
	} else {
		s.check(false, "version picker has an OK button for a custom version")
	}

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
		s.check(!uiSmokeHasWindowTitle(WINDOW_TITLE_PREFIX+"Downloading"),
			"a decryption batch reports in the run bar, not a progress window")
		os.RemoveAll(decryptRoot)
		uiSmokePump()
	}

	// --- the queue pane's run bar is the app's only progress surface ---
	mw.queuePane.AddTitles([]wiiudownloader.TitleEntry{first, second})
	uiSmokeSettle()

	s.check(!mw.queuePane.statusColumn.Visible(),
		"the Status column stays out of the narrow pane until a run starts")

	toplevelsBeforeRun := uiSmokeToplevelCount()
	run := mw.beginRun("Preparing...")
	uiSmokeSettle()
	s.check(mw.queuePane.runBar.Visible(), "the run bar shows while a run is going")
	s.check(mw.queuePane.statusColumn.Visible(), "the Status column appears while a run is going")
	s.check(uiSmokeToplevelCount() == toplevelsBeforeRun,
		"a run opens no window of its own (%d toplevels)", uiSmokeToplevelCount())
	s.check(!uiSmokeHasWindowTitle(WINDOW_TITLE_PREFIX+"Downloading"),
		"the retired progress window is gone")

	// The core reports from a worker goroutine and GTK is not thread-safe, so
	// every run-bar write has to wait for the main loop rather than reaching the
	// widget in place.
	mw.queuePane.SetRunProgress("not yet", 0, "")
	s.check(mw.queuePane.runBarLabel.Text() != "not yet",
		"a run-bar write does not touch GTK off the main loop")
	uiSmokeSettle()
	s.check(mw.queuePane.runBarLabel.Text() == "not yet",
		"the marshalled run-bar write lands once the loop runs")

	// The real calling pattern, all of it off-thread.
	reported := make(chan struct{})
	go func() {
		defer close(reported)
		run.ResetTotalsAndErrors()
		run.SetQueueProgress(1, 2)
		run.SetTitleState(first.TitleID, queueStateDownloading)
		run.SetGameTitle("Mario Kart 8")
		run.SetDownloadSize(2000)
		run.SetTotalDownloadedForFile("f.bin", 200)
		run.UpdateDownloadProgress(300, "f.bin")
	}()
	<-reported
	uiSmokeSettle()

	// 200 + 300 of 2000 bytes for the title on screen, and the second of two
	// queued titles: the bar follows the title, the count follows the queue.
	s.check(mw.queuePane.runBarBar.Fraction() == 0.25,
		"the run bar tracks the title being downloaded (got %v)", mw.queuePane.runBarBar.Fraction())
	s.check(mw.queuePane.runBarBar.Text() == "25%",
		"the run bar is labelled with that title's percentage (got %q)", mw.queuePane.runBarBar.Text())
	s.check(mw.queuePane.runBarCount.Text() == "2/2",
		"the run bar shows the queue position (got %q)", mw.queuePane.runBarCount.Text())

	// The run bar sits above the queue total.
	_, barY, okBar := uiSmokeWidgetPoint(mw.queuePane.runBar, mw.queuePane.container)
	_, totalY, okTotal := uiSmokeWidgetPoint(mw.queuePane.totalSizeLabel, mw.queuePane.container)
	s.check(okBar && okTotal && barY+float32(mw.queuePane.runBar.Height()) <= totalY+1,
		"the run bar sits above the queue total size (%.0f+%d <= %.0f)",
		barY, mw.queuePane.runBar.Height(), totalY)
	s.check(mw.queuePane.rowData[rowKeyForTitleID(first.TitleID)].state == queueStateDownloading,
		"the queue pane shows each title's state")
	s.check(mw.queuePane.runBarLabel.Text() == "Mario Kart 8",
		"the run bar names the title on screen now (got %q)", mw.queuePane.runBarLabel.Text())
	// The window title carries the same run.
	s.check(strings.Contains(mw.window.Title(), "Mario Kart 8") && strings.Contains(mw.window.Title(), "Title 2/2"),
		"the window title mirrors the running title and queue position (%q)", mw.window.Title())

	// The sidebar can be dragged down to QUEUE_PANE_MIN_WIDTH, so the fully
	// populated run bar has to fit there without a control being squeezed out.
	minimum, _, _, _ := gtk.BaseWidget(mw.queuePane.runBar).Measure(gtk.OrientationHorizontal, -1)
	s.check(minimum > 0 && minimum <= QUEUE_PANE_MIN_WIDTH,
		"the populated run bar fits the narrowest queue pane (%d <= %d px)", minimum, QUEUE_PANE_MIN_WIDTH)
	s.check(strings.Contains(mw.queuePane.runBarDetail.Text(), "500 B"),
		"the run bar reports the bytes fetched so far (got %q)", mw.queuePane.runBarDetail.Text())

	// Rate and ETA need two samples an interval apart, which is why they only
	// appear once the average is known.
	time.Sleep(MIN_SAMPLE_INTERVAL + 50*time.Millisecond)
	go run.UpdateDownloadProgress(300, "f.bin")
	uiSmokeSettle()
	s.check(strings.Contains(mw.queuePane.runBarDetail.Text(), "/s") &&
		strings.Contains(mw.queuePane.runBarDetail.Text(), "left"),
		"the run bar reports speed and time left (got %q)", mw.queuePane.runBarDetail.Text())

	// GTK logs an invalid "valuenow" for NaN, so the bar needs the clamp. A 0/0
	// queue only blanks the count.
	mw.queuePane.SetRunProgress("Zero", math.NaN(), "")
	mw.queuePane.SetRunQueueProgress(0, 0)
	uiSmokeSettle()
	s.check(mw.queuePane.runBarBar.Fraction() == 0 && mw.queuePane.runBarBar.Text() == "0%",
		"a NaN run-bar fraction is clamped (got %v %q)", mw.queuePane.runBarBar.Fraction(), mw.queuePane.runBarBar.Text())
	s.check(mw.queuePane.runBarCount.Text() == "",
		"a 0/0 queue position blanks the count (got %q)", mw.queuePane.runBarCount.Text())

	run.TogglePaused()
	uiSmokeSettle()
	s.check(run.Paused(), "the pause control pauses the run")
	s.check(mw.queuePane.runPauseButton.IconName() == "media-playback-start-symbolic",
		"the pause control flips to Resume (%q)", mw.queuePane.runPauseButton.IconName())
	run.TogglePaused()
	uiSmokeSettle()
	s.check(!run.Paused(), "the pause control resumes the run")

	run.SetTitleState(first.TitleID, queueStateDone)
	run.SetTitleState(second.TitleID, queueStateFailed)
	uiSmokeSettle()
	_, statusTexts := uiSmokeScan(mw.queuePane.columnView)
	s.check(uiSmokeHasText(statusTexts, string(queueStateDone)) && uiSmokeHasText(statusTexts, string(queueStateFailed)),
		"finished and failed titles both report their state (%v)", statusTexts)

	// A download removes its title from the queue, and that rebuilds every row:
	// the run states have to survive it.
	mw.queuePane.Update(true)
	uiSmokeSettle()
	s.check(mw.queuePane.rowData[rowKeyForTitleID(first.TitleID)].state == queueStateDone,
		"a queue rebuild keeps each row's run state")

	// Cancelling once disables the run controls.
	mw.queuePane.runCancelButton.Emit("clicked")
	uiSmokeSettle()
	s.check(run.Cancelled(), "the run bar's cancel control cancels the run")
	s.check(!mw.queuePane.runCancelButton.Sensitive() && !mw.queuePane.runPauseButton.Sensitive(),
		"cancelling disables the run controls once pressed")
	s.check(mw.queuePane.runBarLabel.Text() == "Cancelling...",
		"the run bar reports a cancelled run (%q)", mw.queuePane.runBarLabel.Text())

	// Cancelling keeps the title in the queue, marked cancelled: only the run
	// stops, so the title can be retried or removed on purpose. This drives the
	// same two functions the download loop uses.
	queuedBefore := mw.queuePane.GetTitleQueueSize()
	mw.applyQueueStep(run, first, nextQueueStep(context.Canceled, true, true), true)
	uiSmokeSettle()
	s.check(mw.queuePane.GetTitleQueueSize() == queuedBefore,
		"cancelling keeps the title in the queue (%d -> %d)", queuedBefore, mw.queuePane.GetTitleQueueSize())
	s.check(mw.queuePane.rowData[rowKeyForTitleID(first.TitleID)].state == queueStateCancelled,
		"a cancelled title is marked cancelled, not done (got %q)",
		mw.queuePane.rowData[rowKeyForTitleID(first.TitleID)].state)

	run.Finish()
	uiSmokeSettle()
	s.check(!mw.queuePane.runBar.Visible(), "the run bar hides when the run ends")
	s.check(!mw.queuePane.statusColumn.Visible(), "the Status column goes away with the run")
	s.check(mw.window.Title() == APP_NAME, "the window title goes back to the app name (%q)", mw.window.Title())

	mw.queuePane.Clear()
	uiSmokeSettle()

	uiSmokeRetiredProgressWindow(s)

	uiSmokeTitlePrefixes(s, "with every dialog open")

	about := mw.showAboutDialog()
	uiSmokeSettle()
	_, aboutTexts := uiSmokeScan(about)
	s.check(uiSmokeHasText(aboutTexts, APP_NAME), "the about dialog names the app (%v)", aboutTexts)
	s.check(uiSmokeTextContains(aboutTexts, APP_VERSION), "the about dialog shows the version %q", APP_VERSION)
	linkRows := uiSmokeVisibleLinkTitles(about)
	s.check(linkRows[REPO_URL] == "GitHub",
		"the repo link row reads GitHub (got %q)", linkRows[REPO_URL])
	websiteRows := 0
	for _, title := range linkRows {
		if title == "Website" {
			websiteRows++
		}
	}
	s.check(websiteRows == 0, "no visible link row still reads Website (%v)", linkRows)

	iconFile := uiSmokeIconFile(APP_ID)
	s.check(iconFile != "" && !strings.Contains(iconFile, "image-missing"),
		"the app icon resolves for the about dialog (%q)", iconFile)
	s.check(strings.HasSuffix(iconFile, filepath.Join("apps", APP_ID+".png")),
		"the resolved icon is the bundled one (%q)", iconFile)

	if source, err := os.ReadFile(filepath.Join("..", "..", "data", "WiiUDownloader.png")); err == nil {
		s.check(bytes.Equal(source, appIconPNG), "the embedded app icon matches data/WiiUDownloader.png")
	}
	s.check(mw.window.ActivateAction("win.about", nil), "the win.about action is registered on the window")
	uiSmokeSettle()

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

	// --- window teardown ---
	// These three used to leave windows behind: settings leaked one per open,
	// dialogs tore themselves down from inside their own click handler, and the
	// native choosers were locals the GC could unref while still on screen.

	// Reopening settings has to close the previous window deliberately instead of
	// dropping it for the GC to destroy at an arbitrary moment.
	if err := mw.createConfigWindow(cfg); err != nil {
		s.check(false, "first settings window opens: %v", err)
	} else {
		firstSettings := mw.configWindow
		firstSettings.Window.Present()
		uiSmokeSettle()
		if err := mw.createConfigWindow(cfg); err != nil {
			s.check(false, "second settings window opens: %v", err)
		} else {
			secondSettings := mw.configWindow
			secondSettings.Window.Present()
			uiSmokeSettle()
			s.check(firstSettings != secondSettings, "reopening settings builds a new window")
			s.check(!firstSettings.Window.Visible(), "reopening settings closes the previous window")
			visibleSettings := 0
			for _, title := range uiSmokeVisibleWindowTitles() {
				if title == WINDOW_TITLE_PREFIX+"Settings" {
					visibleSettings++
				}
			}
			s.check(visibleSettings == 1, "exactly one settings window is on screen (%d)", visibleSettings)
			secondSettings.Window.Destroy()
			uiSmokeSettle()
		}
	}
	mw.configWindow = nil

	// A dialog button must not tear its own mapped window down inside the click
	// handler; the close lands on the next main-loop turn.
	smokeDialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Teardown Check")
	handlerRan := false
	closeButton := smokeDialog.AddButton("Close", func() { handlerRan = true })
	smokeDialog.Present()
	uiSmokeSettle()
	closeButton.Emit("clicked")
	s.check(handlerRan, "a dialog button runs its handler")
	s.check(smokeDialog.Visible(), "a dialog does not tear its own window down inside the click handler")
	uiSmokeSettle()
	s.check(!smokeDialog.Visible(), "the dialog closes once the main loop runs")

	// The native choosers must outlive the GC while their panel is on screen, and
	// must not be pinned forever afterwards.
	liveChooser := gtk.NewFileDialog()
	retainFileDialog(liveChooser)
	s.check(activeFileDialog == liveChooser, "a live chooser is retained while its panel is on screen")
	newestChooser := gtk.NewFileDialog()
	retainFileDialog(newestChooser)
	releaseFileDialog(liveChooser)
	s.check(activeFileDialog == newestChooser, "releasing an older chooser leaves the newest retained")
	releaseFileDialog(newestChooser)
	s.check(activeFileDialog == nil, "the chooser reference is dropped once it finishes")

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

	uiSmokeRendererFallback(s)

	// --- dark mode round trip ---
	setDarkTheme(true)
	darkScheme := adw.StyleManagerGetDefault().ColorScheme()
	setDarkTheme(false)
	lightScheme := adw.StyleManagerGetDefault().ColorScheme()
	s.check(darkScheme == adw.ColorSchemeForceDark && lightScheme == adw.ColorSchemeForceLight,
		"dark mode reaches the style manager both ways (dark=%v light=%v)", darkScheme, lightScheme)

	fmt.Printf("UI smoke: %d passed, %d failed\n", s.pass, s.fail)
	if s.fail > 0 {
		return 1
	}
	return 0
}
