package ajv

import (
	"math"
	"testing"
)

func TestJSOperators(t *testing.T) {
	obj := NewObject()
	obj.Set("own", 1.0)
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"String(1e21)", toString(1e21), "1e+21"},
		{"String([1,null,'a'])", toString([]any{1.0, nil, "a"}), "1,,a"},
		{"String({})", toString(obj), "[object Object]"},
		{"+' 12 '", toNumber(" 12 "), 12.0},
		{"+'0x1A'", toNumber("0x1A"), 26.0},
		{"'b' > 'a'", compare("b", opGT, "a"), true},
		{"'10' < 9", compare("10", opLT, 9.0), false},
		{"undefined < 1", compare(undefined, opLT, 1.0), false},
		{"'1' == 1", looseEquals("1", 1.0), true},
		{"null == undefined", looseEquals(nil, undefined), true},
		{"0 === -0", strictEquals(0.0, math.Copysign(0, -1)), true},
		{"typeof null", typeOf(nil), "object"},
		{"typeof o.toString", typeOf(getProp(obj, "toString")), "function"},
		{"o.own", getProp(obj, "own"), 1.0},
		{"[1,2].length", getProp([]any{1.0, 2.0}, "length"), 2.0},
		{"'ab'[1]", getProp("ab", "1"), "b"},
		{"ucs2length('😀a')", ucs2length("😀a"), 2},
		{"equal(NaN, NaN)", equal(math.NaN(), math.NaN()), true},
		{"equal([], {})", equal([]any{}, NewObject()), false},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: expected %#v, got %#v", tt.name, tt.want, tt.got)
		}
	}
}

func TestStringifyAndParse(t *testing.T) {
	v, err := ParseJSON(`{"b":1,"2":[true,null,"x"],"a":{"1":0,"0":-0}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Stringify(v), `{"2":[true,null,"x"],"b":1,"a":{"0":0,"1":0}}`; got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
	if _, err := ParseJSON(`{"a":}`); err == nil {
		t.Error("expected a syntax error")
	}
}
