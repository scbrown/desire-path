package cli

import (
	"encoding/json"
	"fmt"
	evalpkg "github.com/scbrown/desire-path/internal/eval"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var evalTasks, evalModels, evalSeed, evalArm string
var evalResults string
var evalReplicates int
var evalCmd = &cobra.Command{Use: "eval", Short: "Build reproducible signposting evaluation assignments", Long: "Validates immutable ground-truth tasks and emits a deterministic blocked matrix across every task, condition, and model family. --arm payload plans the payload arm instead, which is scored separately because its adoption metric is a different measurement.", Example: "  dp eval plan --tasks eval/tasks.jsonl --models codex,claude --seed campaign-1"}
var evalPlanCmd = &cobra.Command{Use: "plan", Short: "Validate tasks and emit the assignment matrix", RunE: func(cmd *cobra.Command, _ []string) error {
	f, err := os.Open(evalTasks)
	if err != nil {
		return err
	}
	defer f.Close()
	tasks, err := evalpkg.ReadTasks(f)
	if err != nil {
		return err
	}
	models := strings.Split(evalModels, ",")
	var conditions []string
	switch evalArm {
	case "", "main":
		conditions = evalpkg.Conditions
	case "payload":
		conditions = evalpkg.PayloadConditions
	default:
		return fmt.Errorf("unknown arm %q (main, payload)", evalArm)
	}
	matrix, err := evalpkg.Matrix(tasks, models, evalReplicates, evalSeed, conditions)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	for _, a := range matrix {
		if err := enc.Encode(a); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "planned %d assignments\n", len(matrix))
	return nil
}}
var evalScoreCmd = &cobra.Command{Use: "score", Short: "Aggregate completed assignment outcomes", RunE: func(cmd *cobra.Command, _ []string) error {
	f, err := os.Open(evalResults)
	if err != nil {
		return err
	}
	defer f.Close()
	summary, err := evalpkg.Score(f)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}}

func init() {
	evalPlanCmd.Flags().StringVar(&evalTasks, "tasks", "eval/tasks.jsonl", "ground-truth task JSONL")
	evalPlanCmd.Flags().StringVar(&evalModels, "models", "codex,claude", "comma-separated model families")
	evalPlanCmd.Flags().IntVar(&evalReplicates, "replicates", 1, "replicates per task/model/condition")
	evalPlanCmd.Flags().StringVar(&evalSeed, "seed", "signposting-v1", "deterministic assignment seed")
	evalPlanCmd.Flags().StringVar(&evalArm, "arm", "main", "which arm to plan: main (the five-condition matrix) or payload (the run-the-command arm, scored separately)")
	evalScoreCmd.Flags().StringVar(&evalResults, "results", "results.jsonl", "completed assignment result JSONL")
	evalCmd.AddCommand(evalPlanCmd, evalScoreCmd)
	rootCmd.AddCommand(evalCmd)
}
