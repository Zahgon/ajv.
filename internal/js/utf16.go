package js

import (
	"unicode/utf8"
)

// Error is a JavaScript exception: Name is the constructor name ("SyntaxError", "RangeError",
// "TypeError") and Message the text V8 produces.
type Error struct {
	Name    string
	Message string
}

// Error returns the text of String(error) in JavaScript, "Name: Message".
func (e *Error) Error() string { return e.Name + ": " + e.Message }

// isASCII reports whether s contains only ASCII bytes.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// decodeUnit decodes the code point starting at s[i] and returns it with its byte size. Lone
// surrogates are returned as themselves; invalid bytes decode to U+FFFD with size 1.
func decodeUnit(s string, i int) (rune, int) {
	c := s[i]
	if c < 0x80 {
		return rune(c), 1
	}
	if c == 0xed && i+2 < len(s) && s[i+1] >= 0xa0 && s[i+1] <= 0xbf && s[i+2] >= 0x80 && s[i+2] <= 0xbf {
		return 0xd000 | rune(s[i+1]&0x3f)<<6 | rune(s[i+2]&0x3f), 3
	}
	return utf8.DecodeRuneInString(s[i:])
}

// Len returns the length of s in UTF-16 code units (String.prototype.length).
func Len(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			n++
			i++
			continue
		}
		r, size := decodeUnit(s, i)
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
		i += size
	}
	return n
}

// CodeUnits returns the UTF-16 code units of s.
func CodeUnits(s string) []uint16 {
	out := make([]uint16, 0, len(s))
	return appendUnits(out, s)
}

func appendUnits(out []uint16, s string) []uint16 {
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			out = append(out, uint16(s[i]))
			i++
			continue
		}
		r, size := decodeUnit(s, i)
		i += size
		if r >= 0x10000 {
			r -= 0x10000
			out = append(out, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}

// FromCodeUnits returns the string made of the UTF-16 code units u, in canonical WTF-8.
func FromCodeUnits(u []uint16) string {
	return string(appendWTF8(make([]byte, 0, len(u)), u))
}

func appendWTF8(b []byte, u []uint16) []byte {
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c < 0x80:
			b = append(b, byte(c))
		case c >= 0xd800 && c < 0xdc00 && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] < 0xe000:
			b = utf8.AppendRune(b, (rune(c)-0xd800)<<10+(rune(u[i+1])-0xdc00)+0x10000)
			i++
		default:
			b = appendCodePoint(b, rune(c))
		}
	}
	return b
}

// appendCodePoint appends the WTF-8 encoding of r, which may be a surrogate.
func appendCodePoint(b []byte, r rune) []byte {
	if r >= 0xd800 && r < 0xe000 {
		return append(b, 0xed, byte(0x80|(r>>6)&0x3f), byte(0x80|r&0x3f))
	}
	return utf8.AppendRune(b, r)
}

// codePoints decodes s into code points, joining surrogate pairs (also pairs spelled as two
// lone-surrogate byte forms) and keeping lone surrogates as themselves.
func codePoints(s string) []rune {
	out := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		r, size := decodeUnit(s, i)
		i += size
		if r >= 0xd800 && r < 0xdc00 && i < len(s) {
			if r2, size2 := decodeUnit(s, i); r2 >= 0xdc00 && r2 < 0xe000 {
				r = (r-0xd800)<<10 + (r2 - 0xdc00) + 0x10000
				i += size2
			}
		}
		out = append(out, r)
	}
	return out
}
