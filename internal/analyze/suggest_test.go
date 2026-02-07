package analyze

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase", "read", "read"},
		{"camelCase", "readFile", "read file"},
		{"PascalCase", "ReadFile", "read file"},
		{"snake_case", "read_file", "read file"},
		{"kebab-case", "read-file", "read file"},
		{"single upper", "Read", "read"},
		{"all caps short", "DB", "d b"},
		{"empty", "", ""},
		{"mixed", "myToolName_v2", "my tool name v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.input)
			if got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSuggestExactMatch(t *testing.T) {
	known := []string{"Read", "Write", "Edit", "Bash"}
	results := Suggest("Read", known, 0, 0)
	if len(results) == 0 {
		t.Fatal("expected at least one result for exact match")
	}
	if results[0].Name != "Read" || results[0].Score != 1.0 {
		t.Errorf("expected exact match Read with score 1.0, got %s %.2f", results[0].Name, results[0].Score)
	}
}

func TestSuggestCaseInsensitive(t *testing.T) {
	known := []string{"Read", "Write"}
	results := Suggest("read", known, 0, 0)
	if len(results) == 0 {
		t.Fatal("expected result for case-insensitive match")
	}
	if results[0].Name != "Read" || results[0].Score != 1.0 {
		t.Errorf("expected Read with score 1.0, got %s %.2f", results[0].Name, results[0].Score)
	}
}

func TestSuggestSimilarNames(t *testing.T) {
	known := []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"}

	tests := []struct {
		name    string
		input   string
		wantTop string
	}{
		{"read_file matches Read", "read_file", "Read"},
		{"write_file matches Write", "write_file", "Write"},
		{"edit_file matches Edit", "edit_file", "Edit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := Suggest(tt.input, known, 0.3, 0)
			if len(results) == 0 {
				t.Fatalf("no suggestions for %q", tt.input)
			}
			if results[0].Name != tt.wantTop {
				t.Errorf("top suggestion for %q = %q, want %q", tt.input, results[0].Name, tt.wantTop)
			}
		})
	}
}

func TestSuggestThreshold(t *testing.T) {
	known := []string{"Read", "Write", "Edit"}
	// "zzzzz" should not match anything at default threshold
	results := Suggest("zzzzz", known, 0, 0)
	if len(results) != 0 {
		t.Errorf("expected no results for dissimilar name, got %d", len(results))
	}
}

func TestSuggestTopN(t *testing.T) {
	known := []string{"Read", "ReadFile", "ReadDir", "ReadAll", "Write"}
	results := Suggest("read", known, 0.3, 2)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestSuggestSortedByScore(t *testing.T) {
	known := []string{"Write", "Read", "ReadFile"}
	results := Suggest("read", known, 0.3, 0)
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: [%d].Score=%.2f > [%d].Score=%.2f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestSuggestEmptyKnown(t *testing.T) {
	results := Suggest("read", nil, 0, 0)
	if len(results) != 0 {
		t.Errorf("expected no results for empty known list, got %d", len(results))
	}
}

func TestSuggestEmptyName(t *testing.T) {
	known := []string{"Read", "Write"}
	results := Suggest("", known, 0, 0)
	// Empty name should not crash; results depend on scoring
	_ = results
}

func TestPrefixSuffixBonus(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"shared prefix", "read file", "read dir", 0.1},
		{"shared suffix", "my file", "read file", 0.1},
		{"both shared", "read file", "read file", 0.2},
		{"no shared", "foo bar", "baz qux", 0.0},
		{"empty a", "", "read", 0.0},
		{"empty b", "read", "", 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prefixSuffixBonus(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("prefixSuffixBonus(%q, %q) = %.2f, want %.2f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
