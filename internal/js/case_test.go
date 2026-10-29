package js

import (
	"testing"
)

// fromCodePoint is String.fromCodePoint(cp) (a lone surrogate for a surrogate code point).
func fromCodePoint(cp int) string {
	if cp < 0x10000 {
		return FromCodeUnits([]uint16{uint16(cp)})
	}
	cp -= 0x10000
	return FromCodeUnits([]uint16{uint16(0xd800 + cp>>10), uint16(0xdc00 + cp&0x3ff)})
}

func TestCaseVectors(t *testing.T) {
	recs := readVectors(t, "case.jsonl.gz")
	cpm := &mismatches{t: t, what: "code point mapping"}
	sig := &mismatches{t: t, what: "final sigma probes"}
	ctx := &mismatches{t: t, what: "context strings"}
	listed := map[int]bool{}
	for _, r := range recs {
		switch str(r, "k") {
		case "meta":
			t.Logf("vectors from %s", str(r, "runtime"))
		case "map":
			cp := int(num(r, "cp"))
			listed[cp] = true
			s := fromCodePoint(cp)
			l := ToLowerCase(s)
			cpm.check(l == str(r, "l"), "U+%04X: got %q want %q", cp, l, str(r, "l"))
		case "sigma":
			for _, run := range arr(r, "runs") {
				rr := run.(Array)
				lo, hi, code := int(rr[0].(float64)), int(rr[1].(float64)), int(rr[2].(float64))
				for cp := lo; cp <= hi; cp++ {
					x := fromCodePoint(cp)
					probes := [4]string{"A\u03a3" + x, "A\u03a3" + x + "B", x + "\u03a3", "A" + x + "\u03a3"}
					got := 0
					for i, p := range probes {
						u := CodeUnits(ToLowerCase(FromCodeUnits(CodeUnits(p))))
						var c uint16
						if i < 2 {
							c = u[1]
						} else {
							c = u[len(u)-1]
						}
						if c == 0x3c2 {
							got |= 1 << i
						}
					}
					sig.check(got == code, "U+%04X: probe bits %04b want %04b", cp, got, code)
				}
			}
		case "str":
			s := str(r, "s")
			l := ToLowerCase(s)
			ctx.check(l == str(r, "l"), "%+q: got %+q want %+q", s, l, str(r, "l"))
		}
	}
	for cp := 0; cp < 0x110000; cp++ {
		if listed[cp] {
			continue
		}
		s := fromCodePoint(cp)
		cpm.check(ToLowerCase(s) == s, "U+%04X should map to itself", cp)
	}
	cpm.done()
	sig.done()
	ctx.done()
}
