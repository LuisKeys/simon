package router

import (
	"fmt"
	"strings"
)

// Weights used by scoreFields, per the Knowledge Router scoring spec.
const (
	weightExactPhrase    = 20.0
	weightTitleToken     = 10.0
	weightKeywordExact   = 8.0
	weightAliasExact     = 7.0
	weightDescToken      = 5.0
	weightSummaryToken   = 4.0
	weightCategoryPath   = 3.0
	weightGeneralToken   = 1.0
	weightMultiTokenSame = 3.0
	weightAllTokensBonus = 5.0
)

// Score is a scored candidate's raw value plus a human-readable, ordered
// explanation of how it was reached.
type Score struct {
	Value   float64  `json:"value"`
	Reasons []string `json:"reasons,omitempty"`
}

// Fields is the set of lexical fields a candidate (category, document, or
// section) exposes to the shared scorer.
type Fields struct {
	Title         string
	Description   string
	Summary       string
	Keywords      []string
	Aliases       []string
	CategoryPaths []string
	General       []string
}

// scoreFields deterministically scores a candidate against a query.
//
// Normalization: the raw weighted sum is divided by
// 10 * max(1, len(query.Tokens)) so that scores stay roughly comparable
// across short and long queries (a single strong field match on a
// one-token query and a proportionally weaker match spread across a
// five-token query land in a similar numeric range). This formula is an
// implementation choice, not a probability; only its ordering and
// threshold behavior are guaranteed stable.
func scoreFields(q Query, f Fields) Score {
	var value float64
	var reasons []string

	titleNorm := normalize(f.Title)
	descNorm := normalize(f.Description)
	summaryNorm := normalize(f.Summary)

	if q.Normalized != "" {
		paddedQuery := " " + q.Normalized + " "
		if titleNorm != "" && strings.Contains(" "+titleNorm+" ", paddedQuery) {
			value += weightExactPhrase
			reasons = append(reasons, "exact phrase matched title")
		}
		if descNorm != "" && strings.Contains(" "+descNorm+" ", paddedQuery) {
			value += weightExactPhrase
			reasons = append(reasons, "exact phrase matched description")
		}
		if summaryNorm != "" && strings.Contains(" "+summaryNorm+" ", paddedQuery) {
			value += weightExactPhrase
			reasons = append(reasons, "exact phrase matched summary")
		}
	}

	titleTokens := tokenSet(titleNorm)
	var titleMatches []string
	for _, t := range q.Tokens {
		if titleTokens[t] {
			value += weightTitleToken
			titleMatches = append(titleMatches, t)
		}
	}
	if len(titleMatches) > 0 {
		reasons = append(reasons, fmt.Sprintf("title matched %s", quoteJoin(titleMatches)))
	}
	if len(titleMatches) > 1 {
		value += weightMultiTokenSame
	}

	for _, kw := range f.Keywords {
		kwNorm := normalize(kw)
		if kwNorm == "" {
			continue
		}
		if matchesPhraseOrAllTokens(q, kwNorm) {
			value += weightKeywordExact
			reasons = append(reasons, fmt.Sprintf("keyword matched %q", kw))
		}
	}

	for _, al := range f.Aliases {
		alNorm := normalize(al)
		if alNorm == "" {
			continue
		}
		if matchesPhraseOrAllTokens(q, alNorm) {
			value += weightAliasExact
			reasons = append(reasons, fmt.Sprintf("alias matched %q", al))
		}
	}

	descTokens := tokenSet(descNorm)
	var descMatches []string
	for _, t := range q.Tokens {
		if descTokens[t] {
			value += weightDescToken
			descMatches = append(descMatches, t)
		}
	}
	if len(descMatches) > 0 {
		reasons = append(reasons, fmt.Sprintf("description matched %s", quoteJoin(descMatches)))
	}
	if len(descMatches) > 1 {
		value += weightMultiTokenSame
	}

	summaryTokens := tokenSet(summaryNorm)
	var summaryMatches []string
	for _, t := range q.Tokens {
		if summaryTokens[t] {
			value += weightSummaryToken
			summaryMatches = append(summaryMatches, t)
		}
	}
	if len(summaryMatches) > 0 {
		reasons = append(reasons, fmt.Sprintf("summary matched %s", quoteJoin(summaryMatches)))
	}
	if len(summaryMatches) > 1 {
		value += weightMultiTokenSame
	}

	allMatched := map[string]bool{}
	for _, t := range titleMatches {
		allMatched[t] = true
	}
	for _, t := range descMatches {
		allMatched[t] = true
	}
	for _, t := range summaryMatches {
		allMatched[t] = true
	}

	for _, catPath := range f.CategoryPaths {
		pathTokens := tokenSet(normalize(strings.ReplaceAll(catPath, "/", " ")))
		var pathMatches []string
		for _, t := range q.Tokens {
			if pathTokens[t] {
				value += weightCategoryPath
				pathMatches = append(pathMatches, t)
				allMatched[t] = true
			}
		}
		if len(pathMatches) > 0 {
			reasons = append(reasons, fmt.Sprintf("category path matched %s", quoteJoin(pathMatches)))
		}
	}

	generalTokens := tokenSet(normalize(strings.Join(f.General, " ")))
	var generalMatches []string
	for _, t := range q.Tokens {
		if generalTokens[t] {
			value += weightGeneralToken
			generalMatches = append(generalMatches, t)
			allMatched[t] = true
		}
	}
	if len(generalMatches) > 0 {
		reasons = append(reasons, fmt.Sprintf("matched %s", quoteJoin(generalMatches)))
	}

	if len(q.Tokens) > 0 && len(allMatched) == len(q.Tokens) {
		value += weightAllTokensBonus
		reasons = append(reasons, "all query tokens matched")
	}

	divisor := 10.0
	if n := len(q.Tokens); n > 1 {
		divisor *= float64(n)
	}
	if divisor <= 0 {
		divisor = 10.0
	}

	return Score{Value: value / divisor, Reasons: reasons}
}

// matchesPhraseOrAllTokens reports whether norm (an already-normalized
// keyword/alias) matches the query as a substring in either direction, or
// consists entirely of query tokens.
func matchesPhraseOrAllTokens(q Query, norm string) bool {
	if q.Normalized == "" {
		return false
	}
	// Pad with spaces so containment only matches on whole-token
	// boundaries (e.g. keyword "SQL" must not match inside "postgresql").
	paddedQuery := " " + q.Normalized + " "
	paddedNorm := " " + norm + " "
	if strings.Contains(paddedQuery, paddedNorm) || strings.Contains(paddedNorm, paddedQuery) {
		return true
	}
	kwTokens := strings.Fields(norm)
	if len(kwTokens) == 0 {
		return false
	}
	qset := tokenSet(q.Normalized)
	for _, t := range kwTokens {
		if !qset[t] {
			return false
		}
	}
	return true
}

func tokenSet(normalized string) map[string]bool {
	set := map[string]bool{}
	for _, t := range strings.Fields(normalized) {
		set[t] = true
	}
	return set
}

func quoteJoin(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = fmt.Sprintf("%q", t)
	}
	return strings.Join(parts, " and ")
}
