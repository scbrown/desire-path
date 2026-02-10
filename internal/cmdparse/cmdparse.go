// Package cmdparse provides lightweight shell command parsing and manipulation
// for the pave-check hook. It splits command strings on pipes and chain operators,
// identifies command names and flags, and applies targeted corrections.
package cmdparse

import "strings"

// Segment represents one command in a pipeline or chain.
type Segment struct {
	Command string   // program name (e.g., "scp")
	Tokens  []string // all tokens after the command
	Raw     string   // original text of this segment (trimmed)
	Start   int      // byte offset of Raw in the full command string
	End     int      // byte offset end (exclusive)
}

// Parse splits a command string into Segments on |, &&, ||, ;.
// It respects single and double quotes and backslash escapes.
// Start and End offsets point to the trimmed segment text within
// the original command string.
func Parse(cmd string) []Segment {
	parts := splitOperators(cmd)
	segs := make([]Segment, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p.text)
		if trimmed == "" {
			continue
		}
		// Compute offset of trimmed text within the original string.
		leading := len(p.text) - len(strings.TrimLeft(p.text, " \t"))
		trimStart := p.start + leading
		trimEnd := trimStart + len(trimmed)

		tokens := tokenize(trimmed)
		s := Segment{
			Raw:   trimmed,
			Start: trimStart,
			End:   trimEnd,
		}
		if len(tokens) > 0 {
			s.Command = tokens[0]
			s.Tokens = tokens[1:]
		}
		segs = append(segs, s)
	}
	return segs
}

// CorrectFlag finds a short flag character oldFlag in the segment's tokens
// and replaces it with newFlag. Handles standalone (-r), combined (-rP → -RP),
// and long flags (--recursive). Returns the corrected segment Raw string and
// true if a correction was made.
func CorrectFlag(seg Segment, oldFlag, newFlag string) (string, bool) {
	// Try long flag match first (--flag → --newflag).
	if len(oldFlag) > 1 {
		return correctLongFlag(seg, oldFlag, newFlag)
	}

	if len(oldFlag) != 1 {
		return "", false
	}

	// Short flag: single character.
	oldChar := oldFlag[0]
	tokens := tokenize(seg.Raw)
	found := false
	for i, tok := range tokens {
		if i == 0 {
			continue // skip command name
		}
		if !strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "--") {
			continue
		}
		// It's a short flag group like -r or -rP.
		body := tok[1:] // strip leading dash
		idx := strings.IndexByte(body, oldChar)
		if idx < 0 {
			continue
		}
		// Replace the character.
		newBody := body[:idx] + newFlag + body[idx+1:]
		tokens[i] = "-" + newBody
		found = true
		break // correct first occurrence only
	}
	if !found {
		return "", false
	}
	return strings.Join(tokens, " "), true
}

func correctLongFlag(seg Segment, oldFlag, newFlag string) (string, bool) {
	target := "--" + oldFlag
	newTarget := "--" + newFlag
	tokens := tokenize(seg.Raw)
	found := false
	for i, tok := range tokens {
		if i == 0 {
			continue
		}
		if tok == target {
			tokens[i] = newTarget
			found = true
			break
		}
		// Handle --flag=value.
		if strings.HasPrefix(tok, target+"=") {
			tokens[i] = newTarget + tok[len(target):]
			found = true
			break
		}
	}
	if !found {
		return "", false
	}
	return strings.Join(tokens, " "), true
}

// SubstituteCommand replaces the command name in a segment.
func SubstituteCommand(seg Segment, newCmd string) string {
	tokens := tokenize(seg.Raw)
	if len(tokens) == 0 {
		return seg.Raw
	}
	tokens[0] = newCmd
	return strings.Join(tokens, " ")
}

// ReplaceLiteral performs a literal string replacement within a segment's raw text.
func ReplaceLiteral(seg Segment, old, new string) string {
	return strings.Replace(seg.Raw, old, new, 1)
}

// ApplyToFull replaces the segment at [seg.Start, seg.End) in the original
// full command string with the corrected segment text.
func ApplyToFull(full string, seg Segment, corrected string) string {
	return full[:seg.Start] + corrected + full[seg.End:]
}

// part is an internal type for split results.
type part struct {
	text  string
	start int
	end   int
}

// splitOperators splits on unquoted |, &&, ||, ; while preserving offsets.
func splitOperators(cmd string) []part {
	var parts []part
	inSingle := false
	inDouble := false
	escaped := false
	segStart := 0

	i := 0
	for i < len(cmd) {
		ch := cmd[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			i++
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			i++
			continue
		}
		if inSingle || inDouble {
			i++
			continue
		}

		// Check for operators: &&, ||, |, ;
		if ch == ';' {
			parts = append(parts, part{text: cmd[segStart:i], start: segStart, end: i})
			segStart = i + 1
			i++
			continue
		}
		if ch == '|' {
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				parts = append(parts, part{text: cmd[segStart:i], start: segStart, end: i})
				segStart = i + 2
				i += 2
				continue
			}
			parts = append(parts, part{text: cmd[segStart:i], start: segStart, end: i})
			segStart = i + 1
			i++
			continue
		}
		if ch == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
			parts = append(parts, part{text: cmd[segStart:i], start: segStart, end: i})
			segStart = i + 2
			i += 2
			continue
		}
		i++
	}
	// Final segment.
	if segStart < len(cmd) {
		parts = append(parts, part{text: cmd[segStart:], start: segStart, end: len(cmd)})
	}
	return parts
}

// tokenize splits a command segment into tokens, respecting quotes and escapes.
// It does not interpret shell syntax beyond basic quoting.
func tokenize(s string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			// In double quotes, only escape certain chars; for simplicity
			// we pass through the backslash for the token.
			if inDouble {
				current.WriteByte(ch)
			}
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			current.WriteByte(ch) // preserve quotes in token
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			current.WriteByte(ch) // preserve quotes in token
			continue
		}
		if (ch == ' ' || ch == '\t') && !inSingle && !inDouble {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteByte(ch)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
