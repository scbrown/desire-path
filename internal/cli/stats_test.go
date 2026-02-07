package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/store"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

func TestStatsCmdEmpty(t *testing.T) {
	path := setupTestDB(t)
	dbPath = path
	jsonOutput = false

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"stats"})
	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("Execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Total desires:      0") {
		t.Errorf("expected 'Total desires:      0' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Unique tool names:  0") {
		t.Errorf("expected 'Unique tool names:  0' in output, got:\n%s", output)
	}
}

func TestStatsCmdJSON(t *testing.T) {
	path := setupTestDB(t)
	dbPath = path
	jsonOutput = true
	defer func() { jsonOutput = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := rootCmd
	cmd.SetArgs([]string{"stats", "--json"})
	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("Execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var st store.Stats
	if err := json.Unmarshal(buf.Bytes(), &st); err != nil {
		t.Fatalf("JSON unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if st.TotalDesires != 0 {
		t.Errorf("TotalDesires = %d, want 0", st.TotalDesires)
	}
}

func TestStatsCmdWithData(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedDesires(t, s)
	s.Close()

	dbPath = db
	jsonOutput = false
	defer func() { jsonOutput = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"stats", "--db", db})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("Execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify key sections of text output.
	if !strings.Contains(output, "Total desires:      3") {
		t.Errorf("expected 'Total desires:      3' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Unique tool names:  2") {
		t.Errorf("expected 'Unique tool names:  2' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Date range:") {
		t.Errorf("expected 'Date range:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Last 24h:") {
		t.Errorf("expected 'Last 24h:' in output, got:\n%s", output)
	}
	// Top sources and top desires should appear.
	if !strings.Contains(output, "Top sources:") {
		t.Errorf("expected 'Top sources:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Top desires:") {
		t.Errorf("expected 'Top desires:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "read_file") {
		t.Errorf("expected 'read_file' in top desires, got:\n%s", output)
	}
}

func TestStatsCmdJSONWithData(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedDesires(t, s)
	s.Close()

	dbPath = db
	jsonOutput = true
	defer func() { jsonOutput = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"stats", "--db", db, "--json"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("Execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var st store.Stats
	if err := json.Unmarshal(buf.Bytes(), &st); err != nil {
		t.Fatalf("JSON unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if st.TotalDesires != 3 {
		t.Errorf("TotalDesires = %d, want 3", st.TotalDesires)
	}
	if st.UniquePaths != 2 {
		t.Errorf("UniquePaths = %d, want 2", st.UniquePaths)
	}
}
