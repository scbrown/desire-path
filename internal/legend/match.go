package legend

import "sort"

// Gazetteer is a compiled label set: an Aho-Corasick automaton over the
// lower-cased labels, so one pass over the text finds every label at once.
// The replay used a regex alternation (6.7 min for 2162 texts); the hook path
// has a p95 budget of 50 ms, which an alternation cannot meet.
//
// The automaton is rebuilt on every hook call (the hook is a fresh process),
// so construction cost is part of the latency budget. A map per trie node cost
// 29 ms on the live gazetteer; flat arrays with linked siblings, a direct
// table for the root's children and dictionary-suffix links keep it to a few.
type Gazetteer struct {
	keys  []string  // lower-cased label per pattern id
	iris  [][]Entry // entities per pattern id
	child []int32   // first child of each node, -1 if none
	sib   []int32   // next sibling, -1 if none
	ch    []byte    // byte on the edge INTO each node
	fail  []int32
	pat   []int32 // pattern id ending exactly at this node, -1 if none
	dict  []int32 // nearest node on the fail chain with a pattern, 0 if none
	root  [256]int32
}

// Match is one label found in the text.
type Match struct {
	Key  string // lower-cased label: the dedup key
	Text string // the label as it appears in the text
	IRIs []Entry
	Pos  int
}

// Compile builds a gazetteer. Labels are grouped case-insensitively; a label on
// several IRIs becomes one ambiguous pattern.
func Compile(entries []Entry) *Gazetteer {
	g := &Gazetteer{}
	for i := range g.root {
		g.root[i] = -1
	}
	g.newNode(0)
	ids := map[string]int{}
	for _, e := range entries {
		k := lower(e.Label)
		if k == "" {
			continue
		}
		id, ok := ids[k]
		if !ok {
			id = len(g.keys)
			ids[k] = id
			g.keys = append(g.keys, k)
			g.iris = append(g.iris, nil)
			g.insert(k, int32(id))
		}
		if !hasIRI(g.iris[id], e.IRI) {
			g.iris[id] = append(g.iris[id], e)
		}
	}
	g.link()
	return g
}

func hasIRI(es []Entry, iri string) bool {
	for _, e := range es {
		if e.IRI == iri {
			return true
		}
	}
	return false
}

func (g *Gazetteer) newNode(c byte) int32 {
	g.child = append(g.child, -1)
	g.sib = append(g.sib, -1)
	g.ch = append(g.ch, c)
	g.fail = append(g.fail, 0)
	g.pat = append(g.pat, -1)
	g.dict = append(g.dict, 0)
	return int32(len(g.ch) - 1)
}

// next is the goto function: the child of n on byte c, or -1.
func (g *Gazetteer) next(n int32, c byte) int32 {
	if n == 0 {
		return g.root[c]
	}
	for k := g.child[n]; k != -1; k = g.sib[k] {
		if g.ch[k] == c {
			return k
		}
	}
	return -1
}

func (g *Gazetteer) insert(k string, id int32) {
	cur := int32(0)
	for i := 0; i < len(k); i++ {
		c := k[i]
		nx := g.next(cur, c)
		if nx == -1 {
			nx = g.newNode(c)
			if cur == 0 {
				g.root[c] = nx
			} else {
				g.sib[nx] = g.child[cur]
				g.child[cur] = nx
			}
		}
		cur = nx
	}
	g.pat[cur] = id
}

func (g *Gazetteer) children(n int32, visit func(int32)) {
	if n == 0 {
		for _, k := range g.root {
			if k != -1 {
				visit(k)
			}
		}
		return
	}
	for k := g.child[n]; k != -1; k = g.sib[k] {
		visit(k)
	}
}

func (g *Gazetteer) link() {
	queue := make([]int32, 0, len(g.ch))
	g.children(0, func(k int32) { queue = append(queue, k) })
	for i := 0; i < len(queue); i++ {
		u := queue[i]
		g.children(u, func(v int32) {
			c := g.ch[v]
			f := g.fail[u]
			for f != 0 && g.next(f, c) == -1 {
				f = g.fail[f]
			}
			if t := g.next(f, c); t != -1 && t != v {
				g.fail[v] = t
			}
			if fv := g.fail[v]; g.pat[fv] >= 0 {
				g.dict[v] = fv
			} else {
				g.dict[v] = g.dict[fv]
			}
			queue = append(queue, v)
		})
	}
}

// Match returns each label's FIRST valid occurrence, in text order, choosing
// the longest label at a position and never overlapping an earlier choice —
// the semantics of the replay's longest-first alternation.
func (g *Gazetteer) Match(text string) []Match {
	if g == nil || len(g.keys) == 0 {
		return nil
	}
	type cand struct{ start, end, id int }
	var cands []cand
	cur := int32(0)
	for i := 0; i < len(text); i++ {
		c := lowerByte(text[i])
		for cur != 0 && g.next(cur, c) == -1 {
			cur = g.fail[cur]
		}
		if nx := g.next(cur, c); nx != -1 {
			cur = nx
		}
		// Outputs at this position, longest first: the node itself, then
		// each shorter suffix that is also a label.
		for n := cur; n != 0; n = g.dict[n] {
			id := g.pat[n]
			if id < 0 {
				continue
			}
			end := i + 1
			start := end - len(g.keys[id])
			if boundaryOK(text, start, end) {
				cands = append(cands, cand{start, end, int(id)})
			}
		}
	}
	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].start != cands[b].start {
			return cands[a].start < cands[b].start
		}
		return cands[a].end-cands[a].start > cands[b].end-cands[b].start
	})
	var out []Match
	taken := map[int]bool{}
	lastEnd := 0
	for _, c := range cands {
		if c.start < lastEnd {
			continue
		}
		lastEnd = c.end
		if taken[c.id] {
			continue
		}
		taken[c.id] = true
		out = append(out, Match{Key: g.keys[c.id], Text: text[c.start:c.end], IRIs: g.iris[c.id], Pos: c.start})
	}
	return out
}

// boundaryOK is the replay's word boundary — not preceded or followed by
// [A-Za-z0-9_-] — plus fix 1 from its noise sample: a label followed by
// ".<alnum>" is the stem of a filename (preflight-monitoring-secrets matched
// preflight-monitoring-secrets.sh), not a mention of the entity.
func boundaryOK(text string, start, end int) bool {
	if start > 0 && wordByte(text[start-1]) {
		return false
	}
	if end < len(text) && wordByte(text[end]) {
		return false
	}
	if end+1 < len(text) && text[end] == '.' && alnum(text[end+1]) {
		return false
	}
	return true
}

func wordByte(b byte) bool { return alnum(b) || b == '_' || b == '-' }

func alnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func lowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

func lower(s string) string {
	b := []byte(s)
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\n') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\n') {
		end--
	}
	b = b[start:end]
	for i := range b {
		b[i] = lowerByte(b[i])
	}
	return string(b)
}
