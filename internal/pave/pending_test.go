package pave

import (
	"strings"
	"testing"
	"time"
)

func TestPendingRoundTripAndDeliverOnce(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "sess-1", "python -c 'print(1)'", "use python3"); err != nil {
		t.Fatal(err)
	}
	got, ok := Take(dir, "sess-1", DefaultTTL)
	if !ok || got.Context != "use python3" || got.Command != "python -c 'print(1)'" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
	// Delivered ONCE: a correction repeated on every later call is nagging.
	if _, ok := Take(dir, "sess-1", DefaultTTL); ok {
		t.Fatal("the same correction was delivered twice")
	}
}

func TestPendingIsScopedToItsSession(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "sess-a", "cmd", "for a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Take(dir, "sess-b", DefaultTTL); ok {
		t.Fatal("another session's correction was delivered")
	}
	if got, ok := Take(dir, "sess-a", DefaultTTL); !ok || got.Context != "for a" {
		t.Fatalf("the owning session lost its correction: %+v %v", got, ok)
	}
}

// A stale correction is noise attached to unrelated work, and it must be
// DROPPED rather than left to be reconsidered on every later call.
func TestStalePendingIsDroppedNotKept(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "sess-1", "cmd", "old advice"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Take(dir, "sess-1", time.Nanosecond); ok {
		t.Fatal("a stale correction was delivered")
	}
	if _, ok := Take(dir, "sess-1", time.Hour); ok {
		t.Fatal("the stale correction was left on disk to be reconsidered later")
	}
}

// The newer correction wins: if two commands failed before the agent's next
// call, the relevant one is about what it is currently doing.
func TestNewerCorrectionReplacesOlder(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "s", "first", "first advice"); err != nil {
		t.Fatal(err)
	}
	if err := Store(dir, "s", "second", "second advice"); err != nil {
		t.Fatal(err)
	}
	got, ok := Take(dir, "s", DefaultTTL)
	if !ok || got.Context != "second advice" {
		t.Fatalf("got=%+v", got)
	}
}

// By the time this is read the agent has moved on, so an unattributed
// correction reads as a comment on whatever it is doing now.
func TestRenderNamesTheCommandItIsAbout(t *testing.T) {
	p := Pending{Command: "python -c 'print(1)'", Context: "use python3"}
	got := p.Render()
	if !strings.Contains(got, "python -c 'print(1)'") || !strings.Contains(got, "use python3") {
		t.Fatalf("render=%q", got)
	}
	bare := Pending{Context: "just this"}
	if bare.Render() != "just this" {
		t.Fatalf("render=%q", bare.Render())
	}
}

func TestMissingInputsAreNoOps(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "", "cmd", "ctx"); err != nil {
		t.Fatal(err)
	}
	if err := Store(dir, "s", "cmd", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := Take(dir, "s", DefaultTTL); ok {
		t.Fatal("an empty correction was stored")
	}
	if _, ok := Take("", "s", DefaultTTL); ok {
		t.Fatal("take from an empty dir returned something")
	}
}
