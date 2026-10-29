// Package jsregex implements ECMAScript regular expressions with the exact
// syntax, semantics and error messages of V8's RegExp (Node.js 24, ICU 78,
// Unicode 17).
//
// Strings are Go strings in WTF-8 (lone surrogates encoded as 3-byte
// sequences); every index in the API is a UTF-16 code-unit index.
// Non-unicode patterns match UTF-16 code units, u patterns code points.
// Only what new RegExp(source, flags).test(s) needs is provided, for the
// flags i, m, s and u (the d, g, v and y flags are not supported). A *Regexp
// is immutable and safe for concurrent use.
package jsregex

import (
	"fmt"
	"strings"
	"sync"
)

// SyntaxError is the error returned for invalid patterns or flags. Message
// is exactly the message of the SyntaxError V8 throws.
type SyntaxError struct{ Message string }

// Error returns the message.
func (e *SyntaxError) Error() string { return e.Message }

// Regexp is a compiled ECMAScript regular expression.
type Regexp struct {
	prog                   mnode
	ncap                   int
	nloops                 int
	anchorStart, anchorEnd bool // every alternative starts with ^ / ends with $ (without m)
	maxLen                 int  // maximum match length in code units (infinity if unbounded)
	pool                   sync.Pool
}

func flagsError(flags string) error {
	return &SyntaxError{Message: "Invalid flags supplied to RegExp constructor '" + flags + "'"}
}

// Compile is new RegExp(source, flags). The flags i, m, s and u are supported; any other flag
// (d, g, v, y), or a repeated one, is rejected as invalid.
func Compile(source, flags string) (re *Regexp, err error) {
	var icase, multiline, dotAll, unicode bool
	seen := map[rune]bool{}
	for _, f := range flags {
		if seen[f] {
			return nil, flagsError(flags)
		}
		seen[f] = true
		switch f {
		case 'i':
			icase = true
		case 'm':
			multiline = true
		case 's':
			dotAll = true
		case 'u':
			unicode = true
		default:
			return nil, flagsError(flags)
		}
	}
	var b strings.Builder
	for _, f := range "imsu" {
		if seen[f] {
			b.WriteRune(f)
		}
	}
	canonFlags := b.String()
	defer func() {
		if r := recover(); r != nil {
			re = nil
			if pe, ok := r.(parseError); ok {
				err = &SyntaxError{Message: "Invalid regular expression: /" + source + "/" + canonFlags + ": " + pe.msg}
				return
			}
			err = fmt.Errorf("jsregex: internal error compiling /%s/%s: %v", source, flags, r)
		}
	}()
	p := &parser{in: toUTF16(nil, source), unicode: unicode,
		flags: modFlags{icase: icase, multiline: multiline, dotAll: dotAll}}
	ast := p.parse()
	re = &Regexp{ncap: p.captures}
	c := &compiler{unicode: unicode, ncap: re.ncap}
	re.prog = c.comp(ast, acceptNode{}, false)
	re.anchorStart, re.anchorEnd = anchoredAt(ast, false), anchoredAt(ast, true)
	re.maxLen = ast.maxMatch(unicode)
	re.nloops = c.nloops
	return re, nil
}

func (re *Regexp) getMachine(in []uint16) *machine {
	m, _ := re.pool.Get().(*machine)
	if m == nil {
		m = &machine{caps: make([]int, 2*(re.ncap+1)), open: make([]int, re.ncap+1), loops: make([]loopState, re.nloops)}
	}
	m.in = in
	m.stack = m.stack[:0]
	return m
}

func (re *Regexp) putMachine(m *machine) {
	m.in = nil
	if cap(m.ubuf) > 1<<12 {
		m.ubuf = nil
	}
	re.pool.Put(m)
}

// run searches for a match starting at 0.
func (re *Regexp) run(m *machine) bool {
	n := len(m.in)
	start := 0
	// No match can start elsewhere (V8 makes the same deductions).
	anchored := re.anchorStart
	if re.anchorEnd && re.maxLen < infinity && n-re.maxLen > start {
		start = n - re.maxLen
	}
	for {
		if re.attempt(m, start) {
			return true
		}
		if anchored || start >= n {
			return false
		}
		// V8 retries at every code unit, even inside surrogate pairs.
		start++
	}
}

// attempt tries to match at exactly start.
func (re *Regexp) attempt(m *machine, start int) bool {
	for k := range m.caps {
		m.caps[k] = -1
	}
	if re.prog.match(m, start) {
		m.caps[0], m.caps[1] = start, m.matchEnd
		return true
	}
	return false
}

// Test is re.test(s).
func (re *Regexp) Test(s string) bool {
	m := re.getMachine(nil)
	m.ubuf = toUTF16(m.ubuf[:0], s)
	m.in = m.ubuf
	ok := re.run(m)
	re.putMachine(m)
	return ok
}

// anchoredAt reports whether every alternative of t begins with ^ (or, with
// end, ends with $), outside lookarounds and without the m flag.
func anchoredAt(t *term, end bool) bool {
	switch t.typ {
	case tAssert:
		return (!end && t.assert == aStartInput) || (end && t.assert == aEndInput)
	case tCapture, tGroup:
		return anchoredAt(t.sub, end)
	case tSeq:
		if end {
			return anchoredAt(t.subs[len(t.subs)-1], end)
		}
		return anchoredAt(t.subs[0], end)
	case tAlt:
		for _, a := range t.subs {
			if !anchoredAt(a, end) {
				return false
			}
		}
		return true
	}
	return false
}
