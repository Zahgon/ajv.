package jsregex

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// jsStr is a JS string as recorded by work-regex/lib.mjs: a JSON string, or
// {"u":[code units]} when it contains lone surrogates.
type jsStr struct {
	s     string
	valid bool // false for JSON null (undefined)
}

func (j *jsStr) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*j = jsStr{}
		return nil
	}
	if len(b) > 0 && b[0] == '{' {
		var o struct{ U []uint16 }
		if err := json.Unmarshal(b, &o); err != nil {
			return err
		}
		*j = jsStr{fromUTF16(o.U), true}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*j = jsStr{s, true}
	return nil
}

// vecCase is one subject and whether the regexp matches it.
type vecCase struct {
	S jsStr `json:"s"`
	T bool  `json:"t"`
}

// vector is a pattern with its flags, the error new RegExp throws for it (if any) and the
// results of test on the recorded subjects.
type vector struct {
	P   jsStr     `json:"p"`
	F   string    `json:"f"`
	Err jsStr     `json:"err"`
	C   []vecCase `json:"c"`
}

type mismatch struct {
	what       string
	got, want  string
	pat, flags string
	subject    string
}

func (m mismatch) String() string {
	return fmt.Sprintf("/%s/%s on %q: %s\n   got:  %s\n   want: %s", m.pat, m.flags, m.subject, m.what, m.got, m.want)
}

// gzLines returns the non-empty lines of a gzipped JSONL file.
func gzLines(t *testing.T, path string) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var out [][]byte
	for sc.Scan() {
		if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 {
			out = append(out, append([]byte(nil), line...))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatalf("%s: empty", path)
	}
	return out
}

func loadVectors(t *testing.T, path string) []vector {
	t.Helper()
	lines := gzLines(t, path)
	vecs := make([]vector, len(lines)-1)
	for i, line := range lines[1:] {
		if err := json.Unmarshal(line, &vecs[i]); err != nil {
			t.Fatalf("bad vector %s: %v", line, err)
		}
	}
	return vecs
}

// checkVector compares one recorded vector with this package's behaviour.
func checkVector(v *vector) (out []mismatch) {
	pat := v.P.s
	bad := func(what, subj string, got, want any) {
		out = append(out, mismatch{what, fmt.Sprint(got), fmt.Sprint(want), pat, v.F, subj})
	}
	defer func() {
		if r := recover(); r != nil {
			bad("panic", "", r, "no panic")
		}
	}()
	re, err := Compile(pat, v.F)
	if v.Err.valid {
		if err == nil {
			bad("compile", "", "ok", v.Err.s)
		} else if err.Error() != v.Err.s {
			bad("compile", "", err.Error(), v.Err.s)
		}
		return
	}
	if err != nil {
		bad("compile", "", err.Error(), "ok")
		return
	}
	for _, c := range v.C {
		if g := re.Test(c.S.s); g != c.T {
			bad("test", c.S.s, g, c.T)
		}
	}
	return out
}

func runVectorFile(t *testing.T, path string) {
	vecs := loadVectors(t, path)
	if len(vecs) == 0 {
		t.Fatal("no vectors")
	}
	var fails []mismatch
	failedVecs := 0
	var slow []string
	for i := range vecs {
		t0 := time.Now()
		ms := checkVector(&vecs[i])
		if d := time.Since(t0); d > time.Second {
			slow = append(slow, fmt.Sprintf("/%s/%s took %v", vecs[i].P.s, vecs[i].F, d))
		}
		if len(ms) > 0 {
			failedVecs++
			fails = append(fails, ms...)
		}
	}
	for _, s := range slow {
		t.Logf("slow: %s", s)
	}
	t.Logf("%s: %d vectors, %d mismatching vectors, %d mismatches", path, len(vecs), failedVecs, len(fails))
	if len(fails) > 0 {
		var b strings.Builder
		for i, f := range fails {
			if i == 60 {
				fmt.Fprintf(&b, "... and %d more\n", len(fails)-60)
				break
			}
			b.WriteString(f.String())
			b.WriteByte('\n')
		}
		t.Errorf("%d mismatches:\n%s", len(fails), b.String())
	}
}

// TestVectors checks the differential fuzz vectors recorded from node by
// work-regex/gen_vectors.mjs, restricted to the supported flags (JSREGEX_VECTORS
// overrides the file).
func TestVectors(t *testing.T) {
	path := "testdata/vectors.jsonl.gz"
	if p := os.Getenv("JSREGEX_VECTORS"); p != "" {
		path = p
	}
	runVectorFile(t, path)
}
