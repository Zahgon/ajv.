package uri

import (
	"errors"
	"testing"

	"github.com/ajv-validator/ajv-go/internal/js"
)

func str(s string) *string { return &s }

// Expected values below were produced by fast-uri 3.1.9 under Node 26.10.
func TestParseExamples(t *testing.T) {
	tests := []struct {
		in   string
		want Components
		ser  string // serialize(parse(in)), or the message it throws
		err  string // name of the error serialize throws, "" when it returns
	}{
		{"#/definitions/a%20b", Components{Path: str(""), Fragment: str("/definitions/a%20b"), Reference: str("same-document")}, "#/definitions/a%20b", ""},
		{"http://Example.COM:80/a/./b/../c", Components{Scheme: str("http"), Host: str("example.com"), Port: str("80"), Path: str("/a/./b/../c"), Reference: str("absolute")}, "http://example.com/a/c", ""},
		{"HTTPS://x.y/%7Euser", Components{Scheme: str("https"), Host: str("x.y"), Path: str("/~user"), Reference: str("absolute")}, "https://x.y/~user", ""},
		{"urn:uuid:F81D4FAE-7DEC-11D0-A765-00A0C91E6BF6", Components{Scheme: str("urn"), Reference: str("absolute"), NID: str("uuid"), UUID: str("F81D4FAE-7DEC-11D0-A765-00A0C91E6BF6")}, "urn:uuid:f81d4fae-7dec-11d0-a765-00a0c91e6bf6", ""},
		{"http://bücher.example/ä?q#f", Components{Scheme: str("http"), Host: str("xn--bcher-kva.example"), Path: str("/%C3%A4"), Query: str("q"), Fragment: str("f"), Reference: str("uri")}, "http://xn--bcher-kva.example/%C3%A4?q#f", ""},
		{"ws://H:80/p?q#f", Components{Scheme: str("ws"), Host: str("h"), Port: str("80"), Fragment: str("f"), Reference: str("uri"), Secure: new(bool), ResourceName: str("/p?q")}, "ws://h/p?q", ""},
		{"%zz", Components{Path: str("%25zz"), Error: str("URI contains malformed percent-encoding."), Reference: str("relative")}, "%25zz", ""},
		{"[::1", Components{Scheme: str("["), Path: str(":1"), Error: str("URI scheme is malformed."), Reference: str("absolute")}, "URI scheme is malformed.", "TypeError"},
		{"http://[::1", Components{Scheme: str("http"), Host: str("["), Port: str(""), Path: str(":1"), Error: str(`URI path must start with "/" when authority is present.`), Reference: str("absolute")}, "http://[/:1", ""},
		{"http://[0:0::1]:8080", Components{Scheme: str("http"), Host: str("::1"), Port: str("8080"), Path: str(""), Reference: str("absolute")}, "http://[::1]:8080/", ""},
		{"urn:", Components{Scheme: str("urn"), Path: str(""), Reference: str("absolute"), Error: str("URN can not be parsed")}, "URN without nid cannot be serialized", "Error"},
		{"a b", Components{Path: str("a%20b"), Reference: str("relative")}, "a%20b", ""},
	}
	for _, tt := range tests {
		got := Parse(tt.in)
		if g, w := componentProps(got), componentProps(&tt.want); !equalProps(g, w) {
			t.Errorf("Parse(%q) = %v, want %v", tt.in, g, w)
		}
		s, err := TrySerialize(got)
		if tt.err != "" {
			var je *js.Error
			if !errors.As(err, &je) || je.Name != tt.err || je.Message != tt.ser {
				t.Errorf("TrySerialize(Parse(%q)) error = %v, want %s: %s", tt.in, err, tt.err, tt.ser)
			}
			if Serialize(got) != "" {
				t.Errorf("Serialize(Parse(%q)) should be empty when fast-uri throws", tt.in)
			}
			continue
		}
		if err != nil || s != tt.ser {
			t.Errorf("TrySerialize(Parse(%q)) = %q, %v; want %q", tt.in, s, err, tt.ser)
		}
	}
}

func TestResolveExamples(t *testing.T) {
	tests := []struct{ base, ref, want, err string }{
		{"http://example.com/root.json", "#foo", "http://example.com/root.json#foo", ""},
		{"http://example.com/a/b/", "../x/y.json#/z", "http://example.com/a/x/y.json#/z", ""},
		{"", "#/definitions/a", "#/definitions/a", ""},
		{"urn:uuid:f81d4fae-7dec-11d0-a765-00a0c91e6bf6", "#/a", "urn:uuid:f81d4fae-7dec-11d0-a765-00a0c91e6bf6#/a", ""},
		{"a/b", "../../c", "../c", ""},
		{"http://a:80/x", "y", "http://a:80/y", ""}, // resolve() serializes without the http handler
		{"http://x/", "//bücher.example/", "http://xn--bcher-kva.example/", ""},
		{"http://x/", "//0x7f.1/", "http://127.0.0.1/", ""},
		{"http://x/", "%zz", "", "URI contains malformed percent-encoding."},
		{"http://x/", "//é%zz/", "", "URI contains malformed percent-encoding."},
	}
	for _, tt := range tests {
		got, err := TryResolve(tt.base, tt.ref)
		if tt.err != "" {
			var je *js.Error
			if !errors.As(err, &je) || je.Name != "Error" || je.Message != tt.err {
				t.Errorf("TryResolve(%q, %q) error = %v, want Error: %s", tt.base, tt.ref, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want || Resolve(tt.base, tt.ref) != tt.want {
			t.Errorf("TryResolve(%q, %q) = %q, %v; want %q", tt.base, tt.ref, got, err, tt.want)
		}
	}
}

func TestSerializeNilAndEmpty(t *testing.T) {
	if s, err := TrySerialize(nil); s != "" || err != nil {
		t.Errorf("TrySerialize(nil) = %q, %v", s, err)
	}
	if s := Serialize(&Components{Scheme: str("HTTP"), Host: str("a")}); s != "HTTP://a/" {
		t.Errorf("Serialize = %q", s)
	}
}

// A surrogate pair spelled as two WTF-8 three-byte sequences is read as the code units it
// spells, so it behaves like the four-byte form of the same code point.
func TestSplitSurrogatePair(t *testing.T) {
	split := "a\xed\xa0\xbd\xed\xb8\x80" // U+D83D U+DE00
	joined := "a\U0001F600"
	if g, w := Serialize(Parse(split)), Serialize(Parse(joined)); g != w {
		t.Errorf("split pair path: %q, joined: %q", g, w)
	}
	g, _ := whatwgHostname(split)
	w, _ := whatwgHostname(joined)
	if g != w {
		t.Errorf("split pair host: %q, joined: %q", g, w)
	}
	// A lone surrogate is encoded as U+FFFD, as fast-uri does.
	if g := Serialize(Parse("a\xed\xa0\x80")); g != "a%EF%BF%BD" {
		t.Errorf("lone surrogate: %q", g)
	}
}

func TestTablesSanity(t *testing.T) {
	if idnaMappings[0] != 0 {
		t.Fatal("offset 0 of idnaMappings must be the empty (ignored) mapping")
	}
	if len(idnaStart) != len(idnaStatus) || idnaStart[0] != 0 {
		t.Fatal("bad idna table")
	}
	if len(decompOff) != len(decompCP)+1 || len(compLo) != len(compCP) || len(compHi) != len(compCP) {
		t.Fatal("bad normalization tables")
	}
	if len(dirStart) != len(dirFinal) || len(dirValue) != len(dirFinal) || len(combiningRanges)%2 != 0 {
		t.Fatal("bad validity tables")
	}
}

func FuzzParseSerialize(f *testing.F) {
	for _, s := range []string{"", "#/a", "http://Example.COM:80/a/./b", "urn:uuid:x", "ws://h/p?q", "http://[::1", "http://ü/%zz", "\xed\xa0\x80", "http://xn--a.é/"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c := Parse(s)
		_, _ = TrySerialize(c)
		_, _ = whatwgHostname(s)
	})
}

func FuzzResolve(f *testing.F) {
	f.Add("http://a/b/c", "../d")
	f.Add("", "#x")
	f.Add("urn:a:b", "c")
	f.Add("http://x/", "//é.xn--zz/")
	f.Fuzz(func(t *testing.T, base, ref string) {
		_, _ = TryResolve(base, ref)
	})
}
