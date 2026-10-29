// Ported from Ajv 8.20.0, lib/vocabularies/code.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"sync"

	"github.com/ajv-validator/ajv-go/internal/jsregex"
)

// strMsg builds the runtime message of a str`...` template: literal strings and Codes, each
// converted with String().
func strMsg(parts ...any) func(f *frame) string {
	return func(f *frame) string {
		s := ""
		for _, p := range parts {
			if lit, ok := p.(strLit); ok {
				s += string(lit)
			} else {
				s += toString(eval(p, f))
			}
		}
		return s
	}
}

// strLit is a literal part of a str`...` template.
type strLit string

// constMsg is an error definition's constant message.
func constMsg(msg string) func(*keywordCxt) func(f *frame) string {
	return func(*keywordCxt) func(f *frame) string {
		return func(*frame) string { return msg }
	}
}

// paramsObj builds the runtime params object of an error from key/jsCode pairs.
func paramsObj(kv ...any) func(f *frame) *Object {
	return func(f *frame) *Object {
		o := NewObject()
		for i := 0; i+1 < len(kv); i += 2 {
			// JSON.stringify leaves out undefined and function values alike; a function can
			// only be a property inherited from a prototype, such as data.constructor.
			if v := eval(kv[i+1], f); !isUndefined(v) && typeOf(v) != "function" {
				o.Set(kv[i].(string), v)
			}
		}
		return o
	}
}

// param returns the jsCode of a params entry of the context (undefined when unset).
func (cxt *keywordCxt) param(key string) jsCode {
	if c, ok := cxt.params[key]; ok {
		return c
	}
	return undefined
}

func propGetter(data *jsName, prop jsCode) func(f *frame) any {
	return func(f *frame) any { return getProp(f.get(data), eval(prop, f)) }
}

func checkReportMissingProp(cxt *keywordCxt, prop string) {
	gen := cxt.gen
	gen.ifThen(noPropertyInData(cxt.data, prop, cxt.it.opts.ownProperties))
	cxt.setParams(map[string]jsCode{"missingProperty": prop}, true)
	cxt.error(false)
	gen.endIf()
}

func checkMissingProp(cxt *keywordCxt, properties []any, missing *jsName) jsCode {
	conds := make([]jsCode, len(properties))
	for i, prop := range properties {
		p := prop
		conds[i] = and(noPropertyInData(cxt.data, p, cxt.it.opts.ownProperties), exprFn(func(f *frame) any {
			f.set(missing, p)
			return p
		}))
	}
	return or(conds...)
}

func reportMissingProp(cxt *keywordCxt, missing *jsName) {
	cxt.setParams(map[string]jsCode{"missingProperty": missing}, true)
	cxt.error(false)
}

func propertyInData(data *jsName, property jsCode, ownProperties bool) jsCode {
	get := propGetter(data, property)
	return exprFn(func(f *frame) any {
		if isUndefined(get(f)) {
			return false
		}
		return !ownProperties || hasOwnProperty(f.get(data), eval(property, f))
	})
}

func noPropertyInData(data *jsName, property jsCode, ownProperties bool) jsCode {
	get := propGetter(data, property)
	return exprFn(func(f *frame) any {
		if isUndefined(get(f)) {
			return true
		}
		return ownProperties && !hasOwnProperty(f.get(data), eval(property, f))
	})
}

// allSchemaProperties is code.allSchemaProperties.
func allSchemaProperties(schemaMap any) []string {
	if !truthy(schemaMap) {
		return []string{}
	}
	var out []string
	for _, p := range objectKeys(schemaMap) {
		if p != "__proto__" {
			out = append(out, p)
		}
	}
	return out
}

// callValidateCode calls a generated validation function with the data context of cxt.
func callValidateCode(cxt *keywordCxt, fn func(f *frame) *ValidateFunction) func(f *frame) bool {
	it, data := cxt.it, cxt.data
	errorPath := it.errorPath
	parentData, parentDataProperty := it.parentData, it.parentDataProperty
	return func(f *frame) bool {
		v := fn(f)
		anchors, _ := f.vars[slotDynamicAnchors].(map[string]*ValidateFunction)
		return v.call(f.get(data), &valCxt{
			instancePath:       instancePathOf(f, errorPath),
			parentData:         f.get(parentData),
			parentDataProperty: eval(parentDataProperty, f),
			rootData:           f.vars[slotRootData],
			dynamicAnchors:     anchors,
			depth:              f.depth + 1,
		})
	}
}

// regexpCache holds compiled patterns by flags and source: compiling the same source always
// gives the same result.
var regexpCache sync.Map

type regexpKey struct{ source, flags string }

type regexpEntry struct {
	re  *jsregex.Regexp
	err error
}

// newRegExp is `new RegExp(source, flags)`; an invalid pattern throws its SyntaxError.
func newRegExp(source, flags string) *jsregex.Regexp {
	key := regexpKey{source, flags}
	if e, ok := regexpCache.Load(key); ok {
		entry := e.(regexpEntry)
		if entry.err != nil {
			panic(entry.err)
		}
		return entry.re
	}
	re, err := jsregex.Compile(source, flags)
	var entry regexpEntry
	if err != nil {
		entry.err = err
	} else {
		entry.re = re
	}
	regexpCache.Store(key, entry)
	if err != nil {
		panic(err)
	}
	return re
}

// usePattern is code.usePattern.
func usePattern(cxt *keywordCxt, pattern string) *jsregex.Regexp {
	u := ""
	if truthy(cxt.it.opts.unicodeRegExp) {
		u = "u"
	}
	return newRegExp(pattern, u)
}

// validateArray is code.validateArray.
func validateArray(cxt *keywordCxt) *jsName {
	gen, data, keyword, it := cxt.gen, cxt.data, cxt.keyword, cxt.it
	valid := gen.name("valid")
	validateItems := func(notValid func()) {
		length := gen.constDef("len", exprFn(func(f *frame) any { return getProp(f.get(data), "length") }))
		gen.forRange("i", 0.0, length, func(i *jsName) {
			cxt.subschema(&subschemaArgs{keyword: keyword, dataProp: i, dataPropType: typeNum}, valid)
			gen.ifThen(not(valid))
			notValid()
			gen.endIf()
		})
	}
	if it.allErrors {
		validArr := gen.let("valid", true)
		validateItems(func() { gen.assign(validArr, false) })
		return validArr
	}
	gen.varDef(valid, true)
	validateItems(func() { gen.breakStmt(nil) })
	return valid
}

// validateUnion is code.validateUnion.
func validateUnion(cxt *keywordCxt) {
	gen, keyword, it := cxt.gen, cxt.keyword, cxt.it
	schema, ok := cxt.schema.([]any)
	if !ok {
		panic(newError("ajv implementation error"))
	}
	alwaysValid := false
	for _, sch := range schema {
		if alwaysValidSchema(it, sch) {
			alwaysValid = true
			break
		}
	}
	if alwaysValid && !it.opts.unevaluated {
		return
	}
	valid := gen.let("valid", false)
	schValid := gen.name("_valid")
	gen.block(func() {
		for i := range schema {
			schCxt := cxt.subschema(&subschemaArgs{keyword: keyword, schemaProp: i, compositeRule: true}, schValid)
			gen.assign(valid, exprFn(func(f *frame) any {
				if v := f.get(valid); truthy(v) {
					return v
				}
				return f.get(schValid)
			}))
			if !cxt.mergeValidEvaluated(schCxt, schValid) {
				gen.ifThen(not(valid))
			}
		}
	})
	cxt.result(valid, func() { cxt.reset() }, func() { cxt.error(true) })
}
