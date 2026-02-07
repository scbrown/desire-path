// Package analyze provides similarity analysis for desire-path tool names.
package analyze

import (
	"strings"
	"unicode"
)

// Suggestion pairs a known tool name with its similarity score (0-1, higher is better).
type Suggestion struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// DefaultThreshold is the minimum similarity score for a suggestion to be returned.
const DefaultThreshold = 0.5

// DefaultTopN is the maximum number of suggestions returned.
const DefaultTopN = 5

// Suggest returns known tool names similar to name, ranked by similarity score.
// Only suggestions scoring above DefaultThreshold are returned, up to DefaultTopN results.
func Suggest(name string, known []string) []Suggestion {
	return SuggestN(name, known, DefaultTopN, DefaultThreshold)
}

// SuggestN returns up to topN known tool names similar to name, with score >= threshold.
func SuggestN(name string, known []string, topN int, threshold float64) []Suggestion {
	if name == "" || len(known) == 0 {
		return nil
	}

	normName := normalize(name)
	var results []Suggestion

	for _, k := range known {
		normK := normalize(k)
		score := similarity(normName, normK)
		if score >= threshold {
			results = append(results, Suggestion{Name: k, Score: score})
		}
	}

	sortByScore(results)

	if topN > 0 && len(results) > topN {
		results = results[:topN]
	}
	return results
}

// similarity computes the overall similarity between two normalized strings.
// It combines Levenshtein distance with prefix/suffix bonuses.
func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// Normalized Levenshtein: 1 - (distance / max_length).
	dist := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	lev := 1.0 - float64(dist)/float64(maxLen)

	// Prefix bonus: proportion of shared prefix, weighted at 0.1.
	prefixLen := commonPrefixLen(a, b)
	prefixBonus := 0.1 * float64(prefixLen) / float64(maxLen)

	// Suffix bonus: proportion of shared suffix, weighted at 0.05.
	suffixLen := commonSuffixLen(a, b)
	suffixBonus := 0.05 * float64(suffixLen) / float64(maxLen)

	score := lev + prefixBonus + suffixBonus
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// normalize converts a tool name to a canonical lowercase form,
// splitting camelCase and replacing underscores/hyphens with spaces,
// then joining with single spaces.
func normalize(s string) string {
	runes := []rune(s)
	var parts []string
	var current []rune

	for i, r := range runes {
		switch {
		case r == '_' || r == '-':
			if len(current) > 0 {
				parts = append(parts, string(current))
				current = current[:0]
			}
		case unicode.IsUpper(r):
			if len(current) > 0 {
				prevLower := i > 0 && unicode.IsLower(runes[i-1])
				// Split when transitioning from lowercase to uppercase (camelCase).
				// Also split before the last char of an uppercase run when followed
				// by lowercase (e.g., "XMLParser" → "XML" | "Parser").
				nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				prevUpper := i > 0 && unicode.IsUpper(runes[i-1])
				if prevLower || (prevUpper && nextLower) {
					parts = append(parts, string(current))
					current = current[:0]
				}
			}
			current = append(current, unicode.ToLower(r))
		default:
			current = append(current, unicode.ToLower(r))
		}
	}
	if len(current) > 0 {
		parts = append(parts, string(current))
	}
	return strings.Join(parts, " ")
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use single-row optimization.
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(ins, del, sub)
		}
		prev = curr
	}
	return prev[len(b)]
}

// commonPrefixLen returns the length of the common prefix of a and b.
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// commonSuffixLen returns the length of the common suffix of a and b.
func commonSuffixLen(a, b string) int {
	la, lb := len(a), len(b)
	n := la
	if lb < n {
		n = lb
	}
	for i := 0; i < n; i++ {
		if a[la-1-i] != b[lb-1-i] {
			return i
		}
	}
	return n
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// sortByScore sorts suggestions by score descending using insertion sort
// (sufficient for small result sets).
func sortByScore(s []Suggestion) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j].Score < key.Score {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
