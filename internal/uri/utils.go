// Ported from fast-uri 3.1.9 lib/utils.js (BSD-3-Clause, see doc.go).

package uri

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// String model: a Go string here is the WTF-8 form of a JavaScript string (as in package
// internal/js). Every helper below only ever compares, splits or slices at ASCII bytes, which is
// equivalent to doing so at UTF-16 code units; the percent-encoders decode code points with
// decodeUnit so lone surrogates and invalid bytes encode as U+FFFD, like fast-uri does for
// unpaired surrogates.

// decodeUnit decodes the code point at s[i]. A WTF-8 surrogate is returned as itself; an
// invalid byte decodes to U+FFFD with size 1.
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

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

func upperHex(c byte) byte {
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 'A'
	}
	return c
}

// hasHexPairAt reports whether s[i] is '%' followed by two hex digits (fast-uri's
// `input[i] === '%' && i + 2 < input.length && isHexPair(input.slice(i + 1, i + 3))`).
func hasHexPairAt(s string, i int) bool {
	return s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2])
}

// isUUID is /^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/iu.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			if !isHex(s[i]) {
				return false
			}
		}
	}
	return true
}

// isIPv4 is /^(?:(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]\d|\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]\d|\d)$/u.
func isIPv4(s string) bool {
	parts := 0
	for i := 0; ; {
		j := i
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		n := j - i
		if n == 0 || n > 3 || (n > 1 && s[i] == '0') {
			return false
		}
		if n == 3 {
			v := int(s[i]-'0')*100 + int(s[i+1]-'0')*10 + int(s[i+2]-'0')
			if v > 255 {
				return false
			}
		}
		parts++
		if parts == 4 {
			return j == len(s)
		}
		if j >= len(s) || s[j] != '.' {
			return false
		}
		i = j + 1
	}
}

// isPort is /^\d*$/u.
func isPort(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// isUnreservedByte is /^[\da-z\-._~]$/iu applied to String.fromCharCode(b), b < 256.
func isUnreservedByte(b byte) bool {
	return isDigit(b) || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '-' || b == '.' || b == '_' || b == '~'
}

// isUserinfoCharacter is /^[A-Za-z0-9\-._~!$&'()*+,;=:]$/u.
func isUserinfoCharacter(c rune) bool {
	if c >= 0x80 {
		return false
	}
	b := byte(c)
	return isUnreservedByte(b) || strings.IndexByte("!$&'()*+,;=:", b) >= 0
}

// isPathCharacter is /^[A-Za-z0-9\-._~!$&'()*+,;=:@/]$/u.
func isPathCharacter(c rune) bool {
	return isUserinfoCharacter(c) || c == '@' || c == '/'
}

// isQueryFragmentCharacter is /^[A-Za-z0-9\-._~!$&'()*+,;=:@/?]$/u.
func isQueryFragmentCharacter(c rune) bool {
	return isPathCharacter(c) || c == '?'
}

// isEscapeSafe is fast-uri's isEscapeSafe: the ASCII characters escape() leaves alone.
func isEscapeSafe(c byte) bool {
	return isDigit(c) || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		c == '*' || c == '+' || c == '-' || c == '.' || c == '/' || c == '@' || c == '_'
}

const hexDigits = "0123456789ABCDEF"

func appendByteHex(b []byte, c byte) []byte {
	return append(b, '%', hexDigits[c>>4], hexDigits[c&0xF])
}

// percentEncodeNonASCII is percentEncodeNonAscii: the UTF-8 octets of cp, percent-encoded.
func percentEncodeNonASCII(b []byte, cp rune) []byte {
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], cp)
	for _, c := range buf[:n] {
		b = appendByteHex(b, c)
	}
	return b
}

// appendEncodedCodePoint percent-encodes the non-ASCII code point starting at s[i] the way
// fast-uri's encoders do (pairing surrogates, U+FFFD for lone ones) and returns the new index
// of the last byte consumed.
func appendEncodedCodePoint(b []byte, s string, i int) ([]byte, int) {
	cp, size := decodeUnit(s, i)
	if cp >= 0xd800 && cp <= 0xdfff {
		if cp <= 0xdbff && i+size < len(s) {
			if lo, lsize := decodeUnit(s, i+size); lo >= 0xdc00 && lo <= 0xdfff {
				return percentEncodeNonASCII(b, 0x10000+((cp-0xd800)<<10)+(lo-0xdc00)), i + size + lsize - 1
			}
		}
		return percentEncodeNonASCII(b, 0xfffd), i + size - 1
	}
	return percentEncodeNonASCII(b, cp), i + size - 1
}

// stringArrayToHexStripped is not reached by parse/serialize/resolve and is omitted.

// isHextet is /^[\dA-Fa-f]{1,4}$/.
func isHextet(s string) bool {
	if len(s) < 1 || len(s) > 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHex(s[i]) {
			return false
		}
	}
	return true
}

// isIPvFuture is /^[vV][\dA-Fa-f]+\.[A-Za-z\d\-._~!$&'()*+,;=:]+$/.
func isIPvFuture(s string) bool {
	if len(s) < 4 || (s[0] != 'v' && s[0] != 'V') {
		return false
	}
	i := 1
	for i < len(s) && isHex(s[i]) {
		i++
	}
	if i == 1 || i >= len(s) || s[i] != '.' {
		return false
	}
	i++
	if i == len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if !isUserinfoCharacter(rune(s[i])) {
			return false
		}
	}
	return true
}

// isZoneCharacter is /^[A-Za-z\d\-._~]$/.
func isZoneCharacter(c byte) bool { return isUnreservedByte(c) }

// nonSimpleDomain is /[^!"$&'()*+,\-.;=_`a-z{}~]/u.test.
func nonSimpleDomain(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			continue
		}
		if strings.IndexByte("!\"$&'()*+,-.;=_`{}~", c) < 0 {
			return true
		}
	}
	return false
}

// isZoneIdentifier is fast-uri's isZoneIdentifier.
func isZoneIdentifier(zone string) bool {
	if len(zone) == 0 {
		return false
	}
	for i := 0; i < len(zone); i++ {
		if isZoneCharacter(zone[i]) {
			continue
		}
		if zone[i] == '%' && i+2 < len(zone) && isHex(zone[i+1]) && isHex(zone[i+2]) {
			i += 2
			continue
		}
		return false
	}
	return true
}

// compressIPv6ZeroRun is fast-uri's compressIPv6ZeroRun (RFC 5952 "::" compression).
func compressIPv6ZeroRun(hextets []string) string {
	bestStart, bestLength, runStart, runLength := -1, 0, -1, 0
	for i, h := range hextets {
		if h == "0" {
			if runStart == -1 {
				runStart = i
			}
			runLength++
			if runLength > bestLength {
				bestLength = runLength
				bestStart = runStart
			}
		} else {
			runStart = -1
			runLength = 0
		}
	}
	if bestLength < 2 {
		return strings.Join(hextets, ":")
	}
	return strings.Join(hextets[:bestStart], ":") + "::" + strings.Join(hextets[bestStart+bestLength:], ":")
}

// normalizeIPv6Address is fast-uri's normalizeIPv6Address; ok is false where it returns
// undefined.
func normalizeIPv6Address(input string) (string, bool) {
	compression := strings.Index(input, "::")
	if compression != -1 && strings.Contains(input[compression+1:], "::") {
		return "", false
	}
	var left, right []string
	if compression == -1 {
		left = strings.Split(input, ":")
	} else {
		left = strings.Split(input[:compression], ":")
		right = strings.Split(input[compression+2:], ":")
		if len(left) == 1 && left[0] == "" {
			left = nil
		}
		if len(right) == 1 && right[0] == "" {
			right = nil
		}
	}
	parts := make([]string, 0, len(left)+len(right))
	parts = append(parts, left...)
	parts = append(parts, right...)
	hextetCount := 0
	for i, part := range parts {
		if part == "" {
			return "", false
		}
		if strings.IndexByte(part, '.') != -1 {
			if i != len(parts)-1 || (compression != -1 && len(right) == 0) || !isIPv4(part) {
				return "", false
			}
			hextetCount += 2
			continue
		}
		if !isHextet(part) {
			return "", false
		}
		v, _ := strconv.ParseUint(part, 16, 32)
		parts[i] = strconv.FormatUint(v, 16)
		hextetCount++
	}
	if compression == -1 {
		if hextetCount != 8 {
			return "", false
		}
		return compressIPv6ZeroRun(parts), true
	}
	if hextetCount >= 8 {
		return "", false
	}
	expanded := make([]string, 0, 8)
	expanded = append(expanded, parts[:len(left)]...)
	for i := hextetCount; i < 8; i++ {
		expanded = append(expanded, "0")
	}
	expanded = append(expanded, parts[len(left):]...)
	return compressIPv6ZeroRun(expanded), true
}

// ipv6Result is fast-uri's NormalizeIPv6Result.
type ipv6Result struct {
	host        string
	escapedHost string
	isIPV6      bool
	isIPVFuture bool
	err         bool
}

// normalizeIPv6 is fast-uri's normalizeIPv6.
func normalizeIPv6(host string) ipv6Result {
	first := len(host) > 0 && host[0] == '['
	last := len(host) > 0 && host[len(host)-1] == ']'
	bracketed := first && last
	if (first || last) && !bracketed {
		return ipv6Result{host: host, err: true}
	}
	input := host
	if bracketed {
		// host.slice(1, -1); "[" alone is not bracketed, "[]" gives "".
		if len(host) >= 2 {
			input = host[1 : len(host)-1]
		} else {
			input = ""
		}
	}
	if bracketed && isIPvFuture(input) {
		input = strings.ToLower(input)
		return ipv6Result{host: "[" + input + "]", escapedHost: input, isIPVFuture: true}
	}
	if strings.Count(input, ":") < 2 {
		return ipv6Result{host: host, err: bracketed}
	}
	zoneIdentifier := ""
	if zoneSeparator := strings.IndexByte(input, '%'); zoneSeparator != -1 {
		separatorLength := 1
		if zoneSeparator+3 <= len(input) && strings.EqualFold(input[zoneSeparator:zoneSeparator+3], "%25") {
			separatorLength = 3
		}
		zoneIdentifier = input[zoneSeparator+separatorLength:]
		if !isZoneIdentifier(zoneIdentifier) {
			return ipv6Result{host: host, err: true}
		}
		input = input[:zoneSeparator]
	}
	address, ok := normalizeIPv6Address(input)
	if !ok {
		return ipv6Result{host: host, err: true}
	}
	r := ipv6Result{host: address, escapedHost: address, isIPV6: true}
	if zoneIdentifier != "" {
		r.host += "%" + zoneIdentifier
		r.escapedHost += "%25" + zoneIdentifier
	}
	return r
}

// removeDotSegments is fast-uri's removeDotSegments (RFC 3986 section 5.2.4).
func removeDotSegments(path string) string {
	input := path
	var output []string
	at := func(i int) byte {
		if i < len(input) {
			return input[i]
		}
		return 0
	}
	for len(input) > 0 {
		n := len(input)
		if n == 1 {
			if input == "." {
				break
			} else if input == "/" {
				output = append(output, "/")
				break
			}
			output = append(output, input)
			break
		} else if n == 2 {
			if input[0] == '.' {
				if input[1] == '.' {
					break
				} else if input[1] == '/' {
					input = input[2:]
					continue
				}
			} else if input[0] == '/' {
				if input[1] == '.' || input[1] == '/' {
					output = append(output, "/")
					break
				}
			}
		} else if n == 3 {
			if input == "/.." {
				if len(output) != 0 {
					output = output[:len(output)-1]
				}
				output = append(output, "/")
				break
			}
		}
		if at(0) == '.' {
			if at(1) == '.' {
				if at(2) == '/' {
					input = input[3:]
					continue
				}
			} else if at(1) == '/' {
				input = input[2:]
				continue
			}
		} else if at(0) == '/' {
			if at(1) == '.' {
				if at(2) == '/' {
					input = input[2:]
					continue
				} else if at(2) == '.' {
					if at(3) == '/' {
						input = input[3:]
						if len(output) != 0 {
							output = output[:len(output)-1]
						}
						continue
					}
				}
			}
		}
		// Rule 2E: move normal path segment to output.
		nextSlash := -1
		if len(input) > 1 {
			if k := strings.IndexByte(input[1:], '/'); k != -1 {
				nextSlash = k + 1
			}
		}
		if nextSlash == -1 {
			output = append(output, input)
			break
		}
		output = append(output, input[:nextSlash])
		input = input[nextSlash:]
	}
	return strings.Join(output, "")
}

// removeDotSegmentsRelative is fast-uri's removeDotSegmentsRelative.
func removeDotSegmentsRelative(path string) string {
	if path == "" || path[0] == '/' {
		return removeDotSegments(path)
	}
	segments := strings.Split(path, "/")
	last := len(segments) - 1
	var output []string
	trailingDirectory := false
	for i, segment := range segments {
		if segment == "." || segment == ".." {
			if segment == ".." {
				if len(output) != 0 && output[len(output)-1] != ".." {
					output = output[:len(output)-1]
				} else {
					output = append(output, "..")
				}
			}
			trailingDirectory = i == last
		} else {
			output = append(output, segment)
		}
	}
	if trailingDirectory && (len(output) == 0 || output[len(output)-1] != "..") {
		output = append(output, "")
	}
	if len(output) == 0 || output[0] == "" || strings.IndexByte(output[0], ':') != -1 {
		output = append([]string{"."}, output...)
	}
	return strings.Join(output, "/")
}

// reescapeHostDelimiters is fast-uri's reescapeHostDelimiters.
func reescapeHostDelimiters(host string, isIP bool) string {
	var b []byte
	for i := 0; i < len(host); i++ {
		c := host[i]
		var rep string
		switch c {
		case '@':
			rep = "%40"
		case '/':
			rep = "%2F"
		case '?':
			rep = "%3F"
		case '#':
			rep = "%23"
		case ':':
			if !isIP {
				rep = "%3A"
			}
		}
		if rep == "" {
			if b != nil {
				b = append(b, c)
			}
			continue
		}
		if b == nil {
			b = append(make([]byte, 0, len(host)+8), host[:i]...)
		}
		b = append(b, rep...)
	}
	if b == nil {
		return host
	}
	return string(b)
}

// normalizePercentEncoding is fast-uri's normalizePercentEncoding.
func normalizePercentEncoding(input string, decodeUnreserved bool) string {
	if strings.IndexByte(input, '%') == -1 {
		return input
	}
	out := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		if hasHexPairAt(input, i) {
			d := hexVal(input[i+1])<<4 | hexVal(input[i+2])
			if decodeUnreserved && isUnreservedByte(d) {
				out = append(out, d)
			} else {
				out = append(out, '%', upperHex(input[i+1]), upperHex(input[i+2]))
			}
			i += 2
			continue
		}
		out = append(out, input[i])
	}
	return string(out)
}

// normalizePathEncoding is fast-uri's normalizePathEncoding.
func normalizePathEncoding(input string) string {
	out := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if hasHexPairAt(input, i) {
			d := hexVal(input[i+1])<<4 | hexVal(input[i+2])
			if d != '.' && isUnreservedByte(d) {
				out = append(out, d)
			} else {
				out = append(out, '%', upperHex(input[i+1]), upperHex(input[i+2]))
			}
			i += 2
			continue
		}
		if ch < 0x80 {
			if isPathCharacter(rune(ch)) || isEscapeSafe(ch) {
				out = append(out, ch)
			} else {
				out = appendByteHex(out, ch)
			}
			continue
		}
		out, i = appendEncodedCodePoint(out, input, i)
	}
	return string(out)
}

// serializePathEncoding is fast-uri's serializePathEncoding.
func serializePathEncoding(input string, pathNoScheme bool) string {
	out := make([]byte, 0, len(input))
	firstSegment := pathNoScheme && (len(input) == 0 || input[0] != '/')
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if hasHexPairAt(input, i) {
			out = append(out, '%', upperHex(input[i+1]), upperHex(input[i+2]))
			i += 2
			continue
		}
		if ch == '/' {
			firstSegment = false
		}
		if ch < 0x80 {
			if isPathCharacter(rune(ch)) && (ch != ':' || !firstSegment) {
				out = append(out, ch)
			} else {
				out = appendByteHex(out, ch)
			}
			continue
		}
		out, i = appendEncodedCodePoint(out, input, i)
	}
	return string(out)
}

// encodeComponent is fast-uri's encodeComponent.
func encodeComponent(input string, isAllowed func(rune) bool) string {
	out := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if hasHexPairAt(input, i) {
			out = append(out, '%', upperHex(input[i+1]), upperHex(input[i+2]))
			i += 2
			continue
		}
		if ch < 0x80 {
			if isAllowed(rune(ch)) {
				out = append(out, ch)
			} else {
				out = appendByteHex(out, ch)
			}
			continue
		}
		out, i = appendEncodedCodePoint(out, input, i)
	}
	return string(out)
}

// encodeUserinfo is fast-uri's encodeUserinfo.
func encodeUserinfo(input string) string { return encodeComponent(input, isUserinfoCharacter) }

// encodeQuery is fast-uri's encodeQuery.
func encodeQuery(input string) string { return encodeComponent(input, isQueryFragmentCharacter) }

// encodeFragment is fast-uri's encodeFragment.
func encodeFragment(input string) string { return encodeComponent(input, isQueryFragmentCharacter) }

// normalizeQueryFragmentEncoding is fast-uri's normalizeQueryFragmentEncoding.
func normalizeQueryFragmentEncoding(input string) string {
	out := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if hasHexPairAt(input, i) {
			d := hexVal(input[i+1])<<4 | hexVal(input[i+2])
			if isUnreservedByte(d) {
				out = append(out, d)
			} else {
				out = append(out, '%', upperHex(input[i+1]), upperHex(input[i+2]))
			}
			i += 2
			continue
		}
		if ch < 0x80 {
			if isQueryFragmentCharacter(rune(ch)) || isEscapeSafe(ch) {
				out = append(out, ch)
			} else {
				out = appendByteHex(out, ch)
			}
			continue
		}
		out, i = appendEncodedCodePoint(out, input, i)
	}
	return string(out)
}

// escapePreservingEscapes is not reached by parse/serialize/resolve and is omitted.

// recomposeAuthority is fast-uri's recomposeAuthority. It fails where fast-uri throws.
func recomposeAuthority(c *Components) (string, bool, error) {
	var tokens []string
	if c.Userinfo != nil {
		tokens = append(tokens, encodeUserinfo(*c.Userinfo), "@")
	}
	if c.Host != nil {
		host := *c.Host
		if !isIPv4(host) {
			r := normalizeIPv6(host)
			if !r.isIPV6 && !r.isIPVFuture {
				// Decode only unreserved bytes, once.
				host = normalizePercentEncoding(host, true)
				r = normalizeIPv6(host)
			}
			if r.isIPV6 || r.isIPVFuture {
				host = "[" + r.escapedHost + "]"
			} else {
				host = reescapeHostDelimiters(host, false)
			}
		}
		tokens = append(tokens, host)
	}
	if c.Port != nil {
		if !isPort(*c.Port) {
			return "", false, typeError("URI port is malformed.")
		}
		tokens = append(tokens, ":", *c.Port)
	}
	if len(tokens) == 0 {
		return "", false, nil
	}
	return strings.Join(tokens, ""), true, nil
}
