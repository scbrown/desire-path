// Package adoption records prospective, session-scoped payload path use.
package adoption

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxPaths = 16

// Reference preserves repository identity that a bare relative path cannot carry.
type Reference struct {
	Repo string `json:"repo,omitempty"`
	Path string `json:"path"`
}

// Resolver binds index repository names to local checkouts. Missing bindings are
// unknown, never evidence that an offered file belongs to the caller's project.
type Resolver struct {
	Roots map[string]string
	Scope string
}

// References canonicalizes offered targets without reading their contents.
func (r Resolver) References(refs []Reference, cwd string) ([]string, bool) {
	if len(refs) == 0 || len(refs) > maxPaths {
		return nil, false
	}
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := ref.Repo
		if name == "" {
			name = r.Scope
		}
		path := ref.Path
		if len(name) > 128 {
			return nil, false
		}
		if !filepath.IsAbs(path) {
			root := r.Roots[name]
			if name == "" || root == "" || !filepath.IsAbs(root) {
				return nil, false
			}
			if strings.HasPrefix(path, "repos/"+name+"/") {
				path = strings.TrimPrefix(path, "repos/"+name+"/")
			}
			clean := filepath.Clean(filepath.FromSlash(path))
			if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return nil, false
			}
			path = filepath.Join(root, clean)
		}
		key, ok := r.File(path, cwd)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
	}
	return keys, true
}

// File gives independent clones and linked worktrees the same logical identity.
// It never persists origins or paths; the store hashes this in-memory key.
func (r Resolver) File(path, cwd string) (string, bool) {
	if path == "" || len(path) > 4096 || strings.ContainsRune(path, '\x00') {
		return "", false
	}
	if strings.HasPrefix(filepath.ToSlash(path), "repos/") {
		parts := strings.SplitN(filepath.ToSlash(path), "/", 3)
		if len(parts) != 3 || !filepath.IsAbs(r.Roots[parts[1]]) {
			return "", false
		}
		clean := filepath.Clean(filepath.FromSlash(parts[2]))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", false
		}
		return r.File(filepath.Join(r.Roots[parts[1]], clean), "")
	}
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(cwd) {
			return "", false
		}
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	if len(path) > 4096 {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	for dir, depth := filepath.Dir(path), 0; depth < 64; depth++ {
		marker := filepath.Join(dir, ".git")
		if info, err := os.Stat(marker); err == nil {
			config, err := gitConfig(marker, info.IsDir())
			if err != nil {
				return "", false
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			out, err := exec.CommandContext(ctx, "git", "config", "--file", config,
				"--get", "remote.origin.url").Output()
			cancel()
			origin, ok := originIdentity(strings.TrimSpace(string(out)))
			if err != nil || !ok {
				return "", false
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return "", false
			}
			return "repo:" + origin + ":" + filepath.ToSlash(rel), true
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "file:" + filepath.ToSlash(path), true
}

func gitConfig(marker string, directory bool) (string, error) {
	gitdir := marker
	if !directory {
		b, err := smallFile(marker)
		if err != nil || !strings.HasPrefix(string(b), "gitdir: ") {
			return "", errors.New("invalid git marker")
		}
		gitdir = strings.TrimSpace(strings.TrimPrefix(string(b), "gitdir: "))
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(filepath.Dir(marker), gitdir)
		}
	}
	if b, err := smallFile(filepath.Join(gitdir, "commondir")); err == nil {
		common := strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitdir, common)
		}
		gitdir = common
	}
	return filepath.Join(gitdir, "config"), nil
}

func smallFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 4096 {
		return nil, errors.New("unreadable or oversized marker")
	}
	return os.ReadFile(path)
}

// Origins contain only host/owner/repository: credentials, query strings and
// transport spelling do not affect identity or escape into observation data.
func originIdentity(raw string) (string, bool) {
	if raw == "" || len(raw) > 4096 {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		if before, after, ok := strings.Cut(raw, ":"); ok && strings.Contains(before, "@") {
			raw = "ssh://" + before + "/" + after
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Path == "" {
		return "", false
	}
	return strings.ToLower(u.Hostname()) + "/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), true
}

// LoadRoots reads only the explicitly configured repository map.
func LoadRoots(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 65536 {
		return nil, errors.New("invalid repository map")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("repository map unavailable")
	}
	var roots map[string]string
	if json.Unmarshal(b, &roots) != nil || len(roots) > 256 {
		return nil, errors.New("invalid repository map")
	}
	for name, root := range roots {
		if name == "" || len(name) > 128 || len(root) > 4096 || !filepath.IsAbs(root) {
			return nil, errors.New("invalid repository binding")
		}
	}
	return roots, nil
}
