// Differential tests against fast-uri 3.1.9 running in Node 26.10.
//
// Regenerate the committed vectors (from the ajv directory):
//
//	node work-uri/gen_vectors.mjs 1 5500 5500 3000 gold/internal/uri/testdata/vectors-seed1.jsonl.gz
//	node work-uri/gen_vectors.mjs 2 5500 5500 3000 gold/internal/uri/testdata/vectors-seed2.jsonl.gz
//
// Larger runs are replayed with URI_VECTORS, for example:
//
//	node work-uri/gen_vectors.mjs 1 30000 30000 15000 /tmp/big1.jsonl.gz
//	URI_VECTORS=/tmp/big1.jsonl.gz go test ./internal/uri -run TestVectors -v
//
// The digest in TestIDNAExhaustiveDigest is printed by node work-uri/gen_idna_digest.mjs.

package uri

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ajv-validator/ajv-go/internal/js"
)

// vector is one line of the testdata files written by work-uri/gen_vectors.mjs.
type vector struct {
	T  string         `json:"t"`
	I  string         `json:"i"`
	P  map[string]any `json:"p"`
	S  *string        `json:"s"`
	SE []string       `json:"se"`
	B  string         `json:"b"`
	R  string         `json:"r"`
	O  *string        `json:"o"`
	E  []string       `json:"e"`
	W  int            `json:"w"`
}

// unhex decodes the WTF-8 hex that gen_vectors.mjs writes for records holding lone surrogates.
func unhex(t *testing.T, s string) string {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return string(b)
}

// decodeWTF8 replaces every hex-encoded string of a "w" record by its WTF-8 bytes.
func (v *vector) decodeWTF8(t *testing.T) {
	if v.W == 0 {
		return
	}
	for _, p := range []*string{&v.I, &v.B, &v.R} {
		*p = unhex(t, *p)
	}
	for _, p := range []*string{v.S, v.O} {
		if p != nil {
			*p = unhex(t, *p)
		}
	}
	for _, e := range [][]string{v.SE, v.E} {
		for i := range e {
			e[i] = unhex(t, e[i])
		}
	}
	for k, x := range v.P {
		if str, ok := x.(string); ok && k != "portType" {
			v.P[k] = unhex(t, str)
		}
	}
}

// componentProps renders c the way gen_vectors.mjs snapshots a parse result.
func componentProps(c *Components) map[string]any {
	m := map[string]any{}
	put := func(k string, p *string) {
		if p != nil {
			m[k] = *p
		}
	}
	put("scheme", c.Scheme)
	put("userinfo", c.Userinfo)
	put("host", c.Host)
	if c.Port != nil {
		m["port"] = *c.Port
		if *c.Port == "" {
			m["portType"] = "string"
		} else {
			m["portType"] = "number"
		}
	}
	put("path", c.Path)
	put("query", c.Query)
	put("fragment", c.Fragment)
	put("reference", c.Reference)
	put("error", c.Error)
	if c.Secure != nil {
		m["secure"] = *c.Secure
	}
	put("resourceName", c.ResourceName)
	put("nid", c.NID)
	put("nss", c.NSS)
	put("uuid", c.UUID)
	return m
}

func errPair(err error) []string {
	var je *js.Error
	if errors.As(err, &je) {
		return []string{je.Name, je.Message}
	}
	return []string{"?", err.Error()}
}

func showResult(s *string, e []string) string {
	if e != nil {
		return fmt.Sprintf("throws %s: %q", e[0], e[1])
	}
	if s == nil {
		return "<nil>"
	}
	return strconv.Quote(*s)
}

func sameResult(gotS string, gotErr error, wantS *string, wantE []string) bool {
	if wantE != nil {
		if gotErr == nil {
			return false
		}
		g := errPair(gotErr)
		return g[0] == wantE[0] && g[1] == wantE[1]
	}
	return gotErr == nil && wantS != nil && gotS == *wantS
}

func equalProps(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// checkVector returns a description of the mismatch, or "" when the port agrees with Node.
func checkVector(v *vector) string {
	switch v.T {
	case "p":
		c := Parse(v.I)
		if got := componentProps(c); !equalProps(got, v.P) {
			return fmt.Sprintf("parse(%q)\n  got  %v\n  want %v", v.I, got, v.P)
		}
		s, err := TrySerialize(c)
		if !sameResult(s, err, v.S, v.SE) {
			var gotE []string
			if err != nil {
				gotE = errPair(err)
			}
			return fmt.Sprintf("serialize(parse(%q))\n  got  %s\n  want %s", v.I, showResult(&s, gotE), showResult(v.S, v.SE))
		}
	case "r":
		s, err := TryResolve(v.B, v.R)
		if !sameResult(s, err, v.O, v.E) {
			var gotE []string
			if err != nil {
				gotE = errPair(err)
			}
			return fmt.Sprintf("resolve(%q, %q)\n  got  %s\n  want %s", v.B, v.R, showResult(&s, gotE), showResult(v.O, v.E))
		}
	case "h":
		got, ok := whatwgHostname(v.I)
		if ok != (v.O != nil) || (ok && got != *v.O) {
			return fmt.Sprintf("new URL('http://' + %q).hostname\n  got  %q (ok=%v)\n  want %s", v.I, got, ok, showResult(v.O, nil))
		}
	default:
		return "unknown vector type " + v.T
	}
	return ""
}

func replayFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	counts := map[string]int{}
	mismatches := map[string]int{}
	shown := 0
	for sc.Scan() {
		var v vector
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		v.decodeWTF8(t)
		counts[v.T]++
		if v.W != 0 {
			counts["w"]++
		}
		if msg := checkVector(&v); msg != "" {
			mismatches[v.T]++
			if shown < 25 {
				t.Errorf("%s: %s", filepath.Base(path), msg)
				shown++
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	total, bad := 0, 0
	for k, n := range counts {
		total += n
		bad += mismatches[k]
	}
	total -= counts["w"]
	t.Logf("%s: %d vectors (parse %d, resolve %d, host %d; %d with lone surrogates), %d mismatches",
		filepath.Base(path), total, counts["p"], counts["r"], counts["h"], counts["w"], bad)
	if bad != 0 {
		t.Errorf("%s: %d mismatches (parse %d, resolve %d, host %d)", filepath.Base(path), bad,
			mismatches["p"], mismatches["r"], mismatches["h"])
	}
}

// TestVectors replays the differential vectors in testdata against the port. Extra vector files
// (for example larger runs of gen_vectors.mjs) can be listed in URI_VECTORS, separated by the OS
// path list separator.
func TestVectors(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if extra := os.Getenv("URI_VECTORS"); extra != "" {
		files = append(files, filepath.SplitList(extra)...)
	}
	if len(files) == 0 {
		t.Fatal("no vector files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) { replayFile(t, f) })
	}
}

// idnaDigest is printed by work-uri/gen_idna_digest.mjs (Node 26.10.0, ada 4.0.0).
const idnaDigest = "2a3f516aa9ef175337c541b0057b271bc3431762fb7dc3c2dda55f65edde252b"

// idnaTemplates must match TEMPLATES in work-uri/gen_idna_digest.mjs.
var idnaTemplates = []func(c string) string{
	func(c string) string { return c },
	func(c string) string { return "a" + c },
	func(c string) string { return c + "a" },
	func(c string) string { return "xn--" + c },
	func(c string) string { return "a" + c + "́.b" },
	func(c string) string { return "א" + c },
	func(c string) string { return c + "ا" },
	func(c string) string { return "क्" + c + "ष" },
}

// TestIDNAExhaustiveDigest converts every code point that fast-uri can hand to `new URL`, in
// several label contexts, and compares a digest of all results with Node's. Set
// URI_IDNA_DUMP to a file name to write the lines for diffing against the generator's dump.
func TestIDNAExhaustiveDigest(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive IDNA check skipped in -short mode")
	}
	h := sha256.New()
	var dump *bufio.Writer
	if name := os.Getenv("URI_IDNA_DUMP"); name != "" {
		f, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		dump = bufio.NewWriter(f)
		defer dump.Flush()
	}
	var line strings.Builder
	n := 0
	for cp := rune(0); cp <= 0x10ffff; cp++ {
		if cp >= 0xd800 && cp <= 0xdfff {
			continue
		}
		c := string(cp)
		if hasForbiddenDomainHostCharacter(c) {
			continue
		}
		line.Reset()
		line.WriteString(strconv.FormatInt(int64(cp), 16))
		for _, tpl := range idnaTemplates {
			o, ok := whatwgHostname(tpl(c))
			if !ok {
				o = "<ERR>"
			}
			line.WriteByte('\t')
			line.WriteString(o)
		}
		line.WriteByte('\n')
		h.Write([]byte(line.String()))
		if dump != nil {
			dump.WriteString(line.String())
		}
		n++
	}
	got := hex.EncodeToString(h.Sum(nil))
	t.Logf("%d code points x %d templates: sha256 %s", n, len(idnaTemplates), got)
	if got != idnaDigest {
		t.Errorf("digest mismatch: got %s, want %s", got, idnaDigest)
	}
}
