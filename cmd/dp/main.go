package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "dp",
		Short: "Desire Path - track and analyze failed AI tool calls",
		Long: `dp collects, analyzes, and surfaces patterns from failed AI tool calls.
Failed tool calls are signals that reveal capabilities the AI expects to exist.
By tracking these "desires", developers can implement features or aliases so
future similar attempts succeed.`,
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
