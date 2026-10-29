// Package uprops provides the Unicode property tables used by V8's RegExp
// \p{...} escapes (General_Category, Script, Script_Extensions and the binary
// properties V8 accepts), plus the case-equivalence classes used by
// case-insensitive matching. All data was generated from node (see tables.go).
package uprops

import (
	"sort"
	"sync"
)

// Table is a set of code points stored as sorted, non-overlapping,
// non-adjacent inclusive ranges: lo0, hi0, lo1, hi1, ...
type Table []uint32

var (
	decodeOnce sync.Once
	decoded    []Table
)

func decodeAll() {
	n := len(tableOffsets) - 1
	decoded = make([]Table, n)
	for t := 0; t < n; t++ {
		src := tableData[tableOffsets[t]:tableOffsets[t+1]]
		out := make(Table, len(src))
		prev := int64(-1)
		for i := 0; i+1 < len(src); i += 2 {
			lo := prev + 1 + int64(src[i])
			hi := lo + int64(src[i+1])
			out[i], out[i+1] = uint32(lo), uint32(hi)
			prev = hi
		}
		decoded[t] = out
	}
}

func table(i int) Table {
	decodeOnce.Do(decodeAll)
	return decoded[i]
}

// Lookup resolves a property the way V8 does for \p{name} (value == "") and
// \p{name=value}. Lone names are General_Category values or binary
// properties; name=value accepts gc/General_Category, sc/Script and
// scx/Script_Extensions. Matching is exact (no loose matching), as in V8.
func Lookup(name, value string) (Table, bool) {
	if value == "" {
		i, ok := loneNames[name]
		if !ok {
			return nil, false
		}
		return table(i), true
	}
	var m map[string]int
	switch propNames[name] {
	case "gc":
		m = gcValues
	case "sc":
		m = scValues
	case "scx":
		m = scxValues
	default:
		return nil, false
	}
	i, ok := m[value]
	if !ok {
		return nil, false
	}
	return table(i), true
}

// Is reports whether r is in t.
func Is(t Table, r rune) bool {
	c := uint32(r)
	n := len(t) / 2
	i := sort.Search(n, func(k int) bool { return t[2*k+1] >= c })
	return i < n && t[2*i] <= c
}

// FoldClasses returns the case-equivalence classes (each with at least two
// members, sorted) used by case-insensitive matching: simple case folding
// when unicode is true (u flag), V8's legacy Canonicalize over UTF-16
// code units otherwise.
func FoldClasses(unicode bool) [][]rune {
	src := foldLegacy[:]
	if unicode {
		src = foldUnicode[:]
	}
	var out [][]rune
	for i := 0; i < len(src); {
		n := int(src[i])
		cl := make([]rune, n)
		for k := 0; k < n; k++ {
			cl[k] = rune(src[i+1+k])
		}
		out = append(out, cl)
		i += n + 1
	}
	return out
}
