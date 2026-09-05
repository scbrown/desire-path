// Package eval defines reproducible signposting experiment assignments.
package eval

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Conditions is the MAIN matrix: five arms, unchanged since the pilot so the
// campaigns stay comparable.
var Conditions = []string{"bare-literal", "prompt-semantic", "replacement", "always-signpost", "gated-signpost"}

// PayloadConditions is the payload arm, and it is deliberately NOT a member of
// Conditions. It answers a different question — whether an agent takes an
// ANSWER when it will not take a pointer — and its adoption metric is a
// different measurement (see the campaign harness). Summing the two, or
// planning them in one matrix, would report a rate that means nothing.
//
// Its comparator is always-signpost: the payload arm applies the same gate and
// differs only in what it injects.
var PayloadConditions = []string{"payload-signpost"}

type Task struct {
	TaskID      string   `json:"task_id"`
	Repo        string   `json:"repo"`
	Revision    string   `json:"revision"`
	Query       string   `json:"query"`
	Class       string   `json:"class"`
	GroundTruth []string `json:"ground_truth"`
}

type Assignment struct {
	AssignmentID string   `json:"assignment_id"`
	TaskID       string   `json:"task_id"`
	Repo         string   `json:"repo"`
	Revision     string   `json:"revision"`
	Query        string   `json:"query"`
	Class        string   `json:"class"`
	GroundTruth  []string `json:"ground_truth"`
	ModelFamily  string   `json:"model_family"`
	Condition    string   `json:"condition"`
	Replicate    int      `json:"replicate"`
}

func ReadTasks(r io.Reader) ([]Task, error) {
	s := bufio.NewScanner(r)
	var tasks []Task
	seen := map[string]bool{}
	for line := 1; s.Scan(); line++ {
		var t Task
		if err := json.Unmarshal(s.Bytes(), &t); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := Validate(t); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if seen[t.TaskID] {
			return nil, fmt.Errorf("line %d: duplicate task_id %q", line, t.TaskID)
		}
		seen[t.TaskID] = true
		tasks = append(tasks, t)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks")
	}
	return tasks, nil
}

func Validate(t Task) error {
	if t.TaskID == "" || t.Repo == "" || t.Query == "" {
		return fmt.Errorf("task_id, repo, and query are required")
	}
	if len(t.Revision) != 40 {
		return fmt.Errorf("revision must be a 40-character commit SHA")
	}
	if _, err := hex.DecodeString(t.Revision); err != nil {
		return fmt.Errorf("revision must be hexadecimal")
	}
	if t.Class != "strong" && t.Class != "weak" && t.Class != "misleading" {
		return fmt.Errorf("invalid class %q", t.Class)
	}
	if len(t.GroundTruth) == 0 {
		return fmt.Errorf("ground_truth is required")
	}
	return nil
}

// Matrix builds the blocked assignment matrix. conditions selects the arm;
// pass nil for the main five.
func Matrix(tasks []Task, models []string, replicates int, seed string, conditions []string) ([]Assignment, error) {
	if len(models) < 2 {
		return nil, fmt.Errorf("at least two model families are required")
	}
	if replicates < 1 {
		return nil, fmt.Errorf("replicates must be positive")
	}
	if len(conditions) == 0 {
		conditions = Conditions
	}
	var out []Assignment
	for _, t := range tasks {
		for _, m := range models {
			if m == "" {
				return nil, fmt.Errorf("empty model family")
			}
			for _, c := range conditions {
				for r := 1; r <= replicates; r++ {
					key := fmt.Sprintf("%s|%s|%s|%d", t.TaskID, m, c, r)
					sum := sha256.Sum256([]byte(seed + "|" + key))
					out = append(out, Assignment{AssignmentID: hex.EncodeToString(sum[:8]), TaskID: t.TaskID, Repo: t.Repo, Revision: t.Revision, Query: t.Query, Class: t.Class, GroundTruth: t.GroundTruth, ModelFamily: m, Condition: c, Replicate: r})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a := sha256.Sum256([]byte(seed + out[i].AssignmentID))
		b := sha256.Sum256([]byte(seed + out[j].AssignmentID))
		return string(a[:]) < string(b[:])
	})
	return out, nil
}
