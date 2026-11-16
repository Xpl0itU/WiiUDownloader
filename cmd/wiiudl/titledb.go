package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

const (
	defaultTitleDBURL = "https://napi.v10lator.de/db?t=go"
	titleDBUserAgent  = "NUSspliBuilder/2.2"
	titleDBCacheDir   = "WiiUDownloader"
	titleDBCacheFile  = "titles.json"
	maxTitleDBBytes   = 64 << 20
)

type titleDB struct {
	entries   []wiiudownloader.TitleEntry
	byID      map[uint64]wiiudownloader.TitleEntry
	source    string
	cachePath string
	fetchedAt time.Time
}

func newTitleDB(entries []wiiudownloader.TitleEntry, source string) *titleDB {
	db := &titleDB{
		entries: make([]wiiudownloader.TitleEntry, 0, len(entries)),
		byID:    make(map[uint64]wiiudownloader.TitleEntry, len(entries)),
		source:  source,
	}
	for _, entry := range entries {
		if entry.Category == wiiudownloader.TITLE_CATEGORY_DISC {
			continue
		}
		db.entries = append(db.entries, entry)
		if _, exists := db.byID[entry.TitleID]; !exists {
			db.byID[entry.TitleID] = entry
		}
	}
	return db
}

func (db *titleDB) setGlobal() {
	wiiudownloader.SetTitleDatabase(db.entries)
}

func (db *titleDB) findByID(id uint64) (wiiudownloader.TitleEntry, bool) {
	entry, ok := db.byID[id]
	return entry, ok
}

type titleDBOptions struct {
	path    string
	noFetch bool
	refresh bool
	warn    func(format string, args ...any)
}

func loadTitleDatabase(client *http.Client, opts titleDBOptions) (*titleDB, error) {
	warn := opts.warn
	if warn == nil {
		warn = func(string, ...any) {}
	}

	if opts.path != "" {
		data, err := os.ReadFile(opts.path)
		if err != nil {
			return nil, fmt.Errorf("reading title database %s: %w", opts.path, err)
		}
		entries, err := parseTitleDatabase(data)
		if err != nil {
			return nil, fmt.Errorf("parsing title database %s: %w", opts.path, err)
		}
		db := newTitleDB(entries, opts.path)
		db.cachePath = opts.path
		db.setGlobal()
		return db, nil
	}

	cachePath, cacheErr := titleDBCachePath()
	if cacheErr != nil {
		warn("cannot determine cache directory: %v", cacheErr)
	}

	if !opts.refresh && cachePath != "" {
		cache, err := readTitleDBCache(cachePath)
		if err == nil && len(cache.Entries) > 0 {
			db := newTitleDB(cache.entries(), "cache: "+cachePath)
			db.cachePath = cachePath
			db.fetchedAt = cache.FetchedAt
			db.setGlobal()
			return db, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			warn("ignoring unreadable title database cache %s: %v", cachePath, err)
		}
	}

	embedded := newTitleDB(wiiudownloader.TitleDatabase, "built-in")
	if !opts.refresh && len(embedded.entries) > 0 {
		embedded.cachePath = cachePath
		embedded.setGlobal()
		return embedded, nil
	}

	if opts.noFetch {
		if len(embedded.entries) > 0 {
			embedded.cachePath = cachePath
			embedded.setGlobal()
			return embedded, nil
		}
		if cachePath != "" {
			if cache, err := readTitleDBCache(cachePath); err == nil && len(cache.Entries) > 0 {
				db := newTitleDB(cache.entries(), "cache: "+cachePath)
				db.cachePath = cachePath
				db.fetchedAt = cache.FetchedAt
				db.setGlobal()
				return db, nil
			}
		}
		return nil, errors.New("no title database available: run `" + cliName + " db update` once while online (or pass --db PATH)")
	}

	entries, source, err := fetchTitleDatabase(client, titleDBURL())
	if err != nil {
		if len(embedded.entries) > 0 {
			warn("could not download the title database (%v); using the built-in database", err)
			embedded.cachePath = cachePath
			embedded.setGlobal()
			return embedded, nil
		}
		if cachePath != "" {
			if cache, cacheReadErr := readTitleDBCache(cachePath); cacheReadErr == nil && len(cache.Entries) > 0 {
				warn("could not download the title database (%v); using the cached copy", err)
				db := newTitleDB(cache.entries(), "cache: "+cachePath)
				db.cachePath = cachePath
				db.fetchedAt = cache.FetchedAt
				db.setGlobal()
				return db, nil
			}
		}
		return nil, fmt.Errorf("could not download the title database: %w", err)
	}

	now := time.Now().UTC()
	db := newTitleDB(entries, source)
	db.cachePath = cachePath
	db.fetchedAt = now
	db.setGlobal()

	if cachePath != "" {
		if err := writeTitleDBCache(cachePath, titleDBCache{
			FetchedAt: now,
			Source:    source,
			Entries:   titleEntriesToJSON(entries),
		}); err != nil {
			warn("could not write the title database cache: %v", err)
		}
	}
	return db, nil
}

func titleDBURL() string {
	if url := strings.TrimSpace(os.Getenv("WIIUD_DB_URL")); url != "" {
		return url
	}
	return defaultTitleDBURL
}

func titleDBCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, titleDBCacheDir, titleDBCacheFile), nil
}

type titleDBCache struct {
	FetchedAt time.Time        `json:"fetchedAt"`
	Source    string           `json:"source"`
	Entries   []jsonTitleEntry `json:"entries"`
}

func (c *titleDBCache) entries() []wiiudownloader.TitleEntry {
	entries := make([]wiiudownloader.TitleEntry, 0, len(c.Entries))
	for _, entry := range c.Entries {
		entries = append(entries, entry.TitleEntry())
	}
	return entries
}

func readTitleDBCache(path string) (*titleDBCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache titleDBCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if len(cache.Entries) == 0 {
		return nil, errors.New("cache contains no titles")
	}
	return &cache, nil
}

func writeTitleDBCache(path string, cache titleDBCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), titleDBCacheFile+".*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func fetchTitleDatabase(client *http.Client, url string) ([]wiiudownloader.TitleEntry, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", titleDBUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTitleDBBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxTitleDBBytes {
		return nil, "", fmt.Errorf("title database exceeds %d bytes", maxTitleDBBytes)
	}
	entries, err := parseTitleDatabase(data)
	if err != nil {
		return nil, "", err
	}
	return entries, url, nil
}

// parseTitleDatabase accepts the project's generated Go database as well as
// the JSON cache (or a plain JSON array of titles).
func parseTitleDatabase(data []byte) ([]wiiudownloader.TitleEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty title database")
	}
	if trimmed[0] == '[' || trimmed[0] == '{' {
		entries, jsonErr := parseJSONTitleDatabase(trimmed)
		if jsonErr == nil {
			return entries, nil
		}
		// A generated Go file never starts with { or [, so a JSON failure is
		// final rather than a reason to try the AST parser.
		return nil, jsonErr
	}
	return parseGoTitleDatabase(trimmed)
}

func parseJSONTitleDatabase(data []byte) ([]wiiudownloader.TitleEntry, error) {
	var entries []jsonTitleEntry
	if len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, err
		}
	} else {
		var wrapper struct {
			Entries []jsonTitleEntry `json:"entries"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, err
		}
		entries = wrapper.Entries
	}
	if len(entries) == 0 {
		return nil, errors.New("title database contains no titles")
	}
	out := make([]wiiudownloader.TitleEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "" && entry.TitleID == 0 {
			continue
		}
		out = append(out, entry.TitleEntry())
	}
	if len(out) == 0 {
		return nil, errors.New("title database contains no valid titles")
	}
	return out, nil
}

type jsonTitleEntry struct {
	Name     string
	TitleID  uint64
	Region   uint8
	Key      uint8
	Category uint8
	Version  int
}

func (e jsonTitleEntry) TitleEntry() wiiudownloader.TitleEntry {
	return wiiudownloader.TitleEntry{
		Name:     e.Name,
		TitleID:  e.TitleID,
		Region:   e.Region,
		Key:      e.Key,
		Category: e.Category,
		Version:  e.Version,
	}
}

func titleEntriesToJSON(entries []wiiudownloader.TitleEntry) []jsonTitleEntry {
	out := make([]jsonTitleEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, jsonTitleEntry{
			Name:     entry.Name,
			TitleID:  entry.TitleID,
			Region:   entry.Region,
			Key:      entry.Key,
			Category: entry.Category,
			Version:  entry.Version,
		})
	}
	return out
}

func (e *jsonTitleEntry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name     string          `json:"name"`
		TitleID  json.RawMessage `json:"titleId"`
		Region   *uint8          `json:"region"`
		Key      *uint8          `json:"key"`
		Category *uint8          `json:"category"`
		Version  *int            `json:"version"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Name = raw.Name
	if len(raw.TitleID) > 0 && string(raw.TitleID) != "null" {
		id, err := parseJSONTitleID(raw.TitleID)
		if err != nil {
			return err
		}
		e.TitleID = id
	}
	if raw.Region != nil {
		e.Region = *raw.Region
	}
	if raw.Key != nil {
		e.Key = *raw.Key
	}
	if raw.Category != nil {
		e.Category = *raw.Category
	}
	if raw.Version != nil {
		e.Version = *raw.Version
	}
	return nil
}

func (e jsonTitleEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string `json:"name"`
		TitleID  string `json:"titleId"`
		Region   uint8  `json:"region"`
		Key      uint8  `json:"key"`
		Category uint8  `json:"category"`
		Version  int    `json:"version"`
	}{
		Name:     e.Name,
		TitleID:  fmt.Sprintf("%016x", e.TitleID),
		Region:   e.Region,
		Key:      e.Key,
		Category: e.Category,
		Version:  e.Version,
	})
}

func parseJSONTitleID(raw json.RawMessage) (uint64, error) {
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, err
		}
		s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "0x"), "0X")
		s = strings.ReplaceAll(s, "-", "")
		return strconv.ParseUint(s, 16, 64)
	}
	var number uint64
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, err
	}
	return number, nil
}

func parseGoTitleDatabase(data []byte) ([]wiiudownloader.TitleEntry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "titles.go", data, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("not a generated Go title database: %w", err)
	}

	var entries []wiiudownloader.TitleEntry
	var firstErr error
	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch typ := lit.Type.(type) {
		case *ast.Ident:
			if typ.Name != "TitleEntry" {
				return true
			}
		case *ast.ArrayType:
			elem, ok := typ.Elt.(*ast.Ident)
			if !ok || elem.Name != "TitleEntry" {
				return true
			}
		default:
			return true
		}
		for _, element := range lit.Elts {
			entry, err := decodeTitleEntryLiteral(element)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("at %s: %w", fset.Position(element.Pos()), err)
				}
				continue
			}
			entries = append(entries, entry)
		}
		return false
	})
	if len(entries) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, errors.New("no TitleEntry values found in the Go database")
	}
	return entries, nil
}

func decodeTitleEntryLiteral(expr ast.Expr) (wiiudownloader.TitleEntry, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return wiiudownloader.TitleEntry{}, errors.New("title entry is not a composite literal")
	}

	var entry wiiudownloader.TitleEntry
	if len(lit.Elts) == 0 {
		return entry, nil
	}

	if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); !keyed {
		fields := []any{&entry.Name, &entry.TitleID, &entry.Region, &entry.Key, &entry.Category, &entry.Version}
		if len(lit.Elts) > len(fields) {
			return entry, errors.New("title entry has too many fields")
		}
		for i, element := range lit.Elts {
			if err := assignTitleField(fields[i], element); err != nil {
				return entry, err
			}
		}
		return entry, nil
	}

	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return entry, errors.New("title entry mixes keyed and positional fields")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return entry, errors.New("title entry field is not an identifier")
		}
		var target any
		switch key.Name {
		case "Name":
			target = &entry.Name
		case "TitleID":
			target = &entry.TitleID
		case "Region":
			target = &entry.Region
		case "Key":
			target = &entry.Key
		case "Category":
			target = &entry.Category
		case "Version":
			target = &entry.Version
		default:
			continue
		}
		if err := assignTitleField(target, kv.Value); err != nil {
			return entry, fmt.Errorf("%s: %w", key.Name, err)
		}
	}
	return entry, nil
}

func assignTitleField(target any, expr ast.Expr) error {
	switch ptr := target.(type) {
	case *string:
		value, err := literalString(expr)
		if err != nil {
			return err
		}
		*ptr = value
	case *uint64:
		value, err := literalUint(expr)
		if err != nil {
			return err
		}
		*ptr = value
	case *uint8:
		value, err := literalUint(expr)
		if err != nil {
			return err
		}
		if value > 0xFF {
			return fmt.Errorf("value %d does not fit in uint8", value)
		}
		*ptr = uint8(value)
	case *int:
		value, err := literalInt(expr)
		if err != nil {
			return err
		}
		*ptr = value
	default:
		return fmt.Errorf("unsupported field type %T", target)
	}
	return nil
}

func literalString(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", errors.New("expected a string literal")
	}
	return strconv.Unquote(lit.Value)
}

func literalUint(expr ast.Expr) (uint64, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, errors.New("expected an integer literal")
	}
	value, err := strconv.ParseUint(strings.ReplaceAll(lit.Value, "_", ""), 0, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func literalInt(expr ast.Expr) (int, error) {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.SUB {
		value, err := literalUint(unary.X)
		if err != nil {
			return 0, err
		}
		return -int(value), nil
	}
	value, err := literalUint(expr)
	if err != nil {
		return 0, err
	}
	return int(value), nil
}
