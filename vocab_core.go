// Ported from Ajv 8.20.0, lib/vocabularies/core and lib/vocabularies/dynamic, MIT,
// Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

func coreVocabulary() []any {
	return []any{
		"$schema",
		"$id",
		"$defs",
		"$vocabulary",
		&keywordDef{keywords: []string{"$comment"}},
		"definitions",
		&keywordDef{
			keywords: []string{"id"},
			code: func(*keywordCxt, string) {
				panic(newError(`NOT SUPPORTED: keyword "id", use "$id" for schema ID`))
			},
		},
		refKeyword(),
	}
}

func refKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"$ref"},
		schemaType: []string{"string"},
		code:       refCode,
	}
}

func refCode(cxt *keywordCxt, _ string) {
	ref := cxt.schema.(string)
	it := cxt.it
	baseID, env, self := it.baseID, it.schemaEnv, it.self
	root := env.root
	if (ref == "#" || ref == "#/") && baseID == root.baseID {
		if env == root {
			fn := it.validateFn
			callRef(cxt, func(*frame) *ValidateFunction { return fn }, env)
			return
		}
		callRef(cxt, func(*frame) *ValidateFunction { return root.validate }, root)
		return
	}
	schOrEnv := self.resolveRef(root, baseID, ref)
	if schOrEnv == nil {
		panic(newMissingRefError(baseID, ref))
	}
	if sch, ok := schOrEnv.(*schemaEnv); ok {
		callRef(cxt, getValidate(sch), sch)
		return
	}
	valid := cxt.gen.name("valid")
	schCxt := cxt.subschema(&subschemaArgs{
		schema:        schOrEnv,
		hasSchema:     true,
		dataTypes:     []string{},
		errSchemaPath: ref,
	}, valid)
	cxt.mergeEvaluated(schCxt, false)
	cxt.ok(valid)
}

// getValidate is ref.getValidate: the compiled function of sch, read when it runs if it is
// still being compiled.
func getValidate(sch *schemaEnv) func(f *frame) *ValidateFunction {
	if v := sch.validate; v != nil {
		return func(*frame) *ValidateFunction { return v }
	}
	return func(*frame) *ValidateFunction { return sch.validate }
}

// callRef is ref.callRef for synchronous schemas; sch is nil when unknown.
func callRef(cxt *keywordCxt, v func(f *frame) *ValidateFunction, sch *schemaEnv) {
	gen, it := cxt.gen, cxt.it
	if sch != nil && sch.async {
		if !it.schemaEnv.async {
			panic(newError("async schema referenced by sync schema"))
		}
	}
	call := callValidateCode(cxt, v)
	addErrorsFrom := func() {
		gen.code(func(f *frame) {
			errs := v(f).errors
			if l := vErrorsOf(f); l == nil {
				f.vars[slotVErrors] = errs
			} else {
				if errs == nil {
					errs = &errList{items: []*ErrorObject{nil}}
				}
				f.vars[slotVErrors] = l.concat(errs)
			}
			if l := vErrorsOf(f); l != nil {
				f.vars[slotErrors] = float64(len(l.items))
			} else {
				f.vars[slotErrors] = undefined
			}
		})
	}
	addEvaluatedFrom := func() {
		if !it.opts.unevaluated {
			return
		}
		var schEvaluated *evaluatedInfo
		if sch != nil && sch.validate != nil {
			schEvaluated = sch.validate.evaluated
		}
		if it.props != true {
			if schEvaluated != nil && !schEvaluated.dynamicProps {
				if schEvaluated.staticProps != nil {
					it.props = mergeEvaluatedProps(gen, schEvaluated.staticProps, it.props, false)
				}
			} else {
				props := gen.varDef("props", exprFn(func(f *frame) any { return evaluatedProps(v(f)) }))
				it.props = mergeEvaluatedProps(gen, props, it.props, true)
			}
		}
		if it.items != true {
			if schEvaluated != nil && !schEvaluated.dynamicItems {
				if schEvaluated.items != nil {
					it.items = mergeEvaluatedItems(gen, schEvaluated.items, it.items, false)
				}
			} else {
				items := gen.varDef("items", exprFn(func(f *frame) any { return evaluatedItems(v(f)) }))
				it.items = mergeEvaluatedItems(gen, items, it.items, true)
			}
		}
	}
	cxt.result(exprFn(func(f *frame) any { return call(f) }), addEvaluatedFrom, addErrorsFrom)
}

func evaluatedProps(v *ValidateFunction) any {
	if v.evaluated == nil || v.evaluated.props == nil {
		return undefined
	}
	return v.evaluated.props
}

func evaluatedItems(v *ValidateFunction) any {
	if v.evaluated == nil || v.evaluated.items == nil {
		return undefined
	}
	return v.evaluated.items
}

func dynamicVocabulary() []any {
	return []any{
		&keywordDef{
			keywords:   []string{"$dynamicAnchor"},
			schemaType: []string{"string"},
			code:       func(cxt *keywordCxt, _ string) { dynamicAnchor(cxt, cxt.schema.(string)) },
		},
		&keywordDef{
			keywords:   []string{"$dynamicRef"},
			schemaType: []string{"string"},
			code:       func(cxt *keywordCxt, _ string) { dynamicRef(cxt, cxt.schema.(string)) },
		},
		&keywordDef{
			keywords:   []string{"$recursiveAnchor"},
			schemaType: []string{"boolean"},
			code: func(cxt *keywordCxt, _ string) {
				if cxt.schema == true {
					dynamicAnchor(cxt, "")
				} else {
					checkStrictMode(cxt.it, "$recursiveAnchor: false is ignored", cxt.it.opts.strictSchema)
				}
			},
		},
		&keywordDef{
			keywords:   []string{"$recursiveRef"},
			schemaType: []string{"string"},
			code:       func(cxt *keywordCxt, _ string) { dynamicRef(cxt, cxt.schema.(string)) },
		},
	}
}

func anchorsOf(f *frame) map[string]*ValidateFunction {
	m, _ := f.vars[slotDynamicAnchors].(map[string]*ValidateFunction)
	return m
}

// dynamicAnchor is dynamicAnchor.dynamicAnchor.
func dynamicAnchor(cxt *keywordCxt, anchor string) {
	it := cxt.it
	it.schemaEnv.root.dynamicAnchors[anchor] = true
	var validate func(f *frame) *ValidateFunction
	if it.errSchemaPath == "#" {
		fn := it.validateFn
		validate = func(*frame) *ValidateFunction { return fn }
	} else {
		root := it.schemaEnv.root
		baseID := root.baseID
		sch := newSchemaEnv(schemaEnvArgs{schema: it.schema, root: root.root, baseID: &baseID, localRefs: root.localRefs, meta: root.meta})
		it.self.compileSchema(sch)
		validate = getValidate(sch)
	}
	cxt.gen.code(func(f *frame) {
		anchors := anchorsOf(f)
		if anchors[anchor] == nil {
			anchors[anchor] = validate(f)
		}
	})
}

// dynamicRef is dynamicRef.dynamicRef.
func dynamicRef(cxt *keywordCxt, ref string) {
	gen, it := cxt.gen, cxt.it
	if ref == "" || ref[0] != '#' {
		panic(newError(`"` + cxt.keyword + `" only supports hash fragment reference`))
	}
	anchor := ref[1:]
	self := it.validateFn
	callSelf := func(*frame) *ValidateFunction { return self }
	doRef := func() {
		if it.schemaEnv.root.dynamicAnchors[anchor] {
			v := gen.let("_v", exprFn(func(f *frame) any {
				if fn := anchorsOf(f)[anchor]; fn != nil {
					return fn
				}
				return undefined
			}))
			gen.ifThen(v)
			callRefInBlock(cxt, func(f *frame) *ValidateFunction { return f.get(v).(*ValidateFunction) }, !it.allErrors)
			gen.elseBranch()
			callRefInBlock(cxt, callSelf, !it.allErrors)
			gen.endIf()
		} else {
			callRefInBlock(cxt, callSelf, !it.allErrors)
		}
	}
	if it.allErrors {
		doRef()
		return
	}
	valid := gen.let("valid", false)
	doRef()
	// The generated code declares `let valid = true` inside the block of each call, which
	// shadows valid: the variable tested here is never set to true.
	cxt.ok(valid)
}

func callRefInBlock(cxt *keywordCxt, v func(f *frame) *ValidateFunction, block bool) {
	if !block {
		callRef(cxt, v, nil)
		return
	}
	cxt.gen.block(func() { callRef(cxt, v, nil) })
}
