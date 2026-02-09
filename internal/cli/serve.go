package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/scbrown/desire-path/internal/server"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var serveAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an HTTP server exposing the desire-path store",
	Long: `Start an HTTP server that wraps the local SQLite store and exposes
it over HTTP. This enables remote clients to read and write desire-path data
without direct database access.

The server provides a RESTful JSON API at /api/v1/ with endpoints for desires,
invocations, paths, aliases, stats, and inspection. A health check is available
at /api/v1/health.

Use dp config to set store_mode=remote and remote_url to point other dp instances
at this server instead of a local database.`,
	Example: `  # Start server on default port
  dp serve

  # Start on a custom address
  dp serve --addr :9090

  # Start with a specific database
  dp serve --db /path/to/desires.db --addr localhost:7273`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		srv := server.New(s)

		// Listen first so we can report the actual address.
		ln, err := net.Listen("tcp", serveAddr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", serveAddr, err)
		}

		fmt.Fprintf(os.Stderr, "dp serve listening on %s\n", ln.Addr())

		// Graceful shutdown on SIGINT/SIGTERM.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Serve(ln)
		}()

		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "shutting down...")
			return srv.Shutdown(context.Background())
		case err := <-errCh:
			return err
		}
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", ":7273", "address to listen on (host:port)")
	rootCmd.AddCommand(serveCmd)
}
