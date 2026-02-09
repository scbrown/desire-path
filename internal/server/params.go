package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/scbrown/desire-path/internal/store"
)

// parseSince extracts a "since" query parameter as a time.Time.
// Accepts RFC3339 timestamps or duration shorthand (e.g., "24h", "7d").
func parseSince(r *http.Request) (time.Time, error) {
	s := r.URL.Query().Get("since")
	if s == "" {
		return time.Time{}, nil
	}
	// Try RFC3339 first.
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Try duration shorthand: "7d", "24h", etc.
	if len(s) > 1 {
		numStr := s[:len(s)-1]
		unit := s[len(s)-1]
		if n, err := strconv.Atoi(numStr); err == nil {
			switch unit {
			case 'h':
				return time.Now().UTC().Add(-time.Duration(n) * time.Hour), nil
			case 'd':
				return time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("invalid since value %q: expected RFC3339 timestamp or duration (e.g., 24h, 7d)", s)
}

func parseInt(r *http.Request, key string) (int, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", key, s, err)
	}
	return n, nil
}

func parseBool(r *http.Request, key string) bool {
	s := r.URL.Query().Get(key)
	return s == "true" || s == "1"
}

func parseListOpts(r *http.Request) (store.ListOpts, error) {
	since, err := parseSince(r)
	if err != nil {
		return store.ListOpts{}, err
	}
	limit, err := parseInt(r, "limit")
	if err != nil {
		return store.ListOpts{}, err
	}
	return store.ListOpts{
		Since:    since,
		Source:   r.URL.Query().Get("source"),
		ToolName: r.URL.Query().Get("tool"),
		Limit:    limit,
	}, nil
}

func parsePathOpts(r *http.Request) (store.PathOpts, error) {
	since, err := parseSince(r)
	if err != nil {
		return store.PathOpts{}, err
	}
	top, err := parseInt(r, "top")
	if err != nil {
		return store.PathOpts{}, err
	}
	return store.PathOpts{
		Top:   top,
		Since: since,
	}, nil
}

func parseInspectOpts(r *http.Request) (store.InspectOpts, error) {
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		return store.InspectOpts{}, fmt.Errorf("pattern query parameter is required")
	}
	since, err := parseSince(r)
	if err != nil {
		return store.InspectOpts{}, err
	}
	topN, err := parseInt(r, "top")
	if err != nil {
		return store.InspectOpts{}, err
	}
	return store.InspectOpts{
		Pattern: pattern,
		Since:   since,
		TopN:    topN,
	}, nil
}

func parseInvocationOpts(r *http.Request) (store.InvocationOpts, error) {
	since, err := parseSince(r)
	if err != nil {
		return store.InvocationOpts{}, err
	}
	limit, err := parseInt(r, "limit")
	if err != nil {
		return store.InvocationOpts{}, err
	}
	return store.InvocationOpts{
		Since:      since,
		Source:     r.URL.Query().Get("source"),
		InstanceID: r.URL.Query().Get("instance_id"),
		ToolName:   r.URL.Query().Get("tool"),
		ErrorsOnly: parseBool(r, "errors_only"),
		Limit:      limit,
	}, nil
}
