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
	"strconv"
	"strings"
	"sync"
	"time"

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
	SGDB_REQUEST_TIMEOUT        = 15 * time.Second
	SGDB_SEARCH_ENDPOINT        = "https://www.steamgriddb.com/api/v2/search/autocomplete/%s"
	SGDB_BOXART_ENDPOINT        = "https://www.steamgriddb.com/api/v2/grids/game/%d?dimensions=342x482,600x900,660x930&types=static&nsfw=false&humor=false&epilepsy=false&limit=1"
)

type titleTileCard struct {
	entry      wiiudownloader.TitleEntry
	button     *gtk.Button
	image      *gtk.Image
	titleLabel *gtk.Label
	metaLabel  *gtk.Label
}

type tileArtworkStore struct {
	mu      sync.Mutex
	images  map[uint64][]byte
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
	return imageData, ok
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
	delete(s.failed, titleID)
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

func (mw *MainWindow) visibleTitleEntries() []wiiudownloader.TitleEntry {
	entries := wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_ALL)
	visible := make([]wiiudownloader.TitleEntry, 0, len(entries))
	for _, entry := range entries {
		if mw.titleEntryMatchesCurrentFilters(entry) {
			visible = append(visible, entry)
		}
	}
	return visible
}

func (mw *MainWindow) refreshContentView() {
	if mw.contentScroll == nil {
		return
	}
	if mw.hasTileMode() {
		if err := mw.ensureTileView(); err != nil {
			ShowErrorDialog(mw.window, err)
			return
		}
		mw.swapContentChild(mw.tileFlowBox)
		mw.rebuildTileView()
		return
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
	return nil
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

	entries := mw.visibleTitleEntries()
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
		mw.tileFlowBox.Insert(card.button, -1)
		mw.applyTileQueueState(card)
		mw.loadTileArtwork(entry.TitleID, entry.Name)
	}

	mw.tileFlowBox.ShowAll()
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

	image, err := gtk.ImageNewFromIconName("image-x-generic", gtk.ICON_SIZE_DIALOG)
	if err != nil {
		return nil, err
	}
	image.SetSizeRequest(TITLE_TILE_IMAGE_WIDTH, TITLE_TILE_IMAGE_HEIGHT)
	content.PackStart(image, false, false, 0)

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
		entry:      entry,
		button:     button,
		image:      image,
		titleLabel: titleLabel,
		metaLabel:  metaLabel,
	}, nil
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
	if cachedImageData, ok := mw.tileArtwork.get(titleID); ok {
		mw.setTileImageFromBytes(titleID, cachedImageData)
		return
	}
	if !mw.tileArtwork.start(titleID) {
		return
	}

	go func() {
		mw.tileImageSemaphore <- struct{}{}
		defer func() {
			<-mw.tileImageSemaphore
		}()

		ctx, cancel := context.WithTimeout(context.Background(), SGDB_REQUEST_TIMEOUT)
		defer cancel()

		imageData, err := mw.fetchAndCacheSGDBTile(ctx, titleID, titleName)
		uiIdleAdd(func() {
			mw.tileArtwork.finish(titleID, imageData, err)
			if err != nil {
				return
			}
			mw.setTileImageFromBytes(titleID, imageData)
		})
	}()
}

func (mw *MainWindow) setTileImageFromBytes(titleID uint64, imageData []byte) {
	card, ok := mw.tileCards[titleID]
	if !ok || card == nil || card.image == nil || len(imageData) == 0 {
		return
	}

	loader, err := gdk.PixbufLoaderNew()
	if err != nil {
		return
	}
	pixbuf, err := loader.WriteAndReturnPixbuf(imageData)
	if err != nil {
		return
	}
	scaledPixbuf, err := pixbuf.ScaleSimple(TITLE_TILE_IMAGE_WIDTH, TITLE_TILE_IMAGE_HEIGHT, gdk.INTERP_BILINEAR)
	if err != nil {
		return
	}
	card.image.SetFromPixbuf(scaledPixbuf)
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
		return nil, err
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
	return nil, fmt.Errorf("no SGDB grids found for %s after trying %d candidates", titleName, len(gameIDs))
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

func (mw *MainWindow) fetchSGDBGameIDs(ctx context.Context, titleName string) ([]int, error) {
	searchName := stripSGDBPrefixes(titleName)
	requestURL := fmt.Sprintf(SGDB_SEARCH_ENDPOINT, url.PathEscape(searchName))
	var response sgdbAutocompleteResponse
	if err := mw.doSGDBRequest(ctx, requestURL, &response); err != nil {
		return nil, err
	}
	if !response.Success || len(response.Data) == 0 {
		return nil, fmt.Errorf("no SGDB match found for %s", titleName)
	}

	// Find the single best match by exact or closest name similarity.
	// Prefer exact match; otherwise use SGDB's own result ordering (first = most relevant).
	normalizedTitle := normalizeSearchText(searchName)
	bestID := response.Data[0].ID
	for _, candidate := range response.Data {
		if normalizeSearchText(candidate.Name) == normalizedTitle {
			bestID = candidate.ID
			break
		}
	}

	log.Printf("[SGDB] Best match for '%s': gameID %d ('%s')", searchName, bestID, response.Data[0].Name)
	return []int{bestID}, nil
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

	return io.ReadAll(response.Body)
}
