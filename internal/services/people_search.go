package services

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Keep the tiers in sync with web/src/utils/peopleSearch.ts. Restrict typo
// matching to whole words and queries of at least three characters.
func normalizePeopleSearch(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(strings.TrimSpace(value)))
}

const noPeopleMatch = 4

func peopleMatch(query string, fields ...string) int {
	if query == "" {
		return 0
	}
	rank := noPeopleMatch
	for _, field := range fields {
		value := normalizePeopleSearch(field)
		if value == query {
			return 0
		}
		if strings.Contains(value, query) {
			rank = min(rank, 2)
		}
		for _, word := range strings.FieldsFunc(value, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}) {
			if strings.HasPrefix(word, query) {
				rank = min(rank, 1)
			} else if rank > 3 && onePeopleEdit([]rune(query), []rune(word)) {
				rank = 3
			}
		}
		if strings.HasPrefix(value, query) {
			rank = min(rank, 1)
		}
	}
	return rank
}

func onePeopleEdit(a, b []rune) bool {
	if len(a) < 3 || len(a)-len(b) > 1 || len(b)-len(a) > 1 {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	i := 0
	for i < len(a) && a[i] == b[i] {
		i++
	}
	if len(a) < len(b) {
		return string(a[i:]) == string(b[i+1:])
	}
	return i < len(a) && (string(a[i+1:]) == string(b[i+1:]) ||
		(i+1 < len(a) && a[i] == b[i+1] && a[i+1] == b[i] && string(a[i+2:]) == string(b[i+2:])))
}
