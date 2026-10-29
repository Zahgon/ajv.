// Ported from Ajv 8.20.0, lib/vocabularies/applicator, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"strconv"
	"strings"
)

func applicatorVocabulary(draft2020 bool) []any {
	v := []any{
		notKeyword(),
		anyOfKeyword(),
		oneOfKeyword(),
		allOfKeyword(),
		ifKeyword(),
		thenElseKeyword(),
		propertyNamesKeyword(),
		additionalPropertiesKeyword(),
		dependenciesKeyword(),
		propertiesKeyword(),
		patternPropertiesKeyword(),
	}
	if draft2020 {
		v = append(v, prefixItemsKeyword(), items2020Keyword())
	} else {
		v = append(v, additionalItemsKeyword(), itemsKeyword())
	}
	return append(v, containsKeyword())
}

func lenOf(data *jsName) exprFn {
	return func(f *frame) any { return getProp(f.get(data), "length") }
}

func itemsLimitError() *errorDef {
	return &errorDef{
		message: func(cxt *keywordCxt) func(f *frame) string {
			return strMsg(strLit("must NOT have more than "), cxt.param("len"), strLit(" items"))
		},
		params: func(cxt *keywordCxt) func(f *frame) *Object { return paramsObj("limit", cxt.param("len")) },
	}
}

func additionalItemsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"additionalItems"},
		typ:        []string{"array"},
		schemaType: []string{"boolean", "object"},
		before:     "uniqueItems",
		err:        itemsLimitError(),
		code: func(cxt *keywordCxt, _ string) {
			items, ok := getProp(cxt.parentSchema, "items").([]any)
			if !ok {
				checkStrictMode(cxt.it, `"additionalItems" is ignored when "items" is not an array of schemas`, cxt.it.opts.strictSchema)
				return
			}
			validateAdditionalItems(cxt, items)
		},
	}
}

func validateAdditionalItems(cxt *keywordCxt, items []any) {
	gen, schema, data, keyword, it := cxt.gen, cxt.schema, cxt.data, cxt.keyword, cxt.it
	it.items = true
	length := gen.constDef("len", lenOf(data))
	n := float64(len(items))
	if schema == false {
		cxt.setParams(map[string]jsCode{"len": n}, false)
		cxt.pass(exprFn(func(f *frame) any { return compare(f.get(length), opLTE, n) }), nil)
	} else if typeOf(schema) == "object" && !alwaysValidSchema(it, schema) {
		valid := gen.varDef("valid", exprFn(func(f *frame) any { return compare(f.get(length), opLTE, n) }))
		gen.ifThen(not(valid))
		gen.forRange("i", n, length, func(i *jsName) {
			cxt.subschema(&subschemaArgs{keyword: keyword, dataProp: i, dataPropType: typeNum}, valid)
			if !it.allErrors {
				gen.ifThen(not(valid))
				gen.breakStmt(nil)
				gen.endIf()
			}
		})
		gen.endIf()
		cxt.ok(valid)
	}
}

func itemsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"items"},
		typ:        []string{"array"},
		schemaType: []string{"object", "array", "boolean"},
		before:     "uniqueItems",
		code: func(cxt *keywordCxt, _ string) {
			it := cxt.it
			if arr, ok := cxt.schema.([]any); ok {
				validateTuple(cxt, "additionalItems", arr)
				return
			}
			it.items = true
			if alwaysValidSchema(it, cxt.schema) {
				return
			}
			cxt.ok(validateArray(cxt))
		},
	}
}

func validateTuple(cxt *keywordCxt, extraItems string, schArr []any) {
	gen, parentSchema, data, keyword, it := cxt.gen, cxt.parentSchema, cxt.data, cxt.keyword, cxt.it
	l := float64(len(schArr))
	fullTuple := strictEquals(l, getProp(parentSchema, "minItems")) &&
		(strictEquals(l, getProp(parentSchema, "maxItems")) || getProp(parentSchema, extraItems) == false)
	if truthy(it.opts.strictTuples) && !fullTuple {
		msg := `"` + keyword + `" is ` + strconv.Itoa(len(schArr)) + `-tuple, but minItems or maxItems/` + extraItems +
			` are not specified or different at path "` + it.errSchemaPath + `"`
		checkStrictMode(it, msg, it.opts.strictTuples)
	}
	if it.opts.unevaluated && len(schArr) > 0 && it.items != true {
		it.items = mergeEvaluatedItems(gen, l, it.items, false)
	}
	valid := gen.name("valid")
	length := gen.constDef("len", lenOf(data))
	for i, sch := range schArr {
		if alwaysValidSchema(it, sch) {
			continue
		}
		idx := float64(i)
		gen.ifThen(exprFn(func(f *frame) any { return compare(f.get(length), opGT, idx) }))
		cxt.subschema(&subschemaArgs{keyword: keyword, schemaProp: i, dataProp: idx}, valid)
		gen.endIf()
		cxt.ok(valid)
	}
}

func prefixItemsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"prefixItems"},
		typ:        []string{"array"},
		schemaType: []string{"array"},
		before:     "uniqueItems",
		code: func(cxt *keywordCxt, _ string) {
			arr, _ := cxt.schema.([]any)
			validateTuple(cxt, "items", arr)
		},
	}
}

func items2020Keyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"items"},
		typ:        []string{"array"},
		schemaType: []string{"object", "boolean"},
		before:     "uniqueItems",
		err:        itemsLimitError(),
		code: func(cxt *keywordCxt, _ string) {
			it := cxt.it
			prefixItems := getProp(cxt.parentSchema, "prefixItems")
			it.items = true
			if alwaysValidSchema(it, cxt.schema) {
				return
			}
			if truthy(prefixItems) {
				arr, _ := prefixItems.([]any)
				validateAdditionalItems(cxt, arr)
			} else {
				cxt.ok(validateArray(cxt))
			}
		},
	}
}

func containsKeyword() *keywordDef {
	return &keywordDef{
		keywords:    []string{"contains"},
		typ:         []string{"array"},
		schemaType:  []string{"object", "boolean"},
		before:      "uniqueItems",
		trackErrors: true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				min, max := cxt.param("min"), cxt.param("max")
				if isUndefined(max) {
					return strMsg(strLit("must contain at least "), min, strLit(" valid item(s)"))
				}
				return strMsg(strLit("must contain at least "), min, strLit(" and no more than "), max, strLit(" valid item(s)"))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				min, max := cxt.param("min"), cxt.param("max")
				if isUndefined(max) {
					return paramsObj("minContains", min)
				}
				return paramsObj("minContains", min, "maxContains", max)
			},
		},
		code: containsCode,
	}
}

func containsCode(cxt *keywordCxt, _ string) {
	gen, schema, parentSchema, data, it := cxt.gen, cxt.schema, cxt.parentSchema, cxt.data, cxt.it
	var min, max any = 1.0, undefined
	if it.opts.next {
		minContains := getProp(parentSchema, "minContains")
		if !isUndefined(minContains) {
			min = minContains
		}
		max = getProp(parentSchema, "maxContains")
	}
	length := gen.constDef("len", lenOf(data))
	cxt.setParams(map[string]jsCode{"min": min, "max": max}, false)
	if isUndefined(max) && strictEquals(min, 0.0) {
		checkStrictMode(it, `"minContains" == 0 without "maxContains": "contains" keyword ignored`, it.opts.strictSchema)
		return
	}
	if !isUndefined(max) && compare(min, opGT, max) {
		checkStrictMode(it, `"minContains" > "maxContains" is always invalid`, it.opts.strictSchema)
		cxt.failAlways()
		return
	}
	if alwaysValidSchema(it, schema) {
		cond := exprFn(func(f *frame) any {
			l := f.get(length)
			return compare(l, opGTE, min) && (isUndefined(max) || compare(l, opLTE, max))
		})
		cxt.pass(cond, nil)
		return
	}
	it.items = true
	valid := gen.name("valid")
	validateItems := func(v *jsName, block func()) {
		gen.forRange("i", 0.0, length, func(i *jsName) {
			cxt.subschema(&subschemaArgs{keyword: "contains", dataProp: i, dataPropType: typeNum, compositeRule: true}, v)
			block()
		})
	}
	checkLimits := func(count *jsName) {
		gen.code(func(f *frame) { f.set(count, f.num(count)+1) })
		if isUndefined(max) {
			gen.ifThen(exprFn(func(f *frame) any { return compare(f.get(count), opGTE, min) }))
			gen.assign(valid, true)
			gen.breakStmt(nil)
			gen.endIf()
		} else {
			gen.ifThen(exprFn(func(f *frame) any { return compare(f.get(count), opGT, max) }))
			gen.assign(valid, false)
			gen.breakStmt(nil)
			gen.endIf()
			if strictEquals(min, 1.0) {
				gen.assign(valid, true)
			} else {
				gen.ifThen(exprFn(func(f *frame) any { return compare(f.get(count), opGTE, min) }))
				gen.assign(valid, true)
				gen.endIf()
			}
		}
	}
	validateItemsWithCount := func() {
		schValid := gen.name("_valid")
		count := gen.let("count", 0.0)
		validateItems(schValid, func() {
			gen.ifThen(schValid)
			checkLimits(count)
			gen.endIf()
		})
	}
	switch {
	case isUndefined(max) && strictEquals(min, 1.0):
		validateItems(valid, func() {
			gen.ifThen(valid)
			gen.breakStmt(nil)
			gen.endIf()
		})
	case strictEquals(min, 0.0):
		gen.let(valid, true)
		if !isUndefined(max) {
			gen.ifThen(exprFn(func(f *frame) any { return compare(getProp(f.get(data), "length"), opGT, 0.0) }))
			validateItemsWithCount()
			gen.endIf()
		}
	default:
		gen.let(valid, false)
		validateItemsWithCount()
	}
	cxt.result(valid, func() { cxt.reset() }, nil)
}

func dependenciesError() *errorDef {
	return &errorDef{
		message: func(cxt *keywordCxt) func(f *frame) string {
			propertyIes := "properties"
			if strictEquals(cxt.param("depsCount"), 1.0) {
				propertyIes = "property"
			}
			return strMsg(strLit("must have "+propertyIes+" "), cxt.param("deps"), strLit(" when property "),
				cxt.param("property"), strLit(" is present"))
		},
		params: func(cxt *keywordCxt) func(f *frame) *Object {
			return paramsObj("property", cxt.param("property"), "missingProperty", cxt.param("missingProperty"),
				"depsCount", cxt.param("depsCount"), "deps", cxt.param("deps"))
		},
	}
}

func dependenciesKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"dependencies"},
		typ:        []string{"object"},
		schemaType: []string{"object"},
		err:        dependenciesError(),
		code: func(cxt *keywordCxt, _ string) {
			propDeps, schDeps := NewObject(), NewObject()
			for _, key := range objectKeys(cxt.schema) {
				if key == "__proto__" {
					continue
				}
				v := getProp(cxt.schema, key)
				if isArray(v) {
					propDeps.Set(key, v)
				} else {
					schDeps.Set(key, v)
				}
			}
			validatePropertyDeps(cxt, propDeps)
			validateSchemaDeps(cxt, schDeps)
		},
	}
}

func validatePropertyDeps(cxt *keywordCxt, propertyDeps any) {
	gen, data, it := cxt.gen, cxt.data, cxt.it
	if len(objectKeys(propertyDeps)) == 0 {
		return
	}
	missing := gen.letNoInit("missing")
	for _, prop := range objectKeys(propertyDeps) {
		value := getProp(propertyDeps, prop)
		if value == nil {
			panic(newError("Cannot read properties of null (reading 'length')"))
		}
		if strictEquals(getProp(value, "length"), 0.0) {
			continue
		}
		hasProperty := propertyInData(data, prop, it.opts.ownProperties)
		deps, ok := value.([]any)
		if !ok {
			panic(newError("deps.join is not a function"))
		}
		depStrs := make([]string, len(deps))
		for i, d := range deps {
			if d != nil && !isUndefined(d) {
				depStrs[i] = toString(d)
			}
		}
		cxt.setParams(map[string]jsCode{
			"property":  prop,
			"depsCount": float64(len(deps)),
			"deps":      strings.Join(depStrs, ", "),
		}, false)
		if it.allErrors {
			gen.ifThen(hasProperty)
			for _, depProp := range deps {
				checkReportMissingProp(cxt, toString(depProp))
			}
			gen.endIf()
		} else {
			gen.ifThen(and(hasProperty, checkMissingProp(cxt, deps, missing)))
			reportMissingProp(cxt, missing)
			gen.elseBranch()
		}
	}
}

func validateSchemaDeps(cxt *keywordCxt, schemaDeps any) {
	gen, data, keyword, it := cxt.gen, cxt.data, cxt.keyword, cxt.it
	valid := gen.name("valid")
	for _, prop := range objectKeys(schemaDeps) {
		if alwaysValidSchema(it, getProp(schemaDeps, prop)) {
			continue
		}
		gen.ifThen(propertyInData(data, prop, it.opts.ownProperties))
		schCxt := cxt.subschema(&subschemaArgs{keyword: keyword, schemaProp: prop}, valid)
		cxt.mergeValidEvaluated(schCxt, valid)
		gen.elseBranch()
		gen.varDef(valid, true)
		gen.endIf()
		cxt.ok(valid)
	}
}

func dependentSchemasKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"dependentSchemas"},
		typ:        []string{"object"},
		schemaType: []string{"object"},
		code:       func(cxt *keywordCxt, _ string) { validateSchemaDeps(cxt, cxt.schema) },
	}
}

func dependentRequiredKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"dependentRequired"},
		typ:        []string{"object"},
		schemaType: []string{"object"},
		err:        dependenciesError(),
		code:       func(cxt *keywordCxt, _ string) { validatePropertyDeps(cxt, cxt.schema) },
	}
}

func propertyNamesKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"propertyNames"},
		typ:        []string{"object"},
		schemaType: []string{"object", "boolean"},
		err: &errorDef{
			message: constMsg("property name must be valid"),
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("propertyName", cxt.param("propertyName"))
			},
		},
		code: func(cxt *keywordCxt, _ string) {
			gen, data, it := cxt.gen, cxt.data, cxt.it
			if alwaysValidSchema(it, cxt.schema) {
				return
			}
			valid := gen.name("valid")
			gen.forIn("key", data, it.opts.ownProperties, func(key *jsName) {
				cxt.setParams(map[string]jsCode{"propertyName": key}, false)
				cxt.subschema(&subschemaArgs{
					keyword:       "propertyNames",
					data:          key,
					dataTypes:     []string{"string"},
					propertyName:  key,
					compositeRule: true,
				}, valid)
				gen.ifThen(not(valid))
				cxt.error(true)
				if !it.allErrors {
					gen.breakStmt(nil)
				}
				gen.endIf()
			})
			cxt.ok(valid)
		},
	}
}

func additionalPropertiesKeyword() *keywordDef {
	return &keywordDef{
		keywords:       []string{"additionalProperties"},
		typ:            []string{"object"},
		schemaType:     []string{"boolean", "object"},
		allowUndefined: true,
		trackErrors:    true,
		err: &errorDef{
			message: constMsg("must NOT have additional properties"),
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("additionalProperty", cxt.param("additionalProperty"))
			},
		},
		code: additionalPropertiesCode,
	}
}

func additionalPropertiesCode(cxt *keywordCxt, _ string) {
	gen, schema, parentSchema, data, errsCount, it := cxt.gen, cxt.schema, cxt.parentSchema, cxt.data, cxt.errsCount, cxt.it
	if errsCount == nil {
		panic(newError("ajv implementation error"))
	}
	allErrors, opts := it.allErrors, it.opts
	it.props = true
	if opts.removeAdditional != "all" && alwaysValidSchema(it, schema) {
		return
	}
	props := allSchemaProperties(getProp(parentSchema, "properties"))
	patProps := allSchemaProperties(getProp(parentSchema, "patternProperties"))

	isAdditional := func(key *jsName) jsCode {
		definedProp := nilCode
		if len(props) > 8 {
			propsSchema := getProp(parentSchema, "properties")
			definedProp = exprFn(func(f *frame) any { return hasOwnProperty(propsSchema, f.get(key)) })
		} else if len(props) > 0 {
			conds := make([]jsCode, len(props))
			for i, p := range props {
				name := p
				conds[i] = exprFn(func(f *frame) any { return f.get(key) == name })
			}
			definedProp = or(conds...)
		}
		if len(patProps) > 0 {
			conds := []jsCode{definedProp}
			for _, p := range patProps {
				re := usePattern(cxt, p)
				conds = append(conds, exprFn(func(f *frame) any { return re.Test(toString(f.get(key))) }))
			}
			definedProp = or(conds...)
		}
		return not(definedProp)
	}
	deleteAdditional := func(key *jsName) {
		gen.code(func(f *frame) { deleteProp(f.get(data), f.get(key)) })
	}
	applyAdditionalSchema := func(key, valid *jsName, noErrors bool) {
		a := &subschemaArgs{keyword: "additionalProperties", dataProp: key, dataPropType: typeStr}
		if noErrors {
			a.compositeRule = true
			a.createErrors = &falseValue
			a.allErrors = &falseValue
		}
		cxt.subschema(a, valid)
	}
	additionalPropertyCode := func(key *jsName) {
		if opts.removeAdditional == "all" || (truthy(opts.removeAdditional) && schema == false) {
			deleteAdditional(key)
			return
		}
		if schema == false {
			cxt.setParams(map[string]jsCode{"additionalProperty": key}, false)
			cxt.error(false)
			if !allErrors {
				gen.breakStmt(nil)
			}
			return
		}
		if typeOf(schema) == "object" && !alwaysValidSchema(it, schema) {
			valid := gen.name("valid")
			if opts.removeAdditional == "failing" {
				applyAdditionalSchema(key, valid, true)
				gen.ifThen(not(valid))
				cxt.reset()
				deleteAdditional(key)
				gen.endIf()
			} else {
				applyAdditionalSchema(key, valid, false)
				if !allErrors {
					gen.ifThen(not(valid))
					gen.breakStmt(nil)
					gen.endIf()
				}
			}
		}
	}
	gen.forIn("key", data, opts.ownProperties, func(key *jsName) {
		if len(props) == 0 && len(patProps) == 0 {
			additionalPropertyCode(key)
		} else {
			gen.ifThen(isAdditional(key))
			additionalPropertyCode(key)
			gen.endIf()
		}
	})
	cxt.ok(errorsEqual(errsCount))
}

func propertiesKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"properties"},
		typ:        []string{"object"},
		schemaType: []string{"object"},
		code: func(cxt *keywordCxt, _ string) {
			gen, schema, parentSchema, data, it := cxt.gen, cxt.schema, cxt.parentSchema, cxt.data, cxt.it
			if it.opts.removeAdditional == "all" && isUndefined(getProp(parentSchema, "additionalProperties")) {
				apDef := it.self.rules.all["additionalProperties"].definition
				additionalPropertiesCode(newKeywordCxt(it, apDef, "additionalProperties"), "")
			}
			allProps := allSchemaProperties(schema)
			for _, prop := range allProps {
				it.definedProperties.add(prop)
			}
			if it.opts.unevaluated && len(allProps) > 0 && it.props != true {
				it.props = mergeEvaluatedProps(gen, newPropSet(allProps), it.props, false)
			}
			var properties []string
			for _, p := range allProps {
				if !alwaysValidSchema(it, getProp(schema, p)) {
					properties = append(properties, p)
				}
			}
			if len(properties) == 0 {
				return
			}
			valid := gen.name("valid")
			for _, prop := range properties {
				hasDefault := truthy(it.opts.useDefaults) && !it.compositeRule &&
					!isUndefined(getProp(getProp(schema, prop), "default"))
				apply := func() {
					cxt.subschema(&subschemaArgs{keyword: "properties", schemaProp: prop, dataProp: prop}, valid)
				}
				if hasDefault {
					apply()
				} else {
					gen.ifThen(propertyInData(data, prop, it.opts.ownProperties))
					apply()
					if !it.allErrors {
						gen.elseBranch()
						gen.varDef(valid, true)
					}
					gen.endIf()
				}
				cxt.it.definedProperties.add(prop)
				cxt.ok(valid)
			}
		},
	}
}

func patternPropertiesKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"patternProperties"},
		typ:        []string{"object"},
		schemaType: []string{"object"},
		code:       patternPropertiesCode,
	}
}

func patternPropertiesCode(cxt *keywordCxt, _ string) {
	gen, schema, data, parentSchema, it := cxt.gen, cxt.schema, cxt.data, cxt.parentSchema, cxt.it
	opts := it.opts
	patterns := allSchemaProperties(schema)
	alwaysValidPatterns := map[string]bool{}
	for _, p := range patterns {
		if alwaysValidSchema(it, getProp(schema, p)) {
			alwaysValidPatterns[p] = true
		}
	}
	if len(patterns) == 0 || (len(alwaysValidPatterns) == len(patterns) && (!it.opts.unevaluated || it.props == true)) {
		return
	}
	var checkProperties any
	if truthy(opts.strictSchema) && !opts.allowMatchingProperties {
		checkProperties = getProp(parentSchema, "properties")
	}
	valid := gen.name("valid")
	if it.props != true && !isName(it.props) {
		it.props = evaluatedPropsToName(gen, it.props)
	}
	props := it.props
	checkMatchingProperties := func(pat string) {
		for _, prop := range objectKeys(checkProperties) {
			if newRegExp(pat, "").Test(prop) {
				checkStrictMode(it, "property "+prop+" matches pattern "+pat+" (use allowMatchingProperties)", opts.strictSchema)
			}
		}
	}
	validateProperties := func(pat string) {
		gen.forIn("key", data, opts.ownProperties, func(key *jsName) {
			re := usePattern(cxt, pat)
			gen.ifThen(exprFn(func(f *frame) any { return re.Test(toString(f.get(key))) }))
			alwaysValid := alwaysValidPatterns[pat]
			if !alwaysValid {
				cxt.subschema(&subschemaArgs{keyword: "patternProperties", schemaProp: pat, dataProp: key, dataPropType: typeStr}, valid)
			}
			if pn, ok := props.(*jsName); it.opts.unevaluated && ok {
				gen.code(func(f *frame) { setProp(f.get(pn), f.get(key), true) })
			} else if !alwaysValid && !it.allErrors {
				gen.ifThen(not(valid))
				gen.breakStmt(nil)
				gen.endIf()
			}
			gen.endIf()
		})
	}
	for _, pat := range patterns {
		if truthy(checkProperties) {
			checkMatchingProperties(pat)
		}
		if it.allErrors {
			validateProperties(pat)
		} else {
			gen.varDef(valid, true)
			validateProperties(pat)
			gen.ifThen(valid)
		}
	}
}

func notKeyword() *keywordDef {
	return &keywordDef{
		keywords:    []string{"not"},
		schemaType:  []string{"object", "boolean"},
		trackErrors: true,
		err:         &errorDef{message: constMsg("must NOT be valid")},
		code: func(cxt *keywordCxt, _ string) {
			if alwaysValidSchema(cxt.it, cxt.schema) {
				cxt.failAlways()
				return
			}
			valid := cxt.gen.name("valid")
			cxt.subschema(&subschemaArgs{
				keyword:       "not",
				compositeRule: true,
				createErrors:  &falseValue,
				allErrors:     &falseValue,
			}, valid)
			cxt.failResult(valid, func() { cxt.reset() }, func() { cxt.error(false) })
		},
	}
}

func anyOfKeyword() *keywordDef {
	return &keywordDef{
		keywords:    []string{"anyOf"},
		schemaType:  []string{"array"},
		trackErrors: true,
		code:        func(cxt *keywordCxt, _ string) { validateUnion(cxt) },
		err:         &errorDef{message: constMsg("must match a schema in anyOf")},
	}
}

func oneOfKeyword() *keywordDef {
	return &keywordDef{
		keywords:    []string{"oneOf"},
		schemaType:  []string{"array"},
		trackErrors: true,
		err: &errorDef{
			message: constMsg("must match exactly one schema in oneOf"),
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("passingSchemas", cxt.param("passing"))
			},
		},
		code: oneOfCode,
	}
}

func oneOfCode(cxt *keywordCxt, _ string) {
	gen, parentSchema, it := cxt.gen, cxt.parentSchema, cxt.it
	schArr, ok := cxt.schema.([]any)
	if !ok {
		panic(newError("ajv implementation error"))
	}
	if it.opts.discriminator && truthy(getProp(parentSchema, "discriminator")) {
		return
	}
	valid := gen.let("valid", false)
	passing := gen.let("passing", nil)
	schValid := gen.name("_valid")
	cxt.setParams(map[string]jsCode{"passing": passing}, false)
	gen.block(func() {
		for i, sch := range schArr {
			var schCxt *schemaCxt
			if alwaysValidSchema(it, sch) {
				gen.varDef(schValid, true)
			} else {
				schCxt = cxt.subschema(&subschemaArgs{keyword: "oneOf", schemaProp: i, compositeRule: true}, schValid)
			}
			idx := float64(i)
			if i > 0 {
				gen.ifThen(and(schValid, valid))
				gen.assign(valid, false)
				gen.assign(passing, exprFn(func(f *frame) any { return []any{f.get(passing), idx} }))
				gen.elseBranch()
			}
			gen.ifThen(schValid)
			gen.assign(valid, true)
			gen.assign(passing, idx)
			if schCxt != nil {
				cxt.mergeEvaluated(schCxt, true)
			}
			gen.endIf()
		}
	})
	cxt.result(valid, func() { cxt.reset() }, func() { cxt.error(true) })
}

func allOfKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"allOf"},
		schemaType: []string{"array"},
		code: func(cxt *keywordCxt, _ string) {
			schema, ok := cxt.schema.([]any)
			if !ok {
				panic(newError("ajv implementation error"))
			}
			valid := cxt.gen.name("valid")
			for i, sch := range schema {
				if alwaysValidSchema(cxt.it, sch) {
					continue
				}
				schCxt := cxt.subschema(&subschemaArgs{keyword: "allOf", schemaProp: i}, valid)
				cxt.ok(valid)
				cxt.mergeEvaluated(schCxt, false)
			}
		},
	}
}

func ifKeyword() *keywordDef {
	return &keywordDef{
		keywords:    []string{"if"},
		schemaType:  []string{"object", "boolean"},
		trackErrors: true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit(`must match "`), cxt.param("ifClause"), strLit(`" schema`))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("failingKeyword", cxt.param("ifClause"))
			},
		},
		code: ifCode,
	}
}

func ifCode(cxt *keywordCxt, _ string) {
	gen, parentSchema, it := cxt.gen, cxt.parentSchema, cxt.it
	if isUndefined(getProp(parentSchema, "then")) && isUndefined(getProp(parentSchema, "else")) {
		checkStrictMode(it, `"if" without "then" and "else" is ignored`, it.opts.strictSchema)
	}
	hasSchema := func(keyword string) bool {
		schema := getProp(it.schema, keyword)
		return !isUndefined(schema) && !alwaysValidSchema(it, schema)
	}
	hasThen := hasSchema("then")
	hasElse := hasSchema("else")
	if !hasThen && !hasElse {
		return
	}
	valid := gen.let("valid", true)
	schValid := gen.name("_valid")
	schCxt := cxt.subschema(&subschemaArgs{
		keyword:       "if",
		compositeRule: true,
		createErrors:  &falseValue,
		allErrors:     &falseValue,
	}, schValid)
	cxt.mergeEvaluated(schCxt, false)
	cxt.reset()
	validateClause := func(keyword string, ifClause *jsName) func() {
		return func() {
			sc := cxt.subschema(&subschemaArgs{keyword: keyword}, schValid)
			gen.assign(valid, schValid)
			cxt.mergeValidEvaluated(sc, valid)
			if ifClause != nil {
				gen.assign(ifClause, keyword)
			} else {
				cxt.setParams(map[string]jsCode{"ifClause": keyword}, false)
			}
		}
	}
	switch {
	case hasThen && hasElse:
		ifClause := gen.letNoInit("ifClause")
		cxt.setParams(map[string]jsCode{"ifClause": ifClause}, false)
		gen.ifElse(schValid, validateClause("then", ifClause), validateClause("else", ifClause))
	case hasThen:
		gen.ifElse(schValid, validateClause("then", nil), nil)
	default:
		gen.ifElse(not(schValid), validateClause("else", nil), nil)
	}
	cxt.pass(valid, func() { cxt.error(true) })
}

func thenElseKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"then", "else"},
		schemaType: []string{"object", "boolean"},
		code: func(cxt *keywordCxt, _ string) {
			if isUndefined(getProp(cxt.parentSchema, "if")) {
				checkStrictMode(cxt.it, `"`+cxt.keyword+`" without "if" is ignored`, cxt.it.opts.strictSchema)
			}
		},
	}
}

func limitContainsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"maxContains", "minContains"},
		typ:        []string{"array"},
		schemaType: []string{"number"},
		code: func(cxt *keywordCxt, _ string) {
			if isUndefined(getProp(cxt.parentSchema, "contains")) {
				checkStrictMode(cxt.it, `"`+cxt.keyword+`" without "contains" is ignored`, cxt.it.opts.strictSchema)
			}
		},
	}
}
