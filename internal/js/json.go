package js

const hexDigits = "0123456789abcdef"

// Quote is JSON.stringify(s) for a string: it escapes '"', '\\', \b \f \n \r \t, the other C0
// controls as \u00xx and lone surrogates as \udxxx (lowercase hex), and nothing else.
func Quote(s string) string { return string(AppendQuote(nil, s)) }

// AppendQuote appends Quote(s) to b.
func AppendQuote(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x20 && c < 0x80 && c != '"' && c != '\\' {
			i++
			continue
		}
		if c >= 0x80 {
			r, size := decodeUnit(s, i)
			switch {
			case r >= 0xd800 && r < 0xdc00 && i+size < len(s):
				if r2, size2 := decodeUnit(s, i+size); r2 >= 0xdc00 && r2 < 0xe000 {
					// A pair spelled as two lone-surrogate forms.
					b = append(b, s[start:i]...)
					p := (r-0xd800)<<10 + (r2 - 0xdc00) + 0x10000
					b = appendCodePoint(b, p)
					i += size + size2
					start = i
					continue
				}
				fallthrough
			case r >= 0xd800 && r < 0xe000:
				b = append(b, s[start:i]...)
				b = append(b, '\\', 'u', hexDigits[r>>12], hexDigits[r>>8&0xf], hexDigits[r>>4&0xf], hexDigits[r&0xf])
				i += size
				start = i
				continue
			case r == 0xfffd && size == 1:
				b = append(b, s[start:i]...)
				b = append(b, "\ufffd"...)
				i++
				start = i
				continue
			}
			i += size
			continue
		}
		b = append(b, s[start:i]...)
		switch c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\f':
			b = append(b, '\\', 'f')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		}
		i++
		start = i
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}
