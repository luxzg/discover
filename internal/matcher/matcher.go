package matcher

import "strings"

// MatchRule evaluates a rule pattern as a simple token matcher (no regex).
// Pattern separators: spaces and '+' are both treated as token separators.
func MatchRule(pattern, title, content, domain, articleURL string) bool {
	tokens := tokenizePattern(pattern)
	if len(tokens) == 0 {
		return false
	}
	haystack := strings.ToLower(strings.Join([]string{
		title,
		content,
		domain,
		articleURL,
	}, " "))
	for _, tok := range tokens {
		if !strings.Contains(haystack, tok) {
			return false
		}
	}
	return true
}

func tokenizePattern(pattern string) []string {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return nil
	}
	pattern = strings.ReplaceAll(pattern, "+", " ")
	return strings.Fields(pattern)
}
