package uri

// This file ports the parts of ada-url 4.0.0's IDNA implementation (src/ada_idna.cpp: mapping,
// normalization, punycode, validity and to_ascii) that Node's `new URL` uses for a special
// URL's host. The tables live in idna_tables.go. ada is licensed under Apache-2.0 or MIT.

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	idnaValid      = 0xFFFF
	idnaDisallowed = 0xFFFE

	maxDomainInputBytes = 16384
)

// idnaLookup is ada's idna_lookup: idnaValid, idnaDisallowed, or an offset into idnaMappings.
func idnaLookup(cp rune) uint16 {
	if cp < 0 || cp > 0x10FFFF {
		return idnaDisallowed
	}
	i := sort.Search(len(idnaStart), func(i int) bool { return idnaStart[i] > uint32(cp) }) - 1
	return idnaStatus[i]
}

// idnaMap is ada::idna::map. It fails if any code point is disallowed.
func idnaMap(input []rune) ([]rune, bool) {
	out := make([]rune, 0, len(input))
	for _, x := range input {
		status := idnaLookup(x)
		switch {
		case status == idnaDisallowed:
			return nil, false
		case status == idnaValid:
			out = append(out, x)
		case int(status) >= len(idnaMappings):
			return nil, false
		default:
			s := idnaMappings[status:]
			s = s[:strings.IndexByte(s, 0)]
			for _, r := range s {
				out = append(out, r)
			}
		}
	}
	return out, true
}

// --- Normalization (ada normalization.cpp) ------------------------------------------------

const (
	hangulSBase  = 0xAC00
	hangulTBase  = 0x11A7
	hangulVBase  = 0x1161
	hangulLBase  = 0x1100
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulNCount = hangulVCount * hangulTCount
	hangulSCount = hangulLCount * hangulVCount * hangulTCount
)

func isHangulSyllable(c rune) bool { return c >= hangulSBase && c < hangulSBase+hangulSCount }

// canonicalDecomposition returns the canonical decomposition of c (nil when there is none).
func canonicalDecomposition(c rune) []uint32 {
	i := sort.Search(len(decompCP), func(i int) bool { return decompCP[i] >= uint32(c) })
	if i < len(decompCP) && decompCP[i] == uint32(c) {
		return decompData[decompOff[i]:decompOff[i+1]]
	}
	return nil
}

// getCCC is ada's get_ccc.
func getCCC(c rune) uint8 {
	if c < 0 || c >= 0x110000 {
		return 0
	}
	i := sort.Search(len(cccStart), func(i int) bool { return cccStart[i] > uint32(c) }) - 1
	return cccValue[i]
}

// compositionRow returns ada's composition[0], composition[1] for c.
func compositionRow(c rune) (int, int) {
	i := sort.Search(len(compCP), func(i int) bool { return compCP[i] >= uint32(c) })
	if i < len(compCP) && compCP[i] == uint32(c) {
		return int(compLo[i]), int(compHi[i])
	}
	return 0, 0
}

// findComposition is the binary search over a composition row shared by ada's would_compose
// and compose. It returns the composed code point.
func findComposition(left, right int, x rune) (rune, bool) {
	if left < 0 || right < left || right > len(compData) {
		return 0, false
	}
	for left+2 < right {
		middle := left + (((right - left) >> 1) &^ 1)
		if rune(compData[middle]) <= x {
			left = middle
		}
		if rune(compData[middle]) >= x {
			right = middle
		}
	}
	if left+1 < len(compData) && rune(compData[left]) == x {
		return rune(compData[left+1]), true
	}
	return 0, false
}

// decomposeNFC is ada's decompose_nfc (compute_decomposition_length, decompose, sort_marks).
func decomposeNFC(input []rune) []rune {
	out := make([]rune, 0, len(input)+8)
	for _, c := range input {
		switch {
		case isHangulSyllable(c):
			s := c - hangulSBase
			out = append(out, hangulLBase+s/hangulNCount, hangulVBase+(s%hangulNCount)/hangulTCount)
			if s%hangulTCount != 0 {
				out = append(out, hangulTBase+s%hangulTCount)
			}
		case c < 0x110000:
			if d := canonicalDecomposition(c); d != nil {
				for _, x := range d {
					out = append(out, rune(x))
				}
				continue
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	// sort_marks
	for idx := 1; idx < len(out); idx++ {
		ccc := getCCC(out[idx])
		if ccc == 0 {
			continue
		}
		cur := out[idx]
		back := idx
		for back != 0 && getCCC(out[back-1]) > ccc {
			out[back] = out[back-1]
			back--
		}
		out[back] = cur
	}
	return out
}

// wouldCompose is ada's would_compose.
func wouldCompose(input []rune) bool {
	for i := 0; i < len(input); {
		cur := input[i]
		if cur >= hangulLBase && cur < hangulLBase+hangulLCount {
			if i+1 < len(input) && input[i+1] >= hangulVBase && input[i+1] < hangulVBase+hangulVCount {
				return true
			}
			i++
			continue
		}
		if isHangulSyllable(cur) {
			if (cur-hangulSBase)%hangulTCount != 0 && i+1 < len(input) &&
				input[i+1] > hangulTBase && input[i+1] < hangulTBase+hangulTCount {
				return true
			}
			i++
			continue
		}
		if cur < 0x110000 {
			lo, hi := compositionRow(cur)
			previousCCC := -1
			j := i
			for ; j+1 < len(input); j++ {
				ccc := getCCC(input[j+1])
				if hi != lo && previousCCC < int(ccc) {
					if lo < 0 || hi < lo || hi > len(compData) {
						break
					}
					if _, ok := findComposition(lo, hi, input[j+1]); ok {
						return true
					}
				}
				if ccc == 0 {
					break
				}
				previousCCC = int(ccc)
			}
			i = j + 1
			continue
		}
		i++
	}
	return false
}

// isAlreadyNFC is ada's is_already_nfc.
func isAlreadyNFC(input []rune) bool {
	if len(input) == 0 {
		return true
	}
	for _, c := range input {
		if !isHangulSyllable(c) && c < 0x110000 && len(canonicalDecomposition(c)) == 1 {
			return false
		}
	}
	var prev uint8
	for _, c := range input {
		ccc := getCCC(c)
		if ccc != 0 && prev > ccc {
			return false
		}
		prev = ccc
	}
	return !wouldCompose(input)
}

// compose is ada's compose.
func compose(input []rune) []rune {
	n := len(input)
	ic, cc := 0, 0
	for ; ic < n; ic, cc = ic+1, cc+1 {
		input[cc] = input[ic]
		cur := input[ic]
		switch {
		case cur >= hangulLBase && cur < hangulLBase+hangulLCount:
			if ic+1 < n && input[ic+1] >= hangulVBase && input[ic+1] < hangulVBase+hangulVCount {
				input[cc] = hangulSBase + ((cur-hangulLBase)*hangulVCount+input[ic+1]-hangulVBase)*hangulTCount
				ic++
				if ic+1 < n && input[ic+1] > hangulTBase && input[ic+1] < hangulTBase+hangulTCount {
					ic++
					input[cc] += input[ic] - hangulTBase
				}
			}
		case isHangulSyllable(cur):
			if (cur-hangulSBase)%hangulTCount != 0 && ic+1 < n &&
				input[ic+1] > hangulTBase && input[ic+1] < hangulTBase+hangulTCount {
				ic++
				input[cc] += input[ic] - hangulTBase
			}
		case cur < 0x110000:
			lo, hi := compositionRow(cur)
			initial := cc
			for previousCCC := -1; ic+1 < n; ic++ {
				ccc := getCCC(input[ic+1])
				if hi != lo && previousCCC < int(ccc) {
					if lo < 0 || hi < lo || hi > len(compData) {
						break
					}
					if composed, ok := findComposition(lo, hi, input[ic+1]); ok {
						input[initial] = composed
						lo, hi = compositionRow(composed)
						continue
					}
				}
				if ccc == 0 {
					break
				}
				previousCCC = int(ccc)
				cc++
				input[cc] = input[ic+1]
			}
		}
	}
	if cc < ic {
		input = input[:cc]
	}
	return input
}

// normalizeNFC is ada's normalize.
func normalizeNFC(input []rune) []rune {
	if isAlreadyNFC(input) {
		return input
	}
	return compose(decomposeNFC(input))
}

// --- Punycode (ada punycode.cpp) ----------------------------------------------------------

const (
	punyBase        = 36
	punyTMin        = 1
	punyTMax        = 26
	punySkew        = 38
	punyDamp        = 700
	punyInitialBias = 72
	punyInitialN    = 128
	maxInt32        = 0x7fffffff
)

func punyDigitValue(c byte) int32 {
	switch {
	case c >= 'a' && c <= 'z':
		return int32(c - 'a')
	case c >= '0' && c <= '9':
		return int32(c-'0') + 26
	}
	return -1
}

func punyDigitChar(d int32) byte {
	if d < 26 {
		return byte(d + 97)
	}
	return byte(d + 22)
}

func punyAdapt(d, n int32, firstTime bool) int32 {
	if firstTime {
		d /= punyDamp
	} else {
		d /= 2
	}
	d += d / n
	var k int32
	for d > ((punyBase-punyTMin)*punyTMax)/2 {
		d /= punyBase - punyTMin
		k += punyBase
	}
	return k + (((punyBase - punyTMin + 1) * d) / (d + punySkew))
}

func punyThreshold(k, bias int32) int32 {
	switch {
	case k <= bias:
		return punyTMin
	case k >= bias+punyTMax:
		return punyTMax
	}
	return k - bias
}

// punycodeToUTF32 is ada's punycode_to_utf32.
func punycodeToUTF32(input string) ([]rune, bool) {
	var out []rune
	var writtenOut int32
	n := uint32(punyInitialN)
	var i int32
	bias := int32(punyInitialBias)
	if end := strings.LastIndexByte(input, '-'); end != -1 {
		for k := 0; k < end; k++ {
			if input[k] >= 0x80 {
				return nil, false
			}
			out = append(out, rune(input[k]))
			writtenOut++
		}
		input = input[end+1:]
	}
	for len(input) > 0 {
		oldi := i
		w := int32(1)
		for k := int32(punyBase); ; k += punyBase {
			if len(input) == 0 {
				return nil, false
			}
			digit := punyDigitValue(input[0])
			input = input[1:]
			if digit < 0 {
				return nil, false
			}
			if digit > (maxInt32-i)/w {
				return nil, false
			}
			i += digit * w
			t := punyThreshold(k, bias)
			if digit < t {
				break
			}
			if w > maxInt32/(punyBase-t) {
				return nil, false
			}
			w *= punyBase - t
		}
		bias = punyAdapt(i-oldi, writtenOut+1, oldi == 0)
		if i/(writtenOut+1) > int32(uint32(maxInt32)-n) {
			return nil, false
		}
		n += uint32(i / (writtenOut + 1))
		i %= writtenOut + 1
		if n < 0x80 {
			return nil, false
		}
		out = append(out, 0)
		copy(out[i+1:], out[i:])
		out[i] = rune(n)
		writtenOut++
		i++
	}
	if len(out) >= 4 && out[0] == 'x' && out[1] == 'n' && out[2] == '-' && out[3] == '-' {
		return nil, false
	}
	return out, true
}

// utf32ToPunycode is ada's utf32_to_punycode, appending to out.
func utf32ToPunycode(input []rune, out []byte) ([]byte, bool) {
	n := uint32(punyInitialN)
	var d int32
	bias := int32(punyInitialBias)
	h := 0
	for _, c := range input {
		if c < 0x80 {
			h++
			out = append(out, byte(c))
		}
		if c > 0x10ffff || (c >= 0xd800 && c < 0xe000) {
			return out, false
		}
	}
	b := h
	if b > 0 {
		out = append(out, '-')
	}
	for h < len(input) {
		m := uint32(0x10FFFF)
		for _, cp := range input {
			if uint32(cp) >= n && uint32(cp) < m {
				m = uint32(cp)
			}
		}
		if uint64(m-n) > uint64(maxInt32-d)/uint64(h+1) {
			return out, false
		}
		d += int32(uint64(m-n) * uint64(h+1))
		n = m
		for _, c := range input {
			if uint32(c) < n {
				if d == maxInt32 {
					return out, false
				}
				d++
			}
			if uint32(c) == n {
				q := d
				for k := int32(punyBase); ; k += punyBase {
					t := punyThreshold(k, bias)
					if q < t {
						break
					}
					out = append(out, punyDigitChar(t+((q-t)%(punyBase-t))))
					q = (q - t) / (punyBase - t)
				}
				out = append(out, punyDigitChar(q))
				bias = punyAdapt(d, int32(h+1), h == b)
				d = 0
				h++
			}
		}
		d++
		n++
	}
	return out, true
}

// --- Validity (ada validity.cpp) ----------------------------------------------------------

type direction uint8

const (
	dirNONE direction = iota
	dirBN
	dirCS
	dirES
	dirON
	dirEN
	dirL
	dirR
	dirNSM
	dirAL
	dirAN
	dirET
	dirWS
	dirRLO
	dirLRO
	dirPDF
	dirRLE
	dirRLI
	dirFSI
	dirPDI
	dirLRI
	dirB
	dirS
	dirLRE
)

// findDirection is ada's find_direction.
func findDirection(cp rune) direction {
	lo := sort.Search(len(dirFinal), func(i int) bool { return dirFinal[i] >= uint32(cp) })
	if lo == len(dirFinal) {
		return dirNONE
	}
	if uint32(cp) >= dirStart[lo] {
		return direction(dirValue[lo])
	}
	return dirNONE
}

func isCombiningMark(cp rune) bool {
	n := len(combiningRanges) / 2
	i := sort.Search(n, func(i int) bool { return combiningRanges[2*i+1] >= uint32(cp) })
	return i < n && uint32(cp) >= combiningRanges[2*i]
}

var (
	viramas = [...]rune{
		0x094D, 0x09CD, 0x0A4D, 0x0ACD, 0x0B4D, 0x0BCD, 0x0C4D, 0x0CCD,
		0x0D3B, 0x0D3C, 0x0D4D, 0x0DCA, 0x0E3A, 0x0EBA, 0x0F84, 0x1039,
		0x103A, 0x1714, 0x1734, 0x17D2, 0x1A60, 0x1B44, 0x1BAA, 0x1BAB,
		0x1BF2, 0x1BF3, 0x2D7F, 0xA806, 0xA82C, 0xA8C4, 0xA953, 0xA9C0,
		0xAAF6, 0xABED, 0x10A3F, 0x11046, 0x1107F, 0x110B9, 0x11133, 0x11134,
		0x111C0, 0x11235, 0x112EA, 0x1134D, 0x11442, 0x114C2, 0x115BF, 0x1163F,
		0x116B6, 0x1172B, 0x11839, 0x1193D, 0x1193E, 0x119E0, 0x11A34, 0x11A47,
		0x11A99, 0x11C3F, 0x11D44, 0x11D45, 0x11D97,
	}
	joiningR = [...]rune{
		0x622, 0x623, 0x624, 0x625, 0x627, 0x629, 0x62f, 0x630, 0x631,
		0x632, 0x648, 0x671, 0x672, 0x673, 0x675, 0x676, 0x677, 0x688,
		0x689, 0x68a, 0x68b, 0x68c, 0x68d, 0x68e, 0x68f, 0x690, 0x691,
		0x692, 0x693, 0x694, 0x695, 0x696, 0x697, 0x698, 0x699, 0x6c0,
		0x6c3, 0x6c4, 0x6c5, 0x6c6, 0x6c7, 0x6c8, 0x6c9, 0x6ca, 0x6cb,
		0x6cd, 0x6cf, 0x6d2, 0x6d3, 0x6d5, 0x6ee, 0x6ef, 0x710, 0x715,
		0x716, 0x717, 0x718, 0x719, 0x71e, 0x728, 0x72a, 0x72c, 0x72f,
		0x74d, 0x759, 0x75a, 0x75b, 0x854, 0x8aa, 0x8ab, 0x8ac,
	}
	joiningL = [...]rune{0xa872}
	joiningD = [...]rune{
		0x620, 0x626, 0x628, 0x62a, 0x62b, 0x62c, 0x62d, 0x62e, 0x633,
		0x634, 0x635, 0x636, 0x637, 0x638, 0x639, 0x63a, 0x63b, 0x63c,
		0x63d, 0x63e, 0x63f, 0x641, 0x642, 0x643, 0x644, 0x645, 0x646,
		0x647, 0x649, 0x64a, 0x66e, 0x66f, 0x678, 0x679, 0x67a, 0x67b,
		0x67c, 0x67d, 0x67e, 0x67f, 0x680, 0x681, 0x682, 0x683, 0x684,
		0x685, 0x686, 0x687, 0x69a, 0x69b, 0x69c, 0x69d, 0x69e, 0x69f,
		0x6a0, 0x6a1, 0x6a2, 0x6a3, 0x6a4, 0x6a5, 0x6a6, 0x6a7, 0x6a8,
		0x6a9, 0x6aa, 0x6ab, 0x6ac, 0x6ad, 0x6ae, 0x6af, 0x6b0, 0x6b1,
		0x6b2, 0x6b3, 0x6b4, 0x6b5, 0x6b6, 0x6b7, 0x6b8, 0x6b9, 0x6ba,
		0x6bb, 0x6bc, 0x6bd, 0x6be, 0x6bf, 0x6c1, 0x6c2, 0x6cc, 0x6ce,
		0x6d0, 0x6d1, 0x6fa, 0x6fb, 0x6fc, 0x6ff, 0x712, 0x713, 0x714,
		0x71a, 0x71b, 0x71c, 0x71d, 0x71f, 0x720, 0x721, 0x722, 0x723,
		0x724, 0x725, 0x726, 0x727, 0x729, 0x72b, 0x72d, 0x72e, 0x74e,
		0x74f, 0x750, 0x751, 0x752, 0x753, 0x754, 0x755, 0x756, 0x757,
		0x758, 0x75c, 0x75d, 0x75e, 0x75f, 0x760, 0x761, 0x762, 0x763,
		0x764, 0x765, 0x766, 0x850, 0x851, 0x852, 0x853, 0x855, 0x8a0,
		0x8a2, 0x8a3, 0x8a4, 0x8a5, 0x8a6, 0x8a7, 0x8a8, 0x8a9, 0x1807,
		0x1820, 0x1821, 0x1822, 0x1823, 0x1824, 0x1825, 0x1826, 0x1827, 0x1828,
		0x1829, 0x182a, 0x182b, 0x182c, 0x182d, 0x182e, 0x182f, 0x1830, 0x1831,
		0x1832, 0x1833, 0x1834, 0x1835, 0x1836, 0x1837, 0x1838, 0x1839, 0x183a,
		0x183b, 0x183c, 0x183d, 0x183e, 0x183f, 0x1840, 0x1841, 0x1842, 0x1843,
		0x1844, 0x1845, 0x1846, 0x1847, 0x1848, 0x1849, 0x184a, 0x184b, 0x184c,
		0x184d, 0x184e, 0x184f, 0x1850, 0x1851, 0x1852, 0x1853, 0x1854, 0x1855,
		0x1856, 0x1857, 0x1858, 0x1859, 0x185a, 0x185b, 0x185c, 0x185d, 0x185e,
		0x185f, 0x1860, 0x1861, 0x1862, 0x1863, 0x1864, 0x1865, 0x1866, 0x1867,
		0x1868, 0x1869, 0x186a, 0x186b, 0x186c, 0x186d, 0x186e, 0x186f, 0x1870,
		0x1871, 0x1872, 0x1873, 0x1874, 0x1875, 0x1876, 0x1877, 0x1887, 0x1888,
		0x1889, 0x188a, 0x188b, 0x188c, 0x188d, 0x188e, 0x188f, 0x1890, 0x1891,
		0x1892, 0x1893, 0x1894, 0x1895, 0x1896, 0x1897, 0x1898, 0x1899, 0x189a,
		0x189b, 0x189c, 0x189d, 0x189e, 0x189f, 0x18a0, 0x18a1, 0x18a2, 0x18a3,
		0x18a4, 0x18a5, 0x18a6, 0x18a7, 0x18a8, 0x18aa, 0xa840, 0xa841, 0xa842,
		0xa843, 0xa844, 0xa845, 0xa846, 0xa847, 0xa848, 0xa849, 0xa84a, 0xa84b,
		0xa84c, 0xa84d, 0xa84e, 0xa84f, 0xa850, 0xa851, 0xa852, 0xa853, 0xa854,
		0xa855, 0xa856, 0xa857, 0xa858, 0xa859, 0xa85a, 0xa85b, 0xa85c, 0xa85d,
		0xa85e, 0xa85f, 0xa860, 0xa861, 0xa862, 0xa863, 0xa864, 0xa865, 0xa866,
		0xa867, 0xa868, 0xa869, 0xa86a, 0xa86b, 0xa86c, 0xa86d, 0xa86e, 0xa86f,
		0xa870, 0xa871,
	}
)

func runeIn(sorted []rune, c rune) bool {
	i := sort.Search(len(sorted), func(i int) bool { return sorted[i] >= c })
	return i < len(sorted) && sorted[i] == c
}

// isLabelValid is ada's is_label_valid, including its early returns for joiners.
func isLabelValid(label []rune) bool {
	if len(label) == 0 {
		return true
	}
	if isCombiningMark(label[0]) {
		return false
	}
	for i, c := range label {
		if c == 0x200c {
			if i > 0 && runeIn(viramas[:], label[i-1]) {
				return true
			}
			if i == 0 || i+1 >= len(label) {
				return false
			}
			lOrD := false
			for _, x := range label[:i] {
				if runeIn(joiningL[:], x) || runeIn(joiningD[:], x) {
					lOrD = true
					break
				}
			}
			rOrD := false
			for _, x := range label[i+1:] {
				if runeIn(joiningR[:], x) || runeIn(joiningD[:], x) {
					rOrD = true
					break
				}
			}
			return lOrD && rOrD
		} else if c == 0x200d {
			if i > 0 && runeIn(viramas[:], label[i-1]) {
				return true
			}
			return false
		}
	}

	lastNonNSM := -1
	for i := len(label) - 1; i >= 0; i-- {
		if findDirection(label[i]) != dirNSM {
			lastNonNSM = i
			break
		}
	}
	if lastNonNSM == -1 {
		return false
	}
	rtl := false
	for _, c := range label {
		switch findDirection(c) {
		case dirR, dirAL, dirAN:
			rtl = true
		}
	}
	if !rtl {
		return true
	}
	if findDirection(label[0]) == dirL {
		for i := 0; i <= lastNonNSM; i++ {
			switch findDirection(label[i]) {
			case dirL, dirEN, dirES, dirCS, dirET, dirON, dirBN, dirNSM:
			default:
				return false
			}
		}
		last := findDirection(label[lastNonNSM])
		return last == dirL || last == dirEN
	}
	if first := findDirection(label[0]); first != dirR && first != dirAL {
		return false
	}
	hasAN, hasEN := false, false
	for i := 0; i <= lastNonNSM; i++ {
		d := findDirection(label[i])
		if d == dirEN {
			hasEN = true
			if hasAN {
				return false
			}
		}
		if d == dirAN {
			hasAN = true
			if hasEN {
				return false
			}
		}
		switch d {
		case dirR, dirAL, dirAN, dirEN, dirES, dirCS, dirET, dirON, dirBN, dirNSM:
		default:
			return false
		}
		if i == lastNonNSM && !(d == dirR || d == dirAL || d == dirAN || d == dirEN) {
			return false
		}
	}
	return true
}

// --- to_ascii (ada to_ascii.cpp) ----------------------------------------------------------

func runesASCII(rs []rune) bool {
	for _, r := range rs {
		if r >= 0x80 {
			return false
		}
	}
	return true
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// idnaToASCII is ada::idna::to_ascii (UTS #46 ToASCII as profiled by the WHATWG URL standard:
// CheckHyphens=false, CheckBidi=true, CheckJoiners=true, UseSTD3ASCIIRules=false,
// Transitional=false, VerifyDnsLength=false). The input is UTF-8 bytes after percent-decoding.
func idnaToASCII(s string) (string, bool) {
	if len(s) > maxDomainInputBytes {
		return "", false
	}
	if isASCII(s) {
		// WHATWG "domain to ASCII" with beStrict=false: an ASCII domain is only lowercased.
		b := []byte(s)
		for i, c := range b {
			if c >= 'A' && c <= 'Z' {
				b[i] = c + 0x20
			}
		}
		return string(b), true
	}
	if !utf8.ValidString(s) {
		return "", false
	}
	mapped, ok := idnaMap([]rune(s))
	if !ok {
		return "", false
	}
	if !runesASCII(mapped) && !isAlreadyNFC(mapped) {
		mapped = normalizeNFC(mapped)
	}
	var out []byte
	p := 0
	for p < len(mapped) {
		begin := p
		for p < len(mapped) && mapped[p] != '.' {
			p++
		}
		label := mapped[begin:p]
		isLast := p == len(mapped)
		if p < len(mapped) {
			p++
		}
		switch {
		case len(label) == 0:
		case len(label) >= 4 && label[0] == 'x' && label[1] == 'n' && label[2] == '-' && label[3] == '-':
			if !runesASCII(label) {
				return "", false
			}
			for _, c := range label {
				out = append(out, byte(c))
			}
			working, ok := punycodeToUTF32(string(out[len(out)-len(label)+4:]))
			if !ok || runesASCII(working) {
				return "", false
			}
			postMap, ok := idnaMap(working)
			if !ok || !runesEqual(working, postMap) {
				return "", false
			}
			if !runesASCII(postMap) && !isAlreadyNFC(postMap) {
				if postMap = normalizeNFC(postMap); !runesEqual(postMap, working) {
					return "", false
				}
			}
			if len(postMap) == 0 || !isLabelValid(postMap) {
				return "", false
			}
		case runesASCII(label):
			for _, c := range label {
				out = append(out, byte(c))
			}
		default:
			if !isLabelValid(label) {
				return "", false
			}
			out = append(out, "xn--"...)
			if out, ok = utf32ToPunycode(label, out); !ok {
				return "", false
			}
		}
		if !isLast {
			out = append(out, '.')
		}
	}
	return string(out), true
}
