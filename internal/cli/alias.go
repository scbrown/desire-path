package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/spf13/cobra"
)

// Alias command flags.
var (
	aliasDelete  bool
	aliasCmd_    string // --cmd
	aliasFlag    []string // --flag OLD NEW
	aliasReplace string // --replace NEW
	aliasTool    string // --tool
	aliasParam   string // --param
	aliasRegex   bool   // --regex
	aliasMessage string // --message
)

var aliasCmd = &cobra.Command{
	Use:   "alias [flags] [<from> <to>]",
	Short: "Create or update a tool name alias or command correction rule",
	Long: `Create a mapping from a hallucinated tool name to a real one,
or define a command correction rule for automatic parameter rewriting.

Tool name alias (positional args):
  dp alias read_file Read

Command flag correction (--cmd + --flag):
  dp alias --cmd scp --flag r R
  dp alias --cmd scp --flag r R --message "scp uses -R for recursive"

Command substitution (--cmd + --replace):
  dp alias --cmd grep --replace rg

Literal replacement (--cmd + positional args):
  dp alias --cmd scp "user@host:" "user@newhost:"

Advanced / MCP tools (--tool + --param):
  dp alias --tool MyMCPTool --param input_path "/old/path" "/new/path"
  dp alias --tool Bash --param command --regex "curl -k" "curl --cacert cert.pem"

Delete (specify same flags to identify the rule):
  dp alias --delete read_file
  dp alias --delete --cmd scp --flag r
  dp alias --delete --cmd grep --replace rg`,
	Example: `  dp alias read_file Read
  dp alias --cmd scp --flag r R
  dp alias --cmd grep --replace rg --message "Use ripgrep"
  dp alias --delete read_file`,
	RunE: runAlias,
}

var aliasesCmd = &cobra.Command{
	Use:   "aliases",
	Short: "List all tool name aliases and command correction rules",
	Long:  `Display all configured aliases and correction rules in a table or JSON format.`,
	Example: `  dp aliases
  dp aliases --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listAliases()
	},
}

func init() {
	aliasCmd.Flags().BoolVar(&aliasDelete, "delete", false, "delete the specified alias or rule")
	aliasCmd.Flags().StringVar(&aliasCmd_, "cmd", "", "command name for CLI corrections (implies tool=Bash, param=command)")
	aliasCmd.Flags().StringSliceVar(&aliasFlag, "flag", nil, "flag correction: OLD,NEW (requires --cmd)")
	aliasCmd.Flags().StringVar(&aliasReplace, "replace", "", "substitute command name (requires --cmd)")
	aliasCmd.Flags().StringVar(&aliasTool, "tool", "", "tool name for parameter corrections (advanced)")
	aliasCmd.Flags().StringVar(&aliasParam, "param", "", "parameter name to correct (requires --tool)")
	aliasCmd.Flags().BoolVar(&aliasRegex, "regex", false, "treat FROM as a regex pattern (requires --tool/--param)")
	aliasCmd.Flags().StringVar(&aliasMessage, "message", "", "custom message shown when correction fires")
	rootCmd.AddCommand(aliasCmd)
	rootCmd.AddCommand(aliasesCmd)
}

// runAlias dispatches to the right mode based on flags.
func runAlias(cmd *cobra.Command, args []string) error {
	alias, err := buildAlias(args)
	if err != nil {
		return err
	}

	if aliasDelete {
		return deleteAlias(alias)
	}
	return setAlias(alias)
}

// buildAlias constructs a model.Alias from CLI flags and args, validating constraints.
func buildAlias(args []string) (model.Alias, error) {
	var a model.Alias

	// Validate mutual exclusivity.
	if aliasCmd_ != "" && (aliasTool != "" || aliasParam != "") {
		return a, fmt.Errorf("--cmd and --tool/--param are mutually exclusive")
	}
	if len(aliasFlag) > 0 && aliasCmd_ == "" {
		return a, fmt.Errorf("--flag requires --cmd")
	}
	if aliasReplace != "" && aliasCmd_ == "" {
		return a, fmt.Errorf("--replace requires --cmd")
	}
	if aliasRegex && aliasTool == "" {
		return a, fmt.Errorf("--regex requires --tool/--param")
	}
	if (aliasTool != "" && aliasParam == "") || (aliasTool == "" && aliasParam != "") {
		return a, fmt.Errorf("--tool and --param must be used together")
	}
	if len(aliasFlag) > 0 && aliasReplace != "" {
		return a, fmt.Errorf("--flag and --replace are mutually exclusive")
	}

	a.Message = aliasMessage

	// Mode 1: --cmd with --flag
	if aliasCmd_ != "" && len(aliasFlag) > 0 {
		if len(aliasFlag) != 2 {
			return a, fmt.Errorf("--flag requires exactly two values: OLD,NEW (got %d)", len(aliasFlag))
		}
		a.From = aliasFlag[0]
		a.To = aliasFlag[1]
		a.Tool = "Bash"
		a.Param = "command"
		a.Command = aliasCmd_
		a.MatchKind = "flag"
		return a, nil
	}

	// Mode 2: --cmd with --replace
	if aliasCmd_ != "" && aliasReplace != "" {
		if aliasDelete {
			// For delete, we need the from (the command name) and the replace target.
			a.From = aliasCmd_
			a.To = aliasReplace
		} else {
			a.From = aliasCmd_
			a.To = aliasReplace
		}
		a.Tool = "Bash"
		a.Param = "command"
		a.Command = aliasCmd_
		a.MatchKind = "command"
		return a, nil
	}

	// Mode 3: --cmd with positional args (literal replacement)
	if aliasCmd_ != "" {
		if aliasDelete {
			if len(args) != 1 {
				return a, fmt.Errorf("--delete --cmd requires one positional arg (the FROM pattern to delete)")
			}
			a.From = args[0]
			a.Tool = "Bash"
			a.Param = "command"
			a.Command = aliasCmd_
			a.MatchKind = "literal"
			return a, nil
		}
		if len(args) != 2 {
			return a, fmt.Errorf("--cmd requires two positional arguments: FROM TO")
		}
		a.From = args[0]
		a.To = args[1]
		a.Tool = "Bash"
		a.Param = "command"
		a.Command = aliasCmd_
		a.MatchKind = "literal"
		return a, nil
	}

	// Mode 4: --tool/--param (advanced)
	if aliasTool != "" {
		matchKind := "literal"
		if aliasRegex {
			matchKind = "regex"
		}
		if aliasDelete {
			if len(args) != 1 {
				return a, fmt.Errorf("--delete --tool/--param requires one positional arg (the FROM pattern)")
			}
			a.From = args[0]
			a.Tool = aliasTool
			a.Param = aliasParam
			a.MatchKind = matchKind
			return a, nil
		}
		if len(args) != 2 {
			return a, fmt.Errorf("--tool/--param requires two positional arguments: FROM TO")
		}
		a.From = args[0]
		a.To = args[1]
		a.Tool = aliasTool
		a.Param = aliasParam
		a.MatchKind = matchKind
		return a, nil
	}

	// Mode 5: Plain tool name alias (positional args only)
	if aliasDelete {
		if len(args) != 1 {
			return a, fmt.Errorf("--delete requires exactly one argument: dp alias --delete <from>")
		}
		a.From = args[0]
		return a, nil
	}
	if len(args) != 2 {
		return a, fmt.Errorf("requires exactly two arguments: dp alias <from> <to>")
	}
	a.From = args[0]
	a.To = args[1]
	return a, nil
}

// aliasResult is the JSON structure for alias mutation results.
type aliasResult struct {
	Action string `json:"action"`
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
}

func setAlias(a model.Alias) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.SetAlias(context.Background(), a); err != nil {
		return fmt.Errorf("set alias: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(aliasResult{Action: "set", From: a.From, To: a.To})
	}
	if a.IsToolNameAlias() {
		fmt.Printf("Alias set: %s -> %s\n", a.From, a.To)
	} else {
		fmt.Printf("Rule set: %s %s -> %s (%s)\n", a.Command, a.From, a.To, a.MatchKind)
	}
	return nil
}

func deleteAlias(a model.Alias) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	deleted, err := s.DeleteAlias(context.Background(), a.From, a.Tool, a.Param, a.Command, a.MatchKind)
	if err != nil {
		return fmt.Errorf("delete alias: %w", err)
	}
	if !deleted {
		return fmt.Errorf("alias %q not found", a.From)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(aliasResult{Action: "deleted", From: a.From})
	}
	fmt.Printf("Alias deleted: %s\n", a.From)
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

	tbl := NewTable(os.Stdout, "FROM", "TO", "TYPE", "COMMAND", "CREATED")
	for _, a := range aliases {
		kind := "alias"
		if a.MatchKind != "" {
			kind = a.MatchKind
		}
		tbl.Row(a.From, a.To, kind, a.Command, a.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	return tbl.Flush()
}
