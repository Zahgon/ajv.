package jsregex

// The matcher is a graph of nodes; every node knows its continuation, so
// backtracking is plain recursion: match returns false to backtrack. State
// changed by a node is restored by that node when its continuation fails.

type mnode interface {
	match(m *machine, i int) bool
}

type loopState struct {
	count int
	start int
}

type machine struct {
	in       []uint16
	caps     []int // 2 per group (index 0 = whole match): start, end; -1 = undefined
	open     []int // pending group boundary while a group is being matched
	loops    []loopState
	stack    []int
	ubuf     []uint16 // reusable UTF-16 buffer for Test
	matchEnd int
}

func isLineTerm(c uint16) bool { return c == '\n' || c == '\r' || c == 0x2028 || c == 0x2029 }

// midPair reports whether i lies between the halves of a surrogate pair.
func (m *machine) midPair(i int) bool {
	return i > 0 && i < len(m.in) && isLead(m.in[i-1]) && isTrail(m.in[i])
}

// readFwd returns the character at i and its width; the width is 0 at the
// end of input and, in unicode mode, inside a surrogate pair (V8 never
// matches a character there).
func (m *machine) readFwd(i int, unicode bool) (rune, int) {
	if i >= len(m.in) {
		return -1, 0
	}
	c := m.in[i]
	if unicode {
		if isLead(c) && i+1 < len(m.in) && isTrail(m.in[i+1]) {
			return 0x10000 + (rune(c)-0xD800)<<10 + rune(m.in[i+1]-0xDC00), 2
		}
		if isTrail(c) && m.midPair(i) {
			return -1, 0
		}
	}
	return rune(c), 1
}

// readBack returns the character ending at i and its width (0 at start
// and, in unicode mode, inside a surrogate pair).
func (m *machine) readBack(i int, unicode bool) (rune, int) {
	if i <= 0 {
		return -1, 0
	}
	c := m.in[i-1]
	if unicode {
		if isTrail(c) && i-2 >= 0 && isLead(m.in[i-2]) {
			return 0x10000 + (rune(m.in[i-2])-0xD800)<<10 + rune(c-0xDC00), 2
		}
		if isLead(c) && m.midPair(i) {
			return -1, 0
		}
	}
	return rune(c), 1
}

// ---- terminals ----

type acceptNode struct{}

func (acceptNode) match(m *machine, i int) bool { m.matchEnd = i; return true }

type lookEndNode struct{}

func (lookEndNode) match(m *machine, i int) bool { return true }

// ---- single characters ----

// charTest is a one-character matcher usable both as a node and inside loops.
type charTest interface {
	step(m *machine, i int) int // new position or -1
}

type unitFwd struct{ c uint16 }

func (t *unitFwd) step(m *machine, i int) int {
	if i < len(m.in) && m.in[i] == t.c {
		return i + 1
	}
	return -1
}

type unitBack struct{ c uint16 }

func (t *unitBack) step(m *machine, i int) int {
	if i > 0 && m.in[i-1] == t.c {
		return i - 1
	}
	return -1
}

type unit2Fwd struct{ a, b uint16 }

func (t *unit2Fwd) step(m *machine, i int) int {
	if i < len(m.in) {
		if c := m.in[i]; c == t.a || c == t.b {
			return i + 1
		}
	}
	return -1
}

type unit2Back struct{ a, b uint16 }

func (t *unit2Back) step(m *machine, i int) int {
	if i > 0 {
		if c := m.in[i-1]; c == t.a || c == t.b {
			return i - 1
		}
	}
	return -1
}

// classSet is a compiled code point set with an ASCII bitmap.
type classSet struct {
	ascii [2]uint64
	set   rangeSet
	neg   bool
}

func newClassSet(s rangeSet, neg bool) *classSet {
	cs := &classSet{set: s, neg: neg}
	for c := rune(0); c < 128; c++ {
		if s.has(c) {
			cs.ascii[c>>6] |= 1 << (c & 63)
		}
	}
	return cs
}

func (cs *classSet) has(c rune) bool {
	var in bool
	if c < 128 {
		in = cs.ascii[c>>6]&(1<<(c&63)) != 0
	} else {
		in = cs.set.has(c)
	}
	return in != cs.neg
}

type setFwd struct {
	cs      *classSet
	unicode bool
}

func (t *setFwd) step(m *machine, i int) int {
	if i >= len(m.in) {
		return -1
	}
	c := m.in[i]
	if c < 128 {
		if (t.cs.ascii[c>>6]&(1<<(c&63)) != 0) != t.cs.neg {
			return i + 1
		}
		return -1
	}
	r, w := m.readFwd(i, t.unicode)
	if w > 0 && t.cs.has(r) {
		return i + w
	}
	return -1
}

type setBack struct {
	cs      *classSet
	unicode bool
}

func (t *setBack) step(m *machine, i int) int {
	r, w := m.readBack(i, t.unicode)
	if w > 0 && t.cs.has(r) {
		return i - w
	}
	return -1
}

// charNode wraps a charTest as a graph node.
type charNode struct {
	t    charTest
	next mnode
}

func (n *charNode) match(m *machine, i int) bool {
	j := n.t.step(m, i)
	return j >= 0 && n.next.match(m, j)
}

// fast paths for the most common literal case
type unitNode struct {
	c    uint16
	next mnode
}

func (n *unitNode) match(m *machine, i int) bool {
	return i < len(m.in) && m.in[i] == n.c && n.next.match(m, i+1)
}

// ---- assertions ----

type assertNode struct {
	kind      assertKind
	wordExtra bool // u + i: U+017F and U+212A are word characters
	next      mnode
}

func (n *assertNode) isWord(m *machine, i int) bool {
	if i < 0 || i >= len(m.in) {
		return false
	}
	c := m.in[i]
	if c < 128 {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
	}
	return n.wordExtra && (c == 0x017F || c == 0x212A)
}

func (n *assertNode) match(m *machine, i int) bool {
	switch n.kind {
	case aStartInput:
		if i != 0 {
			return false
		}
	case aStartLine:
		if i != 0 && !isLineTerm(m.in[i-1]) {
			return false
		}
	case aEndInput:
		if i != len(m.in) {
			return false
		}
	case aEndLine:
		if i != len(m.in) && !isLineTerm(m.in[i]) {
			return false
		}
	case aBoundary:
		if n.isWord(m, i-1) == n.isWord(m, i) {
			return false
		}
	case aNonBoundary:
		if n.isWord(m, i-1) != n.isWord(m, i) {
			return false
		}
	}
	return n.next.match(m, i)
}

// ---- groups ----

type groupOpen struct {
	k    int
	next mnode
}

func (n *groupOpen) match(m *machine, i int) bool {
	old := m.open[n.k]
	m.open[n.k] = i
	if n.next.match(m, i) {
		return true
	}
	m.open[n.k] = old
	return false
}

type groupClose struct {
	k    int
	back bool
	next mnode
}

func (n *groupClose) match(m *machine, i int) bool {
	os, oe := m.caps[2*n.k], m.caps[2*n.k+1]
	if n.back {
		m.caps[2*n.k], m.caps[2*n.k+1] = i, m.open[n.k]
	} else {
		m.caps[2*n.k], m.caps[2*n.k+1] = m.open[n.k], i
	}
	if n.next.match(m, i) {
		return true
	}
	m.caps[2*n.k], m.caps[2*n.k+1] = os, oe
	return false
}

// ---- alternation ----

type altNode struct{ alts []mnode }

func (n *altNode) match(m *machine, i int) bool {
	for _, a := range n.alts {
		if a.match(m, i) {
			return true
		}
	}
	return false
}

// ---- lookaround ----

type lookNode struct {
	body         mnode
	neg          bool
	capLo, capHi int // groups capLo+1..capHi are inside
	next         mnode
}

func (n *lookNode) save(m *machine) int {
	mark := len(m.stack)
	m.stack = append(m.stack, m.caps[2*(n.capLo+1):2*(n.capHi+1)]...)
	return mark
}

func (n *lookNode) restore(m *machine, mark int) {
	copy(m.caps[2*(n.capLo+1):2*(n.capHi+1)], m.stack[mark:])
	m.stack = m.stack[:mark]
}

func (n *lookNode) match(m *machine, i int) bool {
	mark := n.save(m)
	ok := n.body.match(m, i)
	if n.neg {
		if ok {
			n.restore(m, mark)
			return false
		}
		m.stack = m.stack[:mark]
		return n.next.match(m, i)
	}
	if !ok {
		m.stack = m.stack[:mark]
		return false
	}
	if n.next.match(m, i) {
		return true
	}
	n.restore(m, mark)
	return false
}

// ---- back references ----

type backrefNode struct {
	refs    []int
	icase   bool
	unicode bool
	back    bool
	fold    *foldData
	next    mnode
}

func (n *backrefNode) match(m *machine, i int) bool {
	s, e := -1, -1
	for _, k := range n.refs {
		if 2*k+1 < len(m.caps) && m.caps[2*k+1] >= 0 {
			s, e = m.caps[2*k], m.caps[2*k+1]
			break
		}
	}
	if s < 0 {
		return n.cont(m, i)
	}
	l := e - s
	var from int
	if n.back {
		from = i - l
		if from < 0 {
			return false
		}
	} else {
		from = i
		if from+l > len(m.in) {
			return false
		}
	}
	if !n.icase {
		for k := 0; k < l; k++ {
			if m.in[s+k] != m.in[from+k] {
				return false
			}
		}
	} else if !n.unicode {
		for k := 0; k < l; k++ {
			a, b := m.in[s+k], m.in[from+k]
			if a != b && n.fold.canonical(rune(a)) != n.fold.canonical(rune(b)) {
				return false
			}
		}
	} else {
		sub := machine{in: m.in[s:e]}
		oth := machine{in: m.in[from : from+l]}
		for k := 0; k < l; {
			a, wa := sub.readFwd(k, true)
			b, wb := oth.readFwd(k, true)
			if wa != wb || (a != b && n.fold.canonical(a) != n.fold.canonical(b)) {
				return false
			}
			k += wa
		}
	}
	if n.back {
		return n.cont(m, from)
	}
	return n.cont(m, from+l)
}

// cont continues after the reference; in unicode mode V8 rejects positions
// inside a surrogate pair (CheckNotInSurrogatePair).
func (n *backrefNode) cont(m *machine, i int) bool {
	if n.unicode && m.midPair(i) {
		return false
	}
	return n.next.match(m, i)
}

// ---- loops ----

// loopNode implements RepeatMatcher for arbitrary bodies.
type loopNode struct {
	id           int
	min, max     int
	greedy       bool
	capLo, capHi int // groups capLo+1..capHi are reset per iteration
	body         mnode
	next         mnode
}

type loopTail struct{ l *loopNode }

func (n *loopNode) match(m *machine, i int) bool {
	saved := m.loops[n.id]
	m.loops[n.id] = loopState{count: 0, start: -1}
	r := n.iterate(m, i)
	m.loops[n.id] = saved
	return r
}

func (n *loopNode) iterate(m *machine, i int) bool {
	count := m.loops[n.id].count
	if n.max != infinity && count >= n.max {
		return n.next.match(m, i)
	}
	if count < n.min {
		return n.tryBody(m, i)
	}
	if n.greedy {
		if n.tryBody(m, i) {
			return true
		}
		return n.next.match(m, i)
	}
	if n.next.match(m, i) {
		return true
	}
	return n.tryBody(m, i)
}

func (n *loopNode) tryBody(m *machine, i int) bool {
	st := &m.loops[n.id]
	oldStart := st.start
	st.start = i
	lo, hi := 2*(n.capLo+1), 2*(n.capHi+1)
	mark := len(m.stack)
	if hi > lo {
		m.stack = append(m.stack, m.caps[lo:hi]...)
		for k := lo; k < hi; k++ {
			m.caps[k] = -1
		}
	}
	if n.body.match(m, i) {
		return true
	}
	if hi > lo {
		copy(m.caps[lo:hi], m.stack[mark:])
		m.stack = m.stack[:mark]
	}
	m.loops[n.id].start = oldStart
	return false
}

func (t *loopTail) match(m *machine, i int) bool {
	n := t.l
	st := &m.loops[n.id]
	if st.count >= n.min && i == st.start {
		return false
	}
	st.count++
	if n.iterate(m, i) {
		return true
	}
	m.loops[n.id].count--
	return false
}

// charLoop is a quantifier over a single-character matcher (no captures,
// never empty), matched without recursion per iteration.
type charLoop struct {
	t        charTest
	min, max int
	greedy   bool
	back     bool
	unicode  bool
	next     mnode
}

// stepBack moves from j one character towards the loop start bound.
func (n *charLoop) undo(m *machine, j, bound int) int {
	if n.back {
		if n.unicode && j+2 <= bound && isLead(m.in[j]) && isTrail(m.in[j+1]) {
			return j + 2
		}
		return j + 1
	}
	if n.unicode && j-2 >= bound && isTrail(m.in[j-1]) && isLead(m.in[j-2]) {
		return j - 2
	}
	return j - 1
}

func (n *charLoop) match(m *machine, i int) bool {
	j := i
	count := 0
	for count < n.min {
		j = n.t.step(m, j)
		if j < 0 {
			return false
		}
		count++
	}
	if !n.greedy {
		for {
			if n.next.match(m, j) {
				return true
			}
			if n.max != infinity && count >= n.max {
				return false
			}
			j = n.t.step(m, j)
			if j < 0 {
				return false
			}
			count++
		}
	}
	minPos := j
	for n.max == infinity || count < n.max {
		k := n.t.step(m, j)
		if k < 0 {
			break
		}
		j = k
		count++
	}
	for {
		if n.next.match(m, j) {
			return true
		}
		if j == minPos {
			return false
		}
		j = n.undo(m, j, minPos)
	}
}
