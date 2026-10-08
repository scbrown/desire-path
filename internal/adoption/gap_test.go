package adoption

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGapMarkerSurvivesDatabaseWriterLockAndSuppressesRates(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "collector")
	s, err := Open(ctx, dir, Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO payloads(id,session,timestamp_ns,boundary,paths,base_paths,base_id,status,adopted,base_adopted) VALUES('event','session',1,0,'[]','[]','base','complete',1,0)`)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Summary(ctx, time.Time{}, time.Now())
	if err != nil || before.AdoptionRate == nil || *before.AdoptionRate != 1 {
		t.Fatal("positive control lacks an adoption rate", err)
	}
	c, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if err = MarkGap(dir); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	c.Close()
	after, err := s.Summary(ctx, time.Time{}, time.Now())
	if err != nil || !after.GapSeen || after.AdoptionRate != nil || after.BaselineRate != nil || after.Complete != 1 {
		t.Fatal("gap promoted missing data to a rate", err)
	}
	if err = MarkGap(dir); err != nil {
		t.Fatal("marker retry is not idempotent", err)
	}
	info, err := os.Stat(filepath.Join(dir, "coverage-gap"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("marker permissions", err)
	}
}

func TestCollectorRejectsDatabaseSymlinkWithoutTouchingTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "collector")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "adoption.sqlite")); err != nil {
		t.Skip(err)
	}
	if s, err := Open(context.Background(), dir, Resolver{}); err == nil {
		s.Close()
		t.Fatal("opened symlink database")
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "untouched" {
		t.Fatal("target changed")
	}
}

func TestFailedToolInvocationIsUnknownNotAdoption(t *testing.T) {
	paths, known := Files(Hook{Tool: "Read", Event: "PostToolUseFailure", Input: []byte(`{"file_path":"offered.go"}`)})
	if known || len(paths) != 0 {
		t.Fatal("failed read claimed a file open")
	}
}
