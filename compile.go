// Ported from Ajv 8.20.0, lib/compile/index.ts and lib/compile/ref_error.ts, MIT,
// Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"strings"

	"github.com/ajv-validator/ajv-go/internal/uri"
)

// schemaEnv is SchemaEnv: a schema with its root, base URI and compiled function.
type schemaEnv struct {
	schema         any
	root           *schemaEnv
	baseID         string
	schemaPath     string
	localRefs      map[string]any
	meta           bool
	async          bool
	refs           map[string]any // *schemaEnv or an inlined schema
	dynamicAnchors map[string]bool
	validate       *ValidateFunction
}

type schemaEnvArgs struct {
	schema    any
	root      *schemaEnv
	baseID    *string
	localRefs map[string]any
	meta      bool
}

func newSchemaEnv(a schemaEnvArgs) *schemaEnv {
	env := &schemaEnv{
		schema:         a.schema,
		root:           a.root,
		localRefs:      a.localRefs,
		meta:           a.meta,
		refs:           map[string]any{},
		dynamicAnchors: map[string]bool{},
	}
	if env.root == nil {
		env.root = env
	}
	if a.baseID != nil {
		env.baseID = *a.baseID
	} else if isPlainObject(a.schema) {
		if id := getProp(a.schema, "$id"); truthy(id) {
			env.baseID = normalizeID(toString(id))
		}
	}
	if isPlainObject(a.schema) {
		env.async = truthy(getProp(a.schema, "$async"))
	}
	return env
}

// evaluatedInfo is the `evaluated` property of a validation function: the properties and items
// it evaluates, statically known or computed by each call.
type evaluatedInfo struct {
	props        any // nil, true or *Object at run time
	items        any // nil, true or float64
	staticProps  any // nil, true or *propSet
	dynamicProps bool
	dynamicItems bool
}

// ValidateFunction is a compiled schema.
type ValidateFunction struct {
	body      *rootNode
	nslots    int
	errors    *errList
	schema    any
	env       *schemaEnv
	evaluated *evaluatedInfo
}

// Validate validates data. Data is modified in place by the options that change it (coercion,
// defaults, removal of additional properties); the root value itself is never replaced.
func (v *ValidateFunction) Validate(data any) bool { return v.call(data, nil) }

// Errors returns the errors of the last validation, nil when there are none (null).
func (v *ValidateFunction) Errors() []*ErrorObject { return errorsOf(v.errors) }

// Schema returns the compiled schema.
func (v *ValidateFunction) Schema() any { return v.schema }

// valCxt is the data context passed between generated functions.
type valCxt struct {
	instancePath       string
	parentData         any
	parentDataProperty any
	rootData           any
	dynamicAnchors     map[string]*ValidateFunction
	depth              int // nesting of the call
}

// call invokes the generated function with a data context (nil when called by the user).
func (v *ValidateFunction) call(data any, vc *valCxt) bool {
	f := &frame{vars: make([]any, v.nslots), fn: v}
	for i := range f.vars {
		f.vars[i] = undefined
	}
	f.vars[slotData] = data
	if vc == nil {
		f.vars[slotInstancePath] = ""
		f.vars[slotRootData] = data
		f.vars[slotDynamicAnchors] = map[string]*ValidateFunction{}
	} else {
		f.vars[slotInstancePath] = vc.instancePath
		f.vars[slotParentData] = vc.parentData
		f.vars[slotParentDataProperty] = vc.parentDataProperty
		f.vars[slotRootData] = vc.rootData
		if isUndefined(vc.rootData) {
			f.vars[slotRootData] = data
		}
		f.vars[slotDynamicAnchors] = vc.dynamicAnchors
		if vc.dynamicAnchors == nil {
			f.vars[slotDynamicAnchors] = map[string]*ValidateFunction{}
		}
	}
	if vc != nil {
		f.depth = vc.depth
		if f.depth > maxCallDepth {
			panic(newError("Maximum call stack size exceeded"))
		}
	}
	v.body.exec(f)
	return truthy(f.ret)
}

// maxCallDepth bounds the nesting of validation function calls: a schema that refers to itself
// without consuming data recurses until JavaScript's call stack overflows.
const maxCallDepth = 10000

// compileSchema is compile.compileSchema.
func (a *Ajv) compileSchema(sch *schemaEnv) *schemaEnv {
	if s := a.getCompilingSchema(sch); s != nil {
		return s
	}
	rootID := getFullPath(sch.root.baseID, true)
	gen := newCodeGen()
	fn := &ValidateFunction{schema: sch.schema, env: sch}
	errSchemaPath := sch.schemaPath
	if errSchemaPath == "" {
		errSchemaPath = "#"
	}
	baseID := sch.baseID
	if baseID == "" {
		baseID = rootID
	}
	it := &schemaCxt{
		gen:                gen,
		allErrors:          a.opts.allErrors,
		data:               names.data,
		parentData:         names.parentData,
		parentDataProperty: names.parentDataProperty,
		dataNames:          []*jsName{names.data},
		dataPathArr:        []jsCode{nilCode},
		dataTypes:          []string{},
		definedProperties:  newStringSet(),
		validateFn:         fn,
		schema:             sch.schema,
		schemaEnv:          sch,
		rootID:             rootID,
		baseID:             baseID,
		errSchemaPath:      errSchemaPath,
		errorPath:          emptyPath,
		opts:               a.opts,
		self:               a,
	}
	a.compilations = append(a.compilations, sch)
	defer a.removeCompilation(sch)
	ok := false
	defer func() {
		if !ok {
			sch.validate = nil
		}
	}()
	validateFunctionCode(it)
	fn.body = gen.root
	fn.nslots = gen.nslots
	if a.opts.unevaluated {
		ev := &evaluatedInfo{}
		if _, dyn := it.props.(*jsName); dyn {
			ev.dynamicProps = true
		} else {
			ev.staticProps = it.props
			ev.props = propsValue(it.props)
		}
		if _, dyn := it.items.(*jsName); dyn {
			ev.dynamicItems = true
		} else {
			ev.items = it.items
		}
		fn.evaluated = ev
	}
	sch.validate = fn
	ok = true
	return sch
}

// propsValue is the run-time value of statically evaluated properties.
func propsValue(props any) any {
	if ps, ok := props.(*propSet); ok {
		o := NewObject()
		for _, k := range ps.keys {
			o.Set(k, true)
		}
		return o
	}
	return props
}

func (a *Ajv) removeCompilation(sch *schemaEnv) {
	for i, s := range a.compilations {
		if s == sch {
			a.compilations = append(a.compilations[:i], a.compilations[i+1:]...)
			return
		}
	}
}

// resolveRef is compile.resolveRef: the schema (to inline) or the compiled schema env a
// reference points to, nil when it cannot be resolved.
func (a *Ajv) resolveRef(root *schemaEnv, baseID, ref string) any {
	ref = resolveURL(baseID, ref)
	if schOrFunc, ok := root.refs[ref]; ok && truthy(schOrFunc) {
		return schOrFunc
	}
	sch := a.resolve(root, ref)
	if sch == nil {
		if schema, ok := root.localRefs[ref]; ok && truthy(schema) {
			sch = newSchemaEnv(schemaEnvArgs{schema: schema, root: root, baseID: &baseID})
		}
	}
	if sch == nil {
		return nil
	}
	res := a.inlineOrCompile(sch)
	root.refs[ref] = res
	return res
}

func (a *Ajv) inlineOrCompile(sch *schemaEnv) any {
	if inlineRef(sch.schema, a.opts.inlineRefs) {
		return sch.schema
	}
	if sch.validate != nil {
		return sch
	}
	return a.compileSchema(sch)
}

func (a *Ajv) getCompilingSchema(env *schemaEnv) *schemaEnv {
	for _, sch := range a.compilations {
		if sameSchema(sch.schema, env.schema) && sch.root == env.root && sch.baseID == env.baseID {
			return sch
		}
	}
	return nil
}

// sameSchema is === on schemas (objects by identity, booleans by value).
func sameSchema(x, y any) bool {
	if b, ok := x.(bool); ok {
		c, ok := y.(bool)
		return ok && b == c
	}
	return strictEquals(x, y)
}

func (a *Ajv) resolve(root *schemaEnv, ref string) *schemaEnv {
	var sch any
	for {
		sch = a.refs[ref]
		s, ok := sch.(string)
		if !ok {
			break
		}
		ref = s
	}
	if env, ok := sch.(*schemaEnv); ok {
		return env
	}
	if env := a.schemas[ref]; env != nil {
		return env
	}
	return a.resolveSchema(root, ref)
}

// resolveSchema is compile.resolveSchema.
func (a *Ajv) resolveSchema(root *schemaEnv, ref string) *schemaEnv {
	a.depth++
	defer func() { a.depth-- }()
	if a.depth > maxSubschemaDepth {
		panic(newError("Maximum call stack size exceeded"))
	}
	p := uri.Parse(ref)
	refPath := getFullPathOf(p)
	baseID := getFullPath(root.baseID, true)
	if len(objectKeys(root.schema)) > 0 && refPath == baseID {
		return a.getJSONPointer(p, root)
	}
	id := normalizeID(refPath)
	var schOrRef any
	if r, ok := a.refs[id]; ok && truthy(r) {
		schOrRef = r
	} else if env := a.schemas[id]; env != nil {
		schOrRef = env
	}
	if s, ok := schOrRef.(string); ok {
		sch := a.resolveSchema(root, s)
		if sch == nil || typeOf(sch.schema) != "object" {
			return nil
		}
		return a.getJSONPointer(p, sch)
	}
	env, _ := schOrRef.(*schemaEnv)
	if env == nil || typeOf(env.schema) != "object" {
		return nil
	}
	if env.validate == nil {
		a.compileSchema(env)
	}
	if id == normalizeID(ref) {
		schema := env.schema
		if schID := getProp(schema, "$id"); truthy(schID) {
			baseID = resolveURL(baseID, toString(schID))
		}
		return newSchemaEnv(schemaEnvArgs{schema: schema, root: root, baseID: &baseID})
	}
	return a.getJSONPointer(p, env)
}

var preventScopeChange = toSet("properties", "patternProperties", "enum", "dependencies", "definitions")

// getJSONPointer is compile.getJsonPointer.
func (a *Ajv) getJSONPointer(parsedRef *uri.Components, env *schemaEnv) *schemaEnv {
	if parsedRef.Fragment == nil || !strings.HasPrefix(*parsedRef.Fragment, "/") {
		return nil
	}
	baseID, schema, root := env.baseID, env.schema, env.root
	for _, part := range strings.Split((*parsedRef.Fragment)[1:], "/") {
		if _, ok := schema.(bool); ok {
			return nil
		}
		partSchema := getProp(schema, unescapeFragment(part))
		if isUndefined(partSchema) {
			return nil
		}
		schema = partSchema
		if isPlainObject(schema) && !preventScopeChange[part] {
			if schID := getProp(schema, "$id"); truthy(schID) {
				baseID = resolveURL(baseID, toString(schID))
			}
		}
	}
	var res *schemaEnv
	if _, ok := schema.(bool); !ok {
		if ref := getProp(schema, "$ref"); truthy(ref) && !schemaHasRulesButRef(schema, a.rules) {
			res = a.resolveSchema(root, resolveURL(baseID, toString(ref)))
		}
	}
	if res == nil {
		res = newSchemaEnv(schemaEnvArgs{schema: schema, root: root, baseID: &baseID})
	}
	if !sameSchema(res.schema, res.root.schema) {
		return res
	}
	return nil
}

// newMissingRefError is the MissingRefError constructor.
func newMissingRefError(baseID, ref string) *MissingRefError {
	missingRef := resolveURL(baseID, ref)
	return &MissingRefError{
		Message:       "can't resolve reference " + ref + " from id " + baseID,
		MissingRef:    missingRef,
		MissingSchema: normalizeID(getFullPath(missingRef, true)),
	}
}
