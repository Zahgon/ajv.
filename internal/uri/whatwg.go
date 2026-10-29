package uri

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// whatwgHostname returns new URL('http://' + host).hostname as computed by Node 26.10, whose
// URL parser is ada 4.0.0 (url_aggregator::parse_host), and false where the constructor throws.
// The caller has already rejected hosts containing any of fast-uri's forbidden domain host
// characters, so the URL parser hands the whole string to the host parser.
func whatwgHostname(host string) (string, bool) {
	input := toUTF8(host)
	if input == "" {
		return "", false
	}
	if input[0] == '[' {
		// IPv6 hosts are never passed here (isIPLiteral excludes them and "[" is forbidden).
		return "", false
	}
	// Fast path: pure decimal IPv4.
	if _, ok := parseIPv4Fast(input); ok {
		return strings.TrimSuffix(input, "."), true
	}
	if !containsForbiddenDomainCodePointOrUpper(input) && !strings.Contains(input, "xn-") {
		if isIPv4Ending(input) {
			return parseWHATWGIPv4(input)
		}
		return input, true
	}
	ascii, ok := unicodeToASCII(input)
	if !ok {
		return "", false
	}
	if isIPv4Ending(ascii) {
		return parseWHATWGIPv4(ascii)
	}
	return ascii, true
}

// toUTF8 converts a WTF-8 string to the UTF-8 that V8 hands to ada: a surrogate pair spelled
// as two three-byte forms becomes one code point and a lone surrogate becomes U+FFFD.
func toUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := decodeUnit(s, i)
		if r >= 0xd800 && r <= 0xdbff && i+size < len(s) {
			if lo, lsize := decodeUnit(s, i+size); lo >= 0xdc00 && lo <= 0xdfff {
				b.WriteRune(0x10000 + ((r - 0xd800) << 10) + (lo - 0xdc00))
				i += size + lsize
				continue
			}
		}
		if r >= 0xd800 && r <= 0xdfff {
			r = utf8.RuneError
		}
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

// isForbiddenDomainCodePoint is the WHATWG forbidden domain code point set over bytes, as in
// ada's is_forbidden_domain_code_point_table (every non-ASCII byte counts as forbidden).
func isForbiddenDomainCodePoint(c byte) bool {
	return c <= 0x20 || c >= 0x7f || strings.IndexByte("#%/:<>?@[\\]^|", c) >= 0
}

// containsForbiddenDomainCodePointOrUpper is ada's contains_forbidden_domain_code_point_or_upper.
func containsForbiddenDomainCodePointOrUpper(s string) bool {
	for i := 0; i < len(s); i++ {
		if isForbiddenDomainCodePoint(s[i]) || (s[i] >= 'A' && s[i] <= 'Z') {
			return true
		}
	}
	return false
}

// unicodeToASCII is ada::unicode::to_ascii: percent-decode, ada::idna::to_ascii, and reject a
// result that is empty or holds a forbidden domain code point.
func unicodeToASCII(plain string) (string, bool) {
	input := plain
	if strings.IndexByte(plain, '%') != -1 {
		input = percentDecode(plain)
	}
	out, ok := idnaToASCII(input)
	if !ok || out == "" {
		return "", false
	}
	for i := 0; i < len(out); i++ {
		if isForbiddenDomainCodePoint(out[i]) {
			return "", false
		}
	}
	return out, true
}

// percentDecode is ada::unicode::percent_decode.
func percentDecode(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b = append(b, hexVal(s[i+1])<<4|hexVal(s[i+2]))
			i += 2
			continue
		}
		b = append(b, s[i])
	}
	return string(b)
}

// parseIPv4Fast is ada's checkers::try_parse_ipv4_fast: a pure dotted-decimal address with an
// optional single trailing dot.
func parseIPv4Fast(s string) (uint32, bool) {
	if len(s) < 7 || len(s) > 16 {
		return 0, false
	}
	p := 0
	var ipv4 uint32
	for i := 0; i < 4; i++ {
		if p == len(s) || !isDigit(s[p]) {
			return 0, false
		}
		val := uint32(s[p] - '0')
		p++
		if p < len(s) && isDigit(s[p]) {
			if val == 0 {
				return 0, false
			}
			val = val*10 + uint32(s[p]-'0')
			p++
			if p < len(s) && isDigit(s[p]) {
				val = val*10 + uint32(s[p]-'0')
				p++
				if val > 255 {
					return 0, false
				}
			}
		}
		ipv4 = ipv4<<8 | val
		if i < 3 {
			if p == len(s) || s[p] != '.' {
				return 0, false
			}
			p++
		}
	}
	if p != len(s) {
		if p == len(s)-1 && s[p] == '.' {
			return ipv4, true
		}
		return 0, false
	}
	return ipv4, true
}

// isIPv4Ending is ada's checkers::is_ipv4 (the WHATWG "ends in a number" checker).
func isIPv4Ending(view string) bool {
	if strings.HasSuffix(view, ".") {
		view = view[:len(view)-1]
		if view == "" {
			return false
		}
	}
	if view == "" {
		return false
	}
	last := view[len(view)-1]
	if !(isDigit(last) || (last >= 'a' && last <= 'f') || last == 'x') {
		return false
	}
	if dot := strings.LastIndexByte(view, '.'); dot != -1 {
		view = view[dot+1:]
	}
	allDigits := true
	for i := 0; i < len(view); i++ {
		if !isDigit(view[i]) {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}
	if len(view) == 1 || !strings.HasPrefix(view, "0x") {
		return false
	}
	for i := 2; i < len(view); i++ {
		if c := view[i]; !(isDigit(c) || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// parseIPv4Number is ada's detail::parse_ipv4_number. It returns the value, whether the part
// was pure decimal, the new position and success.
func parseIPv4Number(s string, p int) (uint64, int, bool) {
	end := len(s)
	if p >= end {
		return 0, p, false
	}
	if end-p >= 2 && s[p] == '0' && (s[p+1]|0x20) == 'x' {
		p += 2
		if p == end || s[p] == '.' {
			return 0, p, true
		}
		var v uint64
		digits := 0
		for p < end && s[p] != '.' {
			if !isHex(s[p]) {
				return 0, p, false
			}
			if v > 0xFFFFFFFF>>4 {
				return 0, p, false
			}
			v = v<<4 | uint64(hexVal(s[p]))
			p++
			digits++
		}
		if digits == 0 {
			return 0, p, false
		}
		return v, p, true
	}
	if end-p >= 2 && s[p] == '0' && isDigit(s[p+1]) {
		p++
		var v uint64
		for p < end && s[p] != '.' {
			c := s[p]
			if c < '0' || c > '7' {
				return 0, p, false
			}
			if v > 0xFFFFFFFF>>3 {
				return 0, p, false
			}
			v = v<<3 | uint64(c-'0')
			p++
		}
		return v, p, true
	}
	if !isDigit(s[p]) {
		return 0, p, false
	}
	v := uint64(s[p] - '0')
	p++
	for p < end && s[p] != '.' {
		c := s[p]
		if !isDigit(c) {
			return 0, p, false
		}
		if v > 429496729 {
			return 0, p, false
		}
		v = v*10 + uint64(c-'0')
		if v > 0xFFFFFFFF {
			return 0, p, false
		}
		p++
	}
	return v, p, true
}

// parseWHATWGIPv4 is ada's url_aggregator::parse_ipv4 followed by the IPv4 serializer.
func parseWHATWGIPv4(input string) (string, bool) {
	if input == "" {
		return "", false
	}
	input = strings.TrimSuffix(input, ".")
	if input == "" {
		return "", false
	}
	if _, ok := parseIPv4Fast(input); ok {
		return input, true
	}
	var ipv4 uint64
	p := 0
	parts := 0
	done := false
	for ; parts < 4 && p < len(input); parts++ {
		segment, np, ok := parseIPv4Number(input, p)
		if !ok {
			return "", false
		}
		p = np
		if p >= len(input) {
			shift := uint(32 - parts*8)
			if segment >= uint64(1)<<shift {
				return "", false
			}
			ipv4 = ipv4<<shift | segment
			done = true
			break
		}
		if segment > 255 || input[p] != '.' {
			return "", false
		}
		ipv4 = ipv4<<8 | segment
		p++
	}
	if !done && (parts != 4 || p != len(input)) {
		return "", false
	}
	a := uint32(ipv4)
	return strconv.Itoa(int(a>>24)) + "." + strconv.Itoa(int(a>>16&0xff)) + "." +
		strconv.Itoa(int(a>>8&0xff)) + "." + strconv.Itoa(int(a&0xff)), true
}
