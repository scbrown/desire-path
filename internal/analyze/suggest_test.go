package analyze

import (
	"math"
	"testing"
)

func TestSuggestExactMatch(t *testing.T) {
	known := []string{"Read", "Write", "Edit"}
	results := Suggest("Read", known)
	if len(results) == 0 {
		t.Fatal("expected at least one suggestion for exact match")
	}
	if results[0].Name != "Read" {
		t.Errorf("Name = %q, want %q", results[0].Name, "Read")
	}
	if results[0].Score != 1.0 {
		t.Errorf("Score = %f, want 1.0", results[0].Score)
	}
}

func TestSuggestCaseInsensitive(t *testing.T) {
	known := []string{"ReadFile", "WriteFile"}
	results := Suggest("readfile", known)
	if len(results) == 0 {
		t.Fatal("expected suggestions for case-insensitive match")
	}
	if results[0].Name != "ReadFile" {
		t.Errorf("Name = %q, want %q", results[0].Name, "ReadFile")
	}
	// "readfile" normalizes to "readfile", "ReadFile" to "read file"
	// so not a perfect match, but should score highly
	if results[0].Score < 0.9 {
		t.Errorf("Score = %f, want >= 0.9 for near-match", results[0].Score)
	}
}

func TestSuggestUnderscoreVsCamelCase(t *testing.T) {
	known := []string{"ReadFile"}
	results := Suggest("read_file", known)
	if len(results) == 0 {
		t.Fatal("expected suggestion for underscore vs camelCase")
	}
	if results[0].Name != "ReadFile" {
		t.Errorf("Name = %q, want %q", results[0].Name, "ReadFile")
	}
	if results[0].Score != 1.0 {
		t.Errorf("Score = %f, want 1.0 for normalized match", results[0].Score)
	}
}

func TestSuggestSimilarNames(t *testing.T) {
	known := []string{"Read", "Write", "Edit", "Glob", "Grep"}
	results := Suggest("red", known)
	// "Read" should be the top suggestion
	if len(results) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	if results[0].Name != "Read" {
		t.Errorf("top suggestion = %q, want %q", results[0].Name, "Read")
	}
}

func TestSuggestBelowThreshold(t *testing.T) {
	known := []string{"CompletelyDifferentTool"}
	results := Suggest("x", known)
	if len(results) != 0 {
		t.Errorf("expected no suggestions for very dissimilar names, got %d", len(results))
	}
}

func TestSuggestEmpty(t *testing.T) {
	if results := Suggest("", []string{"Read"}); results != nil {
		t.Errorf("expected nil for empty name, got %v", results)
	}
	if results := Suggest("Read", nil); results != nil {
		t.Errorf("expected nil for nil known, got %v", results)
	}
	if results := Suggest("Read", []string{}); results != nil {
		t.Errorf("expected nil for empty known, got %v", results)
	}
}

func TestSuggestTopN(t *testing.T) {
	known := []string{"aa", "ab", "ac", "ad", "ae", "af", "ag"}
	results := SuggestN("aa", known, 3, 0.0)
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

func TestSuggestNThreshold(t *testing.T) {
	known := []string{"Read", "CompletelyUnrelatedToolName"}
	results := SuggestN("Reed", known, 10, 0.5)
	for _, s := range results {
		if s.Score < 0.5 {
			t.Errorf("suggestion %q has score %f below threshold 0.5", s.Name, s.Score)
		}
	}
}

func TestSuggestSortedByScore(t *testing.T) {
	known := []string{"Read", "Reed", "Reddish", "Write"}
	results := Suggest("Read", known)
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: [%d].Score=%f > [%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestSuggestOriginalNamesPreserved(t *testing.T) {
	known := []string{"ReadFile", "read_file", "READ_FILE"}
	results := Suggest("readFile", known)
	// All should match (they normalize the same), and original names should be preserved
	for _, r := range results {
		found := false
		for _, k := range known {
			if r.Name == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("suggestion %q not in known list", r.Name)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ReadFile", "read file"},
		{"read_file", "read file"},
		{"read-file", "read file"},
		{"readFile", "read file"},
		{"READ_FILE", "read file"},
		{"read", "read"},
		{"", ""},
		{"XMLParser", "xml parser"},
		{"getHTTPResponse", "get http response"},
	}
	for _, tt := range tests {
		got := normalize(tt.input)
		if got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "b", 1},
		{"kitten", "sitting", 3},
		{"read", "read", 0},
		{"read", "reed", 1},
		{"abc", "xyz", 3},
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSimilarityBounds(t *testing.T) {
	// Score should always be in [0, 1].
	pairs := [][2]string{
		{"read", "read"},
		{"read", "write"},
		{"a", "z"},
		{"short", "muchlongerstring"},
		{"", "nonempty"},
	}
	for _, p := range pairs {
		score := similarity(p[0], p[1])
		if score < 0 || score > 1 {
			t.Errorf("similarity(%q, %q) = %f, out of [0,1]", p[0], p[1], score)
		}
	}
}

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"abc", "abd", 2},
		{"abc", "xyz", 0},
		{"abc", "abc", 3},
		{"ab", "abcdef", 2},
		{"", "abc", 0},
	}
	for _, tt := range tests {
		got := commonPrefixLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPrefixLen(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCommonSuffixLen(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"abc", "xbc", 2},
		{"abc", "xyz", 0},
		{"abc", "abc", 3},
		{"bc", "abcbc", 2},
		{"", "abc", 0},
	}
	for _, tt := range tests {
		got := commonSuffixLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonSuffixLen(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSuggestPrefixBonus(t *testing.T) {
	known := []string{"ReadFile", "FileRead"}
	results := SuggestN("ReadFil", known, 10, 0.0)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// ReadFile shares a longer prefix with "ReadFil", should score higher
	var readFileScore, fileReadScore float64
	for _, r := range results {
		if r.Name == "ReadFile" {
			readFileScore = r.Score
		}
		if r.Name == "FileRead" {
			fileReadScore = r.Score
		}
	}
	if readFileScore <= fileReadScore {
		t.Errorf("ReadFile (%f) should score higher than FileRead (%f) due to prefix bonus",
			readFileScore, fileReadScore)
	}
}

func TestSimilarityIdentical(t *testing.T) {
	score := similarity("read file", "read file")
	if score != 1.0 {
		t.Errorf("identical strings: score = %f, want 1.0", score)
	}
}

func TestSimilarityEmpty(t *testing.T) {
	if s := similarity("", "abc"); s != 0.0 {
		t.Errorf("similarity('', 'abc') = %f, want 0.0", s)
	}
	if s := similarity("abc", ""); s != 0.0 {
		t.Errorf("similarity('abc', '') = %f, want 0.0", s)
	}
}

func TestSuggestHyphenNormalization(t *testing.T) {
	known := []string{"read-file"}
	results := Suggest("ReadFile", known)
	if len(results) == 0 {
		t.Fatal("expected suggestion for hyphen vs camelCase")
	}
	if results[0].Score != 1.0 {
		t.Errorf("Score = %f, want 1.0 for normalized match", results[0].Score)
	}
}

func TestMin3(t *testing.T) {
	tests := []struct {
		a, b, c, want int
	}{
		{1, 2, 3, 1},
		{3, 1, 2, 1},
		{2, 3, 1, 1},
		{5, 5, 5, 5},
	}
	for _, tt := range tests {
		got := min3(tt.a, tt.b, tt.c)
		if got != tt.want {
			t.Errorf("min3(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, got, tt.want)
		}
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestSuggestScoreSymmetry(t *testing.T) {
	// Similarity should be symmetric (for normalized forms).
	a, b := "read", "reed"
	s1 := similarity(a, b)
	s2 := similarity(b, a)
	if !almostEqual(s1, s2) {
		t.Errorf("similarity not symmetric: (%q,%q)=%f vs (%q,%q)=%f", a, b, s1, b, a, s2)
	}
}
