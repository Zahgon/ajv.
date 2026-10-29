// Ported from Ajv 8.20.0, lib/compile/util.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"strings"

	"github.com/ajv-validator/ajv-go/internal/js"
)

// alwaysValidSchema is util.alwaysValidSchema.
func alwaysValidSchema(it *schemaCxt, schema any) bool {
	if b, ok := schema.(bool); ok {
		return b
	}
	if len(objectKeys(schema)) == 0 {
		return true
	}
	checkUnknownRules(it, schema)
	return !schemaHasRules(schema, it.self.rules)
}

// checkUnknownRules is util.checkUnknownRules.
func checkUnknownRules(it *schemaCxt, schema any) {
	if !truthy(it.opts.strictSchema) {
		return
	}
	if _, ok := schema.(bool); ok {
		return
	}
	rules := it.self.rules
	for _, key := range objectKeys(schema) {
		if !rules.hasKeyword(key) {
			checkStrictMode(it, `unknown keyword: "`+key+`"`, it.opts.strictSchema)
		}
	}
}

// schemaHasRules is util.schemaHasRules with RULES.all.
func schemaHasRules(schema any, rules *validationRules) bool {
	if b, ok := schema.(bool); ok {
		return !b
	}
	for _, key := range objectKeys(schema) {
		if rules.hasRule(key) {
			return true
		}
	}
	return false
}

// schemaHasRulesButRef is util.schemaHasRulesButRef.
func schemaHasRulesButRef(schema any, rules *validationRules) bool {
	if b, ok := schema.(bool); ok {
		return !b
	}
	for _, key := range objectKeys(schema) {
		if key != "$ref" && rules.hasRule(key) {
			return true
		}
	}
	return false
}

// unescapeFragment is util.unescapeFragment.
func unescapeFragment(s string) string { return unescapeJSONPointer(decodeURIComponent(s)) }

// escapeFragment is util.escapeFragment.
func escapeFragment(s string) string { return encodeURIComponent(escapeJSONPointer(s)) }

// escapeJSONPointer is util.escapeJsonPointer.
func escapeJSONPointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// unescapeJSONPointer is util.unescapeJsonPointer.
func unescapeJSONPointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
}

// checkStrictMode is util.checkStrictMode: mode true throws, another truthy mode warns.
func checkStrictMode(it *schemaCxt, msg string, mode any) {
	if !truthy(mode) {
		return
	}
	msg = "strict mode: " + msg
	if mode == true {
		panic(newError(msg))
	}
	it.self.logger.Warn(msg)
}

const uriUnreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"

// encodeURIComponent is the global encodeURIComponent; a lone surrogate throws a URIError.
func encodeURIComponent(s string) string {
	units := js.CodeUnits(s)
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u < 0x80 && strings.IndexByte(uriUnreserved, byte(u)) >= 0 {
			b.WriteByte(byte(u))
			continue
		}
		cp := rune(u)
		if u >= 0xdc00 && u <= 0xdfff {
			panic(newError("URI malformed"))
		}
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				panic(newError("URI malformed"))
			}
			cp = (rune(u)-0xd800)<<10 + (rune(units[i+1]) - 0xdc00) + 0x10000
			i++
		}
		var buf [4]byte
		n := encodeRune(buf[:], cp)
		for _, c := range buf[:n] {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	return b.String()
}

func encodeRune(p []byte, r rune) int {
	switch {
	case r < 0x80:
		p[0] = byte(r)
		return 1
	case r < 0x800:
		p[0] = 0xc0 | byte(r>>6)
		p[1] = 0x80 | byte(r)&0x3f
		return 2
	case r < 0x10000:
		p[0] = 0xe0 | byte(r>>12)
		p[1] = 0x80 | byte(r>>6)&0x3f
		p[2] = 0x80 | byte(r)&0x3f
		return 3
	}
	p[0] = 0xf0 | byte(r>>18)
	p[1] = 0x80 | byte(r>>12)&0x3f
	p[2] = 0x80 | byte(r>>6)&0x3f
	p[3] = 0x80 | byte(r)&0x3f
	return 4
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// decodeURIComponent is the global decodeURIComponent; malformed input throws a URIError.
func decodeURIComponent(s string) string {
	if strings.IndexByte(s, '%') < 0 {
		return s
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			out = append(out, s[i])
			continue
		}
		if i+2 >= len(s) || hexVal(s[i+1]) < 0 || hexVal(s[i+2]) < 0 {
			panic(newError("URI malformed"))
		}
		c := byte(hexVal(s[i+1])<<4 | hexVal(s[i+2]))
		if c < 0x80 {
			out = append(out, c)
			i += 2
			continue
		}
		var n int
		switch {
		case c&0xe0 == 0xc0:
			n = 2
		case c&0xf0 == 0xe0:
			n = 3
		case c&0xf8 == 0xf0:
			n = 4
		default:
			panic(newError("URI malformed"))
		}
		seq := []byte{c}
		j := i + 3
		for k := 1; k < n; k++ {
			if j+2 >= len(s) || s[j] != '%' || hexVal(s[j+1]) < 0 || hexVal(s[j+2]) < 0 {
				panic(newError("URI malformed"))
			}
			b := byte(hexVal(s[j+1])<<4 | hexVal(s[j+2]))
			if b&0xc0 != 0x80 {
				panic(newError("URI malformed"))
			}
			seq = append(seq, b)
			j += 3
		}
		r, size := decodeUTF8(seq)
		if size != n || r < 0 || (r >= 0xd800 && r <= 0xdfff) {
			panic(newError("URI malformed"))
		}
		out = append(out, seq...)
		i = j - 1
	}
	return string(out)
}

// decodeUTF8 decodes a whole UTF-8 sequence, rejecting overlong forms.
func decodeUTF8(seq []byte) (rune, int) {
	var r rune
	var minVal rune
	switch len(seq) {
	case 2:
		r, minVal = rune(seq[0]&0x1f), 0x80
	case 3:
		r, minVal = rune(seq[0]&0x0f), 0x800
	case 4:
		r, minVal = rune(seq[0]&0x07), 0x10000
	default:
		return -1, 0
	}
	for _, b := range seq[1:] {
		r = r<<6 | rune(b&0x3f)
	}
	if r < minVal || r > 0x10ffff {
		return -1, 0
	}
	return r, len(seq)
}

// mergeEvaluatedProps is mergeEvaluated.props; toName forces a jsName result.
func mergeEvaluatedProps(gen *codeGen, from, to any, toName bool) any {
	var res any
	switch {
	case to == nil:
		res = from
	case isName(to):
		toN := to.(*jsName)
		if fromN, ok := from.(*jsName); ok {
			mergePropsNames(gen, fromN, toN)
		} else {
			mergePropsToName(gen, from, toN)
		}
		res = to
	case isName(from):
		mergePropsToName(gen, to, from.(*jsName))
		res = from
	default:
		res = mergePropsValues(from, to)
	}
	if toName && !isName(res) {
		return evaluatedPropsToName(gen, res)
	}
	return res
}

func isName(x any) bool {
	_, ok := x.(*jsName)
	return ok
}

func mergePropsNames(gen *codeGen, from, to *jsName) {
	gen.ifThen(exprFn(func(f *frame) any { return f.get(to) != true && !isUndefined(f.get(from)) }))
	gen.code(func(f *frame) {
		fv := f.get(from)
		if fv == true {
			f.set(to, true)
			return
		}
		tv := f.get(to)
		if !truthy(tv) {
			tv = NewObject()
			f.set(to, tv)
		}
		objectAssign(tv, fv)
	})
	gen.endIf()
}

// objectAssign is Object.assign(target, source).
func objectAssign(target, source any) {
	t, ok := target.(*Object)
	if !ok {
		return
	}
	switch s := source.(type) {
	case *Object:
		for _, k := range s.Keys() {
			t.Set(k, s.value(k))
		}
	case []any:
		for i, v := range s {
			t.Set(js.NumberToString(float64(i)), v)
		}
	case string:
		for i := range js.Len(s) {
			t.Set(js.NumberToString(float64(i)), js.CharAt(s, i))
		}
	}
}

func mergePropsToName(gen *codeGen, from any, to *jsName) {
	gen.ifThen(exprFn(func(f *frame) any { return f.get(to) != true }))
	if from == true {
		gen.assign(to, true)
	} else {
		gen.code(func(f *frame) {
			if !truthy(f.get(to)) {
				f.set(to, NewObject())
			}
		})
		setEvaluated(gen, to, from.(*propSet))
	}
	gen.endIf()
}

func mergePropsValues(from, to any) any {
	if from == true {
		return true
	}
	res := &propSet{}
	res.addAll(from.(*propSet))
	res.addAll(to.(*propSet))
	return res
}

// mergeEvaluatedItems is mergeEvaluated.items; toName forces a jsName result.
func mergeEvaluatedItems(gen *codeGen, from, to any, toName bool) any {
	var res any
	switch {
	case to == nil:
		res = from
	case isName(to):
		toN := to.(*jsName)
		if fromN, ok := from.(*jsName); ok {
			gen.ifThen(exprFn(func(f *frame) any { return f.get(toN) != true && !isUndefined(f.get(fromN)) }))
			gen.code(func(f *frame) {
				fv, tv := f.get(fromN), f.get(toN)
				switch {
				case fv == true:
					f.set(toN, true)
				case compare(tv, opGT, fv):
					f.set(toN, tv)
				default:
					f.set(toN, fv)
				}
			})
			gen.endIf()
		} else {
			mergeItemsToName(gen, from, toN)
		}
		res = to
	case isName(from):
		mergeItemsToName(gen, to, from.(*jsName))
		res = from
	default:
		if from == true {
			res = true
		} else {
			res = max(toNumber(from), toNumber(to))
		}
	}
	if toName && !isName(res) {
		return gen.varDef("items", res)
	}
	return res
}

func mergeItemsToName(gen *codeGen, from any, to *jsName) {
	gen.ifThen(exprFn(func(f *frame) any { return f.get(to) != true }))
	if from == true {
		gen.assign(to, true)
	} else {
		gen.code(func(f *frame) {
			if tv := f.get(to); !compare(tv, opGT, from) {
				f.set(to, from)
			}
		})
	}
	gen.endIf()
}

// evaluatedPropsToName is util.evaluatedPropsToName.
func evaluatedPropsToName(gen *codeGen, ps any) *jsName {
	if ps == true {
		return gen.varDef("props", true)
	}
	props := gen.varDef("props", exprFn(func(*frame) any { return NewObject() }))
	if ps != nil {
		setEvaluated(gen, props, ps.(*propSet))
	}
	return props
}

// setEvaluated is util.setEvaluated.
func setEvaluated(gen *codeGen, props *jsName, ps *propSet) {
	for _, p := range ps.keys {
		gen.code(func(f *frame) { setProp(f.get(props), p, true) })
	}
}

// propSet is a static set of evaluated properties ({[prop]: true}), in insertion order.
type propSet struct {
	keys []string
	has  map[string]bool
}

func newPropSet(keys []string) *propSet {
	ps := &propSet{}
	for _, k := range keys {
		ps.add(k)
	}
	return ps
}

func (ps *propSet) add(k string) {
	if ps.has == nil {
		ps.has = map[string]bool{}
	}
	if !ps.has[k] {
		ps.has[k] = true
		ps.keys = append(ps.keys, k)
	}
}

func (ps *propSet) addAll(o *propSet) {
	for _, k := range o.keys {
		ps.add(k)
	}
}
