package js

import (
	"sort"
	"strings"
)

// caseRange maps lo..hi to c+delta. With stride 2 only lo, lo+2, ... map; the code points in
// between are unchanged.
type caseRange struct {
	lo, hi rune
	delta  int32
	stride uint8
}

// caseSpecial is a mapping to more than one code point.
type caseSpecial struct {
	cp rune
	to string
}

// sigmaRange gives the Final_Sigma class (1 cased, 2 case-ignorable) of lo..hi.
type sigmaRange struct {
	lo, hi rune
	class  uint8
}

func lookupCase(r rune, ranges []caseRange, special []caseSpecial) (rune, string) {
	if i := sort.Search(len(special), func(i int) bool { return special[i].cp >= r }); i < len(special) && special[i].cp == r {
		return -1, special[i].to
	}
	i := sort.Search(len(ranges), func(i int) bool { return ranges[i].hi >= r })
	if i < len(ranges) && ranges[i].lo <= r {
		cr := ranges[i]
		if cr.stride == 1 || (r-cr.lo)%2 == 0 {
			return r + rune(cr.delta), ""
		}
	}
	return r, ""
}

func sigmaClass(r rune) uint8 {
	i := sort.Search(len(sigmaClasses), func(i int) bool { return sigmaClasses[i].hi >= r })
	if i < len(sigmaClasses) && sigmaClasses[i].lo <= r {
		return sigmaClasses[i].class
	}
	return 0
}

// casedAround reports whether a cased letter is found from cps[i+dir] onwards in direction dir,
// skipping case-ignorable code points, as ICU's isFollowedByCasedLetter does.
func casedAround(cps []rune, i, dir int) bool {
	for j := i + dir; j >= 0 && j < len(cps); j += dir {
		switch sigmaClass(cps[j]) {
		case 2:
		case 1:
			return true
		default:
			return false
		}
	}
	return false
}

func lowerCase(s string) string {
	cps := codePoints(s)
	b := make([]byte, 0, len(s)+8)
	for i, r := range cps {
		if r == 0x3a3 {
			if casedAround(cps, i, -1) && !casedAround(cps, i, 1) {
				b = append(b, "\u03c2"...)
			} else {
				b = append(b, "\u03c3"...)
			}
			continue
		}
		m, str := lookupCase(r, lowerRanges, lowerSpecial)
		if m < 0 {
			b = append(b, str...)
		} else {
			b = appendCodePoint(b, m)
		}
	}
	return string(b)
}

// ToLowerCase is String.prototype.toLowerCase: Unicode 17 full default lowercase mapping as ICU
// applies it, including the Final_Sigma rule for U+03A3 and U+0130 -> "i\u0307".
func ToLowerCase(s string) string {
	if isASCII(s) {
		return strings.ToLower(s)
	}
	return lowerCase(s)
}
