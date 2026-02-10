package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/spf13/cobra"
)

var aliasDelete bool

var aliasCmd = &cobra.Command{
	Use:   "alias <from> <to>",
	Short: "Create or update a tool name alias",
	Long: `Create a mapping from a hallucinated tool name to a real one.
Uses upsert behavior: if the alias already exists, the target is updated.

Use --delete to remove an existing alias.`,
	Example: `  dp alias read_file Read
  dp alias run_tests Bash
  dp alias --delete read_file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if aliasDelete {
			if len(args) != 1 {
				return fmt.Errorf("--delete requires exactly one argument: dp alias --delete <from>")
			}
			return deleteAlias(args[0])
		}
		if len(args) != 2 {
			return fmt.Errorf("requires exactly two arguments: dp alias <from> <to>")
		}
		return setAlias(args[0], args[1])
	},
}

var aliasesCmd = &cobra.Command{
	Use:   "aliases",
	Short: "List all tool name aliases",
	Long:  `Display all configured tool name aliases in a table or JSON format.`,
	Example: `  dp aliases
  dp aliases --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listAliases()
	},
}

func init() {
	aliasCmd.Flags().BoolVar(&aliasDelete, "delete", false, "delete the specified alias")
	rootCmd.AddCommand(aliasCmd)
	rootCmd.AddCommand(aliasesCmd)
}

// aliasResult is the JSON structure for alias mutation results.
type aliasResult struct {
	Action string `json:"action"`
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
}

func setAlias(from, to string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	a := model.Alias{From: from, To: to}
	if err := s.SetAlias(context.Background(), a); err != nil {
		return fmt.Errorf("set alias: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(aliasResult{Action: "set", From: from, To: to})
	}
	fmt.Printf("Alias set: %s -> %s\n", from, to)
	return nil
}

func deleteAlias(from string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	deleted, err := s.DeleteAlias(context.Background(), from, "", "", "", "")
	if err != nil {
		return fmt.Errorf("delete alias: %w", err)
	}
	if !deleted {
		return fmt.Errorf("alias %q not found", from)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(aliasResult{Action: "deleted", From: from})
	}
	fmt.Printf("Alias deleted: %s\n", from)
	return nil
}

func listAliases() error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		return fmt.Errorf("get aliases: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if aliases == nil {
			aliases = []model.Alias{}
		}
		return enc.Encode(aliases)
	}

	if len(aliases) == 0 {
		fmt.Fprintln(os.Stderr, "No aliases configured.")
		return nil
	}

	tbl := NewTable(os.Stdout, "FROM", "TO", "CREATED")
	for _, a := range aliases {
		tbl.Row(a.From, a.To, a.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	return tbl.Flush()
}
