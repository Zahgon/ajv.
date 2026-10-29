// Ported from Ajv 8.20.0, lib/vocabularies/unevaluated, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

func unevaluatedVocabulary() []any {
	return []any{unevaluatedPropertiesKeyword(), unevaluatedItemsKeyword()}
}

func unevaluatedPropertiesKeyword() *keywordDef {
	return &keywordDef{
		keywords:    []string{"unevaluatedProperties"},
		typ:         []string{"object"},
		schemaType:  []string{"boolean", "object"},
		trackErrors: true,
		err: &errorDef{
			message: constMsg("must NOT have unevaluated properties"),
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("unevaluatedProperty", cxt.param("unevaluatedProperty"))
			},
		},
		code: unevaluatedPropertiesCode,
	}
}

func unevaluatedPropertiesCode(cxt *keywordCxt, _ string) {
	gen, schema, data, errsCount, it := cxt.gen, cxt.schema, cxt.data, cxt.errsCount, cxt.it
	if errsCount == nil {
		panic(newError("ajv implementation error"))
	}
	allErrors, props := it.allErrors, it.props
	unevaluatedPropCode := func(key *jsName) {
		if schema == false {
			cxt.setParams(map[string]jsCode{"unevaluatedProperty": key}, false)
			cxt.error(false)
			if !allErrors {
				gen.breakStmt(nil)
			}
			return
		}
		if !alwaysValidSchema(it, schema) {
			valid := gen.name("valid")
			cxt.subschema(&subschemaArgs{keyword: "unevaluatedProperties", dataProp: key, dataPropType: typeStr}, valid)
			if !allErrors {
				gen.ifThen(not(valid))
				gen.breakStmt(nil)
				gen.endIf()
			}
		}
	}
	if pn, ok := props.(*jsName); ok {
		gen.ifThen(exprFn(func(f *frame) any { return f.get(pn) != true }))
		gen.forIn("key", data, it.opts.ownProperties, func(key *jsName) {
			gen.ifThen(exprFn(func(f *frame) any {
				p := f.get(pn)
				return !truthy(p) || !truthy(getProp(p, f.get(key)))
			}))
			unevaluatedPropCode(key)
			gen.endIf()
		})
		gen.endIf()
	} else if props != true {
		gen.forIn("key", data, it.opts.ownProperties, func(key *jsName) {
			if props == nil {
				unevaluatedPropCode(key)
				return
			}
			ps := props.(*propSet)
			gen.ifThen(exprFn(func(f *frame) any {
				k := f.get(key)
				for _, p := range ps.keys {
					if k == p {
						return false
					}
				}
				return true
			}))
			unevaluatedPropCode(key)
			gen.endIf()
		})
	}
	it.props = true
	cxt.ok(errorsEqual(errsCount))
}

func unevaluatedItemsKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"unevaluatedItems"},
		typ:        []string{"array"},
		schemaType: []string{"boolean", "object"},
		err:        itemsLimitError(),
		code: func(cxt *keywordCxt, _ string) {
			gen, schema, data, it := cxt.gen, cxt.schema, cxt.data, cxt.it
			var items jsCode = it.items
			if items == nil {
				items = 0.0
			}
			if items == true {
				return
			}
			length := gen.constDef("len", lenOf(data))
			if schema == false {
				cxt.setParams(map[string]jsCode{"len": items}, false)
				cxt.fail(exprFn(func(f *frame) any { return compare(f.get(length), opGT, eval(items, f)) }))
			} else if typeOf(schema) == "object" && !alwaysValidSchema(it, schema) {
				valid := gen.varDef("valid", exprFn(func(f *frame) any { return compare(f.get(length), opLTE, eval(items, f)) }))
				gen.ifThen(not(valid))
				gen.forRange("i", items, length, func(i *jsName) {
					cxt.subschema(&subschemaArgs{keyword: "unevaluatedItems", dataProp: i, dataPropType: typeNum}, valid)
					if !it.allErrors {
						gen.ifThen(not(valid))
						gen.breakStmt(nil)
						gen.endIf()
					}
				})
				gen.endIf()
				cxt.ok(valid)
			}
			it.items = true
		},
	}
}
