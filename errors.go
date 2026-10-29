// Ported from Ajv 8.20.0, lib/compile/errors.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import "strings"

// errList is a JavaScript array of errors. Generated code shares these arrays between functions
// (`vErrors = vErrors === null ? v.errors : vErrors.concat(v.errors)`) and truncates them in
// place (`vErrors.length = errsCount`), so they are reference values.
type errList struct{ items []*ErrorObject }

func (l *errList) concat(other *errList) *errList {
	items := make([]*ErrorObject, 0, len(l.items)+len(other.items))
	items = append(items, l.items...)
	items = append(items, other.items...)
	return &errList{items: items}
}

// errorsOf returns the items of a possibly null error array.
func errorsOf(l *errList) []*ErrorObject {
	if l == nil {
		return nil
	}
	if l.items == nil {
		return []*ErrorObject{}
	}
	return l.items
}

// vErrorsOf returns the vErrors variable of a frame (nil when it is null).
func vErrorsOf(f *frame) *errList {
	l, _ := f.vars[slotVErrors].(*errList)
	return l
}

var keywordError = &errorDef{
	message: func(cxt *keywordCxt) func(f *frame) string {
		msg := `must pass "` + cxt.keyword + `" keyword validation`
		return func(*frame) string { return msg }
	},
}

var keywordDataError = &errorDef{
	message: func(cxt *keywordCxt) func(f *frame) string {
		// The schema type is always an array, which is truthy even when empty: the message
		// "keyword is invalid ($data)" of the original is never used.
		msg := `"` + cxt.keyword + `" keyword must be ` + strings.Join(cxt.schemaType, ",") + ` ($data)`
		return func(*frame) string { return msg }
	},
}

// reportError is errors.reportError; overrideAllErrors is nil when not passed.
func reportError(cxt *keywordCxt, e *errorDef, overrideAllErrors *bool) {
	it := cxt.it
	errObj := errorObjectCode(cxt, e)
	add := it.compositeRule || it.allErrors
	if overrideAllErrors != nil {
		add = *overrideAllErrors
	}
	if add {
		addError(it.gen, errObj)
	} else {
		returnErrors(it, func(f *frame) *errList { return &errList{items: []*ErrorObject{errObj(f)}} })
	}
}

// reportExtraError is errors.reportExtraError.
func reportExtraError(cxt *keywordCxt, e *errorDef) {
	it := cxt.it
	errObj := errorObjectCode(cxt, e)
	addError(it.gen, errObj)
	if !(it.compositeRule || it.allErrors) {
		returnErrors(it, vErrorsOf)
	}
}

// resetErrorsCount is errors.resetErrorsCount.
func resetErrorsCount(gen *codeGen, errsCount *jsName) {
	gen.code(func(f *frame) {
		n := f.get(errsCount)
		f.vars[slotErrors] = n
		if l := vErrorsOf(f); l != nil {
			if truthy(n) {
				size := int(toNumber(n))
				if size < len(l.items) {
					l.items = l.items[:size]
				}
			} else {
				f.vars[slotVErrors] = nil
			}
		}
	})
}

// extendErrors is errors.extendErrors.
func extendErrors(cxt *keywordCxt) {
	it := cxt.it
	errsCount := cxt.errsCount
	instPath := it.errorPath
	schemaPath := it.errSchemaPath + "/" + cxt.keyword
	verbose := it.opts.verbose
	schemaValue, data := cxt.schemaValue, cxt.data
	cxt.gen.code(func(f *frame) {
		l := vErrorsOf(f)
		for i := int(f.num(errsCount)); i < int(toNumber(f.vars[slotErrors])); i++ {
			err := l.items[i]
			if err.InstancePath == "" {
				err.InstancePath = instancePathOf(f, instPath)
			}
			err.SchemaPath = schemaPath
			if verbose {
				err.Schema = eval(schemaValue, f)
				err.Data = eval(data, f)
			}
		}
	})
}

func addError(gen *codeGen, errObj func(f *frame) *ErrorObject) {
	gen.code(func(f *frame) {
		err := errObj(f)
		if l := vErrorsOf(f); l == nil {
			f.vars[slotVErrors] = &errList{items: []*ErrorObject{err}}
		} else {
			l.items = append(l.items, err)
		}
		f.vars[slotErrors] = toNumber(f.vars[slotErrors]) + 1
	})
}

func returnErrors(it *schemaCxt, errs func(f *frame) *errList) {
	it.gen.code(func(f *frame) { f.fn.errors = errs(f) })
	it.gen.returnStmt(false)
}

// instancePathOf evaluates strConcat(N.instancePath, path).
func instancePathOf(f *frame, path *pathCode) string {
	base, _ := f.vars[slotInstancePath].(string)
	if path.isEmpty() {
		return base
	}
	return base + path.eval(f)
}

// errorObjectCode returns the runtime constructor of the error object a keyword reports.
func errorObjectCode(cxt *keywordCxt, e *errorDef) func(f *frame) *ErrorObject {
	it := cxt.it
	if it.createErrorsFalse {
		return func(*frame) *ErrorObject { return &ErrorObject{} }
	}
	if e == nil {
		e = keywordError
	}
	instPath := it.errorPath
	schemaPath := it.errSchemaPath + "/" + cxt.keyword
	keyword := cxt.keyword
	var params func(f *frame) *Object
	if e.params != nil {
		params = e.params(cxt)
	}
	var message func(f *frame) string
	if truthy(it.opts.messages) && e.message != nil {
		message = e.message(cxt)
	}
	verbose := it.opts.verbose
	schemaValue, parentSchema, data := cxt.schemaValue, it.schema, cxt.data
	propertyName := it.propertyName
	return func(f *frame) *ErrorObject {
		err := &ErrorObject{
			Keyword:      keyword,
			InstancePath: instancePathOf(f, instPath),
			SchemaPath:   schemaPath,
		}
		if params != nil {
			err.Params = params(f)
		} else {
			err.Params = NewObject()
		}
		if message != nil {
			err.Message = message(f)
		}
		if verbose {
			err.Schema = eval(schemaValue, f)
			err.ParentSchema = parentSchema
			err.Data = eval(data, f)
		}
		if propertyName != nil {
			name := toString(f.get(propertyName))
			err.PropertyName = &name
		}
		return err
	}
}
