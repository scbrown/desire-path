// Package server provides an HTTP server that wraps the store.Store interface,
// enabling remote access to desire-path data over HTTP.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/scbrown/desire-path/internal/ingest"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// Server wraps a store.Store and exposes it over HTTP.
type Server struct {
	store store.Store
	mux   *http.ServeMux
	srv   *http.Server
}

// New creates a Server that delegates to the given store.
func New(s store.Store) *Server {
	srv := &Server{store: s, mux: http.NewServeMux()}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/v1/ingest", s.handleIngest)
	s.mux.HandleFunc("POST /api/v1/desires", s.handleRecordDesire)
	s.mux.HandleFunc("GET /api/v1/desires", s.handleListDesires)
	s.mux.HandleFunc("GET /api/v1/paths", s.handleGetPaths)
	s.mux.HandleFunc("POST /api/v1/aliases", s.handleSetAlias)
	s.mux.HandleFunc("GET /api/v1/aliases", s.handleGetAliases)
	s.mux.HandleFunc("GET /api/v1/aliases/rules", s.handleGetRulesForTool)
	s.mux.HandleFunc("GET /api/v1/aliases/{from}", s.handleGetAlias)
	s.mux.HandleFunc("DELETE /api/v1/aliases/{from}", s.handleDeleteAlias)
	s.mux.HandleFunc("GET /api/v1/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/v1/inspect", s.handleInspectPath)
	s.mux.HandleFunc("POST /api/v1/invocations", s.handleRecordInvocation)
	s.mux.HandleFunc("GET /api/v1/invocations", s.handleListInvocations)
	s.mux.HandleFunc("GET /api/v1/invocations/stats", s.handleInvocationStats)
	s.mux.HandleFunc("GET /api/v1/turns", s.handleGetTurns)
	s.mux.HandleFunc("GET /api/v1/turns/stats", s.handleGetTurnStats)
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return s.srv.ListenAndServe()
}

// Serve accepts connections on the given listener.
func (s *Server) Serve(ln net.Listener) error {
	s.srv = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return s.srv.Serve(ln)
}

// Handler returns the HTTP handler for use with httptest.Server or custom listeners.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	sourceName := r.URL.Query().Get("source")
	if sourceName == "" {
		writeErr(w, http.StatusBadRequest, "source query parameter is required")
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "reading request body: %v", err)
		return
	}
	inv, err := ingest.Ingest(r.Context(), s.store, raw, sourceName)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "ingest: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (s *Server) handleRecordDesire(w http.ResponseWriter, r *http.Request) {
	var d model.Desire
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if err := s.store.RecordDesire(r.Context(), d); err != nil {
		writeErr(w, http.StatusInternalServerError, "recording desire: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleListDesires(w http.ResponseWriter, r *http.Request) {
	opts, err := parseListOpts(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	desires, err := s.store.ListDesires(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "listing desires: %v", err)
		return
	}
	if desires == nil {
		desires = []model.Desire{}
	}
	writeJSON(w, http.StatusOK, desires)
}

func (s *Server) handleGetPaths(w http.ResponseWriter, r *http.Request) {
	opts, err := parsePathOpts(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	paths, err := s.store.GetPaths(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting paths: %v", err)
		return
	}
	if paths == nil {
		paths = []model.Path{}
	}
	writeJSON(w, http.StatusOK, paths)
}

func (s *Server) handleSetAlias(w http.ResponseWriter, r *http.Request) {
	var alias model.Alias
	if err := json.NewDecoder(r.Body).Decode(&alias); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if alias.From == "" || alias.To == "" {
		writeErr(w, http.StatusBadRequest, "both 'from' and 'to' fields are required")
		return
	}
	if err := s.store.SetAlias(r.Context(), alias); err != nil {
		writeErr(w, http.StatusInternalServerError, "setting alias: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, alias)
}

func (s *Server) handleGetAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.store.GetAliases(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting aliases: %v", err)
		return
	}
	if aliases == nil {
		aliases = []model.Alias{}
	}
	writeJSON(w, http.StatusOK, aliases)
}

func (s *Server) handleGetAlias(w http.ResponseWriter, r *http.Request) {
	from := r.PathValue("from")
	if from == "" {
		writeErr(w, http.StatusBadRequest, "alias name is required")
		return
	}
	tool := r.URL.Query().Get("tool")
	param := r.URL.Query().Get("param")
	command := r.URL.Query().Get("command")
	matchKind := r.URL.Query().Get("match_kind")
	alias, err := s.store.GetAlias(r.Context(), from, tool, param, command, matchKind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting alias: %v", err)
		return
	}
	if alias == nil {
		writeErr(w, http.StatusNotFound, "alias %q not found", from)
		return
	}
	writeJSON(w, http.StatusOK, alias)
}

func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request) {
	from := r.PathValue("from")
	if from == "" {
		writeErr(w, http.StatusBadRequest, "alias name is required")
		return
	}
	tool := r.URL.Query().Get("tool")
	param := r.URL.Query().Get("param")
	command := r.URL.Query().Get("command")
	matchKind := r.URL.Query().Get("match_kind")
	deleted, err := s.store.DeleteAlias(r.Context(), from, tool, param, command, matchKind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "deleting alias: %v", err)
		return
	}
	if !deleted {
		writeErr(w, http.StatusNotFound, "alias %q not found", from)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleGetRulesForTool(w http.ResponseWriter, r *http.Request) {
	tool := r.URL.Query().Get("tool")
	if tool == "" {
		writeErr(w, http.StatusBadRequest, "tool query parameter is required")
		return
	}
	rules, err := s.store.GetRulesForTool(r.Context(), tool)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting rules: %v", err)
		return
	}
	if rules == nil {
		rules = []model.Alias{}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting stats: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleInspectPath(w http.ResponseWriter, r *http.Request) {
	opts, err := parseInspectOpts(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	result, err := s.store.InspectPath(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "inspecting path: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRecordInvocation(w http.ResponseWriter, r *http.Request) {
	var inv model.Invocation
	if err := json.NewDecoder(r.Body).Decode(&inv); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if err := s.store.RecordInvocation(r.Context(), inv); err != nil {
		writeErr(w, http.StatusInternalServerError, "recording invocation: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (s *Server) handleListInvocations(w http.ResponseWriter, r *http.Request) {
	opts, err := parseInvocationOpts(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	invocations, err := s.store.ListInvocations(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "listing invocations: %v", err)
		return
	}
	if invocations == nil {
		invocations = []model.Invocation{}
	}
	writeJSON(w, http.StatusOK, invocations)
}

func (s *Server) handleInvocationStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.InvocationStats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting invocation stats: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleGetTurns(w http.ResponseWriter, r *http.Request) {
	opts, err := parseTurnOpts(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	turns, err := s.store.GetTurns(r.Context(), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting turns: %v", err)
		return
	}
	if turns == nil {
		turns = []store.TurnRow{}
	}
	writeJSON(w, http.StatusOK, turns)
}

func (s *Server) handleGetTurnStats(w http.ResponseWriter, r *http.Request) {
	threshold, err := parseInt(r, "threshold")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	if threshold <= 0 {
		threshold = 5
	}
	since, err := parseSince(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	stats, err := s.store.GetPathTurnStats(r.Context(), threshold, since)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "getting turn stats: %v", err)
		return
	}
	if stats == nil {
		stats = []store.ToolTurnStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// writeErr writes a JSON error response.
func writeErr(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	writeJSON(w, status, map[string]string{"error": msg})
}
