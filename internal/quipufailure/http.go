package quipufailure

import (
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/scbrown/desire-path/internal/source"
)

func shellTool(name string) bool {
	return name == "Bash" || name == "command_execution" || name == "exec_command"
}

func knownMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

// Only the first literal command is observable here. Never execute expansions,
// read --data @files, or attribute a URL printed by echo to an HTTP request.
func firstCommand(command string) ([]string, bool) {
	if len(command) > maxInputBytes {
		return nil, false
	}
	var words []string
	var word strings.Builder
	var quote rune
	started, escaped := false, false
	flush := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	for _, r := range command {
		if escaped {
			if r != '\n' {
				word.WriteRune(r)
				started = true
			}
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if (r == '$' || r == 96) && quote != '\'' {
			return nil, false
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
		} else if strings.ContainsRune("|;&<>\n", r) || (r == '#' && !started) {
			flush()
			break
		} else if unicode.IsSpace(r) {
			flush()
		} else {
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, false
	}
	flush()
	return words, len(words) > 0
}

func curlOperation(command string, supplied json.RawMessage, hosts []string) (string, any, bool) {
	words, ok := firstCommand(command)
	if !ok || path.Base(words[0]) != "curl" {
		return "", nil, false
	}
	method, target := "GET", ""
	var body any
	opaque := false
	for i := 1; i < len(words); i++ {
		arg := words[i]
		flag, value, inline := strings.Cut(arg, "=")
		kind := ""
		switch flag {
		case "-X", "--request":
			kind = "method"
		case "-d", "--data", "--data-raw", "--data-binary", "--json":
			kind = "data"
		case "--url":
			kind = "url"
		case "-H", "--header", "-w", "--write-out", "-o", "--output",
			"-u", "--user", "-A", "--user-agent", "--connect-timeout", "--max-time":
			kind = "skip"
		default:
			// Conservatively refuse unknown options rather than mistaking their
			// header/body/proxy arguments for the target URL.
			if strings.HasPrefix(arg, "--") {
				switch arg {
				case "--silent", "--show-error", "--fail", "--fail-with-body", "--location", "--insecure":
					continue
				default:
					return "", nil, false
				}
			}
			if strings.HasPrefix(arg, "-") {
				if strings.Trim(arg[1:], "sSfLk") == "" {
					continue
				}
				return "", nil, false
			}
			if target != "" {
				return "", nil, false
			} // Multi-request curl is ambiguous.
			target = arg
			continue
		}
		if !inline {
			i++
			if i >= len(words) {
				return "", nil, false
			}
			value = words[i]
		}
		switch kind {
		case "method":
			method = strings.ToUpper(value)
			if !knownMethod(method) {
				return "", nil, false
			}
		case "url":
			if target != "" {
				return "", nil, false
			}
			target = value
		case "data":
			if method == "GET" {
				method = "POST"
			}
			if body != nil || len(value) > maxInputBytes || json.Unmarshal([]byte(value), &body) != nil {
				body, opaque = nil, true
			}
		}
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", nil, false
	}
	matched := false
	for _, host := range hosts {
		matched = matched || (host != "" && strings.EqualFold(u.Hostname(), strings.TrimSpace(host)))
	}
	if !matched {
		return "", nil, false
	}
	op := strings.Trim(u.Path, "/")
	if !opRE.MatchString(op) {
		op = "http"
	}
	if len(supplied) > 0 && len(supplied) <= maxInputBytes {
		if json.Unmarshal(supplied, &body) == nil {
			opaque = false
		}
	}
	if body == nil || opaque || u.RawQuery != "" {
		return op, map[string]any{"method": method, "shell": nil}, true
	}
	// The shell's URL/headers may carry semantics not represented by this body.
	// Keep useful patterns but never claim an exact HTTP retry fingerprint.
	return op, map[string]any{"method": method, "input": body, "shell": nil}, true
}

var statusTrailer = regexp.MustCompile("(?:^|\n)DP_QUIPU_HTTP_STATUS=([0-9]{3})\\s*$")
var curlFailure = regexp.MustCompile("(?:^|\n)curl: \\(([0-9]+)\\)")

func classifyHTTP(f *source.Fields) (string, string) {
	observed := false
	for _, key := range []string{"tool_response", "tool_output", "result", "aggregated_output"} {
		raw := f.Extra[key]
		if len(raw) == 0 || len(raw) > maxResponseBytes {
			continue
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			continue
		}
		status, class := httpResponse(value, 0)
		if status == "failure" {
			return status, class
		}
		observed = observed || status == "success"
	}
	if observed {
		return "success", ""
	}
	return "unknown", ""
}

func httpResponse(value any, depth int) (string, string) {
	if depth > 6 {
		return "unknown", ""
	}
	if s, ok := value.(string); ok {
		if match := statusTrailer.FindStringSubmatch(s); match != nil {
			n, _ := strconv.Atoi(match[1])
			if n >= 400 && n < 600 {
				if n == 408 {
					return "failure", "timeout"
				}
				return "failure", strconv.Itoa(n/100) + "xx"
			}
			if n < 200 || n >= 400 {
				return "unknown", ""
			}
			s = statusTrailer.ReplaceAllString(s, "")
		}
		var decoded any
		if len(s) <= maxResponseBytes && json.Unmarshal([]byte(s), &decoded) == nil {
			return response(decoded, depth+1)
		}
		return "unknown", ""
	}
	m, ok := value.(map[string]any)
	if !ok {
		return "unknown", ""
	}
	for _, key := range []string{"status_code", "http_status"} {
		if n, ok := m[key].(float64); ok && n >= 400 && n < 600 {
			if n == 408 {
				return "failure", "timeout"
			}
			return "failure", strconv.Itoa(int(n)/100) + "xx"
		}
	}
	if stderr, ok := m["stderr"].(string); ok && curlFailure.MatchString(stderr) {
		return "failure", errorClass(stderr, "transport")
	}
	observed := false
	for _, key := range []string{"body", "stdout", "result", "aggregated_output"} {
		if child, ok := m[key]; ok {
			status, class := httpResponse(child, depth+1)
			if status == "failure" {
				return status, class
			}
			observed = observed || status == "success"
		}
	}
	if observed {
		return "success", ""
	}
	return "unknown", ""
}
