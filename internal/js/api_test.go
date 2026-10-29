package js

import (
	"strings"
	"testing"
)

func TestNoPanics(t *testing.T) {
	rng := &mulberry32{a: 1}
	alphabet := []string{"{", "}", "[", "]", ",", ":", "\"", "\\", "u", "0", "1", "-", ".", "e", "t", "n", " ", "\x01", "\xed\xa0\x80", "\xff", "\u00e9", "\U0001F600", "\u03a3"}
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for n := rng.next() % 20; n > 0; n-- {
			b.WriteString(alphabet[rng.next()%uint32(len(alphabet))])
		}
		s := b.String()
		_, _ = Parse(s)
		_ = ToLowerCase(s) + Quote(s) + Trim(s)
		_, _ = ToNumber(s), ParseInt(s, int(rng.next()%40))
		k := int(rng.next()%30) - 10
		_ = CharAt(s, k)
		_ = Compare(s, "a") + Len(s)
	}
}
