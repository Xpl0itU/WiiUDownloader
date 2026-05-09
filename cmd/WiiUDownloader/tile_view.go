package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
)

const (
	MAX_CONCURRENT_TILE_FETCHES = 6
	TITLE_TILE_IMAGE_WIDTH      = 170
	TITLE_TILE_IMAGE_HEIGHT     = 240
	TITLE_TILE_MIN_PER_LINE     = 2
	TITLE_TILE_MAX_PER_LINE     = 6
	TITLE_TILE_INITIAL_LOAD     = 24
	TITLE_TILE_LOAD_AHEAD       = 24
	TITLE_TILE_ACTIVE_WINDOW    = 72
	MAX_TILE_ARTWORK_CACHE      = 200
	SGDB_REQUEST_TIMEOUT        = 15 * time.Second
	SGDB_SEARCH_ENDPOINT        = "https://www.steamgriddb.com/api/v2/search/autocomplete/%s"
	SGDB_BOXART_ENDPOINT        = "https://www.steamgriddb.com/api/v2/grids/game/%d?dimensions=342x482,600x900,660x930&types=static&nsfw=false&humor=false&epilepsy=false&limit=1"
	SGDB_DEFAULT_TILE_IMAGE_URL = "https://cdn2.steamgriddb.com/thumb/b4379dcda061fa79353cbe9616a95117.jpg"
)

type titleTileCard struct {
	entry       wiiudownloader.TitleEntry
	button      *gtk.Button
	image       *gtk.Image
	spinner     *gtk.Spinner
	titleLabel  *gtk.Label
	metaLabel   *gtk.Label
	imageLoaded bool
}

type tileArtworkStore struct {
	mu      sync.Mutex
	images  map[uint64][]byte
	order   []uint64
	loading map[uint64]bool
	failed  map[uint64]bool
}

type sgdbIDCacheStore struct {
	mu      sync.Mutex
	gameIDs map[uint64]int
}

type sgdbAutocompleteResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ID       int      `json:"id"`
		Name     string   `json:"name"`
		Verified bool     `json:"verified"`
		Types    []string `json:"types"`
	} `json:"data"`
}

type sgdbGridResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ID    int     `json:"id"`
		URL   string  `json:"url"`
		Score float64 `json:"score"`
	} `json:"data"`
}

func newTileArtworkStore() *tileArtworkStore {
	return &tileArtworkStore{
		images:  make(map[uint64][]byte),
		order:   make([]uint64, 0),
		loading: make(map[uint64]bool),
		failed:  make(map[uint64]bool),
	}
}

func newSGDBIDCacheStore() *sgdbIDCacheStore {
	return &sgdbIDCacheStore{
		gameIDs: make(map[uint64]int),
	}
}

func (s *tileArtworkStore) get(titleID uint64) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	imageData, ok := s.images[titleID]
	if ok {
		s.touchLocked(titleID)
	}
	return imageData, ok
}

func (s *tileArtworkStore) isFailed(titleID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed[titleID]
}

func (s *tileArtworkStore) start(titleID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed[titleID] || s.loading[titleID] {
		return false
	}
	s.loading[titleID] = true
	return true
}

func (s *tileArtworkStore) finish(titleID uint64, imageData []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.loading, titleID)
	if err != nil {
		s.failed[titleID] = true
		return
	}
	s.images[titleID] = imageData
	s.touchLocked(titleID)
	s.evictLocked(MAX_TILE_ARTWORK_CACHE)
	delete(s.failed, titleID)
}

func (s *tileArtworkStore) retainVisible(visible map[uint64]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for titleID := range s.images {
		if _, ok := visible[titleID]; !ok {
			delete(s.images, titleID)
		}
	}
	for titleID := range s.failed {
		if _, ok := visible[titleID]; !ok {
			delete(s.failed, titleID)
		}
	}
	for titleID := range s.loading {
		if _, ok := visible[titleID]; !ok {
			delete(s.loading, titleID)
		}
	}

	filtered := make([]uint64, 0, len(s.order))
	for _, titleID := range s.order {
		if _, ok := visible[titleID]; ok {
			filtered = append(filtered, titleID)
		}
	}
	s.order = filtered
}

func (s *tileArtworkStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.images = make(map[uint64][]byte)
	s.order = make([]uint64, 0)
	s.loading = make(map[uint64]bool)
	s.failed = make(map[uint64]bool)
}

func (s *tileArtworkStore) touchLocked(titleID uint64) {
	for i, existing := range s.order {
		if existing == titleID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.order = append(s.order, titleID)
}

func (s *tileArtworkStore) evictLocked(maxEntries int) {
	if maxEntries <= 0 {
		return
	}
	for len(s.images) > maxEntries && len(s.order) > 0 {
		victim := s.order[0]
		s.order = s.order[1:]
		delete(s.images, victim)
		delete(s.failed, victim)
	}
}

func (s *sgdbIDCacheStore) Get(titleID uint64) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gameID, ok := s.gameIDs[titleID]
	return gameID, ok
}

func (s *sgdbIDCacheStore) Set(titleID uint64, gameID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gameIDs[titleID] = gameID
}

func (s *sgdbIDCacheStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gameIDs = make(map[uint64]int)

	cachePath, err := sgdbIDCachePath()
	if err != nil {
		return err
	}

	// Delete the cache file if it exists
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (s *sgdbIDCacheStore) Load() error {
	cachePath, err := sgdbIDCachePath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var raw map[string]int
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, gameID := range raw {
		titleID, err := strconv.ParseUint(key, 16, 64)
		if err != nil {
			continue
		}
		s.gameIDs[titleID] = gameID
	}
	return nil
}

func (s *sgdbIDCacheStore) Save() error {
	cachePath, err := sgdbIDCachePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), CONFIG_DIR_PERM); err != nil {
		return err
	}

	s.mu.Lock()
	raw := make(map[string]int, len(s.gameIDs))
	for titleID, gameID := range s.gameIDs {
		raw[fmt.Sprintf("%016x", titleID)] = gameID
	}
	s.mu.Unlock()

	content, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, content, CONFIG_FILE_PERM)
}

func sgdbIDCachePath() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirPath(userConfigDir), "sgdb_ids.json"), nil
}

func (mw *MainWindow) applyTileSettings(showTiles bool, apiKey string) {
	trimmedAPIKey := strings.TrimSpace(apiKey)
	normalizedShowTiles := showTiles && trimmedAPIKey != ""
	changed := mw.showTiles != normalizedShowTiles || mw.sgdbAPIKey != trimmedAPIKey

	// If API key is being removed, cancel pending tile loads and clear caches
	if mw.sgdbAPIKey != "" && trimmedAPIKey == "" {
		if mw.tileLoaderCancel != nil {
			mw.tileLoaderCancel()
			mw.tileLoaderCtx, mw.tileLoaderCancel = context.WithCancel(context.Background())
		}
		if mw.tileArtwork != nil {
			mw.tileArtwork.clear()
		}
		if mw.sgdbIDCache != nil {
			if err := mw.sgdbIDCache.Clear(); err != nil {
				log.Printf("[SGDB] Failed to clear SGDB ID cache: %v", err)
			}
		}
	}

	mw.showTiles = normalizedShowTiles
	mw.sgdbAPIKey = trimmedAPIKey
	mw.syncViewModeToggle()
	if changed && mw.uiBuilt {
		mw.refreshContentView()
	}
}

func (mw *MainWindow) syncViewModeToggle() {
	if mw.viewModeToggleBox == nil || mw.viewModeListToggle == nil || mw.viewModeTileToggle == nil {
		return
	}

	hasAPIKey := strings.TrimSpace(mw.sgdbAPIKey) != ""
	tileModeAvailable := mw.currentCategory == wiiudownloader.TITLE_CATEGORY_GAME || mw.currentCategory == wiiudownloader.TITLE_CATEGORY_ALL
	mw.viewModeToggleBox.ToWidget().SetVisible(hasAPIKey)
	mw.viewModeListToggle.SetSensitive(hasAPIKey)
	mw.viewModeTileToggle.SetSensitive(hasAPIKey && tileModeAvailable)

	mw.updatingViewModeToggle = true
	mw.viewModeListToggle.SetActive(!mw.hasTileMode())
	mw.viewModeTileToggle.SetActive(mw.hasTileMode())
	mw.updatingViewModeToggle = false

	if !hasAPIKey {
		return
	}

	if listIcon, err := gtk.ImageNewFromIconName("view-list-symbolic", gtk.ICON_SIZE_BUTTON); err == nil {
		mw.viewModeListToggle.SetImage(listIcon)
		mw.viewModeListToggle.SetAlwaysShowImage(true)
	}
	if tileIcon, err := gtk.ImageNewFromIconName("view-grid-symbolic", gtk.ICON_SIZE_BUTTON); err == nil {
		mw.viewModeTileToggle.SetImage(tileIcon)
		mw.viewModeTileToggle.SetAlwaysShowImage(true)
	}
	mw.viewModeListToggle.ToWidget().SetProperty("tooltip-text", "List mode")
	mw.viewModeTileToggle.ToWidget().SetProperty("tooltip-text", "Tile mode")
}

func (mw *MainWindow) persistTileModePreference(showTiles bool) {
	config, err := loadConfig()
	if err != nil {
		ShowErrorDialog(mw.window, err)
		return
	}

	config.ShowTiles = showTiles && strings.TrimSpace(config.SGDBAPIKey) != ""
	if err := config.Save(); err != nil {
		ShowErrorDialog(mw.window, err)
	}
}

func (mw *MainWindow) hasTileMode() bool {
	if !mw.showTiles || mw.sgdbAPIKey == "" {
		return false
	}
	return mw.currentCategory == wiiudownloader.TITLE_CATEGORY_GAME || mw.currentCategory == wiiudownloader.TITLE_CATEGORY_ALL
}

func (mw *MainWindow) refreshTileViewIfNeeded() {
	if mw.contentScroll == nil {
		return
	}
	mw.refreshContentView()
}

func (mw *MainWindow) titleEntryMatchesCurrentFilters(entry wiiudownloader.TitleEntry) bool {
	if mw.currentCategory != wiiudownloader.TITLE_CATEGORY_ALL && entry.Category != mw.currentCategory {
		return false
	}
	if mw.currentRegion&entry.Region == 0 {
		return false
	}
	tidStr := fmt.Sprintf("%016x", entry.TitleID)
	return titleMatchesSearch(mw.lastSearchText, entry.Name, tidStr)
}

func (mw *MainWindow) updateSortDirButton() {
	if mw.tileSortDirButton == nil {
		return
	}
	iconName := "view-sort-ascending-symbolic"
	tooltip := "Ascending"
	if !mw.tileSortAscending {
		iconName = "view-sort-descending-symbolic"
		tooltip = "Descending"
	}
	if icon, err := gtk.ImageNewFromIconName(iconName, gtk.ICON_SIZE_BUTTON); err == nil {
		mw.tileSortDirButton.SetImage(icon)
		mw.tileSortDirButton.SetAlwaysShowImage(true)
	}
	mw.tileSortDirButton.ToWidget().SetProperty("tooltip-text", tooltip)
}

func (mw *MainWindow) sortTitleEntries(entries []wiiudownloader.TitleEntry) {
	sortIdx := 0
	if mw.tileSortCombo != nil {
		sortIdx = mw.tileSortCombo.GetActive()
	}
	asc := mw.tileSortAscending
	sort.Slice(entries, func(i, j int) bool {
		var less bool
		switch sortIdx {
		case 1: // Title ID
			less = entries[i].TitleID < entries[j].TitleID
		case 2: // Region
			less = strings.ToLower(wiiudownloader.GetFormattedRegion(entries[i].Region)) < strings.ToLower(wiiudownloader.GetFormattedRegion(entries[j].Region))
		default: // Name
			nameI := strings.ToLower(stripSGDBPrefixes(entries[i].Name))
			nameJ := strings.ToLower(stripSGDBPrefixes(entries[j].Name))
			less = nameI < nameJ
		}
		if asc {
			return less
		}
		return !less
	})
}

func (mw *MainWindow) visibleTitleEntries() []wiiudownloader.TitleEntry {
	entries := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL)
	visible := make([]wiiudownloader.TitleEntry, 0, len(entries))
	for _, entry := range entries {
		if mw.titleEntryMatchesCurrentFilters(entry) {
			visible = append(visible, entry)
		}
	}
	mw.sortTitleEntries(visible)
	return visible
}

func (mw *MainWindow) refreshContentView() {
	if mw.contentScroll == nil {
		return
	}
	if mw.hasTileMode() {
		if mw.tileSortBar != nil {
			mw.tileSortBar.ToWidget().SetVisible(true)
		}
		if err := mw.ensureTileView(); err != nil {
			ShowErrorDialog(mw.window, err)
			return
		}
		mw.swapContentChild(mw.tileFlowBox)
		mw.rebuildTileView()
		return
	}
	if mw.tileSortBar != nil {
		mw.tileSortBar.ToWidget().SetVisible(false)
	}
	mw.swapContentChild(mw.treeView)
	if mw.filterModel != nil {
		mw.filterModel.Refilter()
	}
	mw.ensureTreeViewCursor()
}

func (mw *MainWindow) swapContentChild(widget gtk.IWidget) {
	if mw.contentScroll == nil || widget == nil {
		return
	}
	if existing, err := mw.contentScroll.GetChild(); err == nil && existing != nil {
		mw.contentScroll.Remove(existing)
	}
	mw.contentScroll.Add(widget)
	widget.ToWidget().ShowAll()
	mw.contentScroll.ShowAll()
}

func (mw *MainWindow) ensureTileView() error {
	if mw.tileFlowBox != nil {
		mw.ensureTileLazyLoader()
		return nil
	}
	flowBox, err := gtk.FlowBoxNew()
	if err != nil {
		return err
	}
	flowBox.SetSelectionMode(gtk.SELECTION_NONE)
	flowBox.SetActivateOnSingleClick(false)
	flowBox.SetRowSpacing(16)
	flowBox.SetColumnSpacing(10)
	flowBox.SetMinChildrenPerLine(TITLE_TILE_MIN_PER_LINE)
	flowBox.SetMaxChildrenPerLine(TITLE_TILE_MAX_PER_LINE)
	flowBox.SetHomogeneous(true)
	addStyleClass(flowBox.GetStyleContext, "title-tiles")
	mw.tileFlowBox = flowBox
	mw.ensureTileLazyLoader()
	return nil
}

const (
	SCROLL_DEBOUNCE_DELAY = 150 * time.Millisecond
)

func (mw *MainWindow) ensureTileLazyLoader() {
	if mw.tileLazyLoaderConnected || mw.contentScroll == nil {
		return
	}

	vAdjustment := mw.contentScroll.GetVAdjustment()
	if vAdjustment == nil {
		return
	}

	vAdjustment.Connect("value-changed", func() {
		if mw.scrollDebounceTimer != nil {
			mw.scrollDebounceTimer.Stop()
		}
		mw.scrollDebounceTimer = time.AfterFunc(SCROLL_DEBOUNCE_DELAY, func() {
			uiIdleAdd(func() {
				mw.lazyLoadTileArtworkForViewport()
			})
		})
	})
	mw.tileLazyLoaderConnected = true
}

func (mw *MainWindow) lazyLoadTileArtworkForViewport() {
	if !mw.hasTileMode() || len(mw.tileDisplayOrder) == 0 {
		return
	}

	loadStart, loadEnd := mw.currentTileLoadRange()
	keepStart := loadStart - TITLE_TILE_LOAD_AHEAD
	if keepStart < 0 {
		keepStart = 0
	}
	keepEnd := loadEnd + TITLE_TILE_LOAD_AHEAD
	if keepEnd > len(mw.tileDisplayOrder) {
		keepEnd = len(mw.tileDisplayOrder)
	}

	keepSet := make(map[uint64]struct{}, keepEnd-keepStart)
	for i := keepStart; i < keepEnd; i++ {
		keepSet[mw.tileDisplayOrder[i]] = struct{}{}
	}

	for i := loadStart; i < loadEnd; i++ {
		titleID := mw.tileDisplayOrder[i]
		card, ok := mw.tileCards[titleID]
		if ok && card != nil {
			mw.loadTileArtwork(titleID, card.entry.Name)
		}
	}

	for i, titleID := range mw.tileDisplayOrder {
		if i >= keepStart && i < keepEnd {
			continue
		}
		card, ok := mw.tileCards[titleID]
		if ok && card != nil {
			mw.unloadTileCardImage(card)
		}
	}

	if mw.tileArtwork != nil {
		mw.tileArtwork.retainVisible(keepSet)
	}

	if loadStart == 0 {
		initialCount := TITLE_TILE_INITIAL_LOAD
		if initialCount > len(mw.tileDisplayOrder) {
			initialCount = len(mw.tileDisplayOrder)
		}
		for i := 0; i < initialCount; i++ {
			titleID := mw.tileDisplayOrder[i]
			card, ok := mw.tileCards[titleID]
			if ok && card != nil {
				mw.loadTileArtwork(titleID, card.entry.Name)
			}
		}
	}
}

func (mw *MainWindow) currentTileLoadRange() (int, int) {
	total := len(mw.tileDisplayOrder)
	if total == 0 {
		return 0, 0
	}

	window := TITLE_TILE_ACTIVE_WINDOW
	if window < TITLE_TILE_INITIAL_LOAD {
		window = TITLE_TILE_INITIAL_LOAD
	}
	if window > total {
		window = total
	}

	center := window / 2
	vAdjustment := mw.contentScroll.GetVAdjustment()
	if vAdjustment != nil {
		upper := vAdjustment.GetUpper()
		pageSize := vAdjustment.GetPageSize()
		value := vAdjustment.GetValue()
		maxScroll := upper - pageSize
		progress := 0.0
		if maxScroll > 0 {
			progress = value / maxScroll
		}
		if progress < 0 {
			progress = 0
		}
		if progress > 1 {
			progress = 1
		}
		center = int(progress * float64(total-1))
	}

	start := center - window/2
	if start < 0 {
		start = 0
	}
	if start+window > total {
		start = total - window
	}
	if start < 0 {
		start = 0
	}

	end := start + window
	if end > total {
		end = total
	}
	return start, end
}

func (mw *MainWindow) rebuildTileView() {
	if mw.tileFlowBox == nil {
		return
	}
	for {
		child := mw.tileFlowBox.GetChildAtIndex(0)
		if child == nil {
			break
		}
		mw.tileFlowBox.Remove(child)
	}
	mw.tileCards = make(map[uint64]*titleTileCard)
	mw.tileDisplayOrder = mw.tileDisplayOrder[:0]

	entries := mw.visibleTitleEntries()
	visibleSet := make(map[uint64]struct{}, len(entries))
	for _, entry := range entries {
		visibleSet[entry.TitleID] = struct{}{}
	}
	if mw.tileArtwork != nil {
		mw.tileArtwork.retainVisible(visibleSet)
	}

	if len(entries) == 0 {
		emptyLabel, err := gtk.LabelNew("No titles match the current filters.")
		if err == nil {
			emptyLabel.SetHAlign(gtk.ALIGN_CENTER)
			emptyLabel.SetMarginTop(24)
			emptyLabel.SetMarginBottom(24)
			mw.tileFlowBox.Insert(emptyLabel, -1)
		}
		mw.tileFlowBox.ShowAll()
		return
	}

	for _, entry := range entries {
		card, err := mw.createTileCard(entry)
		if err != nil {
			continue
		}
		mw.tileCards[entry.TitleID] = card
		mw.tileDisplayOrder = append(mw.tileDisplayOrder, entry.TitleID)
		mw.tileFlowBox.Insert(card.button, -1)
		mw.applyTileQueueState(card)
	}

	mw.tileFlowBox.ShowAll()
	mw.lazyLoadTileArtworkForViewport()
}

func (mw *MainWindow) createTileCard(entry wiiudownloader.TitleEntry) (*titleTileCard, error) {
	button, err := gtk.ButtonNew()
	if err != nil {
		return nil, err
	}
	button.SetRelief(gtk.RELIEF_NONE)
	button.SetHExpand(true)
	button.SetCanFocus(true)
	addStyleClass(button.GetStyleContext, "title-tile")
	SetupButtonAccessibility(button, fmt.Sprintf("Toggle %s in queue", entry.Name))

	content, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)
	if err != nil {
		return nil, err
	}
	content.SetMarginTop(8)
	content.SetMarginBottom(8)
	content.SetMarginStart(5)
	content.SetMarginEnd(5)

	image, err := gtk.ImageNew()
	if err != nil {
		return nil, err
	}
	image.SetSizeRequest(TITLE_TILE_IMAGE_WIDTH, TITLE_TILE_IMAGE_HEIGHT)
	image.SetHAlign(gtk.ALIGN_CENTER)
	image.SetVAlign(gtk.ALIGN_CENTER)

	spinner, err := gtk.SpinnerNew()
	if err != nil {
		return nil, err
	}
	spinner.SetHAlign(gtk.ALIGN_CENTER)
	spinner.SetVAlign(gtk.ALIGN_CENTER)

	mediaBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	mediaBox.SetSizeRequest(TITLE_TILE_IMAGE_WIDTH, TITLE_TILE_IMAGE_HEIGHT)
	mediaBox.PackStart(spinner, true, true, 0)
	mediaBox.PackStart(image, true, true, 0)
	content.PackStart(mediaBox, false, false, 0)

	titleLabel, err := gtk.LabelNew(entry.Name)
	if err != nil {
		return nil, err
	}
	titleLabel.SetLineWrap(true)
	titleLabel.SetLineWrapMode(pango.WRAP_WORD_CHAR)
	titleLabel.SetMaxWidthChars(24)
	titleLabel.SetHAlign(gtk.ALIGN_START)
	titleLabel.SetXAlign(0)
	addStyleClass(titleLabel.GetStyleContext, "title-tile-name")
	content.PackStart(titleLabel, false, false, 0)

	metaLabel, err := gtk.LabelNew(fmt.Sprintf("%016x\n%s", entry.TitleID, wiiudownloader.GetFormattedRegion(entry.Region)))
	if err != nil {
		return nil, err
	}
	metaLabel.SetLineWrap(true)
	metaLabel.SetHAlign(gtk.ALIGN_START)
	metaLabel.SetXAlign(0)
	addStyleClass(metaLabel.GetStyleContext, "title-tile-meta")
	content.PackStart(metaLabel, false, false, 0)

	button.Add(content)
	button.Connect("clicked", func() {
		mw.toggleQueueForEntry(entry)
	})
	button.ToWidget().SetProperty("tooltip-text", fmt.Sprintf("%s\n%016x\n%s", entry.Name, entry.TitleID, wiiudownloader.GetFormattedRegion(entry.Region)))

	return &titleTileCard{
		entry:       entry,
		button:      button,
		image:       image,
		spinner:     spinner,
		titleLabel:  titleLabel,
		metaLabel:   metaLabel,
		imageLoaded: false,
	}, nil
}

func (mw *MainWindow) unloadTileCardImage(card *titleTileCard) {
	if card == nil {
		return
	}
	if card.spinner != nil {
		card.spinner.Stop()
		card.spinner.Hide()
	}
	if card.image == nil || !card.imageLoaded {
		return
	}
	card.image.SetFromPixbuf(nil)
	card.image.Hide()
	card.imageLoaded = false
}

func (mw *MainWindow) showTileLoading(card *titleTileCard) {
	if card == nil {
		return
	}
	if card.image != nil {
		card.image.Hide()
	}
	if card.spinner != nil {
		card.spinner.Start()
		card.spinner.Show()
	}
	card.imageLoaded = false
}

func (mw *MainWindow) showTileNoImage(card *titleTileCard) {
	if card == nil || card.image == nil {
		return
	}
	if card.spinner != nil {
		card.spinner.Stop()
		card.spinner.Hide()
	}
	card.image.SetFromIconName("image-x-generic", gtk.ICON_SIZE_DIALOG)
	card.image.Show()
	card.imageLoaded = false
}

func (mw *MainWindow) applyTileQueueState(card *titleTileCard) {
	if card == nil || card.button == nil {
		return
	}
	styleContext, err := card.button.GetStyleContext()
	if err == nil && styleContext != nil {
		if mw.queuePane.IsTitleInQueue(card.entry) {
			styleContext.AddClass("in-queue")
		} else {
			styleContext.RemoveClass("in-queue")
		}
	}
	card.button.ToWidget().SetProperty("tooltip-text", fmt.Sprintf("%s\n%016x\n%s", card.entry.Name, card.entry.TitleID, wiiudownloader.GetFormattedRegion(card.entry.Region)))
}

func (mw *MainWindow) updateTileCardsQueueState() {
	for _, card := range mw.tileCards {
		mw.applyTileQueueState(card)
	}
}

func (mw *MainWindow) toggleQueueForEntry(entry wiiudownloader.TitleEntry) {
	if mw.queuePane.IsTitleInQueue(entry) {
		mw.queuePane.RemoveTitles([]uint64{entry.TitleID})
	} else {
		mw.addTitlesToQueue([]wiiudownloader.TitleEntry{entry})
		if mw.suggestRelatedContent {
			candidates := mw.collectRelatedCandidates([]wiiudownloader.TitleEntry{entry})
			if len(candidates) > 0 {
				chosenRelated, accepted := mw.showRelatedTitlesDialog([]wiiudownloader.TitleEntry{entry}, candidates)
				if accepted {
					mw.addTitlesToQueue(chosenRelated)
				}
			}
		}
	}
	mw.updateTitlesInQueue()
}

func (mw *MainWindow) loadTileArtwork(titleID uint64, titleName string) {
	if !mw.hasTileMode() || mw.tileArtwork == nil {
		return
	}
	card, hasCard := mw.tileCards[titleID]
	if !hasCard || card == nil {
		return
	}
	if card.imageLoaded {
		return
	}
	mw.showTileLoading(card)
	if cachedImageData, ok := mw.tileArtwork.get(titleID); ok {
		if !mw.setTileImageFromBytes(titleID, cachedImageData) {
			mw.showTileNoImage(card)
		}
		return
	}
	if !mw.tileArtwork.start(titleID) {
		if mw.tileArtwork.isFailed(titleID) {
			mw.showTileNoImage(card)
		}
		return
	}

	go func() {
		mw.tileImageSemaphore <- struct{}{}
		defer func() {
			<-mw.tileImageSemaphore
		}()

		ctx, cancel := context.WithTimeout(mw.tileLoaderCtx, SGDB_REQUEST_TIMEOUT)
		defer cancel()

		imageData, err := mw.fetchAndCacheSGDBTile(ctx, titleID, titleName)
		uiIdleAdd(func() {
			mw.tileArtwork.finish(titleID, imageData, err)
			if err != nil {
				if failedCard, ok := mw.tileCards[titleID]; ok {
					mw.showTileNoImage(failedCard)
				}
				return
			}
			if !mw.setTileImageFromBytes(titleID, imageData) {
				if failedCard, ok := mw.tileCards[titleID]; ok {
					mw.showTileNoImage(failedCard)
				}
			}
		})
	}()
}

func (mw *MainWindow) setTileImageFromBytes(titleID uint64, imageData []byte) bool {
	card, ok := mw.tileCards[titleID]
	if !ok || card == nil || card.image == nil || len(imageData) == 0 {
		return false
	}
	if card.imageLoaded {
		return true
	}

	loader, err := gdk.PixbufLoaderNew()
	if err != nil {
		return false
	}
	pixbuf, err := loader.WriteAndReturnPixbuf(imageData)
	if err != nil {
		return false
	}
	scaledPixbuf, err := pixbuf.ScaleSimple(TITLE_TILE_IMAGE_WIDTH, TITLE_TILE_IMAGE_HEIGHT, gdk.INTERP_BILINEAR)
	if err != nil {
		return false
	}
	if card.spinner != nil {
		card.spinner.Stop()
		card.spinner.Hide()
	}
	card.image.SetFromPixbuf(scaledPixbuf)
	card.image.Show()
	card.imageLoaded = true
	return true
}

func (mw *MainWindow) fetchAndCacheSGDBTile(ctx context.Context, titleID uint64, titleName string) ([]byte, error) {
	// Search SGDB by name to get the SGDB game ID, then fetch grids for that game.
	if cachedGameID, ok := mw.sgdbIDCache.Get(titleID); ok {
		imageURL, err := mw.fetchSGDBGridURL(ctx, cachedGameID)
		if err == nil {
			log.Printf("[SGDB] Reusing cached SGDB game ID %d for '%s'", cachedGameID, titleName)
			return mw.downloadSGDBImage(ctx, imageURL)
		}
		log.Printf("[SGDB] Cached SGDB game ID %d had no grids for '%s', searching again", cachedGameID, titleName)
	}

	gameIDs, err := mw.fetchSGDBGameIDs(ctx, titleName)
	if err != nil || len(gameIDs) == 0 {
		log.Printf("[SGDB] Failed to find match for: %s - %v", titleName, err)
		return mw.downloadSGDBImage(ctx, SGDB_DEFAULT_TILE_IMAGE_URL)
	}

	for i, gameID := range gameIDs {
		imageURL, err := mw.fetchSGDBGridURL(ctx, gameID)
		if err == nil {
			log.Printf("[SGDB] Found grids for '%s' via gameID %d (candidate %d)", titleName, gameID, i+1)
			mw.sgdbIDCache.Set(titleID, gameID)
			if err := mw.sgdbIDCache.Save(); err != nil {
				log.Printf("[SGDB] Warning: failed to persist SGDB ID cache: %v", err)
			}
			return mw.downloadSGDBImage(ctx, imageURL)
		}
		if i < len(gameIDs)-1 {
			log.Printf("[SGDB] Candidate %d (gameID %d) has no grids, trying next...", i+1, gameID)
		}
	}

	log.Printf("[SGDB] None of %d candidates had grids for: %s", len(gameIDs), titleName)
	return mw.downloadSGDBImage(ctx, SGDB_DEFAULT_TILE_IMAGE_URL)
}

func stripSGDBPrefixes(titleName string) string {
	// Strip common title prefixes that prevent SGDB matching
	prefixes := []string{
		"(Event Preview) ",
		"[Demo] ",
		"(Virtual Console) ",
		"[Virtual Console] ",
		"(demo) ",
		"[demo] ",
	}
	result := titleName
	for _, prefix := range prefixes {
		if strings.HasPrefix(result, prefix) {
			result = strings.TrimPrefix(result, prefix)
			log.Printf("[SGDB] Stripped prefix from '%s' -> '%s'", titleName, result)
			break
		}
	}
	return result
}

func hasLatinOrDigit(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Latin, r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// containsWordAnd returns true if s contains " and " as a whole word (case-insensitive).
func containsWordAnd(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, " and ")
}

// replaceWordAnd replaces all " and " occurrences (case-insensitive) with replacement.
func replaceWordAnd(s string, replacement string) string {
	lower := strings.ToLower(s)
	result := s
	for {
		idx := strings.Index(strings.ToLower(result), " and ")
		if idx == -1 {
			break
		}
		result = result[:idx] + replacement + result[idx+5:]
		_ = lower
	}
	return result
}

func extractAliasesFromDelimitedText(titleName string, openDelimiter string, closeDelimiter string) []string {
	aliases := make([]string, 0)
	searchFrom := 0
	for {
		startRel := strings.Index(titleName[searchFrom:], openDelimiter)
		if startRel < 0 {
			break
		}
		start := searchFrom + startRel + len(openDelimiter)
		endRel := strings.Index(titleName[start:], closeDelimiter)
		if endRel < 0 {
			break
		}
		end := start + endRel
		alias := strings.TrimSpace(titleName[start:end])
		if alias != "" && hasLatinOrDigit(alias) {
			aliases = append(aliases, alias)
		}
		searchFrom = end + len(closeDelimiter)
		if searchFrom >= len(titleName) {
			break
		}
	}
	return aliases
}

func sgdbSearchCandidates(titleName string) []string {
	base := stripSGDBPrefixes(strings.TrimSpace(titleName))
	if base == "" {
		return []string{titleName}
	}

	// Canonicalize & -> and so both "Kick & Fennick" and "Kick and Fennick" start from the same base.
	base = strings.ReplaceAll(base, " & ", " and ")

	candidates := make([]string, 0, 6)
	seen := make(map[string]struct{})
	addCandidate := func(name string) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, trimmed)
	}

	// Prioritize likely English aliases in delimiters before trying the original string.
	for _, alias := range extractAliasesFromDelimitedText(base, "(", ")") {
		addCandidate(alias)
	}
	for _, alias := range extractAliasesFromDelimitedText(base, "（", "）") {
		addCandidate(alias)
	}

	// Also try a simplified variant with delimiters removed.
	clean := strings.NewReplacer("（", "(", "）", ")", "【", "[", "】", "]").Replace(base)
	addCandidate(clean)
	addCandidate(base)
	addCandidate(insertAlnumBoundaries(base))
	addCandidate(removeAlnumBoundarySpaces(base))
	addCandidate(insertAlnumBoundaries(clean))
	addCandidate(removeAlnumBoundarySpaces(clean))

	// Add & <-> and variants for each candidate so far.
	existing := make([]string, len(candidates))
	copy(existing, candidates)
	for _, c := range existing {
		if strings.Contains(c, " & ") {
			addCandidate(strings.ReplaceAll(c, " & ", " and "))
		}
		if containsWordAnd(c) {
			addCandidate(replaceWordAnd(c, " & "))
		}
	}

	if len(candidates) == 0 {
		return []string{titleName}
	}
	return candidates
}

func isAlphaNumBoundary(prev rune, curr rune) bool {
	return (unicode.IsLetter(prev) && unicode.IsDigit(curr)) || (unicode.IsDigit(prev) && unicode.IsLetter(curr))
}

func insertAlnumBoundaries(s string) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) < 2 {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 4)
	b.WriteRune(runes[0])
	lastWasSpace := runes[0] == ' '

	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		curr := runes[i]
		if isAlphaNumBoundary(prev, curr) {
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
		}
		b.WriteRune(curr)
		lastWasSpace = curr == ' '
	}

	return b.String()
}

func removeAlnumBoundarySpaces(s string) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) < 3 {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i, r := range runes {
		if r == ' ' && i > 0 && i < len(runes)-1 {
			if isAlphaNumBoundary(runes[i-1], runes[i+1]) {
				continue
			}
		}
		b.WriteRune(r)
	}

	return b.String()
}

func normalizedSGDBTitleForMatch(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	// Canonicalize symbol variants before normalization.
	trimmed = strings.ReplaceAll(trimmed, "&", " and ")
	trimmed = insertAlnumBoundaries(trimmed)
	return normalizeSearchText(trimmed)
}

func hasWiiUType(types []string) bool {
	for _, t := range types {
		if strings.EqualFold(t, "wiiu") {
			return true
		}
	}
	return false
}

func sgdbIsStopword(token string) bool {
	switch token {
	case "a", "an", "and", "the", "of", "to", "for", "in", "on", "at", "by", "with", "game":
		return true
	default:
		return false
	}
}

func sgdbMeaningfulTokenSet(name string) map[string]struct{} {
	normalized := normalizedSGDBTitleForMatch(name)
	tokens := strings.Fields(normalized)
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if sgdbIsStopword(token) {
			continue
		}
		set[token] = struct{}{}
	}
	return set
}

func sgdbMeaningfulOverlapCount(searchName string, candidateName string) int {
	searchSet := sgdbMeaningfulTokenSet(searchName)
	if len(searchSet) == 0 {
		return 0
	}
	candidateSet := sgdbMeaningfulTokenSet(candidateName)
	overlap := 0
	for token := range candidateSet {
		if _, ok := searchSet[token]; ok {
			overlap++
		}
	}
	return overlap
}

func sgdbCandidateIsRelevant(searchName string, candidateName string) bool {
	searchSet := sgdbMeaningfulTokenSet(searchName)
	if len(searchSet) == 0 {
		// If everything was stripped as stopwords, be permissive.
		return true
	}

	overlap := sgdbMeaningfulOverlapCount(searchName, candidateName)
	if overlap == 0 {
		return false
	}

	// For short names, require all meaningful terms to match.
	if len(searchSet) <= 2 {
		return overlap == len(searchSet)
	}

	// For longer names, require at least half of meaningful terms.
	return overlap*2 >= len(searchSet)
}

func sgdbCandidateScore(searchName string, candidateName string, candidateTypes []string) int {
	normalizedSearch := normalizedSGDBTitleForMatch(searchName)
	normalizedCandidate := normalizedSGDBTitleForMatch(candidateName)
	if normalizedSearch == "" || normalizedCandidate == "" {
		return -1
	}

	searchTokens := strings.Fields(normalizedSearch)
	candidateTokens := strings.Fields(normalizedCandidate)
	if len(searchTokens) == 0 || len(candidateTokens) == 0 {
		return -1
	}

	searchSet := make(map[string]struct{}, len(searchTokens))
	for _, token := range searchTokens {
		searchSet[token] = struct{}{}
	}

	overlap := 0
	seenCandidate := make(map[string]struct{}, len(candidateTokens))
	for _, token := range candidateTokens {
		if _, ok := seenCandidate[token]; ok {
			continue
		}
		seenCandidate[token] = struct{}{}
		if _, ok := searchSet[token]; ok {
			overlap++
		}
	}

	meaningfulOverlap := sgdbMeaningfulOverlapCount(searchName, candidateName)

	// Strongly prefer close textual matches; use platform as a tie-breaker boost.
	score := overlap * 100
	score += meaningfulOverlap * 250
	if normalizedCandidate == normalizedSearch {
		score += 600
	}
	if overlap == len(searchSet) {
		score += 200
	}
	if hasWiiUType(candidateTypes) {
		score += 30
	}
	return score
}

func (mw *MainWindow) fetchSGDBGameIDs(ctx context.Context, titleName string) ([]int, error) {
	searchCandidates := sgdbSearchCandidates(titleName)
	seenIDs := make(map[int]struct{})
	results := make([]int, 0, 4)
	lastErr := error(nil)

	for _, searchName := range searchCandidates {
		requestURL := fmt.Sprintf(SGDB_SEARCH_ENDPOINT, url.PathEscape(searchName))
		var response sgdbAutocompleteResponse
		if err := mw.doSGDBRequest(ctx, requestURL, &response); err != nil {
			lastErr = err
			continue
		}
		if !response.Success || len(response.Data) == 0 {
			continue
		}

		type scoredCandidate struct {
			id    int
			name  string
			score int
		}
		scored := make([]scoredCandidate, 0, len(response.Data))
		for _, candidate := range response.Data {
			if !sgdbCandidateIsRelevant(searchName, candidate.Name) {
				continue
			}
			scored = append(scored, scoredCandidate{
				id:    candidate.ID,
				name:  candidate.Name,
				score: sgdbCandidateScore(searchName, candidate.Name, candidate.Types),
			})
		}
		if len(scored) == 0 {
			log.Printf("[SGDB] No relevant matches for '%s'", searchName)
			continue
		}

		sort.Slice(scored, func(i, j int) bool {
			if scored[i].score == scored[j].score {
				return scored[i].id < scored[j].id
			}
			return scored[i].score > scored[j].score
		})

		bestID := scored[0].id
		bestName := scored[0].name
		bestScore := scored[0].score
		for _, candidate := range scored {
			if len(results) >= 4 {
				break
			}
			if _, exists := seenIDs[candidate.id]; exists {
				continue
			}
			seenIDs[candidate.id] = struct{}{}
			results = append(results, candidate.id)
		}

		log.Printf("[SGDB] Best match for '%s': gameID %d ('%s') score=%d", searchName, bestID, bestName, bestScore)
		if len(results) >= 4 {
			break
		}
	}

	if len(results) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no SGDB match found for %s", titleName)
	}

	return results, nil
}

func (mw *MainWindow) fetchSGDBGridURL(ctx context.Context, gameID int) (string, error) {
	requestURL := fmt.Sprintf(SGDB_BOXART_ENDPOINT, gameID)
	var response sgdbGridResponse
	if err := mw.doSGDBRequest(ctx, requestURL, &response); err != nil {
		return "", err
	}
	if !response.Success || len(response.Data) == 0 {
		return "", fmt.Errorf("no SGDB box art found for game id %d", gameID)
	}

	// Find the highest-scored image URL (trust SGDB API validity)
	var bestImageURL string
	var bestScore float64 = -1
	for _, img := range response.Data {
		if img.URL != "" && img.Score > bestScore {
			bestScore = img.Score
			bestImageURL = img.URL
		}
	}

	if bestImageURL == "" {
		return "", fmt.Errorf("no valid SGDB box art found for game id %d", gameID)
	}
	return bestImageURL, nil
}

func (mw *MainWindow) doSGDBRequest(ctx context.Context, requestURL string, target interface{}) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+mw.sgdbAPIKey)

	response, err := mw.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("SGDB request failed with status %d", response.StatusCode)
	}

	return json.NewDecoder(response.Body).Decode(target)
}

func (mw *MainWindow) downloadSGDBImage(ctx context.Context, imageURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}

	response, err := mw.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Printf("[SGDB] Image download failed with status %d for URL: %s", response.StatusCode, imageURL)
		return nil, fmt.Errorf("SGDB image download failed with status %d", response.StatusCode)
	}

	contentType := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Type")))
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("SGDB image download returned non-image content type %q", contentType)
	}

	return io.ReadAll(response.Body)
}
