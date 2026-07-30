package router

import (
	"strings"
	"unicode"
)

// stopWords is a small built-in stop-word set. Deliberately conservative:
// technical identifiers like "for" in "FOR UPDATE" or single-letter tokens
// are not stripped beyond this list, so queries like "SKIP LOCKED" or
// "FOR UPDATE" survive normalization intact.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true,
	"to": true, "in": true, "on": true, "is": true, "are": true, "how": true,
	"do": true, "does": true, "with": true, "at": true, "by": true, "it": true,
	"this": true, "that": true, "be": true, "can": true, "i": true,
	"what": true, "if": true, "happens": true,
}

// Query is a normalized, tokenized search query.
type Query struct {
	Raw        string
	Normalized string
	Tokens     []string
}

// keepRune reports whether r should be preserved verbatim during
// normalization (rather than treated as a separator), so technical
// identifiers such as "simon-go", "internal/knowledge", "pgvector",
// and "SKIP_LOCKED" survive tokenization.
func keepRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) ||
		r == '_' || r == '-' || r == '.' || r == '/' || r == '+'
}

// normalize lowercases text (Unicode-aware) and replaces any rune that is
// not a letter/number/kept-punctuation with a space.
func normalize(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		lr := unicode.ToLower(r)
		if keepRune(lr) {
			b.WriteRune(lr)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Tokenize normalizes raw and splits it into a deduplicated (order
// preserved), stop-word-filtered token list.
func Tokenize(raw string) Query {
	normalized := normalize(raw)
	if normalized == "" {
		return Query{Raw: raw, Normalized: "", Tokens: nil}
	}

	fields := strings.Split(normalized, " ")
	seen := make(map[string]bool, len(fields))
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "-./+_")
		if f == "" || stopWords[f] || seen[f] {
			continue
		}
		// Single-letter fragments are near-always apostrophe leftovers
		// ("job's" -> "job", "s"), not meaningful single-character
		// identifiers, so they are dropped rather than diluting scoring.
		if r := []rune(f); len(r) == 1 && unicode.IsLetter(r[0]) {
			continue
		}
		seen[f] = true
		tokens = append(tokens, f)
	}
	return Query{Raw: raw, Normalized: normalized, Tokens: tokens}
}
