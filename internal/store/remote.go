package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

// RemoteStore implements Store by forwarding requests over HTTP to a dp serve instance.
type RemoteStore struct {
	baseURL string
	client  *http.Client
}

// NewRemote creates a RemoteStore pointing at the given base URL (e.g., "http://localhost:7273").
func NewRemote(baseURL string) *RemoteStore {
	return &RemoteStore{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *RemoteStore) RecordDesire(ctx context.Context, d model.Desire) error {
	return r.postJSON(ctx, "/api/v1/desires", d, nil)
}

func (r *RemoteStore) ListDesires(ctx context.Context, opts ListOpts) ([]model.Desire, error) {
	q := url.Values{}
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	if opts.Source != "" {
		q.Set("source", opts.Source)
	}
	if opts.ToolName != "" {
		q.Set("tool", opts.ToolName)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	var desires []model.Desire
	if err := r.getJSON(ctx, "/api/v1/desires", q, &desires); err != nil {
		return nil, err
	}
	return desires, nil
}

func (r *RemoteStore) GetPaths(ctx context.Context, opts PathOpts) ([]model.Path, error) {
	q := url.Values{}
	if opts.Top > 0 {
		q.Set("top", strconv.Itoa(opts.Top))
	}
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	var paths []model.Path
	if err := r.getJSON(ctx, "/api/v1/paths", q, &paths); err != nil {
		return nil, err
	}
	return paths, nil
}

func (r *RemoteStore) SetAlias(ctx context.Context, from, to string) error {
	body := map[string]string{"from": from, "to": to}
	return r.postJSON(ctx, "/api/v1/aliases", body, nil)
}

func (r *RemoteStore) GetAlias(ctx context.Context, from string) (*model.Alias, error) {
	var alias model.Alias
	if err := r.getJSON(ctx, "/api/v1/aliases/"+url.PathEscape(from), nil, &alias); err != nil {
		return nil, err
	}
	if alias.From == "" {
		return nil, nil
	}
	return &alias, nil
}

func (r *RemoteStore) GetAliases(ctx context.Context) ([]model.Alias, error) {
	var aliases []model.Alias
	if err := r.getJSON(ctx, "/api/v1/aliases", nil, &aliases); err != nil {
		return nil, err
	}
	return aliases, nil
}

func (r *RemoteStore) DeleteAlias(ctx context.Context, from string) (bool, error) {
	u := r.baseURL + "/api/v1/aliases/" + url.PathEscape(from)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("remote delete alias: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, remoteError(resp)
	}
	return true, nil
}

func (r *RemoteStore) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	if err := r.getJSON(ctx, "/api/v1/stats", nil, &stats); err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *RemoteStore) InspectPath(ctx context.Context, opts InspectOpts) (*InspectResult, error) {
	q := url.Values{}
	q.Set("pattern", opts.Pattern)
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	if opts.TopN > 0 {
		q.Set("top", strconv.Itoa(opts.TopN))
	}
	var result InspectResult
	if err := r.getJSON(ctx, "/api/v1/inspect", q, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *RemoteStore) RecordInvocation(ctx context.Context, inv model.Invocation) error {
	return r.postJSON(ctx, "/api/v1/invocations", inv, nil)
}

func (r *RemoteStore) ListInvocations(ctx context.Context, opts InvocationOpts) ([]model.Invocation, error) {
	q := url.Values{}
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}
	if opts.Source != "" {
		q.Set("source", opts.Source)
	}
	if opts.InstanceID != "" {
		q.Set("instance_id", opts.InstanceID)
	}
	if opts.ToolName != "" {
		q.Set("tool", opts.ToolName)
	}
	if opts.ErrorsOnly {
		q.Set("errors_only", "true")
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	var invocations []model.Invocation
	if err := r.getJSON(ctx, "/api/v1/invocations", q, &invocations); err != nil {
		return nil, err
	}
	return invocations, nil
}

func (r *RemoteStore) InvocationStats(ctx context.Context) (InvocationStatsResult, error) {
	var stats InvocationStatsResult
	if err := r.getJSON(ctx, "/api/v1/invocations/stats", nil, &stats); err != nil {
		return stats, err
	}
	return stats, nil
}

// Close is a no-op for the remote store.
func (r *RemoteStore) Close() error {
	return nil
}

// getJSON performs a GET request and decodes the JSON response into dst.
func (r *RemoteStore) getJSON(ctx context.Context, path string, query url.Values, dst any) error {
	u := r.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("remote request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return remoteError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// postJSON performs a POST request with a JSON body and optionally decodes the response.
func (r *RemoteStore) postJSON(ctx context.Context, path string, body any, dst any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	u := r.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("remote request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return remoteError(resp)
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// remoteError reads an error response from the server and returns it as an error.
func remoteError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("remote store (%d): %s", resp.StatusCode, errResp.Error)
	}
	return fmt.Errorf("remote store (%d): %s", resp.StatusCode, http.StatusText(resp.StatusCode))
}
