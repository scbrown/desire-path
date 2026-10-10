package adoption

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoverageToleratesDamageWithoutLosingLaterOffers(t *testing.T) {
	s, a, b := fixture(t)
	paired(t, s, a, b)
	now := time.Now()
	event := func(id string) string {
		v, _ := json.Marshal(map[string]any{"event_id": id, "timestamp": now, "signpost_shown": true, "payload_paths": []string{a}})
		return string(v) + "\n"
	}
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := "{bad}\n" + strings.Repeat("x", (1<<20)+10) + "\n" + event("donor-event") + event("actual-event") + event("actual-event") + event("unregistered-event")
	if err := os.WriteFile(path, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := s.Coverage(context.Background(), path, time.Time{}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r.SourceOffers != 3 || r.Registered != 2 || r.Missing != 1 || r.DuplicateIDs != 1 || r.Malformed != 2 || r.Oversized != 1 {
		t.Fatalf("coverage lost source evidence: %+v", r)
	}
}

func TestRepositoryQualifiedPathCannotEscapeItsNamedRoot(t *testing.T) {
	r := Resolver{Roots: map[string]string{"demo": t.TempDir()}}
	for _, path := range []string{"repos/demo/../outside.go", "repos/demo//outside.go"} {
		if _, ok := r.File(path, ""); ok {
			t.Fatal("escaping repository-qualified path accepted")
		}
	}
	if _, ok := r.File("repos/demo/inside.go", ""); !ok {
		t.Fatal("positive control missing")
	}
}
