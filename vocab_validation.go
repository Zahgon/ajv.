// Ported from Ajv 8.20.0, lib/vocabularies/validation, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"math"
	"strconv"

	"github.com/ajv-validator/ajv-go/internal/js"
)

func validationVocabulary() []any {
	return []any{
		limitNumberKeyword(),
		multipleOfKeyword(),
		limitLengthKeyword(),
		patternKeyword(),
		limitPropertiesKeyword(),
		requiredKeyword(),
		limitItemsKeyword(),
		uniqueItemsKeyword(),
		&keywordDef{keywords: []string{"type"}, schemaType: []string{"string", "array"}},
		&keywordDef{keywords: []string{"nullable"}, schemaType: []string{"boolean"}},
		constKeyword(),
		enumKeyword(),
	}
}

var limitOps = map[string]struct {
	okStr string
	fail  relOp
}{
	"maximum":          {"<=", opGT},
	"minimum":          {">=", opLT},
	"exclusiveMaximum": {"<", opGTE},
	"exclusiveMinimum": {">", opLTE},
}

func limitNumberKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"maximum", "minimum", "exclusiveMaximum", "exclusiveMinimum"},
		typ:        []string{"number"},
		schemaType: []string{"number"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit("must be "+limitOps[cxt.keyword].okStr+" "), cxt.schemaCode)
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("comparison", limitOps[cxt.keyword].okStr, "limit", cxt.schemaCode)
			},
		},
		code: func(cxt *keywordCxt, _ string) {
			data, schemaCode := cxt.data, cxt.schemaCode
			op := limitOps[cxt.keyword].fail
			cxt.failData(exprFn(func(f *frame) any {
				d := f.get(data)
				return compare(d, op, eval(schemaCode, f)) || isNaNValue(d)
			}))
		},
	}
}

func multipleOfKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"multipleOf"},
		typ:        []string{"number"},
		schemaType: []string{"number"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit("must be multiple of "), cxt.schemaCode)
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("multipleOf", cxt.schemaCode)
			},
		},
		code: func(cxt *keywordCxt, _ string) {
			gen, data, schemaCode := cxt.gen, cxt.data, cxt.schemaCode
			prec := cxt.it.opts.multipleOfPrecision
			res := gen.letNoInit("res")
			eps := js.ToNumber("1e-" + strconv.Itoa(prec))
			cxt.failData(exprFn(func(f *frame) any {
				sc := eval(schemaCode, f)
				if strictEquals(sc, 0.0) {
					return true
				}
				r := toNumber(f.get(data)) / toNumber(sc)
				f.set(res, r)
				if prec != 0 {
					return math.Abs(js.MathRound(r)-r) > eps
				}
				return r != js.ParseInt(js.NumberToString(r), 0)
			}))
		},
	}
}

func limitLengthKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"maxLength", "minLength"},
		typ:        []string{"string"},
		schemaType: []string{"number"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				comp := "fewer"
				if cxt.keyword == "maxLength" {
					comp = "more"
				}
				return strMsg(strLit("must NOT have "+comp+" than "), cxt.schemaCode, strLit(" characters"))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object { return paramsObj("limit", cxt.schemaCode) },
		},
		code: func(cxt *keywordCxt, _ string) {
			data, schemaCode := cxt.data, cxt.schemaCode
			op := opLT
			if cxt.keyword == "maxLength" {
				op = opGT
			}
			cxt.failData(exprFn(func(f *frame) any {
				return compare(float64(ucs2length(toString(f.get(data)))), op, eval(schemaCode, f))
			}))
		},
	}
}

func patternKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"pattern"},
		typ:        []string{"string"},
		schemaType: []string{"string"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit(`must match pattern "`), cxt.schemaCode, strLit(`"`))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object { return paramsObj("pattern", cxt.schemaCode) },
		},
		code: func(cxt *keywordCxt, _ string) {
			gen, data, schemaCode := cxt.gen, cxt.data, cxt.schemaCode
			u := ""
			if truthy(cxt.it.opts.unicodeRegExp) {
				u = "u"
			}
			if cxt.dataRef != "" {
				valid := gen.letNoInit("valid")
				gen.code(func(f *frame) { f.set(valid, testDataPattern(eval(schemaCode, f), u, f.get(data))) })
				cxt.failData(not(valid))
				return
			}
			re := usePattern(cxt, cxt.schema.(string))
			cxt.failData(exprFn(func(f *frame) any { return !re.Test(toString(f.get(data))) }))
		},
	}
}

// testDataPattern is `try { new RegExp(pattern, u).test(data) } catch { false }`.
func testDataPattern(pattern any, u string, data any) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isErr := r.(error); !isErr {
				panic(r)
			}
			ok = false
		}
	}()
	source := "(?:)"
	if !isUndefined(pattern) {
		source = toString(pattern)
	}
	return newRegExp(source, u).Test(toString(data))
}

func limitPropertiesKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"maxProperties", "minProperties"},
		typ:        []string{"object"},
		schemaType: []string{"number"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				comp := "fewer"
				if cxt.keyword == "maxProperties" {
					comp = "more"
				}
				return strMsg(strLit("must NOT have "+comp+" than "), cxt.schemaCode, strLit(" properties"))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object { return paramsObj("limit", cxt.schemaCode) },
		},
		code: func(cxt *keywordCxt, _ string) {
			data, schemaCode := cxt.data, cxt.schemaCode
			op := opLT
			if cxt.keyword == "maxProperties" {
				op = opGT
			}
			cxt.failData(exprFn(func(f *frame) any {
				return compare(float64(len(objectKeys(f.get(data)))), op, eval(schemaCode, f))
			}))
		},
	}
}

func limitItemsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"maxItems", "minItems"},
		typ:        []string{"array"},
		schemaType: []string{"number"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				comp := "fewer"
				if cxt.keyword == "maxItems" {
					comp = "more"
				}
				return strMsg(strLit("must NOT have "+comp+" than "), cxt.schemaCode, strLit(" items"))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object { return paramsObj("limit", cxt.schemaCode) },
		},
		code: func(cxt *keywordCxt, _ string) {
			data, schemaCode := cxt.data, cxt.schemaCode
			op := opLT
			if cxt.keyword == "maxItems" {
				op = opGT
			}
			cxt.failData(exprFn(func(f *frame) any {
				return compare(getProp(f.get(data), "length"), op, eval(schemaCode, f))
			}))
		},
	}
}

func requiredKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"required"},
		typ:        []string{"object"},
		schemaType: []string{"array"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit("must have required property '"), cxt.param("missingProperty"), strLit("'"))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("missingProperty", cxt.param("missingProperty"))
			},
		},
		code: requiredCode,
	}
}

func requiredCode(cxt *keywordCxt, _ string) {
	gen, schemaCode, data, it := cxt.gen, cxt.schemaCode, cxt.data, cxt.it
	opts := it.opts
	schema, _ := cxt.schema.([]any)
	if cxt.dataRef == "" && len(schema) == 0 {
		return
	}
	useLoop := cxt.dataRef == "" && float64(len(schema)) >= opts.loopRequired
	loopAllRequired := func() {
		gen.forOf("prop", schemaCode, func(prop *jsName) {
			cxt.setParams(map[string]jsCode{"missingProperty": prop}, false)
			gen.ifThen(noPropertyInData(data, prop, opts.ownProperties))
			cxt.error(false)
			gen.endIf()
		})
	}
	loopUntilMissing := func(missing, valid *jsName) {
		cxt.setParams(map[string]jsCode{"missingProperty": missing}, false)
		gen.forOf(missing, schemaCode, func(*jsName) {
			gen.assign(valid, propertyInData(data, missing, opts.ownProperties))
			gen.ifThen(not(valid))
			cxt.error(false)
			gen.breakStmt(nil)
			gen.endIf()
		})
	}
	if it.allErrors {
		if useLoop || cxt.dataRef != "" {
			cxt.blockData(nil, loopAllRequired, nilCode)
		} else {
			for _, prop := range schema {
				checkReportMissingProp(cxt, toString(prop))
			}
		}
	} else {
		missing := gen.letNoInit("missing")
		if useLoop || cxt.dataRef != "" {
			valid := gen.let("valid", true)
			cxt.blockData(valid, func() { loopUntilMissing(missing, valid) }, nilCode)
			cxt.ok(valid)
		} else {
			gen.ifThen(checkMissingProp(cxt, schema, missing))
			reportMissingProp(cxt, missing)
			gen.elseBranch()
		}
	}
	if truthy(opts.strictRequired) {
		props := getProp(cxt.parentSchema, "properties")
		definedProperties := cxt.it.definedProperties
		for _, requiredKey := range schema {
			key := toString(requiredKey)
			if isUndefined(getProp(props, key)) && !definedProperties.has(key) {
				schemaPath := it.schemaEnv.baseID + it.errSchemaPath
				msg := `required property "` + key + `" is not defined at "` + schemaPath + `" (strictRequired)`
				checkStrictMode(it, msg, opts.strictRequired)
			}
		}
	}
}

func uniqueItemsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"uniqueItems"},
		typ:        []string{"array"},
		schemaType: []string{"boolean"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit("must NOT have duplicate items (items ## "), cxt.param("j"),
					strLit(" and "), cxt.param("i"), strLit(" are identical)"))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("i", cxt.param("i"), "j", cxt.param("j"))
			},
		},
		code: uniqueItemsCode,
	}
}

func uniqueItemsCode(cxt *keywordCxt, _ string) {
	gen, data, schemaCode, it := cxt.gen, cxt.data, cxt.schemaCode, cxt.it
	if cxt.dataRef == "" && !truthy(cxt.schema) {
		return
	}
	valid := gen.letNoInit("valid")
	var itemTypes []string
	if items := getProp(cxt.parentSchema, "items"); truthy(items) {
		itemTypes = getSchemaTypes(items)
	}
	canOptimize := len(itemTypes) > 0
	for _, t := range itemTypes {
		if t == "object" || t == "array" {
			canOptimize = false
		}
	}
	loopN := func(i, j *jsName) {
		item := gen.name("item")
		wrongType := checkDataTypes(itemTypes, item, it.opts.strictNumbers, true)
		indices := gen.constDef("indices", exprFn(func(*frame) any { return NewObject() }))
		gen.forLoop(decrementLoop(i), func() {
			gen.let(item, exprFn(func(f *frame) any { return getProp(f.get(data), f.get(i)) }))
			gen.ifThen(wrongType)
			gen.continueStmt()
			gen.endIf()
			if len(itemTypes) > 1 {
				gen.code(func(f *frame) {
					if s, ok := f.get(item).(string); ok {
						f.set(item, s+"_")
					}
				})
			}
			gen.ifThen(exprFn(func(f *frame) any {
				return typeOf(getProp(f.get(indices), f.get(item))) == "number"
			}))
			gen.assign(j, exprFn(func(f *frame) any { return getProp(f.get(indices), f.get(item)) }))
			cxt.error(false)
			gen.assign(valid, false)
			gen.breakStmt(nil)
			gen.endIf()
			gen.code(func(f *frame) { setProp(f.get(indices), f.get(item), f.get(i)) })
		})
	}
	loopN2 := func(i, j *jsName) {
		outer := gen.name("outer")
		gen.label(outer)
		gen.forLoop(decrementLoop(i), func() {
			gen.forLoop(func(f *frame, body func() ctl) ctl {
				f.set(j, f.get(i))
				for {
					jv := f.num(j)
					f.set(j, jv-1)
					if !truthy(jv) {
						return ctlNormal
					}
					if done, res := loopResult(body()); done {
						return res
					}
				}
			}, func() {
				gen.ifThen(exprFn(func(f *frame) any {
					d := f.get(data)
					return equal(getProp(d, f.get(i)), getProp(d, f.get(j)))
				}))
				cxt.error(false)
				gen.assign(valid, false)
				gen.breakStmt(outer)
				gen.endIf()
			})
		})
	}
	validateUniqueItems := func() {
		i := gen.let("i", exprFn(func(f *frame) any { return getProp(f.get(data), "length") }))
		j := gen.letNoInit("j")
		cxt.setParams(map[string]jsCode{"i": i, "j": j}, false)
		gen.assign(valid, true)
		gen.ifThen(exprFn(func(f *frame) any { return compare(f.get(i), opGT, 1.0) }))
		if canOptimize {
			loopN(i, j)
		} else {
			loopN2(i, j)
		}
		gen.endIf()
	}
	cxt.blockData(valid, validateUniqueItems, exprFn(func(f *frame) any { return eval(schemaCode, f) == false }))
	cxt.ok(valid)
}

// decrementLoop is the loop `for(;i--;)`.
func decrementLoop(i *jsName) func(f *frame, body func() ctl) ctl {
	return func(f *frame, body func() ctl) ctl {
		for {
			iv := f.num(i)
			f.set(i, iv-1)
			if !truthy(iv) {
				return ctlNormal
			}
			if done, res := loopResult(body()); done {
				return res
			}
		}
	}
}

func constKeyword() *keywordDef {
	return &keywordDef{
		keywords: []string{"const"},
		data:     true,
		err: &errorDef{
			message: constMsg("must be equal to constant"),
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("allowedValue", cxt.schemaCode)
			},
		},
		code: func(cxt *keywordCxt, _ string) {
			data, schemaCode, schema := cxt.data, cxt.schemaCode, cxt.schema
			if cxt.dataRef != "" || (truthy(schema) && typeOf(schema) == "object") {
				cxt.failData(exprFn(func(f *frame) any { return !equal(f.get(data), eval(schemaCode, f)) }))
			} else {
				cxt.fail(exprFn(func(f *frame) any { return !strictEquals(schema, f.get(data)) }))
			}
		},
	}
}

func enumKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"enum"},
		schemaType: []string{"array"},
		data:       true,
		err: &errorDef{
			message: constMsg("must be equal to one of the allowed values"),
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("allowedValues", cxt.schemaCode)
			},
		},
		code: func(cxt *keywordCxt, _ string) {
			gen, data, schemaCode, it := cxt.gen, cxt.data, cxt.schemaCode, cxt.it
			schema, _ := cxt.schema.([]any)
			if cxt.dataRef == "" && len(schema) == 0 {
				panic(newError("enum must have non-empty array"))
			}
			useLoop := cxt.dataRef == "" && float64(len(schema)) >= it.opts.loopEnum
			var valid jsCode
			if useLoop || cxt.dataRef != "" {
				v := gen.letNoInit("valid")
				valid = v
				cxt.blockData(v, func() {
					gen.assign(v, false)
					gen.forOf("v", schemaCode, func(item *jsName) {
						gen.ifThen(exprFn(func(f *frame) any { return equal(f.get(data), f.get(item)) }))
						gen.assign(v, true)
						gen.breakStmt(nil)
						gen.endIf()
					})
				}, nilCode)
			} else {
				gen.constDef("vSchema", schemaCode)
				conds := make([]jsCode, len(schema))
				for i, sch := range schema {
					s := sch
					if typeOf(s) == "object" && s != nil {
						conds[i] = exprFn(func(f *frame) any { return equal(f.get(data), s) })
					} else {
						conds[i] = exprFn(func(f *frame) any { return strictEquals(f.get(data), s) })
					}
				}
				valid = or(conds...)
			}
			cxt.pass(valid, nil)
		},
	}
}
