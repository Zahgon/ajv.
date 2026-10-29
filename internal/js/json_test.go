package js

import (
	"strconv"
	"strings"
	"testing"
)

// buildTagged mirrors build() in work-js/gen_json.mjs.
func buildTagged(t *testing.T, v any) any {
	a := v.(Array)
	switch a[0].(string) {
	case "z":
		return nil
	case "u":
		return Undefined
	case "b":
		return a[1].(bool)
	case "n":
		f, err := strconv.ParseFloat(a[1].(string), 64)
		if err != nil {
			t.Fatalf("bad number %q", a[1])
		}
		return f
	case "s":
		return a[1].(string)
	case "a":
		out := Array{}
		for _, it := range a[1].(Array) {
			out = append(out, buildTagged(t, it))
		}
		return out
	}
	o := NewObject()
	for _, op := range a[1].(Array) {
		op := op.(Array)
		if op[0].(string) == "set" {
			o.Set(op[1].(string), buildTagged(t, op[2]))
		} else {
			o.Delete(op[1].(string))
		}
	}
	return o
}

func dumpUnits(b *strings.Builder, s string) {
	b.WriteByte('s')
	for _, u := range CodeUnits(s) {
		b.WriteString(strconv.FormatUint(uint64(u), 16))
		b.WriteByte(',')
	}
	b.WriteByte(';')
}

// dump mirrors dump() in work-js/gen_json.mjs.
func dump(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteByte('z')
	case bool:
		if x {
			b.WriteByte('t')
		} else {
			b.WriteByte('f')
		}
	case float64:
		b.WriteByte('n')
		if x == 0 && 1/x < 0 {
			b.WriteString("-0")
		} else {
			b.WriteString(strconv.FormatFloat(x, 'g', -1, 64))
		}
	case string:
		dumpUnits(b, x)
	case Array:
		b.WriteByte('[')
		for i, it := range x {
			if i > 0 {
				b.WriteByte(' ')
			}
			dump(b, it)
		}
		b.WriteByte(']')
	case *Object:
		b.WriteByte('{')
		for i, k := range x.Keys() {
			if i > 0 {
				b.WriteByte(' ')
			}
			dumpUnits(b, k)
			b.WriteByte('=')
			v, _ := x.Get(k)
			dump(b, v)
		}
		b.WriteByte('}')
	default:
		b.WriteString("?")
	}
}

// sameDump compares dumps; numbers are compared by value since the JS side writes String(x).
func sameDump(got, want string) bool {
	if got == want {
		return true
	}
	g, w := strings.Fields(strings.NewReplacer("[", " [ ", "]", " ] ", "{", " { ", "}", " } ", "=", " = ").Replace(got)),
		strings.Fields(strings.NewReplacer("[", " [ ", "]", " ] ", "{", " { ", "}", " } ", "=", " = ").Replace(want))
	if len(g) != len(w) {
		return false
	}
	for i := range g {
		if g[i] == w[i] {
			continue
		}
		if !strings.HasPrefix(g[i], "n") || !strings.HasPrefix(w[i], "n") || !sameNumber(mustFloat(g[i][1:]), w[i][1:]) {
			return false
		}
	}
	return true
}

func mustFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func TestJSONVectors(t *testing.T) {
	recs := readVectors(t, "json.jsonl.gz")
	q1 := &mismatches{t: t, what: "Quote of every code unit"}
	qs := &mismatches{t: t, what: "Quote of strings"}
	ok := &mismatches{t: t, what: "Object key order"}
	pv := &mismatches{t: t, what: "Parse results"}
	pe := &mismatches{t: t, what: "Parse errors"}
	listed := map[int]bool{}
	for _, r := range recs {
		switch str(r, "k") {
		case "meta":
			t.Logf("vectors from %s", str(r, "runtime"))
		case "quote1":
			u := int(num(r, "u"))
			listed[u] = true
			got := Quote(FromCodeUnits([]uint16{uint16(u)}))
			q1.check(got == str(r, "q"), "U+%04X: got %+q want %+q", u, got, str(r, "q"))
		case "quote":
			got := Quote(str(r, "s"))
			qs.check(got == str(r, "q"), "%+q: got %+q want %+q", str(r, "s"), got, str(r, "q"))
		case "object":
			v := buildTagged(t, r.vals["v"])
			var want []string
			for _, k := range arr(r, "keys") {
				want = append(want, k.(string))
			}
			got := v.(*Object).Keys()
			ok.check(strings.Join(got, "\x00") == strings.Join(want, "\x00"), "keys: got %q want %q", got, want)
		case "parse":
			text := str(r, "t")
			v, err := Parse(text)
			if e, ok := r.Get("e"); ok {
				got := "<no error>"
				if err != nil {
					got = err.Error()
				}
				pe.check(got == e.(string), "Parse(%+q): got %+q want %+q", text, got, e)
				continue
			}
			if err != nil {
				pv.check(false, "Parse(%+q): unexpected error %v", text, err)
				continue
			}
			var b strings.Builder
			dump(&b, v)
			pv.check(sameDump(b.String(), str(r, "d")), "Parse(%+q): got %s want %s", text, b.String(), str(r, "d"))
		}
	}
	for u := 0; u < 0x10000; u++ {
		if listed[u] {
			continue
		}
		s := FromCodeUnits([]uint16{uint16(u)})
		q1.check(Quote(s) == "\""+s+"\"", "U+%04X should not be escaped", u)
	}
	q1.done()
	qs.done()
	ok.done()
	pv.done()
	pe.done()
}
