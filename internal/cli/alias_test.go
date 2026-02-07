package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func TestAliasCmdSet(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = false

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "read_file", "Read"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Alias set: read_file -> Read") {
		t.Errorf("expected set confirmation, got: %s", output)
	}

	// Verify in database.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].From != "read_file" || aliases[0].To != "Read" {
		t.Errorf("got alias %s->%s, want read_file->Read", aliases[0].From, aliases[0].To)
	}
}

func TestAliasCmdUpsert(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = false

	// Set initial alias.
	rootCmd.SetArgs([]string{"alias", "--db", db, "foo", "bar"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("first set: %v", err)
	}

	// Upsert with new target.
	rootCmd.SetArgs([]string{"alias", "--db", db, "foo", "baz"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias after upsert, got %d", len(aliases))
	}
	if aliases[0].To != "baz" {
		t.Errorf("expected upserted value 'baz', got %q", aliases[0].To)
	}
}

func TestAliasCmdDelete(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed an alias.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	aliasDelete = true
	defer func() { aliasDelete = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "read_file"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Alias deleted: read_file") {
		t.Errorf("expected delete confirmation, got: %s", output)
	}

	// Verify removed.
	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	aliases, err := s2.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestAliasCmdDeleteNotFound(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	aliasDelete = true
	defer func() { aliasDelete = false }()

	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "nonexistent"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for deleting nonexistent alias")
	}
}

func TestAliasCmdWrongArgCount(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = false

	// Too few args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "only_one"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error with 1 arg")
	}

	// Too many args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "a", "b", "c"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error with 3 args")
	}
}

func TestAliasCmdDeleteWrongArgCount(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = true
	defer func() { aliasDelete = false }()

	// --delete with 0 args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for --delete with 0 args")
	}

	// --delete with 2 args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "a", "b"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for --delete with 2 args")
	}
}

func TestAliasesCmdTable(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed aliases.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "write_file", "Write"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"aliases", "--db", db})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "FROM") || !strings.Contains(output, "TO") {
		t.Errorf("expected table headers, got: %s", output)
	}
	if !strings.Contains(output, "read_file") || !strings.Contains(output, "Read") {
		t.Errorf("expected read_file->Read alias, got: %s", output)
	}
	if !strings.Contains(output, "write_file") || !strings.Contains(output, "Write") {
		t.Errorf("expected write_file->Write alias, got: %s", output)
	}
}

func TestAliasesCmdEmpty(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"aliases", "--db", db})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stderr = oldStderr
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No aliases configured.") {
		t.Errorf("expected 'No aliases configured.', got: %s", output)
	}
}

func TestAliasesCmdJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = true
	defer func() { jsonOutput = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"aliases", "--db", db, "--json"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var aliases []model.Alias
	if err := json.Unmarshal(buf.Bytes(), &aliases); err != nil {
		t.Fatalf("json unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].From != "read_file" || aliases[0].To != "Read" {
		t.Errorf("got %s->%s, want read_file->Read", aliases[0].From, aliases[0].To)
	}
}
