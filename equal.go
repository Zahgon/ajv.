// Ported from fast-deep-equal 3.1.3 (MIT, Copyright (c) 2017 Evgeny Poberezkin), used by Ajv
// 8.20.0 lib/runtime/equal.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import "math"

// equal is fast-deep-equal's equal(a, b) on JSON values.
func equal(a, b any) bool {
	if strictEquals(a, b) {
		return true
	}
	if truthy(a) && truthy(b) && typeOf(a) == "object" && typeOf(b) == "object" {
		if !strictEquals(getProp(a, "constructor"), getProp(b, "constructor")) {
			return false
		}
		if x, ok := a.([]any); ok {
			y, ok := b.([]any)
			if !ok || len(x) != len(y) {
				return false
			}
			for i := len(x) - 1; i >= 0; i-- {
				if !equal(x[i], y[i]) {
					return false
				}
			}
			return true
		}
		keys := objectKeys(a)
		if len(keys) != len(objectKeys(b)) {
			return false
		}
		for i := len(keys) - 1; i >= 0; i-- {
			if !hasOwnProperty(b, keys[i]) {
				return false
			}
		}
		for i := len(keys) - 1; i >= 0; i-- {
			if !equal(getProp(a, keys[i]), getProp(b, keys[i])) {
				return false
			}
		}
		return true
	}
	an, aNum := asNumber(a)
	bn, bNum := asNumber(b)
	return aNum && bNum && math.IsNaN(an) && math.IsNaN(bn)
}
