package jsregex

import "unicode/utf8"

// decodeWTF8 decodes one WTF-8 sequence (UTF-8 plus 3-byte encoded
// surrogates). Invalid bytes decode as U+FFFD of width 1.
func decodeWTF8(s string, i int) (rune, int) {
	b := s[i]
	if b < 0x80 {
		return rune(b), 1
	}
	if b == 0xED && i+2 < len(s) {
		b1, b2 := s[i+1], s[i+2]
		if b1 >= 0xA0 && b1 <= 0xBF && b2 >= 0x80 && b2 <= 0xBF {
			return 0xD000 | rune(b1&0x3F)<<6 | rune(b2&0x3F), 3
		}
	}
	return utf8.DecodeRuneInString(s[i:])
}

// toUTF16 appends the UTF-16 code units of the WTF-8 string s to buf.
func toUTF16(buf []uint16, s string) []uint16 {
	for i := 0; i < len(s); {
		if c := s[i]; c < 0x80 {
			buf = append(buf, uint16(c))
			i++
			continue
		}
		r, n := decodeWTF8(s, i)
		i += n
		if r >= 0x10000 {
			r -= 0x10000
			buf = append(buf, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			buf = append(buf, uint16(r))
		}
	}
	return buf
}

// appendWTF8 appends the code point r (which may be a lone surrogate) to b.
func appendWTF8(b []byte, r rune) []byte {
	if r >= 0xD800 && r <= 0xDFFF {
		return append(b, 0xED, byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	}
	return utf8.AppendRune(b, r)
}

// fromUTF16 converts UTF-16 code units to a WTF-8 string.
func fromUTF16(u []uint16) string {
	ascii := true
	for _, c := range u {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		b := make([]byte, len(u))
		for i, c := range u {
			b[i] = byte(c)
		}
		return string(b)
	}
	b := make([]byte, 0, len(u)*2)
	for i := 0; i < len(u); i++ {
		c := rune(u[i])
		if isLead(u[i]) && i+1 < len(u) && isTrail(u[i+1]) {
			c = 0x10000 + (c-0xD800)<<10 + rune(u[i+1]-0xDC00)
			i++
		}
		b = appendWTF8(b, c)
	}
	return string(b)
}

func isLead(c uint16) bool  { return c >= 0xD800 && c <= 0xDBFF }
func isTrail(c uint16) bool { return c >= 0xDC00 && c <= 0xDFFF }
