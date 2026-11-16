package main

import (
	"fmt"
	"strconv"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

// resolveTitleArgument turns a command-line title argument into one or more
// database entries. A 16-digit hex ID is used verbatim (even if it is not in the
// database); anything else is searched by name.
func resolveTitleArgument(application *app, db *titleDB, argument string, regionMask, category uint8, allowMultiple, pickFirst bool) ([]wiiudownloader.TitleEntry, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return nil, usageError{fmt.Errorf("empty title")}
	}

	if id, ok := parseTitleID(argument); ok {
		entry, found := db.findByID(id)
		if !found {
			entry = wiiudownloader.TitleEntry{TitleID: id}
		}
		return []wiiudownloader.TitleEntry{entry}, nil
	}

	matches := filterTitles(db.entries, argument, regionMask, category)
	if len(matches) == 0 {
		fallback := filterTitles(db.entries, argument, allRegionMask, wiiudownloader.TITLE_CATEGORY_ALL)
		if len(fallback) > 0 {
			return nil, fmt.Errorf("no title matched %q with the current region/type filters (%d match without them)", argument, len(fallback))
		}
		return nil, fmt.Errorf("no title matched %q", argument)
	}

	exact := make([]wiiudownloader.TitleEntry, 0, len(matches))
	for _, entry := range matches {
		if strings.EqualFold(entry.Name, argument) {
			exact = append(exact, entry)
		}
	}
	if len(exact) > 0 {
		matches = exact
	}
	sortTitles(matches, sortByName, false)

	if len(matches) == 1 {
		return matches, nil
	}
	if pickFirst {
		application.warnf("%q matched %d titles; picking %s", argument, len(matches), formatCandidate(matches[0]))
		return matches[:1], nil
	}
	if !application.interactive {
		return nil, ambiguousTitleError(argument, matches, allowMultiple)
	}
	return promptTitleSelection(application, argument, matches, allowMultiple)
}

func formatCandidate(entry wiiudownloader.TitleEntry) string {
	return fmt.Sprintf("%s (%s, %s, %016x)", entry.Name, wiiudownloader.GetFormattedKind(entry.TitleID), formatRegion(entry.Region), entry.TitleID)
}

func ambiguousTitleError(argument string, matches []wiiudownloader.TitleEntry, allowMultiple bool) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q is ambiguous and stdin is not a terminal:\n", argument)
	for _, entry := range matches {
		fmt.Fprintf(&builder, "  %s\n", formatCandidate(entry))
	}
	if allowMultiple {
		builder.WriteString("Pass the title IDs explicitly, or use --region/--type to narrow the matches.")
	} else {
		builder.WriteString("Pass the title ID explicitly, or use --region/--type to narrow the matches.")
	}
	return usageError{fmt.Errorf("%s", builder.String())}
}

func promptTitleSelection(application *app, argument string, matches []wiiudownloader.TitleEntry, allowMultiple bool) ([]wiiudownloader.TitleEntry, error) {
	fmt.Fprintf(application.stderr, "%q matches %d titles:\n", argument, len(matches))
	for i, entry := range matches {
		fmt.Fprintf(application.stderr, "  %2d) %s\n", i+1, formatCandidate(entry))
	}

	for {
		if allowMultiple {
			fmt.Fprint(application.stderr, "Select one or more (comma-separated, 'a' for all, empty to cancel): ")
		} else {
			fmt.Fprint(application.stderr, "Select one (empty to cancel): ")
		}
		line, err := application.input.ReadString('\n')
		if err != nil && line == "" {
			return nil, fmt.Errorf("reading selection: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return nil, fmt.Errorf("selection cancelled")
		}
		if allowMultiple && (strings.EqualFold(line, "a") || strings.EqualFold(line, "all")) {
			return matches, nil
		}

		selected := make([]wiiudownloader.TitleEntry, 0, 1)
		seen := make(map[int]struct{})
		valid := true
		for _, token := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		}) {
			index, convErr := strconv.Atoi(token)
			if convErr != nil || index < 1 || index > len(matches) {
				fmt.Fprintf(application.stderr, "Invalid selection %q; enter a number from 1 to %d.\n", token, len(matches))
				valid = false
				break
			}
			if _, duplicate := seen[index]; duplicate {
				continue
			}
			seen[index] = struct{}{}
			selected = append(selected, matches[index-1])
			if !allowMultiple {
				break
			}
		}
		if !valid {
			continue
		}
		if len(selected) == 0 {
			fmt.Fprintln(application.stderr, "No selection made.")
			continue
		}
		return selected, nil
	}
}

// parseTitleVersion converts a --version value. "latest" uses the database's
// preferred version when it has one (0 means latest, as in the GUI).
func parseTitleVersion(value string, entry wiiudownloader.TitleEntry) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "latest") {
		if entry.Version > 0 {
			return entry.Version, nil
		}
		return wiiudownloader.VersionLatest, nil
	}
	version, err := strconv.Atoi(value)
	if err != nil || version < 0 {
		return 0, usageError{fmt.Errorf("invalid --version %q: want \"latest\" or a non-negative number", value)}
	}
	return version, nil
}

// resolveOneTitle is the single-title variant used by info/files/extract.
func resolveOneTitle(application *app, db *titleDB, argument string, regionMask, category uint8, pickFirst bool) (wiiudownloader.TitleEntry, error) {
	entries, err := resolveTitleArgument(application, db, argument, regionMask, category, false, pickFirst)
	if err != nil {
		return wiiudownloader.TitleEntry{}, err
	}
	if len(entries) != 1 {
		return wiiudownloader.TitleEntry{}, fmt.Errorf("expected one title, got %d", len(entries))
	}
	return entries[0], nil
}
