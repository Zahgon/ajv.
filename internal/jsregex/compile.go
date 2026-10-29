package jsregex

type compiler struct {
	unicode bool // u flag
	nloops  int
	ncap    int
}

// capRange returns the lowest and highest capture index inside t (0, 0 if none).
func capRange(t *term) (int, int) {
	lo, hi := 0, 0
	var walk func(t *term)
	walk = func(t *term) {
		if t == nil {
			return
		}
		if t.typ == tCapture {
			if lo == 0 || t.index < lo {
				lo = t.index
			}
			if t.index > hi {
				hi = t.index
			}
		}
		walk(t.sub)
		for _, s := range t.subs {
			walk(s)
		}
	}
	walk(t)
	return lo, hi
}

func (c *compiler) fold() *foldData { return getFold(c.unicode) }

// charTestFor builds a one-character matcher for a literal or class term,
// or returns nil if t is not a single-character term.
func (c *compiler) charTestFor(t *term, back bool) charTest {
	switch t.typ {
	case tChar:
		if t.icase {
			v := c.fold().variants(t.c)
			if len(v) > 1 {
				if len(v) == 2 && v[0] <= 0xFFFF && v[1] <= 0xFFFF && !(c.unicode && (isSurrogate(v[0]) || isSurrogate(v[1]))) {
					if back {
						return &unit2Back{uint16(v[0]), uint16(v[1])}
					}
					return &unit2Fwd{uint16(v[0]), uint16(v[1])}
				}
				var s rangeSet
				for _, r := range v {
					s = append(s, r, r)
				}
				return c.setTest(s.norm(), false, back)
			}
		}
		if t.c <= 0xFFFF && !(c.unicode && isSurrogate(t.c)) {
			if back {
				return &unitBack{uint16(t.c)}
			}
			return &unitFwd{uint16(t.c)}
		}
		return c.setTest(rangeSet{t.c, t.c}, false, back)
	case tClass:
		s := t.set
		if t.icase {
			s = c.fold().closure(s)
		}
		return c.setTest(s, t.neg, back)
	}
	return nil
}

func isSurrogate(r rune) bool { return r >= 0xD800 && r <= 0xDFFF }

func (c *compiler) setTest(s rangeSet, neg, back bool) charTest {
	if !c.unicode {
		// Code units only.
		s = s.intersect(rangeSet{0, 0xFFFF})
	}
	cs := newClassSet(s, neg)
	if back {
		return &setBack{cs, c.unicode}
	}
	return &setFwd{cs, c.unicode}
}

func (c *compiler) comp(t *term, next mnode, back bool) mnode {
	switch t.typ {
	case tEmpty:
		return next
	case tChar, tClass:
		ct := c.charTestFor(t, back)
		if u, ok := ct.(*unitFwd); ok {
			return &unitNode{u.c, next}
		}
		return &charNode{ct, next}
	case tAssert:
		return &assertNode{kind: t.assert, wordExtra: c.unicode && t.icase, next: next}
	case tCapture:
		cl := &groupClose{k: t.index, back: back, next: next}
		body := c.comp(t.sub, cl, back)
		return &groupOpen{k: t.index, next: body}
	case tGroup:
		return c.comp(t.sub, next, back)
	case tLook:
		body := c.comp(t.sub, lookEndNode{}, t.behind)
		return &lookNode{body: body, neg: t.neg, capLo: t.capLo, capHi: t.capHi, next: next}
	case tQuant:
		return c.compQuant(t, next, back)
	case tBackref:
		return &backrefNode{refs: t.refs, icase: t.icase, unicode: c.unicode, back: back, fold: c.fold(), next: next}
	case tAlt:
		alts := make([]mnode, len(t.subs))
		for i, s := range t.subs {
			alts[i] = c.comp(s, next, back)
		}
		return &altNode{alts}
	case tSeq:
		n := next
		if back {
			for _, s := range t.subs {
				n = c.comp(s, n, back)
			}
		} else {
			for i := len(t.subs) - 1; i >= 0; i-- {
				n = c.comp(t.subs[i], n, back)
			}
		}
		return n
	}
	return next
}

func (c *compiler) compQuant(t *term, next mnode, back bool) mnode {
	if t.max == 0 {
		return next
	}
	if t.min == 1 && t.max == 1 {
		return c.comp(t.sub, next, back)
	}
	if ct := c.charTestFor(t.sub, back); ct != nil {
		return &charLoop{t: ct, min: t.min, max: t.max, greedy: t.greedy, back: back, unicode: c.unicode, next: next}
	}
	lo, hi := capRange(t.sub)
	l := &loopNode{id: c.nloops, min: t.min, max: t.max, greedy: t.greedy, next: next}
	if hi > 0 {
		l.capLo, l.capHi = lo-1, hi
	}
	c.nloops++
	l.body = c.comp(t.sub, &loopTail{l}, back)
	return l
}
