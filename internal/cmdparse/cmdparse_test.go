package cmdparse

import (
	"testing"
)

func TestParse_Simple(t *testing.T) {
	segs := Parse("scp -r file.txt host:/path")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Command != "scp" {
		t.Errorf("command = %q, want scp", segs[0].Command)
	}
	if len(segs[0].Tokens) != 3 {
		t.Errorf("tokens = %d, want 3", len(segs[0].Tokens))
	}
}

func TestParse_Pipe(t *testing.T) {
	segs := Parse("cat file.txt | grep pattern | sort")
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	want := []string{"cat", "grep", "sort"}
	for i, w := range want {
		if segs[i].Command != w {
			t.Errorf("segment[%d].Command = %q, want %q", i, segs[i].Command, w)
		}
	}
}

func TestParse_And(t *testing.T) {
	segs := Parse("cd /tmp && scp -r file host:/")
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	if segs[0].Command != "cd" {
		t.Errorf("seg[0] = %q, want cd", segs[0].Command)
	}
	if segs[1].Command != "scp" {
		t.Errorf("seg[1] = %q, want scp", segs[1].Command)
	}
}

func TestParse_Or(t *testing.T) {
	segs := Parse("test -f foo || echo missing")
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	if segs[1].Command != "echo" {
		t.Errorf("seg[1] = %q, want echo", segs[1].Command)
	}
}

func TestParse_Semicolon(t *testing.T) {
	segs := Parse("echo hello; echo world")
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
}

func TestParse_QuotedPipe(t *testing.T) {
	segs := Parse(`echo "hello | world" | grep hello`)
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments (pipe inside quotes ignored), got %d", len(segs))
	}
	if segs[0].Command != "echo" {
		t.Errorf("seg[0] = %q, want echo", segs[0].Command)
	}
}

func TestParse_Empty(t *testing.T) {
	segs := Parse("")
	if len(segs) != 0 {
		t.Errorf("expected 0 segments, got %d", len(segs))
	}
}

func TestParse_Offsets(t *testing.T) {
	cmd := "cat f | grep x"
	segs := Parse(cmd)
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	// Offsets point to trimmed content.
	if segs[0].Start != 0 {
		t.Errorf("seg[0].Start = %d, want 0", segs[0].Start)
	}
	if cmd[segs[0].Start:segs[0].End] != "cat f" {
		t.Errorf("seg[0] slice = %q", cmd[segs[0].Start:segs[0].End])
	}
	if cmd[segs[1].Start:segs[1].End] != "grep x" {
		t.Errorf("seg[1] slice = %q", cmd[segs[1].Start:segs[1].End])
	}
}

// --- CorrectFlag tests ---

func TestCorrectFlag_Standalone(t *testing.T) {
	seg := Segment{Command: "scp", Raw: "scp -r file.txt host:/path"}
	got, ok := CorrectFlag(seg, "r", "R")
	if !ok {
		t.Fatal("expected correction")
	}
	if got != "scp -R file.txt host:/path" {
		t.Errorf("got %q", got)
	}
}

func TestCorrectFlag_Combined(t *testing.T) {
	seg := Segment{Command: "scp", Raw: "scp -rP 22 file host:/"}
	got, ok := CorrectFlag(seg, "r", "R")
	if !ok {
		t.Fatal("expected correction")
	}
	if got != "scp -RP 22 file host:/" {
		t.Errorf("got %q", got)
	}
}

func TestCorrectFlag_CombinedMiddle(t *testing.T) {
	seg := Segment{Command: "tar", Raw: "tar -xzf file.tar.gz"}
	got, ok := CorrectFlag(seg, "z", "j")
	if !ok {
		t.Fatal("expected correction")
	}
	if got != "tar -xjf file.tar.gz" {
		t.Errorf("got %q", got)
	}
}

func TestCorrectFlag_Long(t *testing.T) {
	seg := Segment{Command: "scp", Raw: "scp --recursive file host:/"}
	got, ok := CorrectFlag(seg, "recursive", "no-recursive")
	if !ok {
		t.Fatal("expected correction")
	}
	if got != "scp --no-recursive file host:/" {
		t.Errorf("got %q", got)
	}
}

func TestCorrectFlag_LongWithValue(t *testing.T) {
	seg := Segment{Command: "curl", Raw: "curl --output=file.txt http://example.com"}
	got, ok := CorrectFlag(seg, "output", "out")
	if !ok {
		t.Fatal("expected correction")
	}
	if got != "curl --out=file.txt http://example.com" {
		t.Errorf("got %q", got)
	}
}

func TestCorrectFlag_NotFound(t *testing.T) {
	seg := Segment{Command: "scp", Raw: "scp -P 22 file host:/"}
	_, ok := CorrectFlag(seg, "r", "R")
	if ok {
		t.Error("expected no correction")
	}
}

func TestCorrectFlag_NotInFilename(t *testing.T) {
	seg := Segment{Command: "scp", Raw: "scp file-recovery.txt host:/"}
	_, ok := CorrectFlag(seg, "r", "R")
	if ok {
		t.Error("should not match -r in filename")
	}
}

// --- SubstituteCommand tests ---

func TestSubstituteCommand(t *testing.T) {
	seg := Segment{Command: "grep", Raw: "grep -rn pattern ."}
	got := SubstituteCommand(seg, "rg")
	if got != "rg -rn pattern ." {
		t.Errorf("got %q", got)
	}
}

func TestSubstituteCommand_Empty(t *testing.T) {
	seg := Segment{Raw: ""}
	got := SubstituteCommand(seg, "rg")
	if got != "" {
		t.Errorf("got %q", got)
	}
}

// --- ReplaceLiteral tests ---

func TestReplaceLiteral(t *testing.T) {
	seg := Segment{Raw: "scp -r file.txt user@oldhost:/path"}
	got := ReplaceLiteral(seg, "user@oldhost:", "user@newhost:")
	if got != "scp -r file.txt user@newhost:/path" {
		t.Errorf("got %q", got)
	}
}

// --- ApplyToFull tests ---

func TestApplyToFull_Pipe(t *testing.T) {
	full := "cat file | grep pattern | sort"
	segs := Parse(full)
	if len(segs) < 2 {
		t.Fatalf("expected >=2 segments, got %d", len(segs))
	}
	// Replace grep with rg in the full string.
	corrected := SubstituteCommand(segs[1], "rg")
	result := ApplyToFull(full, segs[1], corrected)
	if result != "cat file | rg pattern | sort" {
		t.Errorf("got %q", result)
	}
}

func TestApplyToFull_And(t *testing.T) {
	full := "cd /tmp && scp -r file host:/"
	segs := Parse(full)
	if len(segs) < 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	corrected, ok := CorrectFlag(segs[1], "r", "R")
	if !ok {
		t.Fatal("expected correction")
	}
	result := ApplyToFull(full, segs[1], corrected)
	if result != "cd /tmp && scp -R file host:/" {
		t.Errorf("got %q", result)
	}
}
