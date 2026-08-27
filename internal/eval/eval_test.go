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
	a, err := Matrix(tasks, []string{"codex", "claude"}, 2, "seed")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Matrix(tasks, []string{"codex", "claude"}, 2, "seed")
	if len(a) != 20 || a[0].AssignmentID != b[0].AssignmentID {
		t.Fatalf("len=%d deterministic=%v", len(a), a[0].AssignmentID == b[0].AssignmentID)
	}
}

func TestMatrixRequiresTwoModelFamilies(t *testing.T) {
	tasks, err := ReadTasks(strings.NewReader(`{"task_id":"t1","repo":"o/r","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","query":"q","class":"weak","ground_truth":["x:1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Matrix(tasks, []string{"codex"}, 1, "seed"); err == nil {
		t.Fatal("expected one-family matrix to be refused")
	}
}

func TestInvalidTaskRefused(t *testing.T) {
	_, err := ReadTasks(strings.NewReader(`{"task_id":"t","repo":"r","revision":"short","query":"q","class":"weak","ground_truth":[]}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}
