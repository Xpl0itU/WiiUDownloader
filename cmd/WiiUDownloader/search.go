package main

import (
	"strings"
	"unicode"

	"github.com/lithammer/fuzzysearch/fuzzy"
)

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

func titleMatchesSearch(search, titleName, titleID string) bool {
	if search == "" {
		return true
	}

	searchLower := strings.ToLower(search)
	if strings.Contains(strings.ToLower(titleID), searchLower) {
		return true
	}

	normalizedSearch := normalizeSearchText(search)
	if normalizedSearch == "" {
		return false
	}

	return titleNameMatchesSearch(normalizedSearch, normalizeSearchText(titleName))
}

func titleNameMatchesSearch(normalizedSearch, normalizedTitleName string) bool {
	searchTokens := strings.Fields(normalizedSearch)
	titleTokens := strings.Fields(normalizedTitleName)

	if len(searchTokens) == 0 {
		return false
	}
	if len(titleTokens) == 0 {
		return false
	}

	for _, searchToken := range searchTokens {
		if !anyTitleTokenMatches(searchToken, titleTokens) {
			return false
		}
	}

	return true
}

func anyTitleTokenMatches(searchToken string, titleTokens []string) bool {
	for _, titleToken := range titleTokens {
		if fuzzy.Match(searchToken, titleToken) {
			return true
		}
	}
	return false
}
