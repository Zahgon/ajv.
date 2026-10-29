package js

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// ToNumber is Number(s), the StringToNumber operation: surrounding JavaScript white space is
// ignored, "" is 0, 0x/0o/0b prefixes select a radix, "Infinity" may carry a sign, and anything
// else that is not a StrDecimalLiteral is NaN.
func ToNumber(s string) float64 {
	s = Trim(s)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		radix := 0
		switch s[1] {
		case 'x', 'X':
			radix = 16
		case 'o', 'O':
			radix = 8
		case 'b', 'B':
			radix = 2
		}
		if radix != 0 {
			digits := s[2:]
			if digitPrefix(digits, radix) != len(digits) {
				return math.NaN()
			}
			return exactRadix(digits, radix)
		}
	}
	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if decimalPrefix(s, 1) != len(s) {
		return math.NaN()
	}
	return parseDecimal(s)
}

// parseDecimal converts a validated StrDecimalLiteral (with optional sign) to the nearest double.
func parseDecimal(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f
		}
		return math.NaN()
	}
	return f
}

// decimalPrefix returns the length of the longest StrDecimalLiteral prefix of s (without
// "Infinity"); signs counts how many leading sign characters are allowed (0 or 1).
func decimalPrefix(s string, signs int) int {
	i := 0
	if signs > 0 && i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		j := i + 1
		frac := 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			frac++
		}
		if digits+frac > 0 {
			i = j
			digits += frac
		}
	}
	if digits == 0 {
		return 0
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		exp := 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			exp++
		}
		if exp > 0 {
			i = j
		}
	}
	return i
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}

// digitPrefix returns the number of leading characters of s that are digits in radix.
func digitPrefix(s string, radix int) int {
	i := 0
	for i < len(s) && digitValue(s[i]) < radix {
		i++
	}
	return i
}

// exactRadix converts radix digits to the nearest double (ties to even).
func exactRadix(digits string, radix int) float64 {
	n, ok := new(big.Int).SetString(digits, radix)
	if !ok {
		return math.NaN()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}

// ParseInt is the global parseInt(s, radix). Pass radix 0 for an undefined radix; other values
// are taken as already converted by ToInt32. Radix 10 and the power-of-two radices are exact;
// the other radices accumulate rounding error exactly as V8 does.
func ParseInt(s string, radix int) float64 {
	s = TrimStart(s)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	strip := true
	if radix != 0 {
		if radix < 2 || radix > 36 {
			return math.NaN()
		}
		if radix != 16 {
			strip = false
		}
	} else {
		radix = 10
	}
	if strip && len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
		radix = 16
	}
	end := digitPrefix(s, radix)
	if end == 0 {
		return math.NaN()
	}
	digits := s[:end]
	var f float64
	switch radix {
	case 10:
		f = parseDecimal(digits)
	case 2, 4, 8, 16, 32:
		f = exactRadix(digits, radix)
	default:
		f = genericRadix(strings.TrimLeft(digits, "0"), radix)
	}
	if neg {
		f = -f
	}
	return f
}

// genericRadix is V8's NumberParseIntHelper::HandleGenericCase: digits are gathered into 32-bit
// parts which are folded into a double as number*multiplier+part. The arm64 builds of Node.js
// compile that expression to a fused multiply-add, so it is rounded once.
func genericRadix(digits string, radix int) float64 {
	const maxMultiplier = 0xFFFFFFFF / 36
	number := 0.0
	i := 0
	for done := false; !done; {
		part, multiplier := uint32(0), uint32(1)
		for {
			if i == len(digits) {
				done = true
				break
			}
			m := multiplier * uint32(radix)
			if m > maxMultiplier {
				break
			}
			part = part*uint32(radix) + uint32(digitValue(digits[i]))
			multiplier = m
			i++
		}
		number = math.FMA(number, float64(multiplier), float64(part))
	}
	return number
}

// MathRound is Math.round: halves round towards +Infinity and -0 is kept for -0.5 <= x <= -0.
func MathRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	c := math.Ceil(x)
	if c-0.5 <= x {
		return c
	}
	return c - 1
}
