// Ported from Ajv 8.20.0, lib/compile/validate/defaults.ts and lib/compile/validate/boolSchema.ts,
// MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"regexp"
	"strings"

	"github.com/ajv-validator/ajv-go/internal/js"
)

// assignDefaults is defaults.assignDefaults.
func assignDefaults(it *schemaCxt, ty string) {
	properties := getProp(it.schema, "properties")
	items := getProp(it.schema, "items")
	if ty == "object" && truthy(properties) {
		for _, key := range objectKeys(properties) {
			assignDefault(it, key, getProp(getProp(properties, key), "default"))
		}
	} else if arr, ok := items.([]any); ty == "array" && ok {
		for i, sch := range arr {
			assignDefault(it, float64(i), getProp(sch, "default"))
		}
	}
}

func assignDefault(it *schemaCxt, prop any, defaultValue any) {
	if isUndefined(defaultValue) {
		return
	}
	data := it.data
	if it.compositeRule {
		checkStrictMode(it, "default is ignored for: "+data.str+propertyCode(prop), it.opts.strictSchema)
		return
	}
	empty := it.opts.useDefaults == "empty"
	parentData, parentDataProperty := it.parentData, it.parentDataProperty
	it.gen.code(func(f *frame) {
		obj := f.get(data)
		child := getProp(obj, prop)
		if isUndefined(child) || (empty && (child == nil || child == "")) {
			if arr, ok := obj.([]any); ok {
				if i := int(prop.(float64)); i >= len(arr) {
					// Assigning past the end grows the array: Go slices cannot grow in place,
					// so the grown array replaces the old one in the data variable and in the
					// parent object or array.
					grown := make([]any, i+1)
					copy(grown, arr)
					for k := len(arr); k < i; k++ {
						grown[k] = undefined
					}
					grown[i] = literalCopy(defaultValue)
					f.set(data, grown)
					if pd := f.get(parentData); !isUndefined(pd) {
						setProp(pd, eval(parentDataProperty, f), grown)
					}
					return
				}
			}
			setProp(obj, prop, literalCopy(defaultValue))
		}
	})
}

// identifierRe is codegen's IDENTIFIER.
var identifierRe = regexp.MustCompile(`^(?i)[a-z$_][a-z$_0-9]*$`)

// propertyCode renders codegen's getProperty(key) as JavaScript source.
func propertyCode(key any) string {
	if s, ok := key.(string); ok {
		if identifierRe.MatchString(s) {
			return "." + s
		}
		return "[" + safeStringify(s) + "]"
	}
	return "[" + toString(key) + "]"
}

// safeStringify is codegen's safeStringify for a string.
func safeStringify(s string) string {
	q := js.Quote(s)
	q = strings.ReplaceAll(q, " ", ` `)
	return strings.ReplaceAll(q, " ", ` `)
}

// literalCopy is the value of the JavaScript literal stringify(v) evaluates to: a deep copy of v
// through JSON in which an object member named __proto__ sets the prototype instead of
// creating a property.
func literalCopy(v any) any {
	switch x := deepCopyJSON(v).(type) {
	case *Object:
		dropProto(x)
		return x
	case []any:
		for _, e := range x {
			dropProtoDeep(e)
		}
		return x
	default:
		return x
	}
}

func dropProto(o *Object) {
	o.Delete("__proto__")
	for _, k := range o.Keys() {
		dropProtoDeep(o.value(k))
	}
}

func dropProtoDeep(v any) {
	switch x := v.(type) {
	case *Object:
		dropProto(x)
	case []any:
		for _, e := range x {
			dropProtoDeep(e)
		}
	}
}

var boolError = &errorDef{
	message: func(*keywordCxt) func(f *frame) string {
		return func(*frame) string { return "boolean schema is false" }
	},
}

func topBoolOrEmptySchema(it *schemaCxt) {
	gen := it.gen
	if it.schema == false {
		override := false
		falseSchemaError(it, &override)
	} else if isPlainObject(it.schema) && getProp(it.schema, "$async") == true {
		gen.returnStmt(names.data)
	} else {
		gen.code(func(f *frame) { f.fn.errors = nil })
		gen.returnStmt(true)
	}
}

func boolOrEmptySchema(it *schemaCxt, valid *jsName) {
	if it.schema == false {
		it.gen.varDef(valid, false)
		falseSchemaError(it, nil)
	} else {
		it.gen.varDef(valid, true)
	}
}

func falseSchemaError(it *schemaCxt, overrideAllErrors *bool) {
	cxt := &keywordCxt{
		gen:          it.gen,
		keyword:      "false schema",
		data:         it.data,
		schema:       false,
		schemaCode:   false,
		schemaValue:  false,
		parentSchema: it.schema,
		params:       map[string]jsCode{},
		it:           it,
	}
	reportError(cxt, boolError, overrideAllErrors)
}
