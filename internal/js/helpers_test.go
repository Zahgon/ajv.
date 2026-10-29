package js

import (
	"bufio"
	"compress/gzip"
	"math"
	"os"
	"strconv"
	"testing"
)

// readVectors reads testdata/name (gzipped JSONL written by work-js) with Parse.
func readVectors(t *testing.T, name string) []*Object {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 1<<20), 1<<28)
	var out []*Object
	for sc.Scan() {
		v, err := Parse(sc.Text())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, v.(*Object))
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func str(o *Object, k string) string {
	v, _ := o.Get(k)
	s, _ := v.(string)
	return s
}

func num(o *Object, k string) float64 {
	v, _ := o.Get(k)
	f, _ := v.(float64)
	return f
}

func arr(o *Object, k string) Array {
	v, _ := o.Get(k)
	a, _ := v.(Array)
	return a
}

// mulberry32 mirrors work-js/lib.mjs.
type mulberry32 struct{ a uint32 }

func (m *mulberry32) next() uint32 {
	m.a += 0x6d2b79f5
	t := m.a
	t = (t ^ t>>15) * (1 | t)
	t = (t + (t^t>>7)*(61|t)) ^ t
	return t ^ t>>14
}

func (m *mulberry32) bits() float64 {
	hi := uint64(m.next())
	lo := uint64(m.next())
	return math.Float64frombits(hi<<32 | lo)
}

// fnv mirrors work-js/lib.mjs: FNV-1a over code units, each string followed by a 0 unit.
func fnv(h uint32, strs ...string) uint32 {
	for _, s := range strs {
		for _, u := range CodeUnits(s) {
			h = (h ^ uint32(u)) * 16777619
		}
		h *= 16777619
	}
	return h
}

const fnvBasis = 0x811c9dc5

// numBits mirrors work-js/lib.mjs.
func numBits(f float64) string {
	if math.IsNaN(f) {
		return "nan"
	}
	s := strconv.FormatUint(math.Float64bits(f), 16)
	for len(s) < 16 {
		s = "0" + s
	}
	return s
}

func bitsNum(h string) float64 {
	if h == "nan" {
		return math.NaN()
	}
	u, _ := strconv.ParseUint(h, 16, 64)
	return math.Float64frombits(u)
}

// mismatches collects failures and reports the first few.
type mismatches struct {
	t     *testing.T
	what  string
	count int
	total int
}

func (m *mismatches) check(ok bool, format string, args ...any) {
	m.t.Helper()
	m.total++
	if ok {
		return
	}
	m.count++
	if m.count <= 15 {
		m.t.Errorf(m.what+": "+format, args...)
	}
}

func (m *mismatches) done() {
	m.t.Helper()
	m.t.Logf("%s: %d checks, %d mismatches", m.what, m.total, m.count)
	if m.count > 0 {
		m.t.Errorf("%s: %d of %d mismatched", m.what, m.count, m.total)
	}
}
