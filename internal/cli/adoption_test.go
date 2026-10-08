package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/scbrown/desire-path/internal/adoption"
	"github.com/spf13/cobra"
)

func TestAdoptionDefaultOffDoesNotReadInputOrCreateStore(t *testing.T) {
	t.Setenv("DP_SIGNPOST_ADOPTION_DIR", "")
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("invalid hook input")
	f.Seek(0, io.SeekStart)
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	if err = runAdoptionHook(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil || offset != 0 {
		t.Fatal("disabled collector consumed hook input")
	}
}

func TestMalformedObservationPersistsGapAndLeavesOriginalInputAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "collector")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DP_SIGNPOST_ADOPTION_DIR", dir)
	t.Setenv("DP_SIGNPOST_ADOPTION_ROOTS", "")
	f, err := os.Create(filepath.Join(t.TempDir(), "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("{invalid}")
	f.Seek(0, io.SeekStart)
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	phase := adoptionPhase
	adoptionPhase = "pre"
	defer func() { adoptionPhase = phase }()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err = runAdoptionHook(cmd, nil); err == nil {
		t.Fatal("malformed input accepted")
	}
	if !adoption.GapSeen(dir) {
		t.Fatal("malformed observation left no persistent gap")
	}
}
