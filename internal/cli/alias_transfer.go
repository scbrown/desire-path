package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/spf13/cobra"
)

// AliasCollection is the portable interchange format for aliases.
type AliasCollection struct {
	Meta    CollectionMeta   `json:"meta" toml:"meta"`
	Aliases []ExportedAlias  `json:"aliases" toml:"aliases"`
}

// CollectionMeta holds metadata about an alias collection.
type CollectionMeta struct {
	Version     int       `json:"version" toml:"version"`
	Name        string    `json:"name,omitempty" toml:"name,omitempty"`
	Description string    `json:"description,omitempty" toml:"description,omitempty"`
	Author      string    `json:"author,omitempty" toml:"author,omitempty"`
	ExportedAt  time.Time `json:"exported_at" toml:"exported_at"`
	Source      string    `json:"source,omitempty" toml:"source,omitempty"`
	Count       int       `json:"count" toml:"count"`
}

// ExportedAlias is the portable representation of an alias.
// CreatedAt is omitted — it gets set on import.
type ExportedAlias struct {
	From      string `json:"from" toml:"from"`
	To        string `json:"to" toml:"to"`
	Tool      string `json:"tool,omitempty" toml:"tool,omitempty"`
	Param     string `json:"param,omitempty" toml:"param,omitempty"`
	Command   string `json:"command,omitempty" toml:"command,omitempty"`
	MatchKind string `json:"match_kind,omitempty" toml:"match_kind,omitempty"`
	Message   string `json:"message,omitempty" toml:"message,omitempty"`
}

func toExported(a model.Alias) ExportedAlias {
	return ExportedAlias{
		From:      a.From,
		To:        a.To,
		Tool:      a.Tool,
		Param:     a.Param,
		Command:   a.Command,
		MatchKind: a.MatchKind,
		Message:   a.Message,
	}
}

func toModel(e ExportedAlias) model.Alias {
	return model.Alias{
		From:      e.From,
		To:        e.To,
		Tool:      e.Tool,
		Param:     e.Param,
		Command:   e.Command,
		MatchKind: e.MatchKind,
		Message:   e.Message,
	}
}

// Export flags.
var (
	aliasExportName   string
	aliasExportDesc   string
	aliasExportAuthor string
	aliasExportFmt    string
	aliasExportOutput string
)

// Import flags.
var (
	importConflict string // "skip", "overwrite"
	importDryRun   bool
)

var aliasExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export aliases to a portable file",
	Long: `Export all configured aliases to a portable TOML or JSON file
that can be shared, committed to a repo, or imported on another machine.

The output includes metadata (version, author, description) and all alias
definitions. CreatedAt timestamps are NOT exported — they are set fresh
on import.`,
	Example: `  dp aliases export
  dp aliases export -o my-aliases.toml
  dp aliases export --format json -o aliases.json
  dp aliases export --name "gastown-crew" --author "maldoon"`,
	RunE: runAliasExport,
}

var aliasImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import aliases from a portable file",
	Long: `Import aliases from a TOML or JSON file previously exported with
'dp aliases export' or hand-authored.

By default, existing aliases are skipped (not overwritten). Use
--conflict=overwrite to replace existing aliases with imported ones.

Use --dry-run to preview what would be imported without making changes.`,
	Example: `  dp aliases import crew-aliases.toml
  dp aliases import aliases.json --conflict overwrite
  dp aliases import shared.toml --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runAliasImport,
}

func init() {
	aliasExportCmd.Flags().StringVarP(&aliasExportOutput, "output", "o", "", "output file path (default: stdout)")
	aliasExportCmd.Flags().StringVar(&aliasExportFmt, "format", "toml", "output format: toml or json")
	aliasExportCmd.Flags().StringVar(&aliasExportName, "name", "", "collection name")
	aliasExportCmd.Flags().StringVar(&aliasExportDesc, "description", "", "collection description")
	aliasExportCmd.Flags().StringVar(&aliasExportAuthor, "author", "", "collection author")

	aliasImportCmd.Flags().StringVar(&importConflict, "conflict", "skip", "conflict resolution: skip or overwrite")
	aliasImportCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "preview import without making changes")

	aliasesCmd.AddCommand(aliasExportCmd)
	aliasesCmd.AddCommand(aliasImportCmd)
}

func runAliasExport(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		return fmt.Errorf("get aliases: %w", err)
	}

	exported := make([]ExportedAlias, len(aliases))
	for i, a := range aliases {
		exported[i] = toExported(a)
	}

	hostname, _ := os.Hostname()
	collection := AliasCollection{
		Meta: CollectionMeta{
			Version:     1,
			Name:        aliasExportName,
			Description: aliasExportDesc,
			Author:      aliasExportAuthor,
			ExportedAt:  time.Now().UTC(),
			Source:      hostname,
			Count:       len(exported),
		},
		Aliases: exported,
	}

	var data []byte
	format := strings.ToLower(aliasExportFmt)
	switch format {
	case "toml":
		data, err = toml.Marshal(collection)
	case "json":
		data, err = json.MarshalIndent(collection, "", "  ")
		if err == nil {
			data = append(data, '\n')
		}
	default:
		return fmt.Errorf("unsupported format %q (use toml or json)", format)
	}
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if aliasExportOutput == "" {
		_, err = os.Stdout.Write(data)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(aliasExportOutput), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(aliasExportOutput, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Exported %d aliases to %s\n", len(exported), aliasExportOutput)
	return nil
}

func runAliasImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var collection AliasCollection
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".toml":
		err = toml.Unmarshal(data, &collection)
	case ".json":
		err = json.Unmarshal(data, &collection)
	default:
		// Try TOML first, fall back to JSON.
		if err2 := toml.Unmarshal(data, &collection); err2 != nil {
			if err3 := json.Unmarshal(data, &collection); err3 != nil {
				return fmt.Errorf("cannot parse %s as TOML or JSON", filePath)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", filePath, err)
	}

	if len(collection.Aliases) == 0 {
		fmt.Fprintln(os.Stderr, "No aliases found in file.")
		return nil
	}

	if importConflict != "skip" && importConflict != "overwrite" {
		return fmt.Errorf("--conflict must be 'skip' or 'overwrite' (got %q)", importConflict)
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	ctx := context.Background()
	var imported, skipped, overwritten int

	for _, ea := range collection.Aliases {
		existing, err := s.GetAlias(ctx, ea.From, ea.Tool, ea.Param, ea.Command, ea.MatchKind)
		if err != nil {
			return fmt.Errorf("check existing alias %q: %w", ea.From, err)
		}

		if existing != nil {
			if importConflict == "skip" {
				skipped++
				if importDryRun {
					fmt.Fprintf(os.Stderr, "  skip (exists): %s\n", ea.From)
				}
				continue
			}
			overwritten++
			if importDryRun {
				fmt.Fprintf(os.Stderr, "  overwrite: %s\n", ea.From)
				continue
			}
		} else {
			imported++
			if importDryRun {
				fmt.Fprintf(os.Stderr, "  import: %s -> %s\n", ea.From, truncateTo(ea.To, 40))
				continue
			}
		}

		if err := s.SetAlias(ctx, toModel(ea)); err != nil {
			return fmt.Errorf("set alias %q: %w", ea.From, err)
		}
	}

	if importDryRun {
		fmt.Fprintf(os.Stderr, "\nDry run: %d would import, %d would skip, %d would overwrite (total: %d)\n",
			imported, skipped, overwritten, len(collection.Aliases))
		return nil
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]int{
			"imported":    imported,
			"skipped":     skipped,
			"overwritten": overwritten,
			"total":       len(collection.Aliases),
		})
	}

	fmt.Fprintf(os.Stderr, "Imported %d, skipped %d, overwritten %d (total: %d)\n",
		imported, skipped, overwritten, len(collection.Aliases))
	return nil
}
