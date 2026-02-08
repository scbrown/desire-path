package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
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

func seedInvocations(t *testing.T, s store.Store) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	invocations := []model.Invocation{
		{ID: "inv-1", Source: "claude-code", ToolName: "Read", Timestamp: base},
		{ID: "inv-2", Source: "claude-code", ToolName: "Write", Timestamp: base.Add(-time.Hour)},
		{ID: "inv-3", Source: "cursor", ToolName: "Read", Timestamp: base.Add(-48 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("seed invocation: %v", err)
		}
	}
}

func TestStatsCmdInvocationsEmpty(t *testing.T) {
	path := setupTestDB(t)
	dbPath = path
	jsonOutput = false
	showInvocations = true
	defer func() { showInvocations = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"stats", "--invocations"})
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

	if !strings.Contains(output, "Total invocations:  0") {
		t.Errorf("expected 'Total invocations:  0' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Unique tools:       0") {
		t.Errorf("expected 'Unique tools:       0' in output, got:\n%s", output)
	}
}

func TestStatsCmdInvocationsWithData(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedInvocations(t, s)
	s.Close()

	dbPath = db
	jsonOutput = false
	showInvocations = true
	defer func() { showInvocations = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"stats", "--invocations", "--db", db})
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

	if !strings.Contains(output, "Total invocations:  3") {
		t.Errorf("expected 'Total invocations:  3' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Unique tools:       2") {
		t.Errorf("expected 'Unique tools:       2' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Date range:") {
		t.Errorf("expected 'Date range:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Last 24h:") {
		t.Errorf("expected 'Last 24h:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Top sources:") {
		t.Errorf("expected 'Top sources:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Top tools:") {
		t.Errorf("expected 'Top tools:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Read") {
		t.Errorf("expected 'Read' in top tools, got:\n%s", output)
	}
}

func TestStatsCmdInvocationsJSON(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedInvocations(t, s)
	s.Close()

	dbPath = db
	jsonOutput = true
	showInvocations = true
	defer func() {
		jsonOutput = false
		showInvocations = false
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"stats", "--invocations", "--json", "--db", db})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("Execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var ist store.InvocationStatsResult
	if err := json.Unmarshal(buf.Bytes(), &ist); err != nil {
		t.Fatalf("JSON unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if ist.Total != 3 {
		t.Errorf("Total = %d, want 3", ist.Total)
	}
	if ist.UniqueTools != 2 {
		t.Errorf("UniqueTools = %d, want 2", ist.UniqueTools)
	}
}

func TestStatsCmdWithoutInvocationsFlag(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedDesires(t, s)
	seedInvocations(t, s)
	s.Close()

	dbPath = db
	jsonOutput = false
	showInvocations = false

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

	// Without --invocations, should show desire stats, not invocation stats.
	if !strings.Contains(output, "Total desires:") {
		t.Errorf("expected desire stats without --invocations flag, got:\n%s", output)
	}
	if strings.Contains(output, "Total invocations:") {
		t.Errorf("should not show invocation stats without --invocations flag, got:\n%s", output)
	}
}
