package analyze

import (
	"encoding/json"
	"testing"

	"github.com/scbrown/desire-path/internal/model"
)

func TestCategorizeDesire_EnvNeed(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		errMsg   string
		wantCat  string
	}{
		{
			name:    "command not found",
			tool:    "Bash",
			errMsg:  "bash: cargo-insta: command not found",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "not found variant",
			tool:    "Bash",
			errMsg:  "/bin/sh: cargo-nextest: not found",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "exit code 127",
			tool:    "Bash",
			errMsg:  "exit code 127",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "exit status 127",
			tool:    "Bash",
			errMsg:  "exit status 127",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "exit 127",
			tool:    "Bash",
			errMsg:  "exit 127",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "not found in PATH",
			tool:    "Bash",
			errMsg:  "cargo-insta: not found in PATH",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "not installed",
			tool:    "Bash",
			errMsg:  "cargo-nextest is not installed",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "non-Bash tool ignored",
			tool:    "Read",
			errMsg:  "command not found",
			wantCat: "",
		},
		{
			name:    "generic Bash error not categorized",
			tool:    "Bash",
			errMsg:  "permission denied",
			wantCat: "",
		},
		{
			name:    "empty error",
			tool:    "Bash",
			errMsg:  "",
			wantCat: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeDesire(tt.tool, tt.errMsg, nil)
			if got != tt.wantCat {
				t.Errorf("CategorizeDesire(%q, %q) = %q, want %q", tt.tool, tt.errMsg, got, tt.wantCat)
			}
		})
	}
}

func TestEnvNeedCommand(t *testing.T) {
	tests := []struct {
		name      string
		errMsg    string
		toolInput json.RawMessage
		wantCmd   string
	}{
		{
			name:    "bash colon format",
			errMsg:  "bash: cargo-insta: command not found",
			wantCmd: "cargo-insta",
		},
		{
			name:    "sh colon format",
			errMsg:  "/bin/sh: cargo-nextest: not found",
			wantCmd: "cargo-nextest",
		},
		{
			name:    "command not found colon",
			errMsg:  "command not found: rg",
			wantCmd: "rg",
		},
		{
			name:      "from tool_input when error unclear",
			errMsg:    "exit code 127",
			toolInput: json.RawMessage(`{"command":"cargo-insta test"}`),
			wantCmd:   "cargo-insta",
		},
		{
			name:      "from tool_input with env vars",
			errMsg:    "exit code 127",
			toolInput: json.RawMessage(`{"command":"RUST_BACKTRACE=1 cargo-nextest run"}`),
			wantCmd:   "cargo-nextest",
		},
		{
			name:    "no command extractable",
			errMsg:  "some other error",
			wantCmd: "",
		},
		{
			name:      "empty input",
			errMsg:    "",
			toolInput: nil,
			wantCmd:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnvNeedCommand(tt.errMsg, tt.toolInput)
			if got != tt.wantCmd {
				t.Errorf("EnvNeedCommand(%q, %s) = %q, want %q", tt.errMsg, tt.toolInput, got, tt.wantCmd)
			}
		})
	}
}
