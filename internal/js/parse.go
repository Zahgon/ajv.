package js

import (
	"strconv"
)

// Parse is JSON.parse(text). Objects become *Object (in JavaScript key order, a repeated key
// keeping its first position and its last value), arrays Array, numbers float64, and strings,
// booleans and null string, bool and nil. A malformed text returns an *Error named
// "SyntaxError" whose message is V8's.
func Parse(text string) (v any, err error) {
	p := &jsonParser{u: CodeUnits(text)}
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			v, err = nil, e
		}
	}()
	v = p.value()
	p.space()
	if p.i < len(p.u) {
		p.fail(msgNonWhiteSpace)
	}
	return v, nil
}

type jsonToken uint8

const (
	tokIllegal jsonToken = iota
	tokEOS
	tokString
	tokNumber
	tokTrue
	tokFalse
	tokNull
	tokWhitespace
	tokColon
	tokComma
	tokLBrack
	tokRBrack
	tokLBrace
	tokRBrace
)

// tokenOf is V8's one_char_json_tokens lookup; c < 0 is the end of the input.
func tokenOf(c int) jsonToken {
	switch {
	case c < 0:
		return tokEOS
	case c == '"':
		return tokString
	case c == '-' || (c >= '0' && c <= '9'):
		return tokNumber
	case c == 't':
		return tokTrue
	case c == 'f':
		return tokFalse
	case c == 'n':
		return tokNull
	case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		return tokWhitespace
	case c == ':':
		return tokColon
	case c == ',':
		return tokComma
	case c == '[':
		return tokLBrack
	case c == ']':
		return tokRBrack
	case c == '{':
		return tokLBrace
	case c == '}':
		return tokRBrace
	}
	return tokIllegal
}

// Message templates of V8's JSON parser that carry a position.
const (
	msgNonWhiteSpace        = "Unexpected non-whitespace character after JSON"
	msgPropNameOrRBrace     = "Expected property name or '}'"
	msgCommaOrRBrack        = "Expected ',' or ']' after array element"
	msgCommaOrRBrace        = "Expected ',' or '}' after property value"
	msgColon                = "Expected ':' after property name"
	msgDoubleQuotedPropName = "Expected double-quoted property name"
	msgUnterminatedString   = "Unterminated string"
	msgBadControl           = "Bad control character in string literal"
	msgBadEscape            = "Bad escaped character"
	msgBadUnicodeEscape     = "Bad Unicode escape"
	msgNoNumberAfterMinus   = "No number after minus sign"
	msgExponentMissing      = "Exponent part is missing a number"
	msgUnterminatedFraction = "Unterminated fractional number"
	msgUnexpectedNumber     = "Unexpected number"
	msgUnexpectedString     = "Unexpected string"
)

// maxContext is V8's kMaxContextCharacters.
const maxContext = 10

type jsonParser struct {
	u []uint16
	i int
}

func (p *jsonParser) cur() int {
	if p.i < len(p.u) {
		return int(p.u[p.i])
	}
	return -1
}

func (p *jsonParser) space() {
	for p.i < len(p.u) {
		switch p.u[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

// fail reports an error at the current position with an explicit message template.
func (p *jsonParser) fail(msg string) {
	if msg != msgNonWhiteSpace {
		msg += " in JSON"
	}
	panic(&Error{Name: "SyntaxError", Message: msg + p.where()})
}

func (p *jsonParser) where() string {
	line, lastBreak := 1, 0
	c := 0
	for ; c < p.i; c++ {
		if p.u[c] == '\r' && c < p.i-1 && p.u[c+1] == '\n' {
			c++
		}
		if p.u[c] == '\r' || p.u[c] == '\n' {
			line++
			lastBreak = c + 1
		}
	}
	return " at position " + strconv.Itoa(p.i) + " (line " + strconv.Itoa(line) +
		" column " + strconv.Itoa(1+c-lastBreak) + ")"
}

// unexpectedChar is V8's ReportUnexpectedCharacter for the code unit at the cursor (or the end).
func (p *jsonParser) unexpectedChar() {
	c := p.cur()
	if c > 0xff {
		p.unexpected(tokIllegal)
	}
	p.unexpected(tokenOf(c))
}

// unexpected is V8's ReportUnexpectedToken without an explicit message.
func (p *jsonParser) unexpected(tok jsonToken) {
	switch tok {
	case tokEOS:
		panic(&Error{Name: "SyntaxError", Message: "Unexpected end of JSON input"})
	case tokNumber:
		p.fail(msgUnexpectedNumber)
	case tokString:
		p.fail(msgUnexpectedString)
	}
	src := FromCodeUnits(p.u)
	switch src {
	case "[object Object]", "undefined", "Infinity", "NaN":
		panic(&Error{Name: "SyntaxError", Message: "\"" + src + "\" is not valid JSON"})
	}
	ch := FromCodeUnits(p.u[p.i : p.i+1])
	n, pos := len(p.u), p.i
	var msg string
	switch {
	case n < 2*maxContext+1:
		msg = "Unexpected token '" + ch + "', \"" + src + "\" is not valid JSON"
	case pos < maxContext:
		msg = "Unexpected token '" + ch + "', \"" + FromCodeUnits(p.u[:pos+maxContext]) + "\"... is not valid JSON"
	case pos < n-maxContext:
		msg = "Unexpected token '" + ch + "', ...\"" + FromCodeUnits(p.u[pos-maxContext:pos+maxContext]) + "\"... is not valid JSON"
	default:
		msg = "Unexpected token '" + ch + "', ...\"" + FromCodeUnits(p.u[pos-maxContext:]) + "\" is not valid JSON"
	}
	panic(&Error{Name: "SyntaxError", Message: msg})
}

func (p *jsonParser) value() any {
	p.space()
	switch tok := tokenOf(p.cur()); tok {
	case tokString:
		return p.str()
	case tokNumber:
		return p.number()
	case tokLBrace:
		p.i++
		obj := NewObject()
		p.space()
		if p.cur() == '}' {
			p.i++
			return obj
		}
		if p.cur() != '"' {
			p.fail(msgPropNameOrRBrace)
		}
		for {
			key := p.str()
			p.space()
			if p.cur() != ':' {
				p.fail(msgColon)
			}
			p.i++
			obj.Set(key, p.value())
			p.space()
			switch p.cur() {
			case ',':
				p.i++
				p.space()
				if p.cur() != '"' {
					p.fail(msgDoubleQuotedPropName)
				}
				continue
			case '}':
				p.i++
				return obj
			}
			p.fail(msgCommaOrRBrace)
		}
	case tokLBrack:
		p.i++
		arr := Array{}
		p.space()
		if p.cur() == ']' {
			p.i++
			return arr
		}
		for {
			arr = append(arr, p.value())
			p.space()
			switch p.cur() {
			case ',':
				p.i++
				continue
			case ']':
				p.i++
				return arr
			}
			p.fail(msgCommaOrRBrack)
		}
	case tokTrue:
		p.literal("true")
		return true
	case tokFalse:
		p.literal("false")
		return false
	case tokNull:
		p.literal("null")
		return nil
	default:
		p.unexpected(tok)
	}
	return nil
}

// literal is V8's ScanLiteral; the first character has already been matched.
func (p *jsonParser) literal(lit string) {
	remaining := len(p.u) - p.i
	if remaining >= len(lit) {
		ok := true
		for k := 1; k < len(lit); k++ {
			if p.u[p.i+k] != uint16(lit[k]) {
				ok = false
				break
			}
		}
		if ok {
			p.i += len(lit)
			return
		}
	}
	p.i++
	for k := 0; k < min(len(lit)-1, remaining-1); k++ {
		if p.u[p.i] != uint16(lit[1+k]) {
			p.unexpectedChar()
		}
		p.i++
	}
	p.unexpected(tokEOS)
}

func isHex(c int) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (p *jsonParser) str() string {
	p.i++
	var out []uint16
	start := p.i
	for {
		if p.i >= len(p.u) {
			p.fail(msgUnterminatedString)
		}
		c := p.u[p.i]
		switch {
		case c == '"':
			out = append(out, p.u[start:p.i]...)
			p.i++
			return FromCodeUnits(out)
		case c == '\\':
			out = append(out, p.u[start:p.i]...)
			p.i++
			e := p.cur()
			if e < 0 || e > 0xff {
				p.unexpectedChar()
			}
			switch e {
			case '"', '\\', '/':
				out = append(out, uint16(e))
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				var v uint16
				for range 4 {
					p.i++
					h := p.cur()
					if !isHex(h) {
						p.fail(msgBadUnicodeEscape)
					}
					v = v*16 + uint16(digitValue(byte(h)))
				}
				out = append(out, v)
			default:
				p.fail(msgBadEscape)
			}
			p.i++
			start = p.i
		case c < 0x20:
			p.fail(msgBadControl)
		default:
			p.i++
		}
	}
}

func isDigit(c int) bool { return c >= '0' && c <= '9' }

func (p *jsonParser) number() float64 {
	start := p.i
	if p.cur() == '-' {
		p.i++
	}
	if p.cur() == '0' {
		p.i++
		if isDigit(p.cur()) {
			p.fail(msgUnexpectedNumber)
		}
	} else {
		if !isDigit(p.cur()) {
			p.fail(msgNoNumberAfterMinus)
		}
		for isDigit(p.cur()) {
			p.i++
		}
	}
	if p.cur() == '.' {
		p.i++
		if !isDigit(p.cur()) {
			p.fail(msgUnterminatedFraction)
		}
		for isDigit(p.cur()) {
			p.i++
		}
	}
	if c := p.cur(); c == 'e' || c == 'E' {
		p.i++
		if c := p.cur(); c == '+' || c == '-' {
			p.i++
		}
		if !isDigit(p.cur()) {
			p.fail(msgExponentMissing)
		}
		for isDigit(p.cur()) {
			p.i++
		}
	}
	b := make([]byte, p.i-start)
	for k := range b {
		b[k] = byte(p.u[start+k])
	}
	return parseDecimal(string(b))
}
