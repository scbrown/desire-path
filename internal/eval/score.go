package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type Result struct {
	AssignmentID    string `json:"assignment_id"`
	ModelFamily     string `json:"model_family"`
	Condition       string `json:"condition"`
	Shown           bool   `json:"signpost_shown"`
	Adopted         bool   `json:"adopted"`
	Correct         bool   `json:"correct"`
	SignpostCorrect bool   `json:"signpost_correct"`
	Turns           int    `json:"turns_to_locate"`
	Tokens          int    `json:"tokens_to_resolution"`
}
type Summary struct {
	ModelFamily       string  `json:"model_family"`
	Condition         string  `json:"condition"`
	Runs              int     `json:"runs"`
	Shown             int     `json:"shown"`
	AdoptionRate      float64 `json:"adoption_rate"`
	CorrectnessRate   float64 `json:"correctness_rate"`
	SignpostPrecision float64 `json:"signpost_precision"`
	AvgTurns          float64 `json:"avg_turns_to_locate"`
	AvgTokens         float64 `json:"avg_tokens_to_resolution"`
}

func Score(r io.Reader) ([]Summary, error) {
	s := bufio.NewScanner(r)
	groups := map[string]*struct {
		Summary
		adopted, correct, spCorrect, turns, tokens int
	}{}
	for line := 1; s.Scan(); line++ {
		var x Result
		if err := json.Unmarshal(s.Bytes(), &x); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if x.AssignmentID == "" || x.ModelFamily == "" || x.Condition == "" {
			return nil, fmt.Errorf("line %d: assignment_id, model_family, and condition are required", line)
		}
		k := x.ModelFamily + "\x00" + x.Condition
		g := groups[k]
		if g == nil {
			g = &struct {
				Summary
				adopted, correct, spCorrect, turns, tokens int
			}{Summary: Summary{ModelFamily: x.ModelFamily, Condition: x.Condition}}
			groups[k] = g
		}
		g.Runs++
		if x.Shown {
			g.Shown++
		}
		if x.Adopted {
			g.adopted++
		}
		if x.Correct {
			g.correct++
		}
		if x.SignpostCorrect {
			g.spCorrect++
		}
		g.turns += x.Turns
		g.tokens += x.Tokens
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no results")
	}
	out := make([]Summary, 0, len(groups))
	for _, g := range groups {
		g.AdoptionRate = ratio(g.adopted, g.Shown)
		g.CorrectnessRate = ratio(g.correct, g.Runs)
		g.SignpostPrecision = ratio(g.spCorrect, g.Shown)
		g.AvgTurns = float64(g.turns) / float64(g.Runs)
		g.AvgTokens = float64(g.tokens) / float64(g.Runs)
		out = append(out, g.Summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModelFamily == out[j].ModelFamily {
			return out[i].Condition < out[j].Condition
		}
		return out[i].ModelFamily < out[j].ModelFamily
	})
	return out, nil
}
func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}
