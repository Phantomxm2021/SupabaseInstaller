package project

import (
	"strings"
	"unicode"
)

func NormalizeSlug(name string) string {
	var slug strings.Builder
	separatorPending := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if separatorPending && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			separatorPending = false
			slug.WriteRune(r)
		case r == '-' || r == '_' || unicode.IsSpace(r):
			separatorPending = slug.Len() > 0
		default:
			separatorPending = slug.Len() > 0
		}
	}
	return strings.Trim(slug.String(), "-")
}
