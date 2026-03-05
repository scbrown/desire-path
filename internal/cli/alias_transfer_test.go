package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/scbrown/desire-path/internal/store"
)

func openStoreAt(path string) (store.Store, error) {
	return store.New(path)
}

func TestAliasExportTOML(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "test.db")
	out := filepath.Join(tmp, "aliases.toml")

	// Create an alias.
	rootCmd.SetArgs([]string{"--db", db, "alias", "read_file", "Read"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("alias set: %v", err)
	}

	// Export.
	rootCmd.SetArgs([]string{"--db", db, "aliases", "export", "-o", out, "--name", "test-collection", "--author", "test"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	var col AliasCollection
	if err := toml.Unmarshal(data, &col); err != nil {
		t.Fatalf("parse toml: %v", err)
	}
	if col.Meta.Version != 1 {
		t.Errorf("version = %d, want 1", col.Meta.Version)
	}
	if col.Meta.Name != "test-collection" {
		t.Errorf("name = %q, want %q", col.Meta.Name, "test-collection")
	}
	if col.Meta.Author != "test" {
		t.Errorf("author = %q, want %q", col.Meta.Author, "test")
	}
	if col.Meta.Count != 1 {
		t.Errorf("count = %d, want 1", col.Meta.Count)
	}
	if len(col.Aliases) != 1 {
		t.Fatalf("aliases = %d, want 1", len(col.Aliases))
	}
	if col.Aliases[0].From != "read_file" || col.Aliases[0].To != "Read" {
		t.Errorf("alias = %q -> %q, want read_file -> Read", col.Aliases[0].From, col.Aliases[0].To)
	}
}

func TestAliasExportJSON(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "test.db")
	out := filepath.Join(tmp, "aliases.json")

	rootCmd.SetArgs([]string{"--db", db, "alias", "read_file", "Read"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("alias set: %v", err)
	}

	rootCmd.SetArgs([]string{"--db", db, "aliases", "export", "-o", out, "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	var col AliasCollection
	if err := json.Unmarshal(data, &col); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if len(col.Aliases) != 1 {
		t.Fatalf("aliases = %d, want 1", len(col.Aliases))
	}
	if col.Aliases[0].From != "read_file" {
		t.Errorf("from = %q, want read_file", col.Aliases[0].From)
	}
}

func TestAliasImportRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	dbSrc := filepath.Join(tmp, "src.db")
	dbDst := filepath.Join(tmp, "dst.db")
	exportFile := filepath.Join(tmp, "aliases.toml")

	// Create aliases in source DB.
	rootCmd.SetArgs([]string{"--db", dbSrc, "alias", "read_file", "Read"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("alias set 1: %v", err)
	}
	rootCmd.SetArgs([]string{"--db", dbSrc, "alias", "write_file", "Write"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("alias set 2: %v", err)
	}

	// Export from source.
	rootCmd.SetArgs([]string{"--db", dbSrc, "aliases", "export", "-o", exportFile, "--format", "toml"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Import into destination (fresh DB).
	rootCmd.SetArgs([]string{"--db", dbDst, "aliases", "import", exportFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify destination has both aliases.
	rootCmd.SetArgs([]string{"--db", dbDst, "aliases", "--json"})
	// Capture output by re-reading the DB directly.
	s, err := openStoreAt(dbDst)
	if err != nil {
		t.Fatalf("open dst store: %v", err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(t.Context())
	if err != nil {
		t.Fatalf("get aliases: %v", err)
	}
	if len(aliases) != 2 {
		t.Fatalf("imported aliases = %d, want 2", len(aliases))
	}

	found := map[string]string{}
	for _, a := range aliases {
		found[a.From] = a.To
	}
	if found["read_file"] != "Read" {
		t.Errorf("read_file -> %q, want Read", found["read_file"])
	}
	if found["write_file"] != "Write" {
		t.Errorf("write_file -> %q, want Write", found["write_file"])
	}
}

func TestAliasImportSkipExisting(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "test.db")
	exportFile := filepath.Join(tmp, "aliases.toml")

	// Create alias in DB.
	rootCmd.SetArgs([]string{"--db", db, "alias", "read_file", "Read"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("alias set: %v", err)
	}

	// Export it.
	rootCmd.SetArgs([]string{"--db", db, "aliases", "export", "-o", exportFile, "--format", "toml"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Import back (should skip since it exists).
	rootCmd.SetArgs([]string{"--db", db, "aliases", "import", exportFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify still only 1 alias.
	s, err := openStoreAt(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(t.Context())
	if err != nil {
		t.Fatalf("get aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Errorf("aliases = %d, want 1 (skip should prevent duplicate)", len(aliases))
	}
}
