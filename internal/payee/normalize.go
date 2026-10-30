package payee

import (
	"strings"
	"unicode"
)

// Normalize applies the compare form: trim, upper, collapse space.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			b.WriteByte(' ')
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(unicode.ToUpper(r))
	}
	return strings.TrimSpace(b.String())
}
