package js

import (
	"fmt"
	"strconv"
	"testing"
)

// miscOutputs mirrors the outputs of work-js/gen_misc.mjs kept for this package: length,
// trim(), trimStart(), charAt(a) and the comparison of s and t.
func miscOutputs(s, t string, a int) []any {
	return []any{Len(s), Trim(s), TrimStart(s), CharAt(s, a), Compare(s, t)}
}

func sameValue(got, want any) bool {
	if g, ok := got.(int); ok {
		w, ok := want.(float64)
		return ok && float64(g) == w
	}
	return got == want
}

func TestMiscVectors(t *testing.T) {
	recs := readVectors(t, "misc.jsonl.gz")
	sm := &mismatches{t: t, what: "string methods"}
	nm := &mismatches{t: t, what: "Math.round"}
	for _, r := range recs {
		switch str(r, "k") {
		case "meta":
			t.Logf("vectors from %s", str(r, "runtime"))
		case "str":
			s, tt := str(r, "s"), str(r, "t")
			a := int(num(r, "a"))
			got := miscOutputs(s, tt, a)
			for j, w := range arr(r, "o") {
				sm.check(sameValue(got[j], w), "#%d s=%+q t=%+q a=%d: got %+q want %+q", j, s, tt, a, fmt.Sprint(got[j]), fmt.Sprint(w))
			}
		case "num":
			x, _ := strconv.ParseFloat(str(r, "x"), 64)
			nm.check(sameNumber(MathRound(x), str(r, "o")), "Math.round(%v) = %v want %v", x, MathRound(x), str(r, "o"))
		}
	}
	sm.done()
	nm.done()
}
