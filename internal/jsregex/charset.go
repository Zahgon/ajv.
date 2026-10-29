package jsregex

import (
	"sort"
	"sync"

	"github.com/ajv-validator/ajv-go/internal/uprops"
)

// rangeSet is a set of code points as sorted, merged inclusive [lo, hi] pairs.
type rangeSet []rune

func (s *rangeSet) add(lo, hi rune) { *s = append(*s, lo, hi) }

func (s *rangeSet) addSet(o rangeSet) { *s = append(*s, o...) }

// norm sorts and merges the ranges.
func (s rangeSet) norm() rangeSet {
	n := len(s) / 2
	if n <= 1 {
		return s
	}
	sorted := true
	for i := 1; i < n; i++ {
		if s[2*i] <= s[2*i-1]+1 {
			sorted = false
			break
		}
	}
	if sorted {
		return s
	}
	pairs := make([][2]rune, n)
	for i := range pairs {
		pairs[i] = [2]rune{s[2*i], s[2*i+1]}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a][0] < pairs[b][0] })
	out := make(rangeSet, 0, len(s))
	for _, p := range pairs {
		if k := len(out); k > 0 && p[0] <= out[k-1]+1 {
			if p[1] > out[k-1] {
				out[k-1] = p[1]
			}
			continue
		}
		out = append(out, p[0], p[1])
	}
	return out
}

func (s rangeSet) has(c rune) bool {
	n := len(s) / 2
	lo, hi := 0, n
	for lo < hi {
		m := (lo + hi) / 2
		if s[2*m+1] < c {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return lo < n && s[2*lo] <= c
}

// complement returns [0, max] minus s (s must be normalized).
func (s rangeSet) complement(max rune) rangeSet {
	var out rangeSet
	next := rune(0)
	for i := 0; i+1 < len(s); i += 2 {
		if s[i] > next {
			out = append(out, next, s[i]-1)
		}
		next = s[i+1] + 1
	}
	if next <= max {
		out = append(out, next, max)
	}
	return out
}

func (s rangeSet) intersect(o rangeSet) rangeSet {
	var out rangeSet
	i, j := 0, 0
	for i < len(s) && j < len(o) {
		lo, hi := s[i], s[i+1]
		if o[j] > lo {
			lo = o[j]
		}
		if o[j+1] < hi {
			hi = o[j+1]
		}
		if lo <= hi {
			out = append(out, lo, hi)
		}
		if s[i+1] < o[j+1] {
			i += 2
		} else {
			j += 2
		}
	}
	return out
}

func tableSet(t uprops.Table) rangeSet {
	out := make(rangeSet, len(t))
	for i, v := range t {
		out[i] = rune(v)
	}
	return out
}

// foldData holds case-equivalence classes for one matching mode.
type foldData struct {
	keys  []rune        // sorted code points belonging to a non-trivial class
	class []int32       // class index for keys[i]
	cls   [][]rune      // the classes
	canon map[rune]rune // code point -> class representative
}

var (
	foldOnce        sync.Once
	foldU, foldNonU *foldData
)

func loadFolds() {
	build := func(classes [][]rune) *foldData {
		f := &foldData{cls: classes, canon: map[rune]rune{}}
		type kv struct {
			r rune
			c int32
		}
		var all []kv
		for i, cl := range classes {
			for _, r := range cl {
				all = append(all, kv{r, int32(i)})
				f.canon[r] = cl[0]
			}
		}
		sort.Slice(all, func(a, b int) bool { return all[a].r < all[b].r })
		for _, e := range all {
			f.keys = append(f.keys, e.r)
			f.class = append(f.class, e.c)
		}
		return f
	}
	foldU = build(uprops.FoldClasses(true))
	foldNonU = build(uprops.FoldClasses(false))
}

func getFold(unicode bool) *foldData {
	foldOnce.Do(loadFolds)
	if unicode {
		return foldU
	}
	return foldNonU
}

func (f *foldData) canonical(c rune) rune {
	if r, ok := f.canon[c]; ok {
		return r
	}
	return c
}

// closure adds every case variant of every member of s.
func (f *foldData) closure(s rangeSet) rangeSet {
	out := append(rangeSet(nil), s...)
	for i := 0; i+1 < len(s); i += 2 {
		lo, hi := s[i], s[i+1]
		k := sort.Search(len(f.keys), func(j int) bool { return f.keys[j] >= lo })
		for ; k < len(f.keys) && f.keys[k] <= hi; k++ {
			for _, r := range f.cls[f.class[k]] {
				out = append(out, r, r)
			}
		}
	}
	return out.norm()
}

// variants returns the case-equivalence class of c (c itself when trivial).
func (f *foldData) variants(c rune) []rune {
	k := sort.Search(len(f.keys), func(j int) bool { return f.keys[j] >= c })
	if k < len(f.keys) && f.keys[k] == c {
		return f.cls[f.class[k]]
	}
	return []rune{c}
}

// Standard class escapes.
var (
	digitSet = rangeSet{'0', '9'}
	wordSet  = rangeSet{'0', '9', 'A', 'Z', '_', '_', 'a', 'z'}
	spaceSet = rangeSet{
		0x09, 0x0D, 0x20, 0x20, 0xA0, 0xA0, 0x1680, 0x1680, 0x2000, 0x200A,
		0x2028, 0x2029, 0x202F, 0x202F, 0x205F, 0x205F, 0x3000, 0x3000, 0xFEFF, 0xFEFF,
	}
	lineTermSet = rangeSet{0x0A, 0x0A, 0x0D, 0x0D, 0x2028, 0x2029}
	wordSetUI   = rangeSet{'0', '9', 'A', 'Z', '_', '_', 'a', 'z', 0x017F, 0x017F, 0x212A, 0x212A}
)
