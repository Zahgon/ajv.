// Ported from Ajv 8.20.0, lib/vocabularies/format/format.ts and
// lib/vocabularies/discriminator/index.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

// formatKeyword is the format keyword. No formats are ever added to an instance, so a format is
// always unknown.
func formatKeyword() *keywordDef {
	return &keywordDef{
		keywords:   []string{"format"},
		typ:        []string{"number", "string"},
		schemaType: []string{"string"},
		data:       true,
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				return strMsg(strLit(`must match format "`), cxt.schemaCode, strLit(`"`))
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object { return paramsObj("format", cxt.schemaCode) },
		},
		code: func(cxt *keywordCxt, _ string) {
			it := cxt.it
			opts := it.opts
			if !truthy(opts.validateFormats) {
				return
			}
			if cxt.dataRef != "" {
				schemaCode := cxt.schemaCode
				// No format is defined: fDef and format are undefined and fType is "string".
				var unknownFmt jsCode = nilCode
				if opts.strictSchema != false {
					unknownFmt = exprFn(func(f *frame) any {
						if v := eval(schemaCode, f); !truthy(v) {
							return v
						}
						return true
					})
				}
				cxt.failData(or(unknownFmt, undefined))
				return
			}
			msg := `unknown format "` + toString(cxt.schema) + `" ignored in schema at path "` + it.errSchemaPath + `"`
			if opts.strictSchema == false {
				it.self.logger.Warn(msg)
				return
			}
			panic(newError(msg))
		},
	}
}

const (
	discrErrorTag     = "tag"
	discrErrorMapping = "mapping"
)

func discriminatorKeyword() userKeyword {
	def := &keywordDef{
		keywords:   []string{"discriminator"},
		typ:        []string{"object"},
		schemaType: []string{"object"},
		err: &errorDef{
			message: func(cxt *keywordCxt) func(f *frame) string {
				tagName := toString(cxt.param("tagName"))
				msg := `value of tag "` + tagName + `" must be in oneOf`
				if cxt.param("discrError") == discrErrorTag {
					msg = `tag "` + tagName + `" must be string`
				}
				return func(*frame) string { return msg }
			},
			params: func(cxt *keywordCxt) func(f *frame) *Object {
				return paramsObj("error", cxt.param("discrError"), "tag", cxt.param("tagName"), "tagValue", cxt.param("tag"))
			},
		},
		code: discriminatorCode,
	}
	return userKeyword{def: def, typ: def.typ, schemaType: def.schemaType}
}

func discriminatorCode(cxt *keywordCxt, _ string) {
	gen, data, schema, parentSchema, it := cxt.gen, cxt.data, cxt.schema, cxt.parentSchema, cxt.it
	oneOf := getProp(parentSchema, "oneOf")
	if !it.opts.discriminator {
		panic(newError("discriminator: requires discriminator option"))
	}
	tagName, ok := getProp(schema, "propertyName").(string)
	if !ok {
		panic(newError("discriminator: requires propertyName"))
	}
	if truthy(getProp(schema, "mapping")) {
		panic(newError("discriminator: mapping is not supported"))
	}
	if !truthy(oneOf) {
		panic(newError("discriminator: requires oneOf keyword"))
	}
	valid := gen.let("valid", false)
	tag := gen.constDef("tag", exprFn(func(f *frame) any { return getProp(f.get(data), tagName) }))

	hasRequired := func(sch any) bool {
		req, ok := getProp(sch, "required").([]any)
		if !ok {
			return false
		}
		for _, r := range req {
			if r == tagName {
				return true
			}
		}
		return false
	}
	getMapping := func() *Object {
		oneOfMapping := NewObject()
		topRequired := hasRequired(parentSchema)
		tagRequired := true
		addMapping := func(tagValue any, i int) {
			s, ok := tagValue.(string)
			if !ok || oneOfMapping.has(s) || inObjectProto(s) {
				panic(newError(`discriminator: "` + tagName + `" values must be unique strings`))
			}
			oneOfMapping.Set(s, float64(i))
		}
		n := int(toNumber(getProp(oneOf, "length")))
		for i := 0; i < n; i++ {
			sch := getProp(oneOf, float64(i))
			if ref := getProp(sch, "$ref"); truthy(sch) && truthy(ref) && !schemaHasRulesButRef(sch, it.self.rules) {
				r := toString(ref)
				res := it.self.resolveRef(it.schemaEnv.root, it.baseID, r)
				if env, ok := res.(*schemaEnv); ok {
					res = env.schema
				}
				if res == nil {
					panic(newMissingRefError(it.baseID, r))
				}
				sch = res
			}
			propSch := getProp(getProp(sch, "properties"), tagName)
			if typeOf(propSch) != "object" {
				panic(newError(`discriminator: oneOf subschemas (or referenced schemas) must have "properties/` + tagName + `"`))
			}
			tagRequired = tagRequired && (topRequired || hasRequired(sch))
			if c := getProp(propSch, "const"); truthy(c) {
				addMapping(c, i)
			} else if e := getProp(propSch, "enum"); truthy(e) {
				for _, tagValue := range iterate(e) {
					addMapping(tagValue, i)
				}
			} else {
				panic(newError(`discriminator: "properties/` + tagName + `" must have "const" or "enum"`))
			}
		}
		if !tagRequired {
			panic(newError(`discriminator: "` + tagName + `" must be required`))
		}
		return oneOfMapping
	}
	gen.ifThen(exprFn(func(f *frame) any { return typeOf(f.get(tag)) == "string" }))
	mapping := getMapping()
	gen.ifThen(false)
	for _, tagValue := range mapping.Keys() {
		tv := tagValue
		gen.elseIf(exprFn(func(f *frame) any { return f.get(tag) == tv }))
		v := gen.name("valid")
		i, _ := mapping.Get(tv)
		schCxt := cxt.subschema(&subschemaArgs{keyword: "oneOf", schemaProp: int(i.(float64))}, v)
		cxt.mergeEvaluated(schCxt, true)
		gen.assign(valid, v)
	}
	gen.elseBranch()
	cxt.errorWithParams(false, map[string]jsCode{"discrError": discrErrorMapping, "tag": tag, "tagName": tagName})
	gen.endIf()
	gen.elseBranch()
	cxt.errorWithParams(false, map[string]jsCode{"discrError": discrErrorTag, "tag": tag, "tagName": tagName})
	gen.endIf()
	cxt.ok(valid)
}
