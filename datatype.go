// Ported from Ajv 8.20.0, lib/compile/validate/dataType.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"math"
	"strings"
)

// getSchemaTypes is dataType.getSchemaTypes.
func getSchemaTypes(schema any) []string {
	types := getJSONTypes(getProp(schema, "type"))
	nullable := getProp(schema, "nullable")
	if contains(types, "null") {
		if nullable == false {
			panic(newError("type: null contradicts nullable: false"))
		}
	} else {
		if len(types) == 0 && !isUndefined(nullable) {
			panic(newError(`"nullable" cannot be used without "type"`))
		}
		if nullable == true {
			types = append(types, "null")
		}
	}
	return types
}

// getJSONTypes is dataType.getJSONTypes.
func getJSONTypes(ts any) []string {
	var types []any
	switch x := ts.(type) {
	case []any:
		types = x
	case []string:
		types = stringsToAny(x)
	default:
		if truthy(ts) {
			types = []any{ts}
		}
	}
	out := make([]string, 0, len(types))
	ok := true
	for _, t := range types {
		if !isJSONType(t) {
			ok = false
			break
		}
		out = append(out, t.(string))
	}
	if ok {
		return out
	}
	parts := make([]string, len(types))
	for i, t := range types {
		if t != nil && !isUndefined(t) {
			parts[i] = toString(t)
		}
	}
	panic(newError("type must be JSONType or JSONType[]: " + strings.Join(parts, ",")))
}

var coercible = toSet("string", "number", "integer", "boolean", "null")

func coerceToTypes(types []string, coerceTypes any) []string {
	if !truthy(coerceTypes) {
		return nil
	}
	var out []string
	for _, t := range types {
		if coercible[t] || (coerceTypes == "array" && t == "array") {
			out = append(out, t)
		}
	}
	return out
}

// coerceAndCheckDataType is dataType.coerceAndCheckDataType.
func coerceAndCheckDataType(it *schemaCxt, types []string) bool {
	coerceTo := coerceToTypes(types, it.opts.coerceTypes)
	checkTypes := len(types) > 0 &&
		!(len(coerceTo) == 0 && len(types) == 1 && schemaHasRulesForType(it, types[0]))
	if checkTypes {
		wrongType := checkDataTypes(types, it.data, it.opts.strictNumbers, true)
		it.gen.ifThen(wrongType)
		if len(coerceTo) > 0 {
			coerceData(it, types, coerceTo)
		} else {
			reportTypeError(it)
		}
		it.gen.endIf()
	}
	return checkTypes
}

func coerceData(it *schemaCxt, types, coerceTo []string) {
	gen, data, opts := it.gen, it.data, it.opts
	dataType := gen.let("dataType", exprFn(func(f *frame) any { return typeOf(f.get(data)) }))
	coerced := gen.let("coerced", undefined)
	if opts.coerceTypes == "array" {
		gen.ifThen(exprFn(func(f *frame) any {
			arr, ok := f.get(data).([]any)
			return f.get(dataType) == "object" && ok && len(arr) == 1
		}))
		gen.code(func(f *frame) {
			f.set(data, f.get(data).([]any)[0])
			f.set(dataType, typeOf(f.get(data)))
		})
		gen.ifThen(checkDataTypes(types, data, opts.strictNumbers, false))
		gen.assign(coerced, data)
		gen.endIf()
		gen.endIf()
	}
	gen.ifThen(exprFn(func(f *frame) any { return !isUndefined(f.get(coerced)) }))
	for _, t := range coerceTo {
		if coercible[t] || (t == "array" && opts.coerceTypes == "array") {
			coerceSpecificType(gen, t, data, dataType, coerced)
		}
	}
	gen.elseBranch()
	reportTypeError(it)
	gen.endIf()

	gen.ifThen(exprFn(func(f *frame) any { return !isUndefined(f.get(coerced)) }))
	gen.assign(data, coerced)
	assignParentData(it, coerced)
	gen.endIf()
}

func coerceSpecificType(gen *codeGen, t string, data, dataType, coerced *jsName) {
	d := func(f *frame) any { return f.get(data) }
	dt := func(f *frame) any { return f.get(dataType) }
	switch t {
	case "string":
		gen.elseIf(exprFn(func(f *frame) any { return dt(f) == "number" || dt(f) == "boolean" }))
		gen.assign(coerced, exprFn(func(f *frame) any { return toString(d(f)) }))
		gen.elseIf(exprFn(func(f *frame) any { return d(f) == nil }))
		gen.assign(coerced, "")
	case "number":
		gen.elseIf(exprFn(func(f *frame) any {
			v := d(f)
			return dt(f) == "boolean" || v == nil || (dt(f) == "string" && truthy(v) && looseEquals(v, toNumber(v)))
		}))
		gen.assign(coerced, exprFn(func(f *frame) any { return toNumber(d(f)) }))
	case "integer":
		gen.elseIf(exprFn(func(f *frame) any {
			v := d(f)
			return dt(f) == "boolean" || v == nil ||
				(dt(f) == "string" && truthy(v) && looseEquals(v, toNumber(v)) && !truthy(math.Mod(toNumber(v), 1)))
		}))
		gen.assign(coerced, exprFn(func(f *frame) any { return toNumber(d(f)) }))
	case "boolean":
		gen.elseIf(exprFn(func(f *frame) any {
			v := d(f)
			return v == "false" || strictEquals(v, 0.0) || v == nil
		}))
		gen.assign(coerced, false)
		gen.elseIf(exprFn(func(f *frame) any {
			v := d(f)
			return v == "true" || strictEquals(v, 1.0)
		}))
		gen.assign(coerced, true)
	case "null":
		gen.elseIf(exprFn(func(f *frame) any {
			v := d(f)
			return v == "" || strictEquals(v, 0.0) || v == false
		}))
		gen.assign(coerced, nil)
	case "array":
		gen.elseIf(exprFn(func(f *frame) any {
			k := dt(f)
			return k == "string" || k == "number" || k == "boolean" || d(f) == nil
		}))
		gen.assign(coerced, exprFn(func(f *frame) any { return []any{d(f)} }))
	}
}

func assignParentData(it *schemaCxt, expr *jsName) {
	parentData, parentDataProperty := it.parentData, it.parentDataProperty
	it.gen.code(func(f *frame) {
		if pd := f.get(parentData); !isUndefined(pd) {
			setProp(pd, eval(parentDataProperty, f), f.get(expr))
		}
	})
}

// checkDataType is dataType.checkDataType; wrong selects DataType.Wrong.
func checkDataType(dataType string, data *jsName, strictNums any, wrong bool) jsCode {
	strict := truthy(strictNums)
	var cond func(v any) bool
	switch dataType {
	case "null":
		return exprFn(func(f *frame) any { return (f.get(data) == nil) != wrong })
	case "array":
		cond = isArray
	case "object":
		cond = func(v any) bool { return truthy(v) && typeOf(v) == "object" && !isArray(v) }
	case "integer":
		cond = func(v any) bool {
			if typeOf(v) != "number" {
				return false
			}
			n := toNumber(v)
			return !truthy(math.Mod(n, 1)) && !math.IsNaN(n) && (!strict || isFiniteValue(n))
		}
	case "number":
		cond = func(v any) bool { return typeOf(v) == "number" && (!strict || isFiniteValue(v)) }
	default:
		return exprFn(func(f *frame) any { return (typeOf(f.get(data)) == dataType) != wrong })
	}
	return exprFn(func(f *frame) any { return cond(f.get(data)) != wrong })
}

// checkDataTypes is dataType.checkDataTypes.
func checkDataTypes(dataTypes []string, data *jsName, strictNums any, wrong bool) jsCode {
	if len(dataTypes) == 1 {
		return checkDataType(dataTypes[0], data, strictNums, wrong)
	}
	types := newPropSet(dataTypes)
	cond := nilCode
	if types.has["array"] && types.has["object"] {
		withNull := types.has["null"]
		cond = exprFn(func(f *frame) any {
			v := f.get(data)
			notObj := typeOf(v) != "object"
			if withNull {
				return notObj
			}
			return !truthy(v) || notObj
		})
		delete(types.has, "null")
		delete(types.has, "array")
		delete(types.has, "object")
	}
	if types.has["number"] {
		delete(types.has, "integer")
	}
	for _, t := range types.keys {
		if types.has[t] {
			cond = and(cond, checkDataType(t, data, strictNums, wrong))
		}
	}
	return cond
}

var typeError = &errorDef{
	message: func(cxt *keywordCxt) func(f *frame) string {
		msg := "must be " + toString(cxt.schema)
		return func(*frame) string { return msg }
	},
	params: func(cxt *keywordCxt) func(f *frame) *Object {
		schema := cxt.schema
		return func(*frame) *Object { return newObjectKV("type", schema) }
	},
}

// reportTypeError is dataType.reportTypeError.
func reportTypeError(it *schemaCxt) {
	schemaType := getProp(it.schema, "type")
	cxt := &keywordCxt{
		gen:          it.gen,
		keyword:      "type",
		data:         it.data,
		schema:       schemaType,
		schemaCode:   schemaType,
		schemaValue:  schemaType,
		parentSchema: it.schema,
		params:       map[string]jsCode{},
		it:           it,
	}
	reportError(cxt, typeError, nil)
}
