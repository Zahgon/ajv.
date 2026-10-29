package uprops

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"slices"
	"testing"
)

// TestAgainstNode checks every \p{...} spelling V8 accepts (and a list it
// rejects) against data recorded from node by work-regex/gen_uprops_check.mjs:
// the code point count and an FNV-1a hash of the range list of each table.
func TestAgainstNode(t *testing.T) {
	f, err := os.Open("testdata/props.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Node, Unicode string
		Accept        [][4]any
		Reject        [][2]string
	}
	if err := json.NewDecoder(zr).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Unicode != "17.0" {
		t.Errorf("check data is Unicode %s, tables are 17.0", data.Unicode)
	}
	for _, a := range data.Accept {
		name, value := a[0].(string), a[1].(string)
		count, hash := int(a[2].(float64)), uint32(a[3].(float64))
		tab, ok := Lookup(name, value)
		if !ok {
			t.Errorf("Lookup(%q, %q) rejected; V8 accepts it", name, value)
			continue
		}
		h := fnv.New32a()
		n := 0
		for i := 0; i+1 < len(tab); i += 2 {
			n += int(tab[i+1]-tab[i]) + 1
			fmt.Fprintf(h, "%d-%d,", tab[i], tab[i+1])
		}
		if n != count || h.Sum32() != hash {
			t.Errorf("Lookup(%q, %q): %d code points (hash %08x), node has %d (hash %08x)", name, value, n, h.Sum32(), count, hash)
		}
	}
	for _, r := range data.Reject {
		if _, ok := Lookup(r[0], r[1]); ok {
			t.Errorf("Lookup(%q, %q) accepted; V8 rejects it", r[0], r[1])
		}
	}
	t.Logf("node %s: %d accepted and %d rejected spellings checked", data.Node, len(data.Accept), len(data.Reject))
}

func TestSpotChecks(t *testing.T) {
	cases := []struct {
		name, value string
		r           rune
		want        bool
	}{
		{"L", "", 'a', true}, {"Letter", "", '1', false}, {"gc", "L", 'é', true}, {"General_Category", "Letter", 'Ж', true},
		{"Lu", "", 'A', true}, {"Lu", "", 'a', false}, {"Ll", "", 'ſ', true}, {"Lt", "", 'ǅ', true}, {"Nd", "", '٣', true},
		{"sc", "Grek", 'α', true}, {"Script", "Greek", 'a', false}, {"sc", "Latn", 'K', true}, {"scx", "Hira", 'ー', true}, {"sc", "Hira", 'ー', false},
		{"Script_Extensions", "Han", '中', true}, {"sc", "Zyyy", ' ', true}, {"sc", "Inherited", '\u0301', true},
		{"Extended_Pictographic", "", '😀', true}, {"Emoji", "", '#', true}, {"Emoji_Presentation", "", '#', false}, {"Emoji_Modifier", "", 0x1F3FD, true},
		{"Emoji_Modifier_Base", "", '👍', true}, {"Emoji_Component", "", 0x200D, true}, {"Regional_Indicator", "", 0x1F1EF, true},
		{"Any", "", 0x10FFFF, true}, {"ASCII", "", 0x7F, true}, {"ASCII", "", 0x80, false}, {"Assigned", "", 0x0378, false}, {"Alphabetic", "", 'ª', true},
		{"White_Space", "", 0x3000, true}, {"White_Space", "", 0x200B, false}, {"Uppercase", "", 'Ⓐ', true}, {"Lowercase", "", 'ª', true},
		{"Cn", "", 0x0378, true}, {"Cs", "", 0xD800, true}, {"Co", "", 0xE000, true},
	}
	for _, c := range cases {
		tab, ok := Lookup(c.name, c.value)
		if !ok {
			t.Errorf("Lookup(%q, %q) failed", c.name, c.value)
			continue
		}
		if got := Is(tab, c.r); got != c.want {
			t.Errorf("Is(%s=%s, %U) = %v, want %v", c.name, c.value, c.r, got, c.want)
		}
	}
	for _, bad := range [][2]string{{"lu", ""}, {"Hyphen", ""}, {"sc", "Hrkt"}, {"Greek", ""}, {"gc", "Greek"}, {"Block", "Basic_Latin"}, {"", ""}} {
		if _, ok := Lookup(bad[0], bad[1]); ok {
			t.Errorf("Lookup(%q, %q) should fail", bad[0], bad[1])
		}
	}
}

func TestFoldClasses(t *testing.T) {
	has := func(classes [][]rune, want []rune) bool {
		for _, c := range classes {
			if slices.Equal(c, want) {
				return true
			}
		}
		return false
	}
	u, legacy := FoldClasses(true), FoldClasses(false)
	if !has(u, []rune{'K', 'k', 0x212A}) || !has(u, []rune{'S', 's', 0x17F}) || !has(u, []rune{0xDF, 0x1E9E}) {
		t.Error("simple case folding classes missing K/k/U+212A, S/s/U+017F or ß/ẞ")
	}
	// Non-unicode Canonicalize never maps non-ASCII to ASCII: ſ and U+212A stay alone.
	if !has(legacy, []rune{'K', 'k'}) || !has(legacy, []rune{'S', 's'}) || !has(legacy, []rune{0xB5, 0x39C, 0x3BC}) {
		t.Error("legacy Canonicalize classes wrong")
	}
	for _, c := range legacy {
		for _, r := range c {
			if r > 0xFFFF {
				t.Fatalf("legacy class with astral member %U", r)
			}
		}
	}
}
