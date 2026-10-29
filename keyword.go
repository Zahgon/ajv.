// Ported from Ajv 8.20.0, lib/compile/validate/keyword.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import "strings"

// macroKeywordCode is keyword.macroKeywordCode.
func macroKeywordCode(cxt *keywordCxt, def *keywordDef) {
	it := cxt.it
	macroSchema := def.macro(cxt.schema, cxt.parentSchema)
	if macroSchema == nil || isUndefined(macroSchema) {
		panic(newError(`keyword "` + cxt.keyword + `" failed to compile`))
	}
	if it.opts.validateSchema != false {
		it.self.validateSchemaThrow(macroSchema)
	}
	valid := cxt.gen.name("valid")
	cxt.subschema(&subschemaArgs{
		schema:        macroSchema,
		hasSchema:     true,
		errSchemaPath: it.errSchemaPath + "/" + cxt.keyword,
		compositeRule: true,
	}, valid)
	cxt.pass(valid, func() { cxt.error(true) })
}

// keywordFunc is a compiled or plain validation function of a function keyword together with
// its `errors` property.
type keywordFunc struct {
	fn     func(args []any, cxt DataValidationCxt) (bool, []*ErrorObject)
	errors *errList
}

// funcKeywordCode is keyword.funcKeywordCode.
func funcKeywordCode(cxt *keywordCxt, def *keywordDef) {
	gen := cxt.gen
	kf := &keywordFunc{}
	passSchema := !(def.compile != nil && cxt.dataRef == "")
	if cxt.dataRef == "" && def.compile != nil {
		compiled, err := def.compile(cxt.schema, cxt.parentSchema)
		if err != nil {
			panic(err)
		}
		if compiled == nil {
			panic(newError(`keyword "` + cxt.keyword + `" failed to compile`))
		}
		kf.fn = func(args []any, c DataValidationCxt) (bool, []*ErrorObject) { return compiled(args[0], c) }
	} else {
		validate := def.validate
		if validate == nil {
			panic(newError(`keyword "` + cxt.keyword + `" failed to compile`))
		}
		kf.fn = func(args []any, c DataValidationCxt) (bool, []*ErrorObject) {
			return validate(args[0], args[1], args[2], c)
		}
	}
	valid := gen.letNoInit("valid")
	// When def.valid is set the variable valid is never read, so the code generator's optimizer
	// drops the assignment `valid = validate(...)` and the function is not called at all, unless
	// it modifies the data.
	callable := def.valid == nil || def.modifying
	cxt.blockData(valid, func() {
		if def.errors == false {
			if callable {
				assignValid(cxt, kf, valid, passSchema, false)
			}
			if def.modifying {
				modifyData(cxt)
			}
			reportErrs(cxt, def, valid, func() { cxt.error(false) })
		} else {
			if callable {
				assignValid(cxt, kf, valid, passSchema, true)
			} else {
				gen.code(func(*frame) { kf.errors = nil })
			}
			if def.modifying {
				modifyData(cxt)
			}
			reportErrs(cxt, def, valid, func() { addErrs(cxt, kf) })
		}
	}, nilCode)
	if def.valid != nil {
		cxt.ok(def.valid)
	} else {
		cxt.ok(valid)
	}
}

func assignValid(cxt *keywordCxt, kf *keywordFunc, valid *jsName, passSchema, resetErrors bool) {
	it := cxt.it
	schemaCode, data := cxt.schemaCode, cxt.data
	parentSchema := cxt.parentSchema
	errorPath := it.errorPath
	parentData, parentDataProperty := it.parentData, it.parentDataProperty
	cxt.gen.code(func(f *frame) {
		if resetErrors {
			kf.errors = nil
		}
		vc := DataValidationCxt{
			InstancePath:       instancePathOf(f, errorPath),
			ParentData:         apiValue(f.get(parentData)),
			ParentDataProperty: apiProperty(eval(parentDataProperty, f)),
			RootData:           f.vars[slotRootData],
		}
		var args []any
		if passSchema {
			args = []any{apiValue(eval(schemaCode, f)), f.get(data), parentSchema}
		} else {
			args = []any{f.get(data)}
		}
		ok, errs := kf.fn(args, vc)
		if errs != nil {
			kf.errors = &errList{items: errs}
		} else {
			kf.errors = nil
		}
		f.set(valid, ok)
	})
}

// apiProperty is the parentDataProperty handed to user functions: a string key, an int index,
// or nil at the root.
func apiProperty(v any) any {
	if n, ok := v.(float64); ok {
		return int(n)
	}
	return apiValue(v)
}

// apiValue maps undefined to nil for values handed to user functions.
func apiValue(v any) any {
	if isUndefined(v) {
		return nil
	}
	return v
}

func reportErrs(cxt *keywordCxt, def *keywordDef, valid *jsName, errors func()) {
	if def.valid != nil {
		cxt.gen.ifThen(not(def.valid))
	} else {
		cxt.gen.ifThen(not(valid))
	}
	errors()
	cxt.gen.endIf()
}

func modifyData(cxt *keywordCxt) {
	it, data := cxt.it, cxt.data
	parentData, parentDataProperty := it.parentData, it.parentDataProperty
	cxt.gen.code(func(f *frame) {
		if pd := f.get(parentData); truthy(pd) {
			f.set(data, getProp(pd, eval(parentDataProperty, f)))
		}
	})
}

func addErrs(cxt *keywordCxt, kf *keywordFunc) {
	gen := cxt.gen
	gen.ifThen(exprFn(func(*frame) any { return kf.errors != nil }))
	gen.code(func(f *frame) {
		if l := vErrorsOf(f); l == nil {
			f.vars[slotVErrors] = kf.errors
		} else {
			f.vars[slotVErrors] = l.concat(kf.errors)
		}
		f.vars[slotErrors] = float64(len(vErrorsOf(f).items))
	})
	extendErrors(cxt)
	gen.elseBranch()
	cxt.error(false)
	gen.endIf()
}

// validSchemaType is keyword.validSchemaType.
func validSchemaType(schema any, schemaType []string, allowUndefined bool) bool {
	if len(schemaType) == 0 {
		return true
	}
	for _, st := range schemaType {
		switch st {
		case "array":
			if isArray(schema) {
				return true
			}
		case "object":
			if isPlainObject(schema) {
				return true
			}
		default:
			if typeOf(schema) == st || (allowUndefined && isUndefined(schema)) {
				return true
			}
		}
	}
	return false
}

// validateKeywordUsage is keyword.validateKeywordUsage.
func validateKeywordUsage(it *schemaCxt, def *keywordDef, keyword string) {
	for _, kwd := range def.dependencies {
		if !hasOwnProperty(it.schema, kwd) {
			panic(newError("parent schema must have dependencies of " + keyword + ": " + strings.Join(def.dependencies, ",")))
		}
	}
	if def.validateSchema != nil {
		vs := def.validateSchema
		if !vs.call(getProp(it.schema, keyword), nil) {
			msg := `keyword "` + keyword + `" value is invalid at path "` + it.errSchemaPath + `": ` +
				it.self.errorsText(errorsOf(vs.errors), ", ", "data")
			if it.opts.validateSchema == "log" {
				it.self.logger.Error(msg)
			} else {
				panic(newError(msg))
			}
		}
	}
}
