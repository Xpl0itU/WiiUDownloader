package main

import "strings"

func normalizeFilename(filename string) string {
	var out strings.Builder
	shouldAppend := true

	for _, c := range filename {
		if c == '_' {
			if shouldAppend {
				out.WriteRune('_')
				shouldAppend = false
			}
		} else if c == ' ' {
			if shouldAppend {
				out.WriteRune(' ')
				shouldAppend = false
			}
		} else if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out.WriteRune(c)
			shouldAppend = true
		}
	}

	result := out.String()
	if len(result) > 0 && result[len(result)-1] == '_' {
		result = result[:len(result)-1]
	}

	return result
}
