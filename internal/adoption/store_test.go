package adoption

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(root, "state"), Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.Draw = func(n int64) (int64, error) { return 0, nil }
	return s, filepath.Join(root, "offered.go"), filepath.Join(root, "baseline.go")
}

func hook(session, id, tool, path string) Hook {
	input, _ := json.Marshal(map[string]string{"file_path": path})
	return Hook{Session: session, ID: id, Tool: tool, Input: input}
}

func start(t *testing.T, s *Store, h Hook) {
	t.Helper()
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
}
func finish(t *testing.T, s *Store, h Hook) {
	t.Helper()
	if err := s.Finish(context.Background(), h); err != nil {
		t.Fatal(err)
	}
}
func publish(t *testing.T, s *Store, h Hook, id, path string) {
	t.Helper()
	if err := s.Publish(context.Background(), h, id, time.Now(), []Reference{{Path: path}}); err != nil {
		t.Fatal(err)
	}
}
func summary(t *testing.T, s *Store) Summary {
	t.Helper()
	r, err := s.Summary(context.Background(), time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func paired(t *testing.T, s *Store, offered, baseline string) Hook {
	t.Helper()
	donor := hook("donor", "d0", "Bash", "")
	start(t, s, donor)
	publish(t, s, donor, "donor-event", baseline)
	finish(t, s, donor)
	for i := 0; i < 10; i++ {
		h := hook("donor", fmt.Sprint("next-", i), "Other", "")
		start(t, s, h)
		finish(t, s, h)
	}
	origin := hook("actor", "a0", "Bash", "")
	start(t, s, origin)
	publish(t, s, origin, "actual-event", offered)
	finish(t, s, origin)
	return origin
}

func TestTenCallsSameSessionAndDifferentEventBaseline(t *testing.T) {
	s, a, b := fixture(t)
	paired(t, s, a, b)
	other := hook("different-session", "x", "Read", b)
	start(t, s, other)
	finish(t, s, other)
	for i := 1; i <= 10; i++ {
		h := hook("actor", fmt.Sprint(i), "Other", "")
		if i == 2 {
			h = hook("actor", fmt.Sprint(i), "Read", a)
		}
		start(t, s, h)
		finish(t, s, h)
		if i < 10 && summary(t, s).Complete != 0 {
			t.Fatal("negative labels before ten calls completed")
		}
	}
	r := summary(t, s)
	if r.Complete != 1 || r.Adopted != 1 || r.BaselineAdopted != 0 {
		t.Fatalf("want adoption 1/base 0, got %+v", r)
	}
	if r.Statuses["unpaired"] != 1 {
		t.Fatal("cold-start donor must be explicitly unpaired")
	}
}

func TestBaselinePositiveAndEleventhCallExcluded(t *testing.T) {
	s, a, b := fixture(t)
	paired(t, s, a, b)
	for i := 1; i <= 11; i++ {
		h := hook("actor", fmt.Sprint(i), "Other", "")
		if i == 1 {
			h = hook("actor", fmt.Sprint(i), "Read", b)
		}
		if i == 11 {
			h = hook("actor", fmt.Sprint(i), "Read", a)
		}
		start(t, s, h)
		finish(t, s, h)
	}
	r := summary(t, s)
	if r.Complete != 1 || r.Adopted != 0 || r.BaselineAdopted != 1 {
		t.Fatalf("eleventh call leaked into window: %+v", r)
	}
}

func TestAlreadyStartedParallelCallIsNotAdoption(t *testing.T) {
	s, a, b := fixture(t)
	d := hook("donor", "d", "Other", "")
	start(t, s, d)
	publish(t, s, d, "donor", b)
	origin := hook("actor", "origin", "Other", "")
	start(t, s, origin)
	earlier := hook("actor", "parallel", "Read", a)
	start(t, s, earlier)
	publish(t, s, origin, "payload", a)
	finish(t, s, earlier)
	finish(t, s, origin)
	for i := 0; i < 10; i++ {
		h := hook("actor", fmt.Sprint(i), "Other", "")
		start(t, s, h)
		finish(t, s, h)
	}
	if r := summary(t, s); r.Complete != 1 || r.Adopted != 0 {
		t.Fatalf("pre-existing call falsely adopted: %+v", r)
	}
}

func TestDuplicateHooksAndPublishAreIdempotent(t *testing.T) {
	s, a, b := fixture(t)
	origin := paired(t, s, a, b)
	start(t, s, origin)
	finish(t, s, origin)
	publish(t, s, origin, "actual-event", a)
	for i := 0; i < 10; i++ {
		h := hook("actor", fmt.Sprint(i), "Read", a)
		start(t, s, h)
		start(t, s, h)
		finish(t, s, h)
		finish(t, s, h)
	}
	r := summary(t, s)
	if r.Complete != 1 || r.Counters["offers"] != 2 || r.Counters["pre"] != 22 || r.Counters["post"] != 22 {
		t.Fatalf("duplicates changed event population: %+v", r)
	}
}

func TestMissingPreAndAmbiguousTargetsRemainUnknown(t *testing.T) {
	s, a, b := fixture(t)
	paired(t, s, a, b)
	finish(t, s, hook("actor", "no-start", "Read", a))
	r := summary(t, s)
	if r.Statuses["unknown"] != 1 || r.Complete != 0 || r.AdoptionRate != nil {
		t.Fatalf("missing hook became a negative: %+v", r)
	}
	h := hook("other", "origin", "Other", "")
	start(t, s, h)
	if err := s.Publish(context.Background(), h, "ambiguous", time.Now(), []Reference{{Path: "src/same.go"}}); err != nil {
		t.Fatal(err)
	}
	if r = summary(t, s); r.Statuses["unknown"] != 2 {
		t.Fatalf("unscoped relative target guessed a repo: %+v", r)
	}
}

func TestPersistentStoreContainsNoRawInputsOrPaths(t *testing.T) {
	s, a, _ := fixture(t)
	h := hook("PRIVATE_MARKER_session", "PRIVATE_MARKER_call", "Read", a)
	start(t, s, h)
	publish(t, s, h, "PRIVATE_MARKER_event", a)
	finish(t, s, h)
	var path string
	if err := s.db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-journal", "-wal"} {
		data, err := os.ReadFile(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("PRIVATE_MARKER")) || bytes.Contains(data, []byte(a)) {
			t.Fatal("raw identity/path escaped into collector")
		}
	}
}

func TestOutOfOrderCompletionsStillRequireAllTen(t *testing.T) {
	s, a, b := fixture(t)
	paired(t, s, a, b)
	var calls []Hook
	for i := 0; i < 10; i++ {
		h := hook("actor", fmt.Sprint(i), "Other", "")
		if i == 0 {
			h = hook("actor", fmt.Sprint(i), "Read", a)
		}
		start(t, s, h)
		calls = append(calls, h)
	}
	for i := 9; i > 0; i-- {
		finish(t, s, calls[i])
	}
	if summary(t, s).Complete != 0 {
		t.Fatal("completion of tenth-started call falsely closes missing first call")
	}
	finish(t, s, calls[0])
	if r := summary(t, s); r.Complete != 1 || r.Adopted != 1 {
		t.Fatalf("missing late positive: %+v", r)
	}
}

func TestSeparateProcessesShareExactSequence(t *testing.T) {
	s, a, b := fixture(t)
	paired(t, s, a, b)
	var path string
	if err := s.db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path); err != nil {
		t.Fatal(err)
	}
	other, err := Open(context.Background(), filepath.Dir(path), Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	for i := 0; i < 10; i++ {
		h := hook("actor", fmt.Sprint(i), "Read", a)
		start(t, s, h)
		finish(t, other, h)
	}
	if r := summary(t, s); r.Complete != 1 || r.Adopted != 1 {
		t.Fatalf("process key/counters diverged: %+v", r)
	}
}
