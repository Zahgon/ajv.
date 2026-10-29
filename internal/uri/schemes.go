// Ported from fast-uri 3.1.9 lib/schemes.js (BSD-3-Clause, see doc.go).

package uri

import (
	"github.com/ajv-validator/ajv-go/internal/js"
)

// schemeKind selects the parse/serialize pair of a scheme handler.
type schemeKind uint8

const (
	kindHTTP schemeKind = iota
	kindWS
	kindURN
	kindURNUUID
)

// schemeHandler is fast-uri's SchemeHandler.
type schemeHandler struct {
	scheme        string
	kind          schemeKind
	domainHost    bool
	skipNormalize bool
}

var (
	httpHandler    = &schemeHandler{scheme: "http", kind: kindHTTP, domainHost: true}
	httpsHandler   = &schemeHandler{scheme: "https", kind: kindHTTP, domainHost: true}
	wsHandler      = &schemeHandler{scheme: "ws", kind: kindWS, domainHost: true}
	wssHandler     = &schemeHandler{scheme: "wss", kind: kindWS, domainHost: true}
	urnHandler     = &schemeHandler{scheme: "urn", kind: kindURN, skipNormalize: true}
	urnUUIDHandler = &schemeHandler{scheme: "urn:uuid", kind: kindURNUUID, skipNormalize: true}
)

// parse is schemeHandler.parse.
func (h *schemeHandler) parse(c *Components, o *options) {
	switch h.kind {
	case kindHTTP:
		httpParse(c, o)
	case kindWS:
		wsParse(c, o)
	case kindURN:
		urnParse(c, o)
	case kindURNUUID:
		urnuuidParse(c, o)
	}
}

// serialize is schemeHandler.serialize.
func (h *schemeHandler) serialize(c *Components, o *options) error {
	switch h.kind {
	case kindHTTP:
		return httpSerialize(c, o)
	case kindWS:
		return wsSerialize(c, o)
	case kindURN:
		return urnSerialize(c, o)
	default:
		return urnuuidSerialize(c, o)
	}
}

// lookupScheme is SCHEMES[name] (a null-prototype object).
func lookupScheme(name string) *schemeHandler {
	switch name {
	case "http":
		return httpHandler
	case "https":
		return httpsHandler
	case "ws":
		return wsHandler
	case "wss":
		return wssHandler
	case "urn":
		return urnHandler
	case "urn:uuid":
		return urnUUIDHandler
	}
	return nil
}

// getSchemeHandler is fast-uri's getSchemeHandler; an empty name is undefined.
func getSchemeHandler(scheme string) *schemeHandler {
	if scheme == "" {
		return nil
	}
	if h := lookupScheme(scheme); h != nil {
		return h
	}
	return lookupScheme(js.ToLowerCase(scheme))
}

// wsIsSecure is fast-uri's wsIsSecure.
func wsIsSecure(c *Components) bool {
	if c.Secure != nil {
		return *c.Secure
	}
	if c.Scheme != nil && *c.Scheme != "" {
		s := *c.Scheme
		return len(s) == 3 && (s[0] == 'w' || s[0] == 'W') && (s[1] == 's' || s[1] == 'S') && (s[2] == 's' || s[2] == 'S')
	}
	return false
}

func httpParse(c *Components, _ *options) {
	if !truthy(c.Host) {
		setErrorIfUnset(c, "HTTP URIs must have a host.")
	}
}

func httpSerialize(c *Components, _ *options) error {
	secure := js.ToLowerCase(jsString(c.Scheme)) == "https"
	// normalize the default port
	if c.Port != nil && (*c.Port == "" || (secure && *c.Port == "443") || (!secure && *c.Port == "80")) {
		c.Port = nil
	}
	// normalize the empty path
	if !truthy(c.Path) {
		c.Path = sp("/")
	}
	return nil
}

func wsParse(c *Components, _ *options) {
	secure := wsIsSecure(c)
	c.Secure = &secure
	resourceName := "/"
	if truthy(c.Path) {
		resourceName = *c.Path
	}
	if truthy(c.Query) {
		resourceName += "?" + *c.Query
	}
	c.ResourceName = &resourceName
	c.Path = nil
	c.Query = nil
}

func wsSerialize(c *Components, _ *options) error {
	// normalize the default port
	if c.Port != nil {
		def := "80"
		if wsIsSecure(c) {
			def = "443"
		}
		if *c.Port == def || *c.Port == "" {
			c.Port = nil
		}
	}
	// ensure scheme matches secure flag
	if c.Secure != nil {
		if *c.Secure {
			c.Scheme = sp("wss")
		} else {
			c.Scheme = sp("ws")
		}
		c.Secure = nil
	}
	// reconstruct path from resource name
	if truthy(c.ResourceName) {
		rn := *c.ResourceName
		path, query := rn, (*string)(nil)
		if q := indexByte(rn, '?'); q != -1 {
			path = rn[:q]
			query = sp(rn[q+1:])
		}
		if path != "" && path != "/" {
			c.Path = sp(path)
		} else {
			c.Path = nil
		}
		c.Query = query
		c.ResourceName = nil
	}
	// forbid fragment component
	c.Fragment = nil
	return nil
}

// matchURN is URN_REG = /^([\da-z][\d\-a-z]{0,31}):((?:[\w!$'()*+,\-./:;=@]|%[\da-f]{2})+)$/iu.
// With the i and u flags the letter classes also match U+017F and U+212A, which case-fold to
// "s" and "k".
func matchURN(path string) (nid, nss string, ok bool) {
	isFolded := func(r rune) bool { return r == 0x17f || r == 0x212a }
	isLetterDigit := func(r rune) bool {
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || isFolded(r)
	}
	colon := indexByte(path, ':')
	if colon <= 0 {
		return "", "", false
	}
	n := 0
	for i := 0; i < colon; {
		r, size := decodeUnit(path, i)
		if !(isLetterDigit(r) || (n > 0 && r == '-')) {
			return "", "", false
		}
		n++
		i += size
	}
	if n > 32 {
		return "", "", false
	}
	rest := path[colon+1:]
	if rest == "" {
		return "", "", false
	}
	for i := 0; i < len(rest); {
		r, size := decodeUnit(rest, i)
		switch {
		case isLetterDigit(r) || r == '_':
		case r < 0x80 && indexByte("!$'()*+,-./:;=@", byte(r)) >= 0:
		case r == '%' && i+2 < len(rest) && isHex(rest[i+1]) && isHex(rest[i+2]):
			size = 3
		default:
			return "", "", false
		}
		i += size
	}
	return path[:colon], rest, true
}

func urnParse(c *Components, o *options) {
	if !truthy(c.Path) {
		c.Error = sp("URN can not be parsed")
		return
	}
	nid, nss, ok := matchURN(*c.Path)
	if !ok {
		setErrorIfUnset(c, "URN can not be parsed.")
		return
	}
	scheme := firstTruthy(o.scheme, jsString(c.Scheme), "urn")
	lowerNID := js.ToLowerCase(nid)
	c.NID = &lowerNID
	c.NSS = sp(nss)
	h := getSchemeHandler(scheme + ":" + lowerNID)
	c.Path = nil
	if h != nil {
		h.parse(c, o)
	}
}

func urnSerialize(c *Components, o *options) error {
	if c.NID == nil {
		return jsError("URN without nid cannot be serialized")
	}
	scheme := firstTruthy(o.scheme, jsString(c.Scheme), "urn")
	nid := js.ToLowerCase(*c.NID)
	h := getSchemeHandler(scheme + ":" + nid)
	if h != nil {
		if err := h.serialize(c, o); err != nil {
			return err
		}
	}
	// `${nid || options.nid}:${nss}`: options.nid is always undefined here.
	if nid == "" {
		nid = "undefined"
	}
	nss := "undefined"
	if c.NSS != nil {
		nss = *c.NSS
	}
	c.Path = sp(nid + ":" + nss)
	o.skipEscape = true
	return nil
}

func urnuuidParse(c *Components, _ *options) {
	c.UUID = c.NSS
	c.NSS = nil
	if !truthy(c.UUID) || !isUUID(*c.UUID) {
		setErrorIfUnset(c, "UUID is not valid.")
	}
}

func urnuuidSerialize(c *Components, _ *options) error {
	nss := js.ToLowerCase(jsString(c.UUID))
	c.NSS = &nss
	return nil
}
