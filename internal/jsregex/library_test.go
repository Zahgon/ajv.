package jsregex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestLibrary checks every regex of compromise (model dump and fork/src
// literals, recorded by work-regex/gen_library.mjs) with the supported flags:
// Test against the whole word list (a bitset recorded from node) and on a
// sample of words plus punctuation-heavy tokens.
func TestLibrary(t *testing.T) {
	lines := gzLines(t, "testdata/library.jsonl.gz")
	var h struct {
		Words []jsStr
	}
	if err := json.Unmarshal(lines[0], &h); err != nil {
		t.Fatalf("header: %v", err)
	}
	words := make([]string, len(h.Words))
	for i, w := range h.Words {
		words[i] = w.s
	}
	var fails []string
	tests := 0
	for _, line := range lines[1:] {
		var v struct {
			vector
			O    string `json:"o"`
			Bits string `json:"bits"`
		}
		if err := json.Unmarshal(line, &v); err != nil {
			t.Fatalf("bad vector: %v", err)
		}
		for _, m := range checkVector(&v.vector) {
			fails = append(fails, v.O+": "+m.String())
		}
		re, err := Compile(v.P.s, v.F)
		if err != nil {
			continue
		}
		bits, err := base64.StdEncoding.DecodeString(v.Bits)
		if err != nil {
			t.Fatal(err)
		}
		for i, w := range words {
			tests++
			if want := bits[i>>3]&(1<<(i&7)) != 0; re.Test(w) != want {
				fails = append(fails, fmt.Sprintf("%s: /%s/%s.test(%q) = %v, want %v", v.O, v.P.s, v.F, w, !want, want))
			}
		}
	}
	t.Logf("%d regexes, %d words, %d Test calls, %d mismatches", len(lines)-1, len(words), tests, len(fails))
	if len(fails) > 0 {
		if len(fails) > 40 {
			fails = append(fails[:40], fmt.Sprintf("... and %d more", len(fails)-40))
		}
		t.Errorf("mismatches:\n%s", strings.Join(fails, "\n"))
	}
}
