// Package analyze provides similarity analysis for tool name suggestions.
package analyze

import (
	"strings"
	"unicode"

	"github.com/agnivade/levenshtein"
)

// Suggestion pairs a known tool name with its similarity score.
type Suggestion struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// normalize converts a tool name to a canonical lowercase form by splitting
// camelCase and underscore/hyphen boundaries into space-separated tokens.
func normalize(s string) string {
	var tokens []string
	var current []rune
	for i, r := range s {
		switch {
		case r == '_' || r == '-':
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = current[:0]
			}
		case unicode.IsUpper(r) && i > 0:
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = current[:0]
			}
			current = append(current, unicode.ToLower(r))
		default:
			current = append(current, unicode.ToLower(r))
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}
	return strings.Join(tokens, " ")
}

// prefixSuffixBonus returns a bonus score (0.0-0.2) for shared prefix or
// suffix tokens between two normalized names.
func prefixSuffixBonus(a, b string) float64 {
	aToks := strings.Fields(a)
	bToks := strings.Fields(b)
	if len(aToks) == 0 || len(bToks) == 0 {
		return 0
	}

	bonus := 0.0

	// Shared prefix tokens.
	minLen := len(aToks)
	if len(bToks) < minLen {
		minLen = len(bToks)
	}
	shared := 0
	for i := 0; i < minLen; i++ {
		if aToks[i] == bToks[i] {
			shared++
		} else {
			break
		}
	}
	if shared > 0 {
		bonus += 0.1
	}

	// Shared suffix tokens.
	shared = 0
	for i := 0; i < minLen; i++ {
		if aToks[len(aToks)-1-i] == bToks[len(bToks)-1-i] {
			shared++
		} else {
			break
		}
	}
	if shared > 0 {
		bonus += 0.1
	}

	return bonus
}

// Suggest returns known tool names similar to name, ranked by descending score.
// Only suggestions with a score >= threshold are returned. If threshold is 0,
// a default of 0.5 is used. At most topN results are returned; 0 means no limit.
func Suggest(name string, known []string, threshold float64, topN int) []Suggestion {
	if threshold == 0 {
		threshold = 0.5
	}

	normName := normalize(name)
	lowerName := strings.ToLower(name)

	var results []Suggestion
	for _, k := range known {
		lowerK := strings.ToLower(k)

		// Exact match (case-insensitive) gets perfect score.
		if lowerName == lowerK {
			results = append(results, Suggestion{Name: k, Score: 1.0})
			continue
		}

		normK := normalize(k)

		// Levenshtein distance on normalized forms.
		dist := levenshtein.ComputeDistance(normName, normK)
		maxLen := len(normName)
		if len(normK) > maxLen {
			maxLen = len(normK)
		}
		if maxLen == 0 {
			continue
		}

		// Normalized similarity: 1.0 means identical, 0.0 means completely different.
		sim := 1.0 - float64(dist)/float64(maxLen)

		// Add bonus for shared prefix/suffix tokens.
		sim += prefixSuffixBonus(normName, normK)

		// Clamp to [0, 1].
		if sim > 1.0 {
			sim = 1.0
		}
		if sim < 0 {
			sim = 0
		}

		if sim >= threshold {
			results = append(results, Suggestion{Name: k, Score: sim})
		}
	}

	// Sort by score descending (insertion sort is fine for small N).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	if topN > 0 && len(results) > topN {
		results = results[:topN]
	}

	return results
}
