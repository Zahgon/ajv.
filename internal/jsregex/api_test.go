package jsregex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mustCompile(tb testing.TB, source, flags string) *Regexp {
	tb.Helper()
	re, err := Compile(source, flags)
	if err != nil {
		tb.Fatal(err)
	}
	return re
}

func TestAPI(t *testing.T) {
	re := mustCompile(t, `(?<y>\d{4})-(?<m>\d\d)|/`, "i")
	if !re.Test("on 2024-05!") || re.Test("2024") {
		t.Error("Test results")
	}
	_, err := Compile("(", "")
	if se, ok := err.(*SyntaxError); !ok || se.Message != "Invalid regular expression: /(/: Unterminated group" {
		t.Errorf("error = %v", err)
	}
	if _, err := Compile("(", "um"); err == nil || err.Error() != "Invalid regular expression: /(/mu: Unterminated group" {
		t.Errorf("flags order in error = %v", err)
	}
	for _, flags := range []string{"uu", "x", "g", "v"} {
		if _, err := Compile("a", flags); err == nil || err.Error() != "Invalid flags supplied to RegExp constructor '"+flags+"'" {
			t.Errorf("flags %q error = %v", flags, err)
		}
	}
}

// TestUTF16 checks that lone surrogates (WTF-8) are matched as code units and
// that non-unicode patterns see the two units of an astral character.
func TestUTF16(t *testing.T) {
	lone := "\xed\xa0\xbd" // U+D83D
	if !mustCompile(t, `^x\ud83d$`, "").Test("x" + lone) {
		t.Error("lone surrogate")
	}
	if !mustCompile(t, `^..$`, "").Test("😀") || mustCompile(t, `^..$`, "u").Test("😀") {
		t.Error(". on an astral character")
	}
}

// TestLongSubjects checks matching on 20k code-unit subjects (no Go stack
// overflow, reasonable time).
func TestLongSubjects(t *testing.T) {
	ab := strings.Repeat("ab", 10000)
	cases := []struct {
		pat, flags, s string
		want          bool
	}{
		{`^(a|b)*c$`, "", ab + "c", true},
		{`^(?:a|b)*$`, "", ab, true},
		{`^(?:(a)|b)+$`, "", ab, true},
		{`^(ab)+\1$`, "", ab, true},
		{`(?<=^(?:ab)*)$`, "", ab, true},
		{`^.*?x`, "s", ab, false},
		{`^[\s\S]*$`, "u", strings.Repeat("😀", 10000), true},
		{`^(?:\p{L}|\s)+$`, "u", strings.Repeat("ab ", 7000), true},
		{`(\w+)\s(\w+)$`, "", strings.Repeat("x", 20000) + " y", true},
	}
	for _, c := range cases {
		t0 := time.Now()
		got := mustCompile(t, c.pat, c.flags).Test(c.s)
		if got != c.want {
			t.Errorf("/%s/%s on %d units = %v", c.pat, c.flags, len(c.s), got)
		}
		if d := time.Since(t0); d > 2*time.Second {
			t.Errorf("/%s/%s took %v", c.pat, c.flags, d)
		}
	}
}

func libraryRegexes(tb testing.TB, n int) [][2]string {
	lines := gzLines(tb.(*testing.T), "testdata/library.jsonl.gz")
	var out [][2]string
	for _, line := range lines[1:] {
		var v vector
		if err := json.Unmarshal(line, &v); err != nil || v.Err.valid {
			continue
		}
		out = append(out, [2]string{v.P.s, v.F})
		if len(out) == n {
			break
		}
	}
	return out
}

// TestCompileSpeed compiles 200 of compromise's regexes.
func TestCompileSpeed(t *testing.T) {
	res := libraryRegexes(t, 200)
	best := time.Hour
	for k := 0; k < 5; k++ {
		t0 := time.Now()
		for _, r := range res {
			mustCompile(t, r[0], r[1])
		}
		if d := time.Since(t0); d < best {
			best = d
		}
	}
	t.Logf("compiled %d library regexes in %v", len(res), best)
	if best > 200*time.Millisecond {
		t.Errorf("compiling %d regexes took %v", len(res), best)
	}
}

func BenchmarkCompile200(b *testing.B) {
	t := &testing.T{}
	res := libraryRegexes(t, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, r := range res {
			mustCompile(b, r[0], r[1])
		}
	}
}

func BenchmarkTestShortWord(b *testing.B) {
	re := mustCompile(b, `[aeiou](pp|ll|ss|ff|gg|tt|rr|bb|nn|mm)ed$`, "")
	words := []string{"walked", "stopped", "happy", "running", "tagged"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.Test(words[i%len(words)])
	}
}

func BenchmarkTestAnchored(b *testing.B) {
	re := mustCompile(b, `^[0-9]+(st|nd|rd|th)$`, "i")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		re.Test("3rd")
	}
}
