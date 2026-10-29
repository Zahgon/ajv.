package js

import (
	"strings"
)

// CharAt is String.prototype.charAt: the code unit at UTF-16 index i, or "" when out of range.
func CharAt(s string, i int) string {
	if isASCII(s) {
		if i < 0 || i >= len(s) {
			return ""
		}
		return s[i : i+1]
	}
	u := CodeUnits(s)
	if i < 0 || i >= len(u) {
		return ""
	}
	return FromCodeUnits(u[i : i+1])
}

// IsWhiteSpace reports whether r is JavaScript WhiteSpace or LineTerminator, the set that
// String.prototype.trim removes.
func IsWhiteSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// Trim is String.prototype.trim.
func Trim(s string) string { return strings.TrimFunc(s, IsWhiteSpace) }

// TrimStart is String.prototype.trimStart.
func TrimStart(s string) string { return strings.TrimLeftFunc(s, IsWhiteSpace) }

// Compare compares strings as JavaScript's relational operators do (UTF-16 code unit order). It
// returns -1, 0 or 1.
func Compare(a, b string) int {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	if i == len(a) && i == len(b) {
		return 0
	}
	if (i == len(a) || a[i] < 0x80) && (i == len(b) || b[i] < 0x80) {
		switch {
		case i == len(a):
			return -1
		case i == len(b):
			return 1
		case a[i] < b[i]:
			return -1
		}
		return 1
	}
	ua, ub := CodeUnits(a), CodeUnits(b)
	for j := 0; j < len(ua) && j < len(ub); j++ {
		if ua[j] != ub[j] {
			if ua[j] < ub[j] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ua) < len(ub):
		return -1
	case len(ua) > len(ub):
		return 1
	}
	return 0
}
