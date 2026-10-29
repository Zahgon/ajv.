// Ported from fast-uri 3.1.9 index.js (BSD-3-Clause, see doc.go).

package uri

import (
	"strconv"
	"strings"

	"github.com/ajv-validator/ajv-go/internal/js"
)

// Components mirrors fast-uri's URIComponent object. A nil pointer field is a property that
// JavaScript leaves undefined (or never sets); a non-nil pointer to "" is the empty string.
type Components struct {
	// Scheme is the scheme, lowercased when it is valid.
	Scheme *string
	// Userinfo is the raw userinfo before "@".
	Userinfo *string
	// Host is the host. It is nil when the reference has no authority.
	Host *string
	// Port is String(port). fast-uri stores a parsed port as a number, except that an empty
	// port (as in "http://a:/") stays the string "". Serialize treats a non-empty Port as that
	// number and "" as the empty string.
	Port *string
	// Path is the path; nil for ws/wss (see ResourceName) and urn (see NID and NSS).
	Path *string
	// Query is the query without "?".
	Query *string
	// Fragment is the fragment without "#".
	Fragment *string
	// Reference is "same-document", "relative", "absolute" or "uri".
	Reference *string
	// Error is the first parse error fast-uri recorded, nil when there was none.
	Error *string
	// Secure is the ws/wss secure flag.
	Secure *bool
	// ResourceName is the ws/wss path and query.
	ResourceName *string
	// NID is the URN namespace identifier (lowercased).
	NID *string
	// NSS is the URN namespace-specific string.
	NSS *string
	// UUID is the UUID of a urn:uuid URN.
	UUID *string
}

// options is the subset of fast-uri's Options reachable without user options: resolve() parses
// and serializes with { scheme: 'null' } and sets skipEscape, and the urn serializer sets
// skipEscape.
type options struct {
	scheme     string
	skipEscape bool
}

const (
	malformedSchemeError = "URI scheme is malformed."
	malformedHostError   = "URI host is malformed."
)

func sp(s string) *string { return &s }

func truthy(p *string) bool { return p != nil && *p != "" }

// jsString returns *p, or "" when p is nil (callers only use it where undefined and "" behave
// the same).
func jsString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func firstTruthy(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func indexByte(s string, c byte) int { return strings.IndexByte(s, c) }

func setErrorIfUnset(c *Components, msg string) {
	if !truthy(c.Error) {
		c.Error = &msg
	}
}

func jsError(msg string) error { return &js.Error{Name: "Error", Message: msg} }

func typeError(msg string) error { return &js.Error{Name: "TypeError", Message: msg} }

// clone returns a shallow copy of c (nil becomes an empty object).
func (c *Components) clone() *Components {
	if c == nil {
		return &Components{}
	}
	cp := *c
	return &cp
}

// isValidScheme is VALID_SCHEME = /^[A-Za-z][A-Za-z0-9+.-]*$/u.
func isValidScheme(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !alpha && (i == 0 || !(isDigit(c) || c == '+' || c == '.' || c == '-')) {
			return false
		}
	}
	return true
}

// validateScheme is fast-uri's validateScheme.
func validateScheme(scheme string) (string, error) {
	if !isValidScheme(scheme) {
		return "", typeError(malformedSchemeError)
	}
	return scheme, nil
}

// isForbiddenDomainHostCharacter is FORBIDDEN_DOMAIN_HOST_CHARACTER =
// /[\0-\x20"#/:<>?@[\\\]^`{|}\x7F]/u.
func hasForbiddenDomainHostCharacter(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c == 0x7f || (c < 0x80 && strings.IndexByte("\"#/:<>?@[\\]^`{|}", c) >= 0) {
			return true
		}
	}
	return false
}

// isIPLiteral is fast-uri's isIPLiteral.
func isIPLiteral(host string) bool {
	return len(host) > 0 && host[0] == '[' && host[len(host)-1] == ']'
}

// hasForbiddenDomainHost is fast-uri's hasForbiddenDomainHost (no user options).
func hasForbiddenDomainHost(c *Components, h *schemeHandler, isIP bool) bool {
	return truthy(c.Host) && !isIP && !isIPLiteral(*c.Host) && h != nil && h.domainHost &&
		hasForbiddenDomainHostCharacter(*c.Host)
}

// canonicalizeHost is fast-uri's canonicalizeHost (no user options). It reports whether host
// conversion failed.
func canonicalizeHost(c *Components, h *schemeHandler, isIP bool) bool {
	if hasForbiddenDomainHost(c, h, isIP) {
		setErrorIfUnset(c, malformedHostError)
		return true
	}
	if truthy(c.Host) && !isIPLiteral(*c.Host) && h != nil && h.domainHost && !isIP && nonSimpleDomain(*c.Host) {
		// new URL('http://' + parsed.host); isHostOnlyURL always holds for a host that passed the
		// forbidden-character check, since the WHATWG parser then sees no delimiter in it.
		hostname, ok := whatwgHostname(*c.Host)
		if !ok {
			setErrorIfUnset(c, "Host's domain name can not be converted to ASCII: TypeError: Invalid URL")
			return true
		}
		c.Host = &hostname
	}
	return false
}

// uriMatch holds the capture groups of URI_PARSE; nil is an unmatched group.
type uriMatch struct {
	scheme, authority, userinfo, host, port, path, query, fragment *string
}

// matchURI applies URI_PARSE =
// /^(?:([^#/:?]+):)?(?:\/\/((?:([^#/?@]*)@)?(\[[^#/?\]]+\]|[^#/:?]*)(?::(\d*))?))?([^#?]*)(?:\?([^#]*))?(?:#((?:.|[\n\r])*))?/u.
// Every part after the scheme can match empty, so the first alternative tried at each step is
// the one the backtracking engine keeps.
func matchURI(s string) uriMatch {
	var m uriMatch
	i := 0
	// scheme: [^#/:?]+ followed by ':'
	if j := strings.IndexAny(s, "#/:?"); j > 0 && s[j] == ':' {
		m.scheme = sp(s[:j])
		i = j + 1
	}
	// authority
	if strings.HasPrefix(s[i:], "//") {
		start := i + 2
		k := start
		if j := strings.IndexAny(s[k:], "#/?@"); j != -1 && s[k+j] == '@' {
			m.userinfo = sp(s[k : k+j])
			k += j + 1
		}
		hostEnd := -1
		if k < len(s) && s[k] == '[' {
			if j := strings.IndexAny(s[k+1:], "#/?]"); j > 0 && s[k+1+j] == ']' {
				hostEnd = k + 1 + j + 1
			}
		}
		if hostEnd == -1 {
			hostEnd = len(s)
			if j := strings.IndexAny(s[k:], "#/:?"); j != -1 {
				hostEnd = k + j
			}
		}
		m.host = sp(s[k:hostEnd])
		k = hostEnd
		if k < len(s) && s[k] == ':' {
			j := k + 1
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			m.port = sp(s[k+1 : j])
			k = j
		}
		m.authority = sp(s[start:k])
		i = k
	}
	// path
	j := len(s)
	if k := strings.IndexAny(s[i:], "#?"); k != -1 {
		j = i + k
	}
	m.path = sp(s[i:j])
	i = j
	// query
	if i < len(s) && s[i] == '?' {
		j = len(s)
		if k := indexByte(s[i+1:], '#'); k != -1 {
			j = i + 1 + k
		}
		m.query = sp(s[i+1 : j])
		i = j
	}
	// fragment: (?:.|[\n\r])* stops at U+2028 and U+2029.
	if i < len(s) && s[i] == '#' {
		frag := s[i+1:]
		for k := 0; k+2 < len(frag); k++ {
			if frag[k] == 0xe2 && frag[k+1] == 0x80 && (frag[k+2] == 0xa8 || frag[k+2] == 0xa9) {
				frag = frag[:k]
				break
			}
		}
		m.fragment = sp(frag)
	}
	return m
}

// authorityPrefix applies AUTHORITY_PREFIX = /^(?:[^#/:?]+:)?\/\/([^/?#]*)/ and returns the
// captured authority.
func authorityPrefix(s string) (string, bool) {
	rest := ""
	switch {
	case func() bool {
		j := strings.IndexAny(s, "#/:?")
		if j > 0 && s[j] == ':' && strings.HasPrefix(s[j+1:], "//") {
			rest = s[j+3:]
			return true
		}
		return false
	}():
	case strings.HasPrefix(s, "//"):
		rest = s[2:]
	default:
		return "", false
	}
	if j := strings.IndexAny(rest, "/?#"); j != -1 {
		rest = rest[:j]
	}
	return rest, true
}

// introducerRegion applies AUTHORITY_INTRODUCER_REGION = /^(?:[^#/:?]+:)?([/\\\t\n\r]*)/.
func introducerRegion(s string) string {
	if j := strings.IndexAny(s, "#/:?"); j > 0 && s[j] == ':' {
		s = s[j+1:]
	}
	k := 0
	for k < len(s) && strings.IndexByte("/\\\t\n\r", s[k]) >= 0 {
		k++
	}
	return s[:k]
}

// hasMalformedPercentEncoding is fast-uri's hasMalformedPercentEncoding.
func hasMalformedPercentEncoding(component *string) bool {
	if component == nil {
		return false
	}
	s := *component
	percent := indexByte(s, '%')
	for percent != -1 {
		if percent+2 >= len(s) || !isHex(s[percent+1]) || !isHex(s[percent+2]) {
			return true
		}
		next := indexByte(s[percent+3:], '%')
		if next == -1 {
			break
		}
		percent += 3 + next
	}
	return false
}

// parseStatus is the status record of fast-uri's parseWithStatus.
type parseStatus struct {
	malformedAuthorityOrPort bool
	malformedPercentEncoding bool
	malformedSchemeSpecific  bool
	malformedHost            bool
	malformedScheme          bool
}

func (s parseStatus) any() bool {
	return s.malformedAuthorityOrPort || s.malformedPercentEncoding || s.malformedSchemeSpecific ||
		s.malformedHost || s.malformedScheme
}

// parseWithStatus is fast-uri's parseWithStatus (without the "suffix" reference option).
func parseWithStatus(uri string, o options) (*Components, parseStatus) {
	parsed := &Components{Host: sp(""), Path: sp("")}
	var st parseStatus
	malformedIPLiteral := false
	isIP := false

	if auth, ok := authorityPrefix(uri); ok && indexByte(auth, '\\') != -1 {
		parsed.Error = sp("URI authority must not contain a literal backslash.")
		st.malformedAuthorityOrPort = true
	}
	region := introducerRegion(uri)
	normalizedRegion := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, region)
	if len(normalizedRegion) >= 2 {
		if normalizedRegion[:2] != "//" {
			setErrorIfUnset(parsed, "URI authority must not contain a literal backslash.")
			st.malformedAuthorityOrPort = true
		} else if len(region) != len(normalizedRegion) {
			setErrorIfUnset(parsed, "URI authority introducer must not contain whitespace.")
			st.malformedAuthorityOrPort = true
		}
	}

	m := matchURI(uri)
	parsed.Scheme = m.scheme
	parsed.Userinfo = m.userinfo
	parsed.Host = m.host
	parsed.Path = sp(jsString(m.path))
	parsed.Query = m.query
	parsed.Fragment = m.fragment
	// parseInt(matches[5], 10), and the string itself when that is NaN.
	var portValue float64
	portIsNumber := false
	if m.port != nil {
		if *m.port == "" {
			parsed.Port = sp("")
		} else {
			portValue, _ = strconv.ParseFloat(*m.port, 64)
			portIsNumber = true
			parsed.Port = sp(js.NumberToString(portValue))
		}
	}

	if parsed.Scheme != nil {
		if isValidScheme(*parsed.Scheme) {
			parsed.Scheme = sp(strings.ToLower(*parsed.Scheme))
		} else {
			setErrorIfUnset(parsed, malformedSchemeError)
			st.malformedScheme = true
		}
	}

	st.malformedPercentEncoding = hasMalformedPercentEncoding(m.userinfo) ||
		(m.host != nil && !isIPLiteral(*m.host) && hasMalformedPercentEncoding(m.host)) ||
		hasMalformedPercentEncoding(m.path) ||
		hasMalformedPercentEncoding(m.query) ||
		hasMalformedPercentEncoding(m.fragment)
	if st.malformedPercentEncoding {
		setErrorIfUnset(parsed, "URI contains malformed percent-encoding.")
	}

	// getParseError
	parseError := ""
	if m.authority != nil && truthy(parsed.Path) && (*parsed.Path)[0] != '/' {
		parseError = `URI path must start with "/" when authority is present.`
	} else if portIsNumber && portValue > 65535 {
		parseError = "URI port is malformed."
	}
	if parseError != "" {
		setErrorIfUnset(parsed, parseError)
		st.malformedAuthorityOrPort = true
	}

	if truthy(parsed.Host) {
		if !isIPv4(*parsed.Host) {
			host := *parsed.Host
			bracketed := isIPLiteral(host)
			hasBracket := strings.ContainsAny(host, "[]")
			r := normalizeIPv6(host)
			isIP = r.isIPV6 || r.isIPVFuture
			malformedIPLiteral = hasBracket && (!bracketed || r.err)
			if isIP {
				parsed.Host = sp(r.host)
			} else {
				parsed.Host = sp(js.ToLowerCase(r.host))
			}
			if malformedIPLiteral {
				setErrorIfUnset(parsed, malformedHostError)
				st.malformedAuthorityOrPort = true
			}
		} else {
			isIP = true
		}
	}

	switch {
	case parsed.Scheme == nil && parsed.Userinfo == nil && parsed.Host == nil && parsed.Port == nil &&
		parsed.Query == nil && !truthy(parsed.Path):
		parsed.Reference = sp("same-document")
	case parsed.Scheme == nil:
		parsed.Reference = sp("relative")
	case parsed.Fragment == nil:
		parsed.Reference = sp("absolute")
	default:
		parsed.Reference = sp("uri")
	}

	h := getSchemeHandler(firstTruthy(o.scheme, jsString(parsed.Scheme)))

	// convert Unicode IDN -> ASCII IDN when the effective scheme uses domain hosts
	if !malformedIPLiteral {
		st.malformedHost = canonicalizeHost(parsed, h, isIP)
	}

	if indexByte(uri, '%') != -1 && parsed.Host != nil && !malformedIPLiteral {
		host := *parsed.Host
		if !isIP {
			host = normalizePercentEncoding(host, true)
			host = normalizePercentEncoding(js.ToLowerCase(host), false)
		}
		if isIP || o.scheme != "null" {
			host = reescapeHostDelimiters(host, isIP)
		}
		parsed.Host = &host
	}

	if h == nil || !h.skipNormalize {
		if truthy(parsed.Path) {
			parsed.Path = sp(normalizePathEncoding(*parsed.Path))
		}
		if truthy(parsed.Query) {
			parsed.Query = sp(normalizeQueryFragmentEncoding(*parsed.Query))
		}
		if truthy(parsed.Fragment) {
			parsed.Fragment = sp(normalizeQueryFragmentEncoding(*parsed.Fragment))
		}
	}

	// perform scheme specific parsing
	if h != nil {
		h.parse(parsed, &o)
		if h == urnHandler && parsed.NID == nil {
			st.malformedSchemeSpecific = true
		}
	}
	return parsed, st
}

// Parse is fast-uri's parse(uri) with no options. It never fails: problems are reported in
// the Error field, as fast-uri does.
func Parse(s string) *Components {
	c, _ := parseWithStatus(s, options{})
	return c
}

// Serialize is fast-uri's serialize(components) with no options. It returns "" where fast-uri
// throws; use TrySerialize to get the exception.
func Serialize(c *Components) string {
	s, _ := TrySerialize(c)
	return s
}

// TrySerialize is fast-uri's serialize(components) with no options. The error is a *js.Error
// carrying the exception fast-uri throws: a TypeError for a malformed scheme or port, or an
// Error for a URN without a namespace identifier.
func TrySerialize(c *Components) (string, error) {
	return serialize(c, options{})
}

func serialize(cmpts *Components, opts options) (string, error) {
	src := cmpts.clone()
	component := &Components{
		Host:         src.Host,
		Scheme:       src.Scheme,
		Userinfo:     src.Userinfo,
		Port:         src.Port,
		Path:         src.Path,
		Query:        src.Query,
		NID:          src.NID,
		NSS:          src.NSS,
		UUID:         src.UUID,
		Fragment:     src.Fragment,
		Reference:    src.Reference,
		ResourceName: src.ResourceName,
		Secure:       src.Secure,
		Error:        sp(""),
	}
	o := opts
	var tokens []string

	if truthy(component.Scheme) {
		s, err := validateScheme(*component.Scheme)
		if err != nil {
			return "", err
		}
		component.Scheme = &s
	}

	h := getSchemeHandler(firstTruthy(o.scheme, jsString(component.Scheme)))
	if h != nil {
		if err := h.serialize(component, &o); err != nil {
			return "", err
		}
	}

	hasAuthority := component.Userinfo != nil || component.Host != nil || component.Port != nil
	schemelessReference := component.Scheme == nil && !hasAuthority
	pathNoScheme := !o.skipEscape && schemelessReference

	if component.Path != nil {
		if !o.skipEscape {
			component.Path = sp(serializePathEncoding(*component.Path, pathNoScheme))
		} else {
			component.Path = sp(normalizePercentEncoding(*component.Path, false))
		}
	}

	if truthy(component.Scheme) {
		// Scheme handlers may replace the scheme during serialization.
		s, err := validateScheme(*component.Scheme)
		if err != nil {
			return "", err
		}
		tokens = append(tokens, s, ":")
	}

	authority, hasAuth, err := recomposeAuthority(component)
	if err != nil {
		return "", err
	}
	if hasAuth {
		tokens = append(tokens, "//", authority)
		if truthy(component.Path) && (*component.Path)[0] != '/' {
			tokens = append(tokens, "/")
		}
	}
	if component.Path != nil {
		s := *component.Path
		if schemelessReference && (s == "" || s[0] != '/') {
			s = removeDotSegmentsRelative(s)
		} else {
			s = removeDotSegments(s)
		}
		if schemelessReference {
			s = serializePathEncoding(s, true)
		}
		if !hasAuth && strings.HasPrefix(s, "//") {
			// don't allow the path to start with "//"
			s = "/%2F" + s[2:]
		}
		tokens = append(tokens, s)
	}
	if component.Query != nil {
		tokens = append(tokens, "?", encodeQuery(*component.Query))
	}
	if component.Fragment != nil {
		tokens = append(tokens, "#", encodeFragment(*component.Fragment))
	}
	return strings.Join(tokens, ""), nil
}

// Resolve is fast-uri's resolve(base, ref) with no options. It returns "" where fast-uri
// throws; use TryResolve to get the exception.
func Resolve(base, ref string) string {
	s, _ := TryResolve(base, ref)
	return s
}

// TryResolve is fast-uri's resolve(base, ref) with no options. The error is a *js.Error
// carrying the exception fast-uri throws for a malformed base or reference (for example
// "URI contains malformed percent-encoding.") or an unconvertible domain host.
func TryResolve(base, ref string) (string, error) {
	schemeless := options{scheme: "null"}
	baseParsed, baseStatus := parseWithStatus(base, schemeless)
	relParsed, relStatus := parseWithStatus(ref, schemeless)
	if baseStatus.any() || relStatus.any() {
		return "", jsError(firstTruthy(jsString(baseParsed.Error), jsString(relParsed.Error), "URI is malformed."))
	}
	resolved := resolveComponent(baseParsed, relParsed)
	h := getSchemeHandler(jsString(resolved.Scheme))
	resolvedHost := resolved.Host
	resolvedHostIsIP := truthy(resolvedHost) && (isIPv4(*resolvedHost) || normalizeIPv6(*resolvedHost).isIPV6)
	if hasForbiddenDomainHost(resolved, h, resolvedHostIsIP) {
		return "", jsError(malformedHostError)
	}
	canonicalizeHost(resolved, h, resolvedHostIsIP)
	encodedASCIIHost := truthy(resolvedHost) && indexByte(*resolvedHost, '%') != -1 && isASCII(*resolvedHost)
	if truthy(resolved.Error) && !encodedASCIIHost {
		return "", jsError(*resolved.Error)
	}
	schemeless.skipEscape = true
	return serialize(resolved, schemeless)
}

// resolveComponent is fast-uri's resolveComponent with skipNormalization and no tolerant
// option.
func resolveComponent(base, relative *Components) *Components {
	target := &Components{}
	if truthy(relative.Scheme) {
		target.Scheme = relative.Scheme
		target.Userinfo = relative.Userinfo
		target.Host = relative.Host
		target.Port = relative.Port
		target.Path = sp(removeDotSegments(jsString(relative.Path)))
		target.Query = relative.Query
	} else {
		if relative.Userinfo != nil || relative.Host != nil || relative.Port != nil {
			target.Userinfo = relative.Userinfo
			target.Host = relative.Host
			target.Port = relative.Port
			target.Path = sp(removeDotSegments(jsString(relative.Path)))
			target.Query = relative.Query
		} else {
			if !truthy(relative.Path) {
				target.Path = base.Path
				if relative.Query != nil {
					target.Query = relative.Query
				} else {
					target.Query = base.Query
				}
			} else {
				rp := *relative.Path
				if rp[0] == '/' {
					target.Path = sp(removeDotSegments(rp))
				} else {
					baseHasAuthority := base.Userinfo != nil || base.Host != nil || base.Port != nil
					var p string
					switch {
					case baseHasAuthority && !truthy(base.Path):
						p = "/" + rp
					case !truthy(base.Path):
						p = rp
					default:
						bp := *base.Path
						p = bp[:strings.LastIndexByte(bp, '/')+1] + rp
					}
					if base.Scheme == nil && !baseHasAuthority && (p == "" || p[0] != '/') {
						p = removeDotSegmentsRelative(p)
					} else {
						p = removeDotSegments(p)
					}
					target.Path = &p
				}
				target.Query = relative.Query
			}
			target.Userinfo = base.Userinfo
			target.Host = base.Host
			target.Port = base.Port
		}
		target.Scheme = base.Scheme
	}
	target.Fragment = relative.Fragment
	return target
}
