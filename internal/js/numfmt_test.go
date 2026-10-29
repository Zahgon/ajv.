package js

import (
	"math"
	"testing"
)

var fmtSpecials = []float64{0, math.Copysign(0, -1), math.NaN(), math.Inf(1), math.Inf(-1), 5e-324, -5e-324, 1.7976931348623157e308, 2.2250738585072014e-308, 0.5, 1.5, 2.5, -0.5, -1.5, 1e21, 999999999999999900000, 1e-7, 1.2e-6, 123e-20, 0.1, 0.2, 0.3, 1.0 / 3, 2.0 / 3, math.Pi, math.E, 1.0005, 0.0125, 2.5e-7, -1234.5678, 1.005, 1.0045, 4.35, 0.0005, -0.0005, 0.0004999, 999.9995, 9.9995, 0.9995, 1e15, 1e16, 1e17, 1 << 53, 1<<53 + 2, 1 << 64, 1 << 31, -(1 << 31), 4294967295, 4294967296, 1e100, 1e-100, 1.5e300, 123456789.123456789, 0.000001, 0.0000001, 100, 1000, 1234567, -0.0001}

var pow10 = []float64{1, 10, 100, 1e3, 1e4, 1e5, 1e6, 1e7}

func fromParts(hi, lo uint32) float64 { return math.Float64frombits(uint64(hi)<<32 | uint64(lo)) }

func bump(x float64, d int) float64 {
	if d == 0 || math.IsNaN(x) || math.IsInf(x, 0) || x == 0 {
		return x
	}
	return math.Float64frombits(uint64(int64(math.Float64bits(x)) + int64(d)))
}

type fmtCase struct {
	x                float64
	fd, p, ed, radix int // arguments of toFixed, toPrecision, toExponential, toString(radix) in the generator
}

// genFmtCase mirrors genCase in work-js/gen_numfmt.mjs.
func genFmtCase(i int, rng *mulberry32) fmtCase {
	cat := i % 10
	var x float64
	switch cat {
	case 0, 1:
		hi := rng.next()
		x = fromParts(hi, rng.next())
	case 2:
		k := int(rng.next() % 65)
		hi := rng.next()
		lo := rng.next()
		m := float64(hi>>11)*4294967296 + float64(lo)
		x = math.Floor(math.Ldexp(m, k-53))
	case 3:
		k := float64(rng.next() % 10000000)
		x = (2*k + 1) / 2000
		x = bump(x, int(rng.next()%3)-1)
	case 4:
		k := float64(rng.next())
		x = k / pow10[rng.next()%8]
	case 5, 6:
		hi := rng.next()
		lo := rng.next()
		var e uint32
		if cat == 5 {
			e = 1073 + rng.next()%974
		} else {
			e = rng.next() % 1000
		}
		x = fromParts(hi&0x800fffff|e<<20, lo)
	case 7:
		k := float64(rng.next() % 1000000)
		x = (2*k + 1) / (2 * pow10[rng.next()%7])
	case 8:
		k := int(rng.next()%2000) - 1000
		if rng.next()&1 != 0 {
			x = 1e21 + float64(k*65536)
		} else {
			x = 1<<53 + float64(k)
		}
	default:
		x = fmtSpecials[(i/10)%len(fmtSpecials)]
	}
	if cat >= 2 && cat <= 8 && rng.next()&1 != 0 {
		x = -x
	}
	c := fmtCase{x: x}
	c.fd = int(rng.next() % 101)
	c.p = 1 + int(rng.next()%100)
	c.ed = int(rng.next() % 101)
	c.radix = 2 + int(rng.next()%35)
	return c
}

// TestNumberFormatVectors checks String(x) for the 200000 numbers of genFmtCase: the first
// records hold the outputs, the others FNV hashes of each output.
func TestNumberFormatVectors(t *testing.T) {
	recs := readVectors(t, "numfmt.jsonl.gz")
	meta := recs[0]
	t.Logf("vectors from %s", str(meta, "runtime"))
	n := int(num(meta, "n"))
	rng := &mulberry32{a: uint32(num(meta, "seed"))}
	cases := make([]fmtCase, n)
	for i := range cases {
		cases[i] = genFmtCase(i, rng)
	}
	full := &mismatches{t: t, what: "String(x) (full outputs)"}
	hashed := &mismatches{t: t, what: "String(x) (hashed outputs)"}
	for _, r := range recs[1:] {
		switch str(r, "k") {
		case "full":
			i := int(num(r, "i"))
			got := NumberToString(cases[i].x)
			full.check(got == str(r, "o"), "#%d String(%v): got %q want %q", i, cases[i].x, got, str(r, "o"))
		case "hash":
			from := int(num(r, "from"))
			for k, h := range arr(r, "h") {
				i := from + k
				got := NumberToString(cases[i].x)
				g := fnv(fnvBasis, got)
				hashed.check(int((g^g>>16)&0xffff) == int(h.(float64)), "#%d String(%v) = %s", i, cases[i].x, got)
			}
		}
	}
	full.done()
	hashed.done()
}
