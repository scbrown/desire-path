// Package pave carries a correction from the failure that produced it to the
// next moment the agent can actually be told about it.
//
// WHY THIS EXISTS AT ALL. `dp pave-correct` runs on PostToolUseFailure, which is
// the obvious place for it: a correction is worth having exactly when a command
// has just failed. But additionalContext emitted from that event never reaches
// the model on the Claude Code harness — measured with two independent hooks,
// each proven to have RUN by its own side effect while its text was discarded,
// against a PostToolUse control that surfaced the identical text (aegis-c2g1s5).
//
// So the correction is stored here and delivered on the agent's next PreToolUse,
// where injection is proven to reach the model. One turn of latency, and no new
// channel to maintain. The alternative — arguing with the harness — leaves both
// of the fleet's corrective mechanisms mute.
package pave

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTTL bounds how long a pending correction stays deliverable.
//
// A correction is advice about the command that just failed, and it stops being
// that quickly: delivered half an hour later it is noise attached to unrelated
// work, and noise on every tool call is how a channel gets ignored. Sessions
// also outlive the failures inside them, so without a TTL one stale correction
// would sit waiting for a call that comes tomorrow.
const DefaultTTL = 10 * time.Minute

// Pending is one correction waiting for a moment it can be delivered in.
type Pending struct {
	SessionID string    `json:"session_id"`
	Command   string    `json:"command,omitempty"`
	Context   string    `json:"context"`
	Written   time.Time `json:"written"`
}

// Dir is where pending corrections live. Per-user, not per-repo: the session is
// the thing being carried across, and an agent changes directory mid-session.
func Dir() string {
	if d := os.Getenv("DP_PAVE_PENDING_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "desire-path-pave-pending")
}

// Store publishes a correction for sessionID, atomically, replacing any earlier
// one. Replacing is deliberate: if two commands failed before the agent's next
// call, the newer correction is the one about what it is currently doing.
func Store(dir, sessionID, command, context string) error {
	if dir == "" || sessionID == "" || context == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(Pending{SessionID: sessionID, Command: command, Context: context, Written: time.Now().UTC()})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, pathFor(dir, sessionID))
}

// Take returns the pending correction for sessionID and REMOVES it, whether or
// not it was still fresh. Delivering once is the contract — a correction
// repeated on every subsequent call is nagging, not help — and a stale one is
// dropped here rather than left to be reconsidered forever.
func Take(dir, sessionID string, ttl time.Duration) (Pending, bool) {
	if dir == "" || sessionID == "" {
		return Pending{}, false
	}
	path := pathFor(dir, sessionID)
	b, err := os.ReadFile(path)
	if err != nil {
		return Pending{}, false
	}
	_ = os.Remove(path)
	var p Pending
	if json.Unmarshal(b, &p) != nil {
		return Pending{}, false
	}
	if ttl > 0 && time.Since(p.Written) > ttl {
		return Pending{}, false
	}
	return p, true
}

// Render is the text injected on the next call. It names the command the
// correction is ABOUT, because by the time the agent reads this it has moved on
// and an unattributed correction reads as a comment on whatever it is doing now.
func (p Pending) Render() string {
	if p.Command == "" {
		return p.Context
	}
	return fmt.Sprintf("Correction for your previous failed command `%s`:\n%s", p.Command, p.Context)
}

func pathFor(dir, sessionID string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, sessionID)
	return filepath.Join(dir, safe+".json")
}
