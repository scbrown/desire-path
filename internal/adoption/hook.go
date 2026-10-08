package adoption

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

// Hook is the normalized per-tool input, not a Codex turn-complete notification.
// Tool responses and transcripts are deliberately never decoded or retained.
type Hook struct {
	Session string          `json:"session_id"`
	ID      string          `json:"tool_use_id"`
	Event   string          `json:"hook_event_name"`
	Tool    string          `json:"tool_name"`
	CWD     string          `json:"cwd"`
	Input   json.RawMessage `json:"tool_input"`
}

// Parse requires exact invocation context; guessing IDs makes retries consume
// the ten-call budget and makes missing observations look like negative data.
func Parse(raw []byte) (Hook, error) {
	var h Hook
	if len(raw) > 1<<20 || json.Unmarshal(raw, &h) != nil {
		return h, errors.New("malformed or oversized hook")
	}
	if h.Session == "" || h.ID == "" || h.Tool == "" || len(h.Session) > 512 || len(h.ID) > 512 || len(h.Tool) > 128 || len(h.CWD) > 4096 || strings.ContainsRune(h.CWD, '\x00') {
		return Hook{}, errors.New("missing or invalid invocation context")
	}
	return h, nil
}

// Files identifies explicit path arguments to file operations. It is not a
// filesystem tracer: builds and directory-wide searches are not counted as
// reading every file they might access. Dynamic file operations are unknown.
func Files(h Hook) ([]string, bool) {
	if h.Event == "PostToolUseFailure" {
		return nil, false // a failed invocation does not prove the path was opened
	}
	t := strings.ToLower(h.Tool)
	if t == "apply_patch" || t == "applypatch" || strings.HasSuffix(t, ".apply_patch") {
		var patch string
		if json.Unmarshal(h.Input, &patch) != nil {
			var input struct{ Patch, Input string }
			if json.Unmarshal(h.Input, &input) != nil {
				return nil, false
			}
			patch = input.Patch
			if patch == "" {
				patch = input.Input
			}
		}
		var paths []string
		for _, line := range strings.Split(patch, "\n") {
			for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
				if strings.HasPrefix(line, prefix) {
					paths = append(paths, strings.TrimPrefix(line, prefix))
				}
			}
		}
		return paths, len(paths) > 0 && len(paths) <= maxPaths
	}
	if t == "bash" || t == "exec_command" || strings.HasSuffix(t, ".exec_command") {
		var input struct{ Command, Cmd string }
		if json.Unmarshal(h.Input, &input) != nil {
			return nil, false
		}
		command := input.Command
		if command == "" {
			command = input.Cmd
		}
		return shellFiles(command)
	}
	fileTool := t == "read" || t == "edit" || t == "write" || t == "multiedit" ||
		t == "read_file" || t == "open_file" || strings.HasSuffix(t, "__read_chunk") ||
		strings.HasSuffix(t, "__read_file") || strings.HasSuffix(t, ".read_file")
	if !fileTool {
		if strings.Contains(t, "read") || strings.Contains(t, "edit") || strings.Contains(t, "write") || strings.Contains(t, "open") {
			return nil, false
		}
		return nil, true
	}
	var input struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
		File     string `json:"file"`
	}
	if json.Unmarshal(h.Input, &input) != nil {
		return nil, false
	}
	for _, path := range []string{input.FilePath, input.Path, input.File} {
		if path != "" && len(path) <= 4096 {
			return []string{path}, true
		}
	}
	return nil, false
}

func shellFiles(command string) ([]string, bool) {
	words, simple := shellWords(command)
	if len(words) == 0 {
		return nil, simple
	}
	name := filepath.Base(words[0])
	fileCommand := name == "cat" || name == "head" || name == "tail" || name == "sed" || name == "less" || name == "more"
	if !simple {
		return nil, false
	}
	if !fileCommand {
		// Unsupported commands can open files themselves or through children.
		// A narrow literal non-file allowlist avoids scoring these as unused.
		if name != "echo" && name != "printf" && name != "pwd" && name != "true" && name != "false" {
			return nil, false
		}
		for _, word := range words {
			if strings.ContainsAny(word, "$`") {
				return nil, false
			}
		}
		return nil, true
	}
	var paths []string
	skip := false
	expression := name != "sed"
	for _, word := range words[1:] {
		if skip {
			skip = false
			continue
		}
		if strings.HasPrefix(word, "-") {
			if word == "-f" || word == "--file" {
				return nil, false // script-file options need separate path accounting
			}
			if word == "-n" && name != "sed" || word == "-c" || word == "-e" || word == "--expression" || word == "--lines" || word == "--bytes" {
				skip = true
				if name == "sed" && (word == "-e" || word == "--expression") {
					expression = true
				}
			}
			continue
		}
		if !expression {
			expression = true
			continue // the expression is not a file
		}
		if strings.ContainsAny(word, "$`*?[") {
			return nil, false
		}
		paths = append(paths, word)
	}
	return paths, !skip && len(paths) <= maxPaths
}

// shellWords accepts a literal single command, preserving quoted spaces. It
// declines conditionals/redirections instead of pretending every path mentioned
// in an unexecuted branch was opened (the echo-path/failed-pipeline false pass).
func shellWords(command string) ([]string, bool) {
	var words []string
	var word strings.Builder
	var quote rune
	escape, started := false, false
	for _, ch := range command {
		if escape {
			word.WriteRune(ch)
			escape, started = false, true
			continue
		}
		if ch == '\\' && quote != '\'' {
			escape = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				word.WriteRune(ch)
			}
			continue
		}
		switch ch {
		case '#':
			if !started {
				return words, true
			}
			word.WriteRune(ch)
			started = true
		case '\'', '"':
			quote, started = ch, true
		case ';', '|', '&', '>', '<', '\n', '\r':
			return wordsWithTail(words, word.String(), started), false
		case ' ', '\t':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(ch)
			started = true
		}
	}
	return wordsWithTail(words, word.String(), started), quote == 0 && !escape
}

func wordsWithTail(words []string, tail string, started bool) []string {
	if started {
		return append(words, tail)
	}
	return words
}

// WorkingDirectory follows an exec command's explicit directory instead of
// matching a relative path against the session's unrelated checkout.
func WorkingDirectory(h Hook) (string, bool) {
	t := strings.ToLower(h.Tool)
	if t != "bash" && t != "exec_command" && !strings.HasSuffix(t, ".exec_command") {
		return h.CWD, true
	}
	var input struct {
		Workdir string `json:"workdir"`
	}
	if json.Unmarshal(h.Input, &input) != nil {
		return "", false
	}
	if input.Workdir == "" {
		return h.CWD, true
	}
	if len(input.Workdir) > 4096 || strings.ContainsRune(input.Workdir, '\x00') {
		return "", false
	}
	if filepath.IsAbs(input.Workdir) {
		return input.Workdir, true
	}
	if !filepath.IsAbs(h.CWD) {
		return "", false
	}
	return filepath.Join(h.CWD, input.Workdir), true
}
