package jsregex

import (
	"github.com/ajv-validator/ajv-go/internal/uprops"
)

// V8 RegExpError detail texts.
const (
	errUnterminatedGroup       = "Unterminated group"
	errUnmatchedParen          = "Unmatched ')'"
	errEscapeAtEnd             = "\\ at end of pattern"
	errInvalidPropertyName     = "Invalid property name"
	errInvalidEscape           = "Invalid escape"
	errInvalidDecimalEscape    = "Invalid decimal escape"
	errInvalidUnicodeEscape    = "Invalid Unicode escape"
	errNothingToRepeat         = "Nothing to repeat"
	errLoneQuantifierBrackets  = "Lone quantifier brackets"
	errRangeOutOfOrder         = "numbers out of order in {} quantifier"
	errIncompleteQuantifier    = "Incomplete quantifier"
	errInvalidQuantifier       = "Invalid quantifier"
	errInvalidGroup            = "Invalid group"
	errMultipleFlagDashes      = "Multiple dashes in flag group"
	errRepeatedFlag            = "Repeated flag in flag group"
	errInvalidFlagGroup        = "Invalid flag group"
	errTooManyCaptures         = "Too many captures"
	errInvalidCaptureGroupName = "Invalid capture group name"
	errDuplicateCaptureName    = "Duplicate capture group name"
	errInvalidNamedReference   = "Invalid named reference"
	errInvalidNamedCaptureRef  = "Invalid named capture referenced"
	errInvalidClassProperty    = "Invalid property name in character class"
	errInvalidCharacterClass   = "Invalid character class"
	errUnterminatedClass       = "Unterminated character class"
	errOutOfOrderClass         = "Range out of order in character class"
)

// endMarker is V8's kEndMarker: the "character" read past the end.
const endMarker rune = 1 << 21

// maxCaptures is V8's limit on capture groups (found by experiment).
const maxCaptures = 32767

type parseError struct{ msg string }

type groupKind uint8

const (
	gInitial groupKind = iota
	gCapture
	gGrouping
	gPosLook
	gNegLook
)

// builder collects the alternatives of one disjunction (V8 RegExpBuilder).
type builder struct {
	flags        modFlags
	alts         []*term
	terms        []*term
	pendingEmpty bool
}

func (b *builder) addAtom(t *term) {
	if t.typ == tEmpty {
		b.pendingEmpty = true
		return
	}
	b.pendingEmpty = false
	b.terms = append(b.terms, t)
}

func (b *builder) addTerm(t *term) {
	b.pendingEmpty = false
	b.terms = append(b.terms, t)
}

func (b *builder) newAlternative() {
	b.alts = append(b.alts, seqOf(b.terms))
	b.terms = nil
	b.pendingEmpty = false
}

func seqOf(ts []*term) *term {
	switch len(ts) {
	case 0:
		return &term{typ: tEmpty}
	case 1:
		return ts[0]
	}
	return &term{typ: tSeq, subs: ts}
}

func (b *builder) toRegExp() *term {
	b.newAlternative()
	if len(b.alts) == 1 {
		return b.alts[0]
	}
	return &term{typ: tAlt, subs: b.alts}
}

// pstate is V8's RegExpParserState: one open group.
type pstate struct {
	prev         *pstate
	kind         groupKind
	behind       bool
	captureIndex int
	name         []uint16
	hasName      bool
	b            *builder
	disj         int // disjunction id of this group's body
	alt          int // current alternative index within it
}

type namedGroup struct {
	name  []uint16
	index int
	path  [][2]int // (disjunction, alternative) of every enclosing group
}

// parser mirrors V8's RegExpParserImpl over UTF-16 code units.
type parser struct {
	in       []uint16
	nextPos  int
	curPos   int
	cur      rune
	hasMore  bool
	unicode  bool // u flag
	forceUni bool
	flags    modFlags
	// leakIcase reproduces a V8 quirk: the i flag used for the case
	// equivalents of \w and \W (u mode) is that of the most recently
	// opened group and is not restored when a group closes.
	leakIcase   bool
	captures    int // captures started
	scanned     bool
	captureCnt  int
	hasNamed    bool
	named       []namedGroup
	namedRefs   []*term
	capTerms    map[int]*term
	disjCounter int
}

func (p *parser) fail(msg string) { panic(parseError{msg}) }

func (p *parser) unicodeMode() bool { return p.unicode || p.forceUni }

func (p *parser) readNext(update bool) rune {
	pos := p.nextPos
	c0 := rune(p.in[pos])
	pos++
	if p.unicodeMode() && pos < len(p.in) && isLead(uint16(c0)) && isTrail(p.in[pos]) {
		c0 = 0x10000 + (c0-0xD800)<<10 + rune(p.in[pos]-0xDC00)
		pos++
	}
	if update {
		p.nextPos = pos
	}
	return c0
}

func (p *parser) hasNext() bool { return p.nextPos < len(p.in) }

func (p *parser) next() rune {
	if p.hasNext() {
		return p.readNext(false)
	}
	return endMarker
}

func (p *parser) advance() {
	p.curPos = p.nextPos
	if p.hasNext() {
		p.cur = p.readNext(true)
	} else {
		p.cur = endMarker
		p.nextPos = len(p.in) + 1
		p.hasMore = false
	}
}

func (p *parser) advanceN(n int) {
	p.nextPos += n - 1
	p.advance()
}

func (p *parser) position() int { return p.nextPos - 1 }

func (p *parser) reset(pos int) {
	p.nextPos = pos
	p.hasMore = pos < len(p.in)
	p.advance()
}

func isDecimal(c rune) bool { return c >= '0' && c <= '9' }

func hexValue(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func isSyntaxOrSlash(c rune) bool {
	switch c {
	case '^', '$', '\\', '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|', '/':
		return true
	}
	return false
}

// parse parses the whole pattern (V8 ParsePattern).
func (p *parser) parse() *term {
	p.hasMore = true
	p.capTerms = map[int]*term{}
	p.leakIcase = p.flags.icase
	p.advance()
	t := p.parseDisjunction()
	p.patchNamedRefs()
	return t
}

func (p *parser) newState(prev *pstate, kind groupKind, behind bool, capIdx int, name []uint16, hasName bool, flags modFlags) *pstate {
	p.disjCounter++
	return &pstate{prev: prev, kind: kind, behind: behind, captureIndex: capIdx, name: name, hasName: hasName,
		b: &builder{flags: flags}, disj: p.disjCounter}
}

func (p *parser) parseDisjunction() *term {
	state := p.newState(nil, gInitial, false, 0, nil, false, p.flags)
	b := state.b
	for {
		switch p.cur {
		case endMarker:
			if state.kind != gInitial {
				p.fail(errUnterminatedGroup)
			}
			return b.toRegExp()
		case ')':
			if state.kind == gInitial {
				p.fail(errUnmatchedParen)
			}
			p.advance()
			body := b.toRegExp()
			endCapture := p.captures
			capIdx := state.captureIndex
			switch state.kind {
			case gCapture:
				if state.hasName {
					p.createNamedCapture(state, capIdx)
				}
				ct := p.capTerms[capIdx]
				ct.sub = body
				body = ct
			case gGrouping:
				body = &term{typ: tGroup, sub: body}
			default:
				body = &term{typ: tLook, sub: body, neg: state.kind == gNegLook, behind: state.behind,
					capLo: capIdx, capHi: endCapture}
			}
			state = state.prev
			b = state.b
			p.flags = b.flags
			b.addAtom(body)
		case '|':
			p.advance()
			state.alt++
			b.newAlternative()
			continue
		case '*', '+', '?':
			p.fail(errNothingToRepeat)
		case '^':
			p.advance()
			k := aStartInput
			if b.flags.multiline {
				k = aStartLine
			}
			b.addTerm(&term{typ: tAssert, assert: k})
			continue
		case '$':
			p.advance()
			k := aEndInput
			if b.flags.multiline {
				k = aEndLine
			}
			b.addTerm(&term{typ: tAssert, assert: k})
			continue
		case '.':
			p.advance()
			var set rangeSet
			if b.flags.dotAll {
				set = rangeSet{0, 0x10FFFF}
			} else {
				set = lineTermSet.complement(0x10FFFF)
			}
			b.addAtom(&term{typ: tClass, set: set, icase: p.flags.icase})
		case '(':
			state = p.parseOpenParenthesis(state)
			b = state.b
			p.flags = b.flags
			continue
		case '[':
			b.addAtom(p.parseCharacterClass())
		case '\\':
			if p.parseAtomEscape(state, b) {
				continue
			}
		case '{':
			if _, _, ok := p.parseIntervalQuantifier(); ok {
				p.fail(errNothingToRepeat)
			}
			if p.unicodeMode() {
				p.fail(errLoneQuantifierBrackets)
			}
			b.addAtom(p.charTerm(p.cur))
			p.advance()
		case '}', ']':
			if p.unicodeMode() {
				p.fail(errLoneQuantifierBrackets)
			}
			b.addAtom(p.charTerm(p.cur))
			p.advance()
		default:
			b.addAtom(p.charTerm(p.cur))
			p.advance()
		}

		var min, max int
		switch p.cur {
		case '*':
			min, max = 0, infinity
			p.advance()
		case '+':
			min, max = 1, infinity
			p.advance()
		case '?':
			min, max = 0, 1
			p.advance()
		case '{':
			var ok bool
			if min, max, ok = p.parseIntervalQuantifier(); ok {
				if max < min {
					p.fail(errRangeOutOfOrder)
				}
			} else if p.unicodeMode() {
				p.fail(errIncompleteQuantifier)
			} else {
				continue
			}
		default:
			continue
		}
		greedy := true
		if p.cur == '?' {
			greedy = false
			p.advance()
		}
		if !p.addQuantifier(b, min, max, greedy) {
			p.fail(errInvalidQuantifier)
		}
	}
}

func (p *parser) charTerm(c rune) *term {
	return &term{typ: tChar, c: c, icase: p.flags.icase}
}

// addQuantifier is V8's RegExpBuilder::AddQuantifierToAtom.
func (p *parser) addQuantifier(b *builder, min, max int, greedy bool) bool {
	if b.pendingEmpty {
		b.pendingEmpty = false
		return true
	}
	n := len(b.terms)
	if n == 0 {
		return true
	}
	atom := b.terms[n-1]
	b.terms = b.terms[:n-1]
	if atom.typ == tLook {
		if p.unicode || atom.behind {
			return false
		}
	}
	if atom.maxMatch(p.unicode) == 0 {
		if min == 0 {
			return true
		}
		b.terms = append(b.terms, atom)
		return true
	}
	b.terms = append(b.terms, &term{typ: tQuant, min: min, max: max, greedy: greedy, sub: atom})
	return true
}

// parseAtomEscape handles '\' outside a class. It returns true when the
// result must not be followed by quantifier parsing (assertions).
func (p *parser) parseAtomEscape(state *pstate, b *builder) bool {
	switch nx := p.next(); nx {
	case endMarker:
		p.fail(errEscapeAtEnd)
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if idx, ok := p.parseBackReferenceIndex(); ok {
			if insideCapture(state, idx) {
				b.addAtom(&term{typ: tEmpty})
			} else {
				b.addAtom(&term{typ: tBackref, refs: []int{idx}, icase: p.flags.icase})
			}
			return false
		}
		if p.unicodeMode() {
			p.fail(errInvalidEscape)
		}
		if nx == '8' || nx == '9' {
			b.addAtom(p.charTerm(nx))
			p.advanceN(2)
			return false
		}
		p.advance()
		b.addAtom(p.charTerm(p.parseOctalLiteral()))
		return false
	case '0':
		p.advance()
		if p.unicodeMode() && isDecimal(p.next()) {
			p.fail(errInvalidDecimalEscape)
		}
		b.addAtom(p.charTerm(p.parseOctalLiteral()))
		return false
	case 'b', 'B':
		p.advanceN(2)
		k := aBoundary
		if nx == 'B' {
			k = aNonBoundary
		}
		b.addTerm(&term{typ: tAssert, assert: k, icase: p.flags.icase})
		return true
	case 'd', 'D', 's', 'S', 'w', 'W':
		set := p.classEscapeSet(nx, p.unicodeMode() && p.leakIcase)
		p.advanceN(2)
		b.addAtom(&term{typ: tClass, set: set, icase: p.flags.icase})
		return false
	case 'p', 'P':
		p.advanceN(2)
		if p.unicodeMode() {
			if n1, n2, ok := p.parsePropertyClassName(); ok {
				if set, ok := p.propertySet(n1, n2, nx == 'P'); ok {
					b.addAtom(&term{typ: tClass, set: set, icase: p.flags.icase})
					return false
				}
			}
			p.fail(errInvalidPropertyName)
		}
		b.addAtom(p.charTerm(nx))
		return false
	case 'k':
		if p.unicodeMode() || p.hasNamedCaptures(false) {
			p.advanceN(2)
			p.parseNamedBackReference(state, b)
			return false
		}
	}
	c := p.parseCharacterEscape(false)
	b.addAtom(p.charTerm(c))
	return false
}

func insideCapture(s *pstate, idx int) bool {
	for ; s != nil; s = s.prev {
		if s.kind == gCapture && s.captureIndex == idx {
			return true
		}
	}
	return false
}

func insideNamedCapture(s *pstate, name []uint16) bool {
	for ; s != nil; s = s.prev {
		if s.kind == gCapture && s.hasName && equalU16(s.name, name) {
			return true
		}
	}
	return false
}

func equalU16(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// classEscapeSet returns \d \D \s \S \w \W; with addCaseEquivalents (u + i)
// \w also contains U+017F and U+212A and \W excludes them.
func (p *parser) classEscapeSet(c rune, addCaseEquivalents bool) rangeSet {
	var s rangeSet
	switch c {
	case 'd':
		s = digitSet
	case 'D':
		s = digitSet.complement(0x10FFFF)
	case 's':
		s = spaceSet
	case 'S':
		s = spaceSet.complement(0x10FFFF)
	case 'w', 'W':
		s = wordSet
		if addCaseEquivalents {
			s = wordSetUI
		}
		if c == 'W' {
			s = s.complement(0x10FFFF)
		}
	}
	return append(rangeSet(nil), s...)
}

// parseOpenParenthesis is V8's ParseOpenParenthesis (groups, lookarounds, modifiers).
func (p *parser) parseOpenParenthesis(state *pstate) *pstate {
	behind := state.behind
	kind := gCapture
	flags := state.b.flags
	parsingModifiers := false
	polarity := true
	var modifiers [3]bool
	anyModifier := false
	isNamed := false
	p.advance()
	if p.cur == '?' {
		for {
			switch p.next() {
			case '-':
				p.advance()
				parsingModifiers = true
				if !polarity {
					p.fail(errMultipleFlagDashes)
				}
				polarity = false
			case 'm', 'i', 's':
				p.advance()
				parsingModifiers = true
				k := 0
				switch p.cur {
				case 'm':
					k = 1
				case 's':
					k = 2
				}
				if modifiers[k] {
					p.fail(errRepeatedFlag)
				}
				modifiers[k] = true
				anyModifier = true
				switch k {
				case 0:
					flags.icase = polarity
				case 1:
					flags.multiline = polarity
				case 2:
					flags.dotAll = polarity
				}
			case ':':
				p.advanceN(2)
				parsingModifiers = false
				kind = gGrouping
				if !polarity && !anyModifier {
					p.fail(errInvalidFlagGroup)
				}
			case '=', '!':
				if parsingModifiers {
					p.fail(errInvalidGroup)
				}
				kind = gPosLook
				if p.next() == '!' {
					kind = gNegLook
				}
				p.advanceN(2)
				behind = false
			case '<':
				if parsingModifiers {
					p.fail(errInvalidGroup)
				}
				p.advance()
				switch p.next() {
				case '=':
					p.advanceN(2)
					behind = true
					kind = gPosLook
				case '!':
					p.advanceN(2)
					behind = true
					kind = gNegLook
				default:
					isNamed = true
					p.hasNamed = true
					p.advance()
				}
			default:
				p.fail(errInvalidGroup)
			}
			if !parsingModifiers {
				break
			}
		}
	}
	capIdx := p.captures
	var name []uint16
	if kind == gCapture {
		if p.captures >= maxCaptures {
			p.fail(errTooManyCaptures)
		}
		p.captures++
		capIdx = p.captures
		if isNamed {
			name = p.parseCaptureGroupName()
		}
		ct := &term{typ: tCapture, index: capIdx}
		if isNamed {
			ct.name = fromUTF16(name)
		}
		p.capTerms[capIdx] = ct
	}
	p.leakIcase = flags.icase
	return p.newState(state, kind, behind, capIdx, name, isNamed, flags)
}

// createNamedCapture registers a named group at its ')'. Groups with the same
// name must be in different alternatives of some enclosing disjunction.
func (p *parser) createNamedCapture(state *pstate, idx int) {
	var path [][2]int
	for s := state.prev; s != nil; s = s.prev {
		if s.kind == gCapture && s.hasName && equalU16(s.name, state.name) {
			p.fail(errDuplicateCaptureName)
		}
		path = append(path, [2]int{s.disj, s.alt})
	}
	for _, g := range p.named {
		if !equalU16(g.name, state.name) {
			continue
		}
		compatible := false
		for _, a := range path {
			for _, b := range g.path {
				if a[0] == b[0] && a[1] != b[1] {
					compatible = true
				}
			}
		}
		if !compatible {
			p.fail(errDuplicateCaptureName)
		}
	}
	p.named = append(p.named, namedGroup{name: state.name, index: idx, path: path})
}

var idStart, idContinue uprops.Table

func init() {
	idStart, _ = uprops.Lookup("ID_Start", "")
	idContinue, _ = uprops.Lookup("ID_Continue", "")
}

func isIDStart(c rune) bool {
	return c == '$' || c == '_' || c != endMarker && uprops.Is(idStart, c)
}

func isIDPart(c rune) bool {
	return c == '$' || c == 0x200C || c == 0x200D || c != endMarker && uprops.Is(idContinue, c)
}

func appendCodePointU16(b []uint16, c rune) []uint16 {
	if c >= 0x10000 {
		c -= 0x10000
		return append(b, uint16(0xD800+(c>>10)), uint16(0xDC00+(c&0x3FF)))
	}
	return append(b, uint16(c))
}

// parseCaptureGroupName parses RegExpIdentifierName '>' with the current
// character being the first character of the name. RegExpIdentifierName is
// always parsed in unicode mode (surrogate pairs, \u{...}).
func (p *parser) parseCaptureGroupName() []uint16 {
	saved := p.forceUni
	p.forceUni = true
	defer func() { p.forceUni = saved }()
	name := p.scanCaptureGroupName()
	// Re-read the character after '>' in the pattern's own mode.
	p.forceUni = saved
	p.reset(p.curPos)
	return name
}

func (p *parser) scanCaptureGroupName() []uint16 {
	p.nextPos = p.curPos
	p.hasMore = true
	p.advance()
	var name []uint16
	atStart := true
	for {
		c := p.cur
		p.advance()
		if c == '\\' && p.cur == 'u' {
			p.advance()
			v, ok := p.parseUnicodeEscape()
			if !ok {
				p.fail(errInvalidUnicodeEscape)
			}
			c = v
		} else if c == '\\' {
			p.fail(errInvalidCaptureGroupName)
		}
		if atStart {
			if !isIDStart(c) {
				p.fail(errInvalidCaptureGroupName)
			}
			atStart = false
		} else if c == '>' {
			break
		} else if !isIDPart(c) {
			p.fail(errInvalidCaptureGroupName)
		}
		name = appendCodePointU16(name, c)
	}
	return name
}

func (p *parser) parseNamedBackReference(state *pstate, b *builder) {
	if p.cur != '<' {
		p.fail(errInvalidNamedReference)
	}
	p.advance()
	name := p.parseCaptureGroupName()
	if insideNamedCapture(state, name) {
		b.addAtom(&term{typ: tEmpty})
		return
	}
	r := &term{typ: tBackref, refRaw: name, icase: p.flags.icase}
	p.namedRefs = append(p.namedRefs, r)
	b.addAtom(r)
}

func (p *parser) patchNamedRefs() {
	for _, r := range p.namedRefs {
		for _, g := range p.named {
			if equalU16(g.name, r.refRaw) {
				r.refs = append(r.refs, g.index)
			}
		}
		if len(r.refs) == 0 {
			p.fail(errInvalidNamedCaptureRef)
		}
	}
}

func (p *parser) parseBackReferenceIndex() (int, bool) {
	start := p.position()
	value := int(p.next() - '0')
	p.advanceN(2)
	for isDecimal(p.cur) {
		value = 10*value + int(p.cur-'0')
		if value > maxCaptures {
			p.reset(start)
			return 0, false
		}
		p.advance()
	}
	if value > p.captures {
		if !p.scanned {
			p.scanForCaptures(false)
		}
		if value > p.captureCnt {
			p.reset(start)
			return 0, false
		}
	}
	return value, true
}

// scanForCaptures counts all capture groups and detects named groups (V8 ScanForCaptures).
func (p *parser) scanForCaptures(inClass bool) {
	saved := p.position()
	count := p.captures
	skipClass := func() {
		for p.cur != endMarker {
			c := p.cur
			p.advance()
			if c == '\\' {
				p.advance()
			} else if c == ']' {
				break
			}
		}
	}
	if inClass {
		skipClass()
	}
	for p.cur != endMarker {
		n := p.cur
		p.advance()
		switch n {
		case '\\':
			p.advance()
		case '[':
			skipClass()
		case '(':
			if p.cur == '?' {
				p.advance()
				if p.cur != '<' {
					break
				}
				p.advance()
				if p.cur == '=' || p.cur == '!' {
					break
				}
				p.hasNamed = true
			}
			count++
		}
	}
	p.captureCnt = count
	p.scanned = true
	p.reset(saved)
}

func (p *parser) hasNamedCaptures(inClass bool) bool {
	if p.hasNamed || p.scanned {
		return p.hasNamed
	}
	p.scanForCaptures(inClass)
	return p.hasNamed
}

func (p *parser) parseOctalLiteral() rune {
	value := p.cur - '0'
	p.advance()
	if p.cur >= '0' && p.cur <= '7' {
		value = value*8 + p.cur - '0'
		p.advance()
		if value < 32 && p.cur >= '0' && p.cur <= '7' {
			value = value*8 + p.cur - '0'
			p.advance()
		}
	}
	return value
}

func (p *parser) parseHexEscape(length int) (rune, bool) {
	start := p.position()
	var val rune
	for i := 0; i < length; i++ {
		d := hexValue(p.cur)
		if d < 0 {
			p.reset(start)
			return 0, false
		}
		val = val*16 + rune(d)
		p.advance()
	}
	return val, true
}

func (p *parser) parseUnlimitedHex(max rune) (rune, bool) {
	x := rune(0)
	d := hexValue(p.cur)
	if d < 0 {
		return 0, false
	}
	for d >= 0 {
		x = x*16 + rune(d)
		if x > max {
			return 0, false
		}
		p.advance()
		d = hexValue(p.cur)
	}
	return x, true
}

// parseUnicodeEscape is called after "\u" (V8 ParseUnicodeEscape).
func (p *parser) parseUnicodeEscape() (rune, bool) {
	if p.cur == '{' && p.unicodeMode() {
		start := p.position()
		p.advance()
		if v, ok := p.parseUnlimitedHex(0x10FFFF); ok && p.cur == '}' {
			p.advance()
			return v, true
		}
		p.reset(start)
		return 0, false
	}
	v, ok := p.parseHexEscape(4)
	if ok && p.unicodeMode() && isLead(uint16(v)) && p.cur == '\\' {
		start := p.position()
		if p.next() == 'u' {
			p.advanceN(2)
			if t, ok2 := p.parseHexEscape(4); ok2 && isTrail(uint16(t)) {
				return 0x10000 + (v-0xD800)<<10 + (t - 0xDC00), true
			}
		}
		p.reset(start)
	}
	return v, ok
}

// parseCharacterEscape is called on '\' (V8 ParseCharacterEscape).
func (p *parser) parseCharacterEscape(inClass bool) rune {
	p.advance()
	c := p.cur
	switch c {
	case 'f':
		p.advance()
		return '\f'
	case 'n':
		p.advance()
		return '\n'
	case 'r':
		p.advance()
		return '\r'
	case 't':
		p.advance()
		return '\t'
	case 'v':
		p.advance()
		return '\v'
	case 'c':
		ctl := p.next()
		letter := ctl &^ ('A' ^ 'a')
		if letter >= 'A' && letter <= 'Z' {
			p.advanceN(2)
			return ctl & 0x1F
		}
		if p.unicodeMode() {
			p.fail(errInvalidUnicodeEscape)
		}
		if inClass && (isDecimal(ctl) || ctl == '_') {
			p.advanceN(2)
			return ctl & 0x1F
		}
		return '\\'
	case '0':
		if !isDecimal(p.next()) {
			p.advance()
			return 0
		}
		fallthrough
	case '1', '2', '3', '4', '5', '6', '7':
		if p.unicodeMode() {
			p.fail(errInvalidDecimalEscape)
		}
		return p.parseOctalLiteral()
	case 'x':
		p.advance()
		if v, ok := p.parseHexEscape(2); ok {
			return v
		}
		if p.unicodeMode() {
			p.fail(errInvalidEscape)
		}
		return 'x'
	case 'u':
		p.advance()
		if v, ok := p.parseUnicodeEscape(); ok {
			return v
		}
		if p.unicodeMode() {
			p.fail(errInvalidUnicodeEscape)
		}
		return 'u'
	}
	if !p.unicodeMode() {
		if c == 'k' && inClass && p.hasNamedCaptures(true) {
			p.fail(errInvalidEscape)
		}
		p.advance()
		return c
	}
	if isSyntaxOrSlash(c) {
		p.advance()
		return c
	}
	p.fail(errInvalidEscape)
	return 0
}

// parseIntervalQuantifier parses {n}, {n,}, {n,m} at '{'. On failure the
// position is restored.
func (p *parser) parseIntervalQuantifier() (int, int, bool) {
	start := p.position()
	p.advance()
	if !isDecimal(p.cur) {
		p.reset(start)
		return 0, 0, false
	}
	readNum := func() int {
		v := 0
		for isDecimal(p.cur) {
			d := int(p.cur - '0')
			if v > (infinity-d)/10 {
				for isDecimal(p.cur) {
					p.advance()
				}
				return infinity
			}
			v = 10*v + d
			p.advance()
		}
		return v
	}
	min := readNum()
	max := 0
	switch p.cur {
	case '}':
		max = min
		p.advance()
	case ',':
		p.advance()
		if p.cur == '}' {
			max = infinity
			p.advance()
		} else {
			max = readNum()
			if p.cur != '}' {
				p.reset(start)
				return 0, 0, false
			}
			p.advance()
		}
	default:
		p.reset(start)
		return 0, 0, false
	}
	return min, max, true
}

// parsePropertyClassName parses {name} or {name=value} after \p.
func (p *parser) parsePropertyClassName() (string, string, bool) {
	if p.cur != '{' {
		return "", "", false
	}
	var n1, n2 []byte
	hasValue := false
	for p.advance(); p.cur != '}' && p.cur != '='; p.advance() {
		if !isPropChar(p.cur) || !p.hasNext() {
			return "", "", false
		}
		n1 = append(n1, byte(p.cur))
	}
	if p.cur == '=' {
		hasValue = true
		for p.advance(); p.cur != '}'; p.advance() {
			if !isPropChar(p.cur) || !p.hasNext() {
				return "", "", false
			}
			n2 = append(n2, byte(p.cur))
		}
	}
	p.advance()
	if hasValue && len(n2) == 0 {
		return string(n1), "\x00", true
	}
	return string(n1), string(n2), true
}

func isPropChar(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// propertySet resolves \p{n1=n2} (negated for \P).
func (p *parser) propertySet(n1, n2 string, negate bool) (rangeSet, bool) {
	if n2 == "\x00" {
		return nil, false
	}
	t, ok := uprops.Lookup(n1, n2)
	if !ok {
		return nil, false
	}
	s := tableSet(t)
	if negate {
		s = s.complement(0x10FFFF)
	}
	return s, true
}

// parseCharacterClass parses a class at '['.
func (p *parser) parseCharacterClass() *term {
	p.advance()
	neg := false
	if p.cur == '^' {
		neg = true
		p.advance()
	}
	addCase := p.unicodeMode() && p.leakIcase
	var set rangeSet
	for p.hasMore && p.cur != ']' {
		c1, isClass1 := p.parseClassEscape(&set, addCase)
		if p.cur == '-' {
			p.advance()
			if p.cur == endMarker {
				break
			} else if p.cur == ']' {
				if !isClass1 {
					set.add(c1, c1)
				}
				set.add('-', '-')
				break
			}
			c2, isClass2 := p.parseClassEscape(&set, addCase)
			if isClass1 || isClass2 {
				if p.unicodeMode() {
					p.fail(errInvalidCharacterClass)
				}
				if !isClass1 {
					set.add(c1, c1)
				}
				set.add('-', '-')
				if !isClass2 {
					set.add(c2, c2)
				}
				continue
			}
			if c1 > c2 {
				p.fail(errOutOfOrderClass)
			}
			set.add(c1, c2)
		} else if !isClass1 {
			set.add(c1, c1)
		}
	}
	if !p.hasMore {
		p.fail(errUnterminatedClass)
	}
	p.advance()
	// V8 quirk: in u mode a class is case-insensitive only if the leaked i
	// flag (see leakIcase) is set as well.
	icase := p.flags.icase && (!p.unicodeMode() || p.leakIcase)
	return &term{typ: tClass, set: set.norm(), neg: neg, icase: icase}
}

// parseClassEscape reads one class atom; class escapes are added to set.
func (p *parser) parseClassEscape(set *rangeSet, addCase bool) (rune, bool) {
	if p.cur != '\\' {
		c := p.cur
		p.advance()
		return c, false
	}
	switch p.next() {
	case 'b':
		p.advanceN(2)
		return '\b', false
	case '-':
		if p.unicodeMode() {
			p.advanceN(2)
			return '-', false
		}
	case endMarker:
		p.fail(errEscapeAtEnd)
	}
	if s, ok := p.tryClassEscape(addCase); ok {
		set.addSet(s)
		return 0, true
	}
	return p.parseCharacterEscape(true), false
}

// tryClassEscape handles \d \D \s \S \w \W and (unicode mode) \p \P inside a
// class at '\'.
func (p *parser) tryClassEscape(addCase bool) (rangeSet, bool) {
	nx := p.next()
	switch nx {
	case 'd', 'D', 's', 'S', 'w', 'W':
		s := p.classEscapeSet(nx, addCase)
		p.advanceN(2)
		return s, true
	case 'p', 'P':
		if !p.unicodeMode() {
			return nil, false
		}
		p.advanceN(2)
		if n1, n2, ok := p.parsePropertyClassName(); ok {
			if s, ok := p.propertySet(n1, n2, nx == 'P'); ok {
				return s, true
			}
		}
		p.fail(errInvalidClassProperty)
	}
	return nil, false
}
