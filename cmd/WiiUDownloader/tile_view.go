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
	"path"
	"path/filepath"
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
	SGDB_TILE_CACHE_VERSION     = "boxart-v1"
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
	paths   map[uint64]string
	loading map[uint64]bool
	failed  map[uint64]bool
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
		paths:   make(map[uint64]string),
		loading: make(map[uint64]bool),
		failed:  make(map[uint64]bool),
	}
}

func (s *tileArtworkStore) get(titleID uint64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.paths[titleID]
	return path, ok
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

func (s *tileArtworkStore) finish(titleID uint64, imagePath string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.loading, titleID)
	if err != nil {
		s.failed[titleID] = true
		return
	}
	s.paths[titleID] = imagePath
	delete(s.failed, titleID)
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
	if mw.viewModeToggle == nil {
		return
	}

	hasAPIKey := strings.TrimSpace(mw.sgdbAPIKey) != ""
	mw.viewModeToggle.ToWidget().SetVisible(hasAPIKey)
	mw.viewModeToggle.SetSensitive(hasAPIKey)

	mw.updatingViewModeToggle = true
	mw.viewModeToggle.SetActive(mw.showTiles)
	mw.updatingViewModeToggle = false

	if !hasAPIKey {
		return
	}

	iconName := "view-list-symbolic"
	tooltip := "Switch to list mode"
	if !mw.showTiles {
		iconName = "view-grid-symbolic"
		tooltip = "Switch to tile mode"
	}

	if icon, err := gtk.ImageNewFromIconName(iconName, gtk.ICON_SIZE_BUTTON); err == nil {
		mw.viewModeToggle.SetImage(icon)
		mw.viewModeToggle.SetAlwaysShowImage(true)
	}
	mw.viewModeToggle.ToWidget().SetProperty("tooltip-text", tooltip)
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
	return mw.showTiles && mw.sgdbAPIKey != ""
}

func (mw *MainWindow) refreshTileViewIfNeeded() {
	if mw.hasTileMode() {
		mw.refreshContentView()
	}
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
	if cachedPath, ok := mw.tileArtwork.get(titleID); ok {
		mw.setTileImageFromPath(titleID, cachedPath)
		return
	}
	cachePath := sgdbTileCachePath(titleID)
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		mw.tileArtwork.finish(titleID, cachePath, nil)
		mw.setTileImageFromPath(titleID, cachePath)
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

		imagePath, err := mw.fetchAndCacheSGDBTile(ctx, titleID, titleName)
		mw.tileArtwork.finish(titleID, imagePath, err)
		if err != nil {
			return
		}
		uiIdleAdd(func() {
			mw.setTileImageFromPath(titleID, imagePath)
		})
	}()
}

func (mw *MainWindow) setTileImageFromPath(titleID uint64, imagePath string) {
	card, ok := mw.tileCards[titleID]
	if !ok || card == nil || card.image == nil {
		return
	}
	pixbuf, err := gdk.PixbufNewFromFileAtScale(imagePath, TITLE_TILE_IMAGE_WIDTH, TITLE_TILE_IMAGE_HEIGHT, false)
	if err != nil {
		return
	}
	card.image.SetFromPixbuf(pixbuf)
}

func (mw *MainWindow) fetchAndCacheSGDBTile(ctx context.Context, titleID uint64, titleName string) (string, error) {
	// Search SGDB by name to get the SGDB game ID, then fetch grids for that game.
	gameIDs, err := mw.fetchSGDBGameIDs(ctx, titleName)
	if err != nil || len(gameIDs) == 0 {
		log.Printf("[SGDB] Failed to find match for: %s - %v", titleName, err)
		return "", err
	}

	for i, gameID := range gameIDs {
		imageURL, err := mw.fetchSGDBGridURL(ctx, gameID)
		if err == nil {
			log.Printf("[SGDB] Found grids for '%s' via gameID %d (candidate %d)", titleName, gameID, i+1)
			return mw.downloadSGDBImage(ctx, titleID, imageURL)
		}
		if i < len(gameIDs)-1 {
			log.Printf("[SGDB] Candidate %d (gameID %d) has no grids, trying next...", i+1, gameID)
		}
	}

	log.Printf("[SGDB] None of %d candidates had grids for: %s", len(gameIDs), titleName)
	return "", fmt.Errorf("no SGDB grids found for %s after trying %d candidates", titleName, len(gameIDs))
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

func (mw *MainWindow) downloadSGDBImage(ctx context.Context, titleID uint64, imageURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}

	response, err := mw.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Printf("[SGDB] Image download failed with status %d for URL: %s", response.StatusCode, imageURL)
		return "", fmt.Errorf("SGDB image download failed with status %d", response.StatusCode)
	}

	cachePath := sgdbTileCachePathWithURL(titleID, imageURL)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", err
	}

	file, err := os.Create(cachePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, response.Body); err != nil {
		return "", err
	}

	log.Printf("[SGDB] Successfully downloaded image to: %s", cachePath)
	return cachePath, nil
}

func sgdbTileCacheDir() string {
	userCacheDir, err := os.UserCacheDir()
	if err != nil || userCacheDir == "" {
		return filepath.Join(os.TempDir(), "WiiUDownloader", "sgdb")
	}
	return filepath.Join(userCacheDir, "WiiUDownloader", "sgdb")
}

func sgdbTileCachePath(titleID uint64) string {
	baseDir := sgdbTileCacheDir()
	for _, extension := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		candidate := filepath.Join(baseDir, fmt.Sprintf("%016x-%s%s", titleID, SGDB_TILE_CACHE_VERSION, extension))
		if info, err := os.Stat(candidate); err == nil && info.Size() > 0 {
			return candidate
		}
	}
	return filepath.Join(baseDir, fmt.Sprintf("%016x-%s.jpg", titleID, SGDB_TILE_CACHE_VERSION))
}

func sgdbTileCachePathWithURL(titleID uint64, imageURL string) string {
	parsedURL, err := url.Parse(imageURL)
	extension := ".jpg"
	if err == nil {
		parsedExtension := strings.ToLower(path.Ext(parsedURL.Path))
		if parsedExtension != "" {
			extension = parsedExtension
		}
	}
	return filepath.Join(sgdbTileCacheDir(), fmt.Sprintf("%016x-%s%s", titleID, SGDB_TILE_CACHE_VERSION, extension))
}
