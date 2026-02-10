package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func TestWriteSuggestTable(t *testing.T) {
	suggestions := []analyze.Suggestion{
		{Name: "Read", Score: 0.82},
		{Name: "ReadDir", Score: 0.65},
	}

	var buf bytes.Buffer
	writeSuggestTable(&buf, "read_file", suggestions)
	out := buf.String()

	if !strings.Contains(out, "RANK") {
		t.Error("missing RANK header")
	}
	if !strings.Contains(out, "TOOL") {
		t.Error("missing TOOL header")
	}
	if !strings.Contains(out, "SCORE") {
		t.Error("missing SCORE header")
	}
	if !strings.Contains(out, "Read") {
		t.Error("missing Read suggestion")
	}
	if !strings.Contains(out, "0.82") {
		t.Error("missing score 0.82")
	}
}

func TestWriteSuggestTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	writeSuggestTable(&buf, "zzzzz", nil)
	out := buf.String()

	if !strings.Contains(out, "No suggestions found") {
		t.Error("expected 'No suggestions found' message")
	}
	if !strings.Contains(out, "zzzzz") {
		t.Error("expected query name in empty message")
	}
}

func TestWriteSuggestJSON(t *testing.T) {
	suggestions := []analyze.Suggestion{
		{Name: "Read", Score: 0.82},
	}

	var buf bytes.Buffer
	if err := writeSuggestJSON(&buf, "read_file", suggestions); err != nil {
		t.Fatalf("writeSuggestJSON: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"query": "read_file"`) {
		t.Error("missing query in JSON")
	}
	if !strings.Contains(out, `"name": "Read"`) {
		t.Error("missing suggestion name in JSON")
	}
	if !strings.Contains(out, `"score": 0.82`) {
		t.Error("missing score in JSON")
	}
}

func TestWriteSuggestAliasJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSuggestAliasJSON(&buf, "read_file", "Read"); err != nil {
		t.Fatalf("writeSuggestAliasJSON: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"query": "read_file"`) {
		t.Error("missing query in JSON")
	}
	if !strings.Contains(out, `"alias": "Read"`) {
		t.Error("missing alias in JSON")
	}
}

func TestSuggestCmdWithAlias(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	s.Close()

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"suggest", "read_file", "--db", db})
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "Alias") {
		t.Error("expected alias output")
	}
	if !strings.Contains(out, "Read") {
		t.Error("expected Read in alias output")
	}
}

func TestSuggestCmdNoMatch(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"suggest", "zzzzzzzzz", "--db", db})
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "No suggestions found") {
		t.Errorf("expected no suggestions message, got: %s", out)
	}
}

func TestSuggestCmdJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	defer func() { jsonOutput = false }()

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"suggest", "read_file", "--db", db, "--json"})
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"query"`) {
		t.Error("expected JSON output with query field")
	}
}

func TestSuggestCmdCustomKnown(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	defer func() { jsonOutput = false }()

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"suggest", "rread", "--db", db, "--known", "Read,Write", "--json"})
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"query": "rread"`) {
		t.Error("expected query in JSON output")
	}
}
