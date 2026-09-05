package eval

import (
	"strings"
	"testing"
)

func TestMatrixCompleteDeterministic(t *testing.T) {
	tasks, err := ReadTasks(strings.NewReader(`{"task_id":"t1","repo":"o/r","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","query":"q","class":"weak","ground_truth":["x:1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Matrix(tasks, []string{"codex", "claude"}, 2, "seed", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Matrix(tasks, []string{"codex", "claude"}, 2, "seed", nil)
	if len(a) != 20 || a[0].AssignmentID != b[0].AssignmentID {
		t.Fatalf("len=%d deterministic=%v", len(a), a[0].AssignmentID == b[0].AssignmentID)
	}
}

func TestMatrixRequiresTwoModelFamilies(t *testing.T) {
	tasks, err := ReadTasks(strings.NewReader(`{"task_id":"t1","repo":"o/r","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","query":"q","class":"weak","ground_truth":["x:1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Matrix(tasks, []string{"codex"}, 1, "seed", nil); err == nil {
		t.Fatal("expected one-family matrix to be refused")
	}
}

func TestInvalidTaskRefused(t *testing.T) {
	_, err := ReadTasks(strings.NewReader(`{"task_id":"t","repo":"r","revision":"short","query":"q","class":"weak","ground_truth":[]}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// The payload arm must never appear in the main matrix, and must plan on its
// own. Mixing them would sum two different adoption measurements into one rate.
func TestPayloadArmIsPlannedSeparately(t *testing.T) {
	tasks := []Task{{TaskID: "t1", Repo: "r", Revision: strings.Repeat("a", 40), Query: "q", Class: "strong", GroundTruth: []string{"f:1"}}}
	models := []string{"codex", "claude"}

	main, err := Matrix(tasks, models, 1, "seed", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range main {
		for _, c := range PayloadConditions {
			if a.Condition == c {
				t.Fatalf("payload condition %q leaked into the main matrix", c)
			}
		}
	}
	if len(main) != len(models)*len(Conditions) {
		t.Fatalf("main matrix has %d assignments, want %d", len(main), len(models)*len(Conditions))
	}

	payload, err := Matrix(tasks, models, 1, "seed", PayloadConditions)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != len(models)*len(PayloadConditions) {
		t.Fatalf("payload arm has %d assignments, want %d", len(payload), len(models)*len(PayloadConditions))
	}
	for _, a := range payload {
		if a.Condition != "payload-signpost" {
			t.Fatalf("payload arm carried condition %q", a.Condition)
		}
	}
}
