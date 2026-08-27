package eval

import (
	"strings"
	"testing"
)

func TestScore(t *testing.T) {
	s, err := Score(strings.NewReader("{\"assignment_id\":\"a\",\"model_family\":\"codex\",\"condition\":\"gated-signpost\",\"signpost_shown\":true,\"adopted\":true,\"correct\":true,\"signpost_correct\":true,\"turns_to_locate\":2,\"tokens_to_resolution\":10}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].AdoptionRate != 1 || s[0].CorrectnessRate != 1 || s[0].SignpostPrecision != 1 || s[0].AvgTurns != 2 {
		t.Fatalf("%+v", s)
	}
}
func TestScoreRefusesEmpty(t *testing.T) {
	if _, err := Score(strings.NewReader("")); err == nil {
		t.Fatal("expected error")
	}
}
