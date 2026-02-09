package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

// testRemote sets up a SQLite store, wraps it in an HTTP server, and returns
// a RemoteStore pointing at it. This tests the full round-trip.
func testRemote(t *testing.T) *RemoteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	local, err := New(dbPath)
	if err != nil {
		t.Fatalf("open local store: %v", err)
	}
	t.Cleanup(func() { local.Close() })

	mux := http.NewServeMux()
	registerRoutes(mux, local)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return NewRemote(ts.URL)
}

// registerRoutes sets up the same HTTP routes that server.Server uses,
// duplicated here to avoid a circular import.
func registerRoutes(mux *http.ServeMux, s Store) {
	mux.HandleFunc("POST /api/v1/desires", func(w http.ResponseWriter, r *http.Request) {
		var d model.Desire
		json.NewDecoder(r.Body).Decode(&d)
		if err := s.RecordDesire(r.Context(), d); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("GET /api/v1/desires", func(w http.ResponseWriter, r *http.Request) {
		desires, err := s.ListDesires(r.Context(), ListOpts{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if desires == nil {
			desires = []model.Desire{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(desires)
	})
	mux.HandleFunc("GET /api/v1/paths", func(w http.ResponseWriter, r *http.Request) {
		paths, err := s.GetPaths(r.Context(), PathOpts{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if paths == nil {
			paths = []model.Path{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paths)
	})
	mux.HandleFunc("POST /api/v1/aliases", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if err := s.SetAlias(r.Context(), req.From, req.To); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(req)
	})
	mux.HandleFunc("GET /api/v1/aliases", func(w http.ResponseWriter, r *http.Request) {
		aliases, err := s.GetAliases(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if aliases == nil {
			aliases = []model.Alias{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aliases)
	})
	mux.HandleFunc("DELETE /api/v1/aliases/{from}", func(w http.ResponseWriter, r *http.Request) {
		from := r.PathValue("from")
		deleted, err := s.DeleteAlias(r.Context(), from)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !deleted {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
	})
	mux.HandleFunc("GET /api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		stats, err := s.Stats(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})
	mux.HandleFunc("GET /api/v1/inspect", func(w http.ResponseWriter, r *http.Request) {
		pattern := r.URL.Query().Get("pattern")
		if pattern == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "pattern required"})
			return
		}
		result, err := s.InspectPath(r.Context(), InspectOpts{Pattern: pattern})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("POST /api/v1/invocations", func(w http.ResponseWriter, r *http.Request) {
		var inv model.Invocation
		json.NewDecoder(r.Body).Decode(&inv)
		if err := s.RecordInvocation(r.Context(), inv); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(inv)
	})
	mux.HandleFunc("GET /api/v1/invocations", func(w http.ResponseWriter, r *http.Request) {
		invocations, err := s.ListInvocations(r.Context(), InvocationOpts{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if invocations == nil {
			invocations = []model.Invocation{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invocations)
	})
	mux.HandleFunc("GET /api/v1/invocations/stats", func(w http.ResponseWriter, r *http.Request) {
		stats, err := s.InvocationStats(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})
}

func TestRemoteRecordAndListDesires(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	d := model.Desire{
		ID:        "rd-1",
		ToolName:  "read_file",
		Error:     "unknown tool",
		Source:    "test",
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}
	if err := remote.RecordDesire(ctx, d); err != nil {
		t.Fatalf("record desire: %v", err)
	}

	desires, err := remote.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("got %d desires, want 1", len(desires))
	}
	if desires[0].ToolName != "read_file" {
		t.Errorf("tool_name = %q, want read_file", desires[0].ToolName)
	}
}

func TestRemoteRecordAndListInvocations(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	inv := model.Invocation{
		ID:        "ri-1",
		Source:    "test",
		ToolName:  "Read",
		IsError:   false,
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}
	if err := remote.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("record invocation: %v", err)
	}

	invocations, err := remote.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("got %d invocations, want 1", len(invocations))
	}
	if invocations[0].ToolName != "Read" {
		t.Errorf("tool_name = %q, want Read", invocations[0].ToolName)
	}
}

func TestRemoteAliases(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	// Set.
	if err := remote.SetAlias(ctx, "read_file", "Read"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	// List.
	aliases, err := remote.GetAliases(ctx)
	if err != nil {
		t.Fatalf("get aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("got %d aliases, want 1", len(aliases))
	}
	if aliases[0].From != "read_file" || aliases[0].To != "Read" {
		t.Errorf("alias = %+v, want read_file → Read", aliases[0])
	}

	// Delete.
	deleted, err := remote.DeleteAlias(ctx, "read_file")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}

	// Delete nonexistent.
	deleted, err = remote.DeleteAlias(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("delete nonexistent: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for nonexistent")
	}
}

func TestRemoteStats(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	stats, err := remote.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalDesires != 0 {
		t.Errorf("total = %d, want 0", stats.TotalDesires)
	}
}

func TestRemoteInvocationStats(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	stats, err := remote.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("invocation stats: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("total = %d, want 0", stats.Total)
	}
}

func TestRemotePaths(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	paths, err := remote.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("get paths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("got %d paths, want 0", len(paths))
	}
}

func TestRemoteInspectPath(t *testing.T) {
	remote := testRemote(t)
	ctx := context.Background()

	result, err := remote.InspectPath(ctx, InspectOpts{Pattern: "read_file"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
}

func TestRemoteClose(t *testing.T) {
	remote := testRemote(t)
	if err := remote.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestRemoteConnectionError(t *testing.T) {
	// Point at a non-existent server.
	remote := NewRemote("http://127.0.0.1:1")
	ctx := context.Background()

	_, err := remote.ListDesires(ctx, ListOpts{})
	if err == nil {
		t.Fatal("expected error connecting to non-existent server")
	}
}
