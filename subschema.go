// Ported from Ajv 8.20.0, lib/compile/validate/subschema.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"github.com/ajv-validator/ajv-go/internal/js"
)

// pathCode is a string expression built by concatenation (the errorPath of a schema context):
// static parts and parts computed at run time.
type pathCode struct{ parts []pathPart }

type pathPart struct {
	static string
	dyn    func(f *frame) string
}

var emptyPath = &pathCode{}

func (p *pathCode) isEmpty() bool { return len(p.parts) == 0 }

func (p *pathCode) eval(f *frame) string {
	s := ""
	for _, part := range p.parts {
		if part.dyn != nil {
			s += part.dyn(f)
		} else {
			s += part.static
		}
	}
	return s
}

func (p *pathCode) with(part pathPart) *pathCode {
	parts := make([]pathPart, len(p.parts), len(p.parts)+1)
	copy(parts, p.parts)
	return &pathCode{parts: append(parts, part)}
}

// dataPropType is util.Type.
type dataPropType int

const (
	typeStr dataPropType = iota
	typeNum
)

// getErrorPath is util.getErrorPath (without jsPropertySyntax).
func getErrorPath(dataProp jsCode, dpType dataPropType) pathPart {
	if n, ok := dataProp.(*jsName); ok {
		if dpType == typeNum {
			return pathPart{dyn: func(f *frame) string { return "/" + toString(f.get(n)) }}
		}
		return pathPart{dyn: func(f *frame) string { return "/" + escapeJSONPointer(toString(f.get(n))) }}
	}
	if s, ok := dataProp.(string); ok {
		return pathPart{static: "/" + escapeJSONPointer(s)}
	}
	return pathPart{static: "/" + toString(dataProp)}
}

// subschemaArgs is SubschemaArgs; the has* flags distinguish set fields from undefined ones.
type subschemaArgs struct {
	keyword       string
	schemaProp    any // nil when undefined; a string or an int
	schema        any
	hasSchema     bool
	errSchemaPath string
	data          jsCode
	dataProp      jsCode
	dataTypes     []string
	propertyName  *jsName
	dataPropType  dataPropType
	compositeRule bool
	createErrors  *bool
	allErrors     *bool
}

var falseValue = false

// subschemaContext holds the fields getSubschema and extendSubschema* set.
type subschemaContext struct {
	schema             any
	errSchemaPath      string
	errorPath          *pathCode
	dataLevel          int
	dataTypes          []string
	hasDataTypes       bool
	data               *jsName
	parentData         *jsName
	parentDataProperty jsCode
	dataNames          []*jsName
	dataPathArr        []jsCode
	propertyName       *jsName
	hasData            bool
	hasDataProp        bool
}

func getSubschema(it *schemaCxt, a *subschemaArgs) *subschemaContext {
	if a.keyword != "" && a.hasSchema {
		panic(newError(`both "keyword" and "schema" passed, only one allowed`))
	}
	if a.keyword != "" {
		sch := getProp(it.schema, a.keyword)
		if a.schemaProp == nil {
			return &subschemaContext{schema: sch, errSchemaPath: it.errSchemaPath + "/" + a.keyword}
		}
		var prop string
		switch p := a.schemaProp.(type) {
		case string:
			prop = escapeFragment(p)
		case int:
			prop = js.NumberToString(float64(p))
		}
		return &subschemaContext{
			schema:        getProp(sch, a.schemaProp),
			errSchemaPath: it.errSchemaPath + "/" + a.keyword + "/" + prop,
		}
	}
	if a.hasSchema {
		return &subschemaContext{schema: a.schema, errSchemaPath: a.errSchemaPath}
	}
	panic(newError(`either "keyword" or "schema" must be passed`))
}

func extendSubschemaData(sub *subschemaContext, it *schemaCxt, a *subschemaArgs) {
	if a.data != nil && a.dataProp != nil {
		panic(newError(`both "data" and "dataProp" passed, only one allowed`))
	}
	gen := it.gen
	dataContextProps := func(nextData *jsName) {
		sub.data = nextData
		sub.dataLevel = it.dataLevel + 1
		sub.dataTypes = []string{}
		sub.hasDataTypes = true
		it.definedProperties = newStringSet()
		sub.parentData = it.data
		sub.dataNames = append(append([]*jsName(nil), it.dataNames...), nextData)
		sub.hasData = true
	}
	if a.dataProp != nil {
		parent, prop := it.data, a.dataProp
		nextData := gen.let("data", exprFn(func(f *frame) any { return getProp(f.get(parent), eval(prop, f)) }))
		dataContextProps(nextData)
		sub.errorPath = it.errorPath.with(getErrorPath(a.dataProp, a.dataPropType))
		sub.parentDataProperty = a.dataProp
		sub.dataPathArr = append(append([]jsCode(nil), it.dataPathArr...), a.dataProp)
		sub.hasDataProp = true
	}
	if a.data != nil {
		nextData, ok := a.data.(*jsName)
		if !ok {
			nextData = gen.let("data", a.data)
		}
		dataContextProps(nextData)
		if a.propertyName != nil {
			sub.propertyName = a.propertyName
		}
	}
	if a.dataTypes != nil {
		sub.dataTypes = a.dataTypes
		sub.hasDataTypes = true
	}
}

// stringSet is a JavaScript Set of strings.
type stringSet struct{ m map[string]bool }

func newStringSet() *stringSet { return &stringSet{m: map[string]bool{}} }

func (s *stringSet) add(k string)      { s.m[k] = true }
func (s *stringSet) has(k string) bool { return s.m[k] }
