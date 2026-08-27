package cli

import (
	"encoding/json"
	"fmt"
	evalpkg "github.com/scbrown/desire-path/internal/eval"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var evalTasks, evalModels, evalSeed string
var evalReplicates int
var evalCmd = &cobra.Command{Use: "eval", Short: "Build reproducible signposting evaluation assignments", Long: "Validates immutable ground-truth tasks and emits a deterministic blocked matrix across every task, condition, and model family.", Example: "  dp eval plan --tasks eval/tasks.jsonl --models codex,claude,gemini --seed campaign-1"}
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
	matrix, err := evalpkg.Matrix(tasks, models, evalReplicates, evalSeed)
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

func init() {
	evalPlanCmd.Flags().StringVar(&evalTasks, "tasks", "eval/tasks.jsonl", "ground-truth task JSONL")
	evalPlanCmd.Flags().StringVar(&evalModels, "models", "codex,claude,gemini", "comma-separated model families")
	evalPlanCmd.Flags().IntVar(&evalReplicates, "replicates", 1, "replicates per task/model/condition")
	evalPlanCmd.Flags().StringVar(&evalSeed, "seed", "signposting-v1", "deterministic assignment seed")
	evalCmd.AddCommand(evalPlanCmd)
	rootCmd.AddCommand(evalCmd)
}
