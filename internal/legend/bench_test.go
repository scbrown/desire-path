package legend

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// BenchmarkLoadParts splits the per-read load cost on a real gazetteer
// (LEGEND_BENCH_GAZETTEER). It is skipped when the file is not provided.
func benchFile(b *testing.B) (string, File) {
	p := os.Getenv("LEGEND_BENCH_GAZETTEER")
	if p == "" {
		b.Skip("LEGEND_BENCH_GAZETTEER not set")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		b.Fatal(err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		b.Fatal(err)
	}
	return p, f
}

func BenchmarkParse(b *testing.B) {
	p, _ := benchFile(b)
	for i := 0; i < b.N; i++ {
		raw, _ := os.ReadFile(p)
		var f File
		_ = json.Unmarshal(raw, &f)
	}
}

func BenchmarkCompile(b *testing.B) {
	_, f := benchFile(b)
	for i := 0; i < b.N; i++ {
		Compile(f.Entries)
	}
}

func BenchmarkLoad(b *testing.B) {
	p, _ := benchFile(b)
	for i := 0; i < b.N; i++ {
		_, _ = Load(p, 0, time.Now())
	}
}
