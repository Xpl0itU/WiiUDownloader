package main

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type titleJSON struct {
	Name     string `json:"name"`
	TitleID  string `json:"titleId"`
	Kind     string `json:"kind"`
	Region   string `json:"region"`
	Category string `json:"category"`
	Version  int    `json:"version,omitempty"`
}

func titleToJSON(entry wiiudownloader.TitleEntry) titleJSON {
	return titleJSON{
		Name:     entry.Name,
		TitleID:  fmt.Sprintf("%016x", entry.TitleID),
		Kind:     wiiudownloader.GetFormattedKind(entry.TitleID),
		Region:   formatRegion(entry.Region),
		Category: categoryName(categoryOf(entry)),
		Version:  entry.Version,
	}
}

func normalizeSearchText(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))

	lastWasSpace := true
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			lastWasSpace = false
			continue
		}
		if !lastWasSpace {
			builder.WriteByte(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(builder.String())
}

// fuzzyMatch reports whether every rune of pattern appears in target in order,
// which is the same rule the GUI's search uses.
func fuzzyMatch(pattern, target string) bool {
	patternRunes := []rune(pattern)
	if len(patternRunes) == 0 {
		return true
	}
	index := 0
	for _, r := range target {
		if r == patternRunes[index] {
			index++
			if index == len(patternRunes) {
				return true
			}
		}
	}
	return false
}

func titleMatchesQuery(entry wiiudownloader.TitleEntry, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}

	lowerQuery := strings.ToLower(query)
	idQuery := strings.TrimPrefix(strings.TrimPrefix(lowerQuery, "0x"), "0X")
	idQuery = strings.ReplaceAll(idQuery, "-", "")
	idHex := fmt.Sprintf("%016x", entry.TitleID)
	if idQuery != "" && strings.Contains(idHex, idQuery) {
		return true
	}

	normalizedQuery := normalizeSearchText(query)
	if normalizedQuery == "" {
		return false
	}
	normalizedTitle := normalizeSearchText(entry.Name)
	if normalizedTitle == "" {
		return false
	}

	searchTokens := strings.Fields(normalizedQuery)
	titleTokens := strings.Fields(normalizedTitle)
	if len(searchTokens) == 0 || len(titleTokens) == 0 {
		return false
	}
	for _, searchToken := range searchTokens {
		matched := false
		for _, titleToken := range titleTokens {
			if fuzzyMatch(searchToken, titleToken) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func filterTitles(entries []wiiudownloader.TitleEntry, query string, regionMask, category uint8) []wiiudownloader.TitleEntry {
	out := make([]wiiudownloader.TitleEntry, 0, len(entries))
	for _, entry := range entries {
		if regionMask != 0 && entry.Region&regionMask == 0 {
			continue
		}
		if category != wiiudownloader.TITLE_CATEGORY_ALL && categoryOf(entry) != category {
			continue
		}
		if !titleMatchesQuery(entry, query) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

type titleSortField int

const (
	sortByName titleSortField = iota
	sortByID
	sortByKind
	sortByRegion
	sortByCategory
)

func parseSortField(value string) (titleSortField, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "name", "title":
		return sortByName, nil
	case "id", "titleid", "title-id":
		return sortByID, nil
	case "kind", "type":
		return sortByKind, nil
	case "region":
		return sortByRegion, nil
	case "category":
		return sortByCategory, nil
	default:
		return 0, fmt.Errorf("unknown sort field %q (want name, id, kind, region or category)", value)
	}
}

func sortTitles(entries []wiiudownloader.TitleEntry, field titleSortField, reverse bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		cmp := compareTitles(entries[i], entries[j], field)
		if reverse {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareTitles(a, b wiiudownloader.TitleEntry, field titleSortField) int {
	switch field {
	case sortByID:
		switch {
		case a.TitleID < b.TitleID:
			return -1
		case a.TitleID > b.TitleID:
			return 1
		default:
			return 0
		}
	case sortByKind:
		if cmp := strings.Compare(wiiudownloader.GetFormattedKind(a.TitleID), wiiudownloader.GetFormattedKind(b.TitleID)); cmp != 0 {
			return cmp
		}
	case sortByRegion:
		if cmp := strings.Compare(formatRegion(a.Region), formatRegion(b.Region)); cmp != 0 {
			return cmp
		}
	case sortByCategory:
		if cmp := strings.Compare(categoryName(categoryOf(a)), categoryName(categoryOf(b))); cmp != 0 {
			return cmp
		}
	}
	return compareTitleNames(a, b)
}

func compareTitleNames(a, b wiiudownloader.TitleEntry) int {
	if cmp := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); cmp != 0 {
		return cmp
	}
	switch {
	case a.TitleID < b.TitleID:
		return -1
	case a.TitleID > b.TitleID:
		return 1
	default:
		return 0
	}
}

func printTitleTable(app *app, entries []wiiudownloader.TitleEntry, idsOnly bool) error {
	if idsOnly {
		for _, entry := range entries {
			if _, err := fmt.Fprintln(app.stdout, fmt.Sprintf("%016x", entry.TitleID)); err != nil {
				return err
			}
		}
		return nil
	}

	writer := tabwriter.NewWriter(app.stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "TITLE ID\tKIND\tREGION\tNAME"); err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := fmt.Fprintf(writer, "%016x\t%s\t%s\t%s\n",
			entry.TitleID,
			wiiudownloader.GetFormattedKind(entry.TitleID),
			formatRegion(entry.Region),
			entry.Name,
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func cmdSearch(app *app, args []string) error {
	fs := newFlagSet(app, "search", "Search the Wii U title database.", `Usage:
  wiiudl search [flags] [query...]

The query is matched against names (fuzzy, token by token) and title IDs.
With no query, every title matching the region/type filters is listed.

Flags:`)
	region := fs.String("region", "all", "Regions to include: usa,eur,jpn,chn,kor,twn,all or a comma list")
	category := fs.String("type", "all", "Content type: game, update, dlc, demo or all")
	limit := fs.Int("limit", 50, "Maximum results (0 = unlimited)")
	sortField := fs.String("sort", "name", "Sort by: name, id, kind, region or category")
	reverse := fs.Bool("reverse", false, "Reverse the sort order")
	idsOnly := fs.Bool("ids", false, "Print only title IDs")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}

	regionMask, err := parseRegionMask(*region)
	if err != nil {
		return usageError{err: err}
	}
	categoryValue, err := parseCategory(*category)
	if err != nil {
		return usageError{err: err}
	}
	field, err := parseSortField(*sortField)
	if err != nil {
		return usageError{err: err}
	}
	if *limit < 0 {
		return usageError{fmt.Errorf("--limit cannot be negative")}
	}

	db, err := app.loadDB()
	if err != nil {
		return err
	}

	query := strings.Join(positional, " ")
	matches := filterTitles(db.entries, query, regionMask, categoryValue)
	sortTitles(matches, field, *reverse)

	total := len(matches)
	if *limit > 0 && len(matches) > *limit {
		matches = matches[:*limit]
	}

	if app.json {
		results := make([]titleJSON, 0, len(matches))
		for _, entry := range matches {
			results = append(results, titleToJSON(entry))
		}
		return writeJSON(app.stdout, results)
	}

	if err := printTitleTable(app, matches, *idsOnly); err != nil {
		return err
	}
	if len(matches) == 0 {
		fmt.Fprintln(app.stderr, "No titles matched.")
	} else if total > len(matches) && !app.quiet {
		fmt.Fprintf(app.stderr, "Showing %d of %d matches; use --limit 0 to list all.\n", len(matches), total)
	}
	return nil
}
