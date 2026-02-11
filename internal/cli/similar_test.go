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

func TestWriteSimilarTable(t *testing.T) {
	suggestions := []analyze.Suggestion{
		{Name: "Read", Score: 0.82},
		{Name: "ReadDir", Score: 0.65},
	}

	var buf bytes.Buffer
	writeSimilarTable(&buf, "read_file", suggestions)
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

func TestWriteSimilarTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	writeSimilarTable(&buf, "zzzzz", nil)
	out := buf.String()

	if !strings.Contains(out, "No suggestions found") {
		t.Error("expected 'No suggestions found' message")
	}
	if !strings.Contains(out, "zzzzz") {
		t.Error("expected query name in empty message")
	}
}

func TestWriteSimilarJSON(t *testing.T) {
	suggestions := []analyze.Suggestion{
		{Name: "Read", Score: 0.82},
	}

	var buf bytes.Buffer
	if err := writeSimilarJSON(&buf, "read_file", suggestions); err != nil {
		t.Fatalf("writeSimilarJSON: %v", err)
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

func TestWriteSimilarAliasJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSimilarAliasJSON(&buf, "read_file", "Read"); err != nil {
		t.Fatalf("writeSimilarAliasJSON: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"query": "read_file"`) {
		t.Error("missing query in JSON")
	}
	if !strings.Contains(out, `"alias": "Read"`) {
		t.Error("missing alias in JSON")
	}
}

func TestSimilarCmdWithAlias(t *testing.T) {
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
	rootCmd.SetArgs([]string{"similar", "read_file", "--db", db})
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

func TestSimilarCmdNoMatch(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"similar", "zzzzzzzzz", "--db", db})
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

func TestSimilarCmdJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	defer func() { jsonOutput = false }()

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"similar", "read_file", "--db", db, "--json"})
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

func TestSimilarCmdCustomKnown(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	defer func() { jsonOutput = false }()

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"similar", "rread", "--db", db, "--known", "Read,Write", "--json"})
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
