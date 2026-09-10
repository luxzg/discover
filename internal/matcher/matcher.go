package matcher

import "strings"

// NormalizeQuery treats '+' and whitespace as separators for both topic queries
// and rule tokens. Preserve case and query operators for the search provider.
func NormalizeQuery(query string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(query, "+", " ")), " ")
}

// MatchRule evaluates a rule pattern as a simple token matcher (no regex).
func MatchRule(pattern, title, content, domain, articleURL string) bool {
	tokens := tokenizePattern(pattern)
	if len(tokens) == 0 {
		return false
	}
	haystack := strings.ToLower(strings.Join([]string{title, content, domain, articleURL}, " "))
	for _, tok := range tokens {
		if !strings.Contains(haystack, tok) {
			return false
		}
	}
	return true
}
func tokenizePattern(pattern string) []string {
	return strings.Fields(strings.ToLower(NormalizeQuery(pattern)))
}
