package cli

import "testing"

// aegis-f014s8: the deployed default must augment (inject results), and stay
// gated so ordinary searches are untouched.
func TestDefaultSignpostConditionInjectsGated(t *testing.T) {
	if defaultSignpostCondition != "payload-gated-signpost" {
		t.Fatalf("default condition %q, want payload-gated-signpost", defaultSignpostCondition)
	}
}
