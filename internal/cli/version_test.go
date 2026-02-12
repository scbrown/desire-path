package cli

import (
	"testing"
)

func TestShortCommit(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"48cae1d7a3b2c1d0e9f8a7b6c5d4e3f2a1b0c9d8", "48cae1d"},
		{"48cae1d", "48cae1d"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := shortCommit(tt.input); got != tt.want {
			t.Errorf("shortCommit(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	// Verify the command is registered and has expected properties.
	cmd, _, err := rootCmd.Find([]string{"version"})
	if err != nil {
		t.Fatalf("version command not found: %v", err)
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %q, want %q", cmd.Use, "version")
	}
	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}
}

func TestVersionOutput(t *testing.T) {
	// Save and restore package-level vars.
	origVersion, origCommit := Version, Commit
	defer func() {
		Version, Commit = origVersion, origCommit
	}()

	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{
			name:    "release build",
			version: "v0.2.0",
			commit:  "48cae1d7a3b2c1d0e9f8a7b6c5d4e3f2a1b0c9d8",
			want:    "dp v0.2.0 (48cae1d)\n",
		},
		{
			name:    "dev build with commit",
			version: "",
			commit:  "abcdef1234567890",
			want:    "dp dev (abcdef1)\n",
		},
		{
			name:    "short commit passthrough",
			version: "v1.0.0",
			commit:  "abc1234",
			want:    "dp v1.0.0 (abc1234)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			Commit = tt.commit

			got := captureStdout(t, func() {
				versionCmd.Run(versionCmd, nil)
			})

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
