package signpost

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Intent families. A family is one observed way of asking a question with a
// shell command, paired with the stack command that answers it directly.
//
// v1 knew exactly one family: a literal search with grep or rg. The families
// below were added because the question an agent is asking is legible from more
// than that one verb — `find -name` is a file lookup, `git log -S` is a history
// search, and a bare identifier handed to grep is a structural question about a
// symbol. Each family is a parser plus a predicate plus its own evaluation row;
// none of them changes how another family behaves.
const (
	FamilyLiteral  = "literal-search"
	FamilyFileFind = "file-find"
	FamilyHistory  = "history-search"
	FamilySymbol   = "symbol-search"
)

// Intent is the discovered purpose behind one observed shell command.
//
// SymbolCandidate is set when the literal query is a bare identifier: whether
// that becomes FamilySymbol is decided by RESOLUTION against the index, not by
// the parse, because an identifier that resolves nowhere is just a string.
type Intent struct {
	Family          string
	Tool            string
	Query           string
	SymbolCandidate string
}

// identifier matches a bare code identifier: no regex metacharacters, no path
// separators, long enough that it is not an accidental match. A query holding
// metacharacters is a pattern, not a symbol, and must not be resolved as one.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{2,}$`)

// globChars are stripped from a -name pattern before it is used as a semantic
// query: `*handler*.go` asks about handlers, and the stars are shell syntax.
const globChars = "*?[]{}"

// DiscoverIntent parses one observed command into an Intent. The second return
// is false when the command asks nothing this package can serve, which is the
// common case and must stay silent.
func DiscoverIntent(command string) (Intent, bool) {
	fields := shellFields(command)
	for i, field := range fields {
		base := filepath.Base(strings.Trim(field, "'\""))
		rest := fields[i+1:]
		switch base {
		case "grep", "rg":
			if in, ok := parseLiteral(base, rest); ok {
				return in, true
			}
		case "find":
			if in, ok := parseFind(rest); ok {
				return in, true
			}
		case "git":
			if in, ok := parseGitLog(rest); ok {
				return in, true
			}
		}
	}
	return Intent{}, false
}

// parseLiteral takes the first non-flag argument as the pattern, which is what
// grep and rg both do for their positional PATTERN.
func parseLiteral(tool string, rest []string) (Intent, bool) {
	for _, arg := range rest {
		arg = strings.Trim(arg, "'\"")
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		in := Intent{Family: FamilyLiteral, Tool: tool, Query: arg}
		if identifier.MatchString(arg) {
			in.SymbolCandidate = arg
		}
		return in, true
	}
	return Intent{}, false
}

// nameFlags are the find predicates that carry a filename pattern. -path and
// -wholename match the whole path, so their value is cleaned the same way.
var nameFlags = map[string]bool{
	"-name": true, "-iname": true, "-path": true, "-ipath": true, "-wholename": true,
}

func parseFind(rest []string) (Intent, bool) {
	for i, arg := range rest {
		if !nameFlags[strings.Trim(arg, "'\"")] || i+1 >= len(rest) {
			continue
		}
		pattern := strings.Trim(rest[i+1], "'\"")
		query := cleanNamePattern(pattern)
		if query == "" {
			continue
		}
		return Intent{Family: FamilyFileFind, Tool: "find", Query: query}, true
	}
	return Intent{}, false
}

// cleanNamePattern turns a filename glob into a query. It drops glob syntax and
// leading directories, and refuses anything too short to retrieve on: a bare
// `*.go` carries no question, and asking the index for "go" returns noise.
func cleanNamePattern(pattern string) string {
	pattern = pattern[strings.LastIndexByte(pattern, '/')+1:]
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(globChars, r) {
			return -1
		}
		return r
	}, pattern)
	cleaned = strings.Trim(cleaned, ".")
	if len(cleaned) < 3 {
		return ""
	}
	return cleaned
}

// gitLogQueryFlags carry a search string when they appear on `git log`.
// -S is a pickaxe over content, -G a regex over the diff, --grep over messages.
var gitLogQueryFlags = map[string]bool{"-S": true, "-G": true, "--grep": true}

func parseGitLog(rest []string) (Intent, bool) {
	sawLog := false
	for i, raw := range rest {
		arg := strings.Trim(raw, "'\"")
		if !sawLog {
			if arg == "log" {
				sawLog = true
			}
			continue
		}
		// Attached forms: -Sfoo, -Gfoo, --grep=foo.
		for flag := range gitLogQueryFlags {
			sep := ""
			if strings.HasPrefix(flag, "--") {
				sep = "="
			}
			prefix := flag + sep
			if arg != flag && strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
				return gitIntent(strings.Trim(arg[len(prefix):], "'\"")), true
			}
		}
		// Detached form: -S foo, --grep foo.
		if gitLogQueryFlags[arg] && i+1 < len(rest) {
			return gitIntent(strings.Trim(rest[i+1], "'\"")), true
		}
	}
	return Intent{}, false
}

func gitIntent(query string) Intent {
	if len(query) < 3 {
		return Intent{}
	}
	return Intent{Family: FamilyHistory, Tool: "git-log", Query: query}
}

// StackCommand is the command this intent should have been, spelled for a human
// or a model to run. It is the POINTER the signpost emits; the payload arm
// injects the RESULT of the same command instead.
//
// The symbol family points at yupana rather than bobbin on purpose: "who calls
// this" is a structural question and bobbin answers a retrieval one. The
// resolution that promotes a query into this family is still bobbin's — see
// Process — so this family is the one place where the check and the pointer
// come from different tools, which is stated rather than hidden.
func (in Intent) StackCommand(repo string) string {
	scope := ""
	if repo != "" {
		scope = " --repo " + shellQuote(repo)
	}
	switch in.Family {
	case FamilySymbol:
		return "yupana callers " + shellQuote(in.Query)
	case FamilyHistory:
		return "bobbin search" + scope + " --type commit " + shellQuote(in.Query)
	default:
		return "bobbin search" + scope + " " + shellQuote(in.Query)
	}
}

// Invite names the ROUTE the pointer leads to. It is family-specific because
// "Try semantic search: yupana callers 'X'" is incoherent — yupana answers a
// structural question and bobbin's commit index answers a historical one, and a
// signpost that mislabels where it is sending the reader has already spent its
// one line of credibility.
func (in Intent) Invite() string {
	switch in.Family {
	case FamilySymbol:
		return "Try the structural index"
	case FamilyHistory:
		return "Try the commit index"
	default:
		return "Try semantic search"
	}
}

// Asks describes what the family is asking, for the one line of context a
// signpost gets. A pointer with no stated purpose reads as a tool advertisement.
func (in Intent) Asks() string {
	switch in.Family {
	case FamilyFileFind:
		return "file lookup"
	case FamilyHistory:
		return "history search"
	case FamilySymbol:
		return "symbol lookup"
	default:
		return "literal search"
	}
}
