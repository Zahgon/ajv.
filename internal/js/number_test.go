package js

import (
	"math"
	"strconv"
	"testing"
)

// sameNumber compares got with a number written as String(x) ("-0" for negative zero), read
// back with strconv.ParseFloat.
func sameNumber(got float64, want string) bool {
	w, err := strconv.ParseFloat(want, 64)
	if err != nil {
		return false
	}
	if math.IsNaN(w) {
		return math.IsNaN(got)
	}
	return math.Float64bits(got) == math.Float64bits(w)
}

func TestNumberParseVectors(t *testing.T) {
	recs := readVectors(t, "numparse.jsonl.gz")
	m := &mismatches{t: t, what: "Number/parseInt"}
	strings := 0
	for _, r := range recs {
		if str(r, "k") == "meta" {
			t.Logf("vectors from %s", str(r, "runtime"))
			continue
		}
		strings++
		s := str(r, "s")
		m.check(sameNumber(ToNumber(s), str(r, "n")), "Number(%+q) = %v want %s", s, ToNumber(s), str(r, "n"))
		m.check(sameNumber(ParseInt(s, 0), str(r, "i")), "parseInt(%+q) = %v want %s", s, ParseInt(s, 0), str(r, "i"))
		for _, rr := range arr(r, "r") {
			pair := rr.(Array)
			radix := int(pair[0].(float64))
			got := ParseInt(s, radix)
			m.check(sameNumber(got, pair[1].(string)), "parseInt(%+q, %d) = %v want %s", s, radix, got, pair[1])
		}
	}
	t.Logf("%d strings", strings)
	m.done()
}
