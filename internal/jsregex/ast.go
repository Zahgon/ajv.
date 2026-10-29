package jsregex

import "math"

// infinity is V8's RegExpTree::kInfinity.
const infinity = math.MaxInt32

type termType uint8

const (
	tEmpty   termType = iota
	tChar             // c
	tClass            // set, neg
	tAssert           // assert
	tCapture          // index, name, sub
	tGroup            // sub
	tLook             // behind, neg, sub, capLo..capHi
	tQuant            // min, max, greedy, sub
	tBackref          // refs (resolved from refRaw for named references)
	tAlt              // subs
	tSeq              // subs
)

type assertKind uint8

const (
	aStartInput assertKind = iota
	aStartLine
	aEndInput
	aEndLine
	aBoundary
	aNonBoundary
)

// modFlags are the flags a (?ims-ims:...) group can change.
type modFlags struct {
	icase, multiline, dotAll bool
}

// term is a parsed regular-expression AST node.
type term struct {
	typ    termType
	c      rune
	set    rangeSet
	neg    bool
	icase  bool
	assert assertKind
	index  int
	name   string
	behind bool
	min    int
	max    int
	greedy bool
	capLo  int // captures with index in (capLo, capHi] are inside
	capHi  int
	refs   []int
	refRaw []uint16 // name of a named reference
	sub    *term
	subs   []*term
}

func addSat(a, b int) int {
	if a >= infinity-b {
		return infinity
	}
	return a + b
}

// maxMatch mirrors V8's RegExpTree::max_match in code units.
func (t *term) maxMatch(unicode bool) int {
	switch t.typ {
	case tChar:
		if t.c > 0xFFFF {
			return 2
		}
		return 1
	case tClass:
		m := 1
		if unicode {
			m = 2
		}
		return m
	case tCapture, tGroup:
		return t.sub.maxMatch(unicode)
	case tQuant:
		b := t.sub.maxMatch(unicode)
		if b == 0 || t.max == 0 {
			return 0
		}
		if t.max == infinity || b > infinity/t.max {
			return infinity
		}
		return b * t.max
	case tBackref:
		return infinity
	case tAlt:
		m := 0
		for _, s := range t.subs {
			if v := s.maxMatch(unicode); v > m {
				m = v
			}
		}
		return m
	case tSeq:
		m := 0
		for _, s := range t.subs {
			m = addSat(m, s.maxMatch(unicode))
		}
		return m
	}
	return 0
}
