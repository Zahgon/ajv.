// Ported from Ajv 8.20.0, lib/core.ts, lib/ajv.ts, lib/2019.ts and lib/2020.ts, MIT,
// Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
)

// jsError is a JavaScript Error thrown by Ajv: its Error() is the message.
type jsError struct{ message string }

func (e *jsError) Error() string { return e.message }

func newError(msg string) error { return &jsError{message: msg} }

// catch recovers the exception a JavaScript call would throw and stores it in *err. Ajv's
// compilation reports errors by throwing from deep inside code generation; the port mirrors
// that control flow with panics carrying error values, which every exported method converts
// back to a returned error.
func catch(err *error) {
	r := recover()
	if r == nil {
		return
	}
	e, ok := r.(error)
	if _, isRuntime := r.(runtime.Error); !ok || isRuntime {
		panic(r)
	}
	*err = e
}

// instanceOptions are the instance options after defaults are applied (InstanceOptions).
type instanceOptions struct {
	strictSchema            any
	strictNumbers           any
	strictTypes             any
	strictTuples            any
	strictRequired          any
	allowMatchingProperties bool
	allowUnionTypes         bool
	validateFormats         any
	data                    bool
	allErrors               bool
	verbose                 bool
	discriminator           bool
	unicodeRegExp           any
	messages                any
	ownProperties           bool
	multipleOfPrecision     int
	coerceTypes             any
	useDefaults             any
	removeAdditional        any
	inlineRefs              any
	validateSchema          any
	addUsedSchema           any
	loadSchema              func(uri string) (any, error)
	next                    bool
	unevaluated             bool
	dynamicRef              bool
	defaultMeta             any
	loopRequired            float64
	loopEnum                float64
}

// jsOption maps an option value to its JavaScript value (Go integers become numbers).
func jsOption(v any) any {
	if n, ok := asNumber(v); ok {
		return n
	}
	return v
}

func coalesce(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return jsOption(v)
		}
	}
	return nil
}

func requiredOptions(o Options, draft2019 bool) *instanceOptions {
	s := o.Strict
	return &instanceOptions{
		strictSchema:            coalesce(o.StrictSchema, s, true),
		strictNumbers:           coalesce(o.StrictNumbers, s, true),
		strictTypes:             coalesce(o.StrictTypes, s, "log"),
		strictTuples:            coalesce(o.StrictTuples, s, "log"),
		strictRequired:          coalesce(o.StrictRequired, s, false),
		allowMatchingProperties: o.AllowMatchingProperties,
		allowUnionTypes:         o.AllowUnionTypes,
		validateFormats:         coalesce(o.ValidateFormats, true),
		data:                    o.Data,
		allErrors:               o.AllErrors,
		verbose:                 o.Verbose,
		discriminator:           o.Discriminator,
		unicodeRegExp:           coalesce(o.UnicodeRegExp, true),
		messages:                coalesce(o.Messages, true),
		ownProperties:           o.OwnProperties,
		multipleOfPrecision:     o.MultipleOfPrecision,
		coerceTypes:             jsOption(o.CoerceTypes),
		useDefaults:             jsOption(o.UseDefaults),
		removeAdditional:        jsOption(o.RemoveAdditional),
		inlineRefs:              coalesce(o.InlineRefs, true),
		validateSchema:          coalesce(o.ValidateSchema, true),
		addUsedSchema:           coalesce(o.AddUsedSchema, true),
		loadSchema:              o.LoadSchema,
		next:                    draft2019,
		unevaluated:             draft2019,
		dynamicRef:              draft2019,
		loopRequired:            200,
		loopEnum:                200,
	}
}

// draft identifies the Ajv class.
type draft int

const (
	draft07 draft = iota
	draft2019
	draft2020
)

// Ajv is a validator instance (an instance of the Ajv, Ajv2019 or Ajv2020 class).
type Ajv struct {
	draft        draft
	opts         *instanceOptions
	metaOpts     *instanceOptions
	errors       *errList
	logger       Logger
	schemas      map[string]*schemaEnv
	refs         map[string]any // *schemaEnv or string
	rules        *validationRules
	compilations []*schemaEnv
	cache        map[any]*schemaEnv
	arrayCache   []*schemaEnv
	loading      map[string]bool
	depth        int // nesting of subschemas being generated
}

// New returns a draft-07 instance (new Ajv(opts)).
func New(opts Options) *Ajv { return newAjv(draft07, opts) }

// New2019 returns a draft 2019-09 instance (new Ajv2019(opts)).
func New2019(opts Options) *Ajv { return newAjv(draft2019, opts) }

// New2020 returns a draft 2020-12 instance (new Ajv2020(opts)).
func New2020(opts Options) *Ajv { return newAjv(draft2020, opts) }

func newAjv(d draft, opts Options) *Ajv {
	a := &Ajv{
		draft:   d,
		opts:    requiredOptions(opts, d != draft07),
		schemas: map[string]*schemaEnv{},
		refs:    map[string]any{},
		cache:   map[any]*schemaEnv{},
		loading: map[string]bool{},
	}
	a.logger = opts.Logger
	if a.logger == nil {
		a.logger = consoleLogger{}
	}
	formatOpt := a.opts.validateFormats
	a.opts.validateFormats = false
	a.rules = getRules()
	meta := *a.opts
	meta.removeAdditional, meta.useDefaults, meta.coerceTypes = nil, nil, nil
	a.metaOpts = &meta
	a.mustRun(func() {
		a.addVocabularies()
		a.addDefaultMetaSchema()
	})
	a.opts.validateFormats = formatOpt
	return a
}

// mustRun runs the initialization of an instance, which cannot fail.
func (a *Ajv) mustRun(fn func()) {
	var err error
	func() {
		defer catch(&err)
		fn()
	}()
	if err != nil {
		panic("ajv: instance initialization failed: " + err.Error())
	}
}

func (a *Ajv) addVocabularies() {
	a.addKeywordName("$async")
	switch a.draft {
	case draft07:
		for _, v := range draft7Vocabularies() {
			a.addVocabulary(v)
		}
	case draft2019:
		a.addVocabulary(dynamicVocabulary())
		for _, v := range draft7Vocabularies() {
			a.addVocabulary(v)
		}
		a.addVocabulary(nextVocabulary())
		a.addVocabulary(unevaluatedVocabulary())
	case draft2020:
		for _, v := range draft2020Vocabularies() {
			a.addVocabulary(v)
		}
	}
	if a.opts.discriminator {
		a.addKeywordDef(discriminatorKeyword())
	}
}

func (a *Ajv) addDefaultMetaSchema() {
	if a.opts.data {
		ds := dataRefSchema()
		a.addMetaSchema(ds, toString(getProp(ds, "$id")), false)
	}
	switch a.draft {
	case draft07:
		metaSchema := draft7MetaSchema()
		if a.opts.data {
			metaSchema = a.dataMetaSchema(metaSchema, []string{"/properties"})
		}
		a.addMetaSchema(metaSchema, draft7MetaSchemaID, false)
		a.refs["http://json-schema.org/schema"] = draft7MetaSchemaID
	case draft2019:
		a.addMetaSchemas(metaSchemas2019(), map[int]bool{4: true, 6: true})
		a.refs["http://json-schema.org/schema"] = draft2019MetaSchemaID
	case draft2020:
		a.addMetaSchemas(metaSchemas2020(), map[int]bool{5: true, 7: true})
		a.refs["http://json-schema.org/schema"] = draft2020MetaSchemaID
	}
}

func (a *Ajv) addMetaSchemas(schemas []any, withData map[int]bool) {
	for i, sch := range schemas {
		if a.opts.data && withData[i] {
			sch = a.dataMetaSchema(sch, []string{"/properties"})
		}
		a.addMetaSchema(sch, "", false)
	}
}

func (a *Ajv) metaSchemaID() string {
	switch a.draft {
	case draft2019:
		return draft2019MetaSchemaID
	case draft2020:
		return draft2020MetaSchemaID
	}
	return draft7MetaSchemaID
}

// defaultMeta is Ajv.defaultMeta of the instance's class.
func (a *Ajv) defaultMeta() any {
	a.opts.defaultMeta = nil
	id := a.metaSchemaID()
	if a.getSchema(id) != nil {
		a.opts.defaultMeta = id
	}
	return a.opts.defaultMeta
}

// Validate validates data against a schema, or against the schema with the given key or
// reference when schemaOrRef is a string (ajv.validate).
func (a *Ajv) Validate(schemaOrRef any, data any) (valid bool, err error) {
	defer catch(&err)
	return a.validate(schemaOrRef, data), nil
}

func (a *Ajv) validate(schemaKeyRef any, data any) bool {
	var v *ValidateFunction
	if s, ok := schemaKeyRef.(string); ok {
		v = a.getSchema(s)
		if v == nil {
			panic(newError(`no schema with key or ref "` + s + `"`))
		}
	} else {
		v = a.compile(schemaKeyRef, false)
	}
	valid := v.call(data, nil)
	a.errors = v.errors
	return valid
}

// Errors returns the errors of the instance's last validation, nil when there are none.
func (a *Ajv) Errors() []*ErrorObject { return errorsOf(a.errors) }

// Compile compiles a schema (ajv.compile).
func (a *Ajv) Compile(schema any) (v *ValidateFunction, err error) {
	defer catch(&err)
	return a.compile(schema, false), nil
}

func (a *Ajv) compile(schema any, meta bool) *ValidateFunction {
	sch := a.addSchemaEnv(schema, meta, "", a.opts.validateSchema, a.opts.addUsedSchema)
	if sch.validate != nil {
		return sch.validate
	}
	return a.compileSchemaEnv(sch)
}

// CompileAsync compiles a schema, loading the schemas it references that are missing with the
// LoadSchema option (ajv.compileAsync, with LoadSchema called synchronously).
func (a *Ajv) CompileAsync(schema any) (v *ValidateFunction, err error) {
	defer catch(&err)
	if a.opts.loadSchema == nil {
		panic(newError("options.loadSchema should be a function"))
	}
	return a.runCompileAsync(schema, false), nil
}

func (a *Ajv) runCompileAsync(schema any, meta bool) *ValidateFunction {
	a.loadMetaSchema(getProp(schema, "$schema"))
	sch := a.addSchemaEnv(schema, meta, "", a.opts.validateSchema, a.opts.addUsedSchema)
	if sch.validate != nil {
		return sch.validate
	}
	return a.compileAsyncEnv(sch)
}

func (a *Ajv) loadMetaSchema(ref any) {
	if truthy(ref) && a.getSchema(toString(ref)) == nil {
		a.runCompileAsync(newObjectKV("$ref", ref), true)
	}
}

func (a *Ajv) compileAsyncEnv(sch *schemaEnv) *ValidateFunction {
	for {
		v, mre := a.tryCompileSchemaEnv(sch)
		if mre == nil {
			return v
		}
		if _, loaded := a.refs[mre.MissingSchema]; loaded && truthy(a.refs[mre.MissingSchema]) {
			panic(newError("AnySchema " + mre.MissingSchema + " is loaded but " + mre.MissingRef + " cannot be resolved"))
		}
		a.loadMissingSchema(mre.MissingSchema)
	}
}

// tryCompileSchemaEnv compiles sch, returning the MissingRefError it throws.
func (a *Ajv) tryCompileSchemaEnv(sch *schemaEnv) (v *ValidateFunction, mre *MissingRefError) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*MissingRefError)
			if !ok {
				panic(r)
			}
			mre = e
		}
	}()
	return a.compileSchemaEnv(sch), nil
}

func (a *Ajv) loadMissingSchema(ref string) {
	schema, err := a.opts.loadSchema(ref)
	if err != nil {
		panic(err)
	}
	if !truthy(a.refs[ref]) {
		a.loadMetaSchema(getProp(schema, "$schema"))
	}
	if !truthy(a.refs[ref]) {
		a.addSchema(schema, ref, false, a.opts.validateSchema)
	}
}

// AddSchema adds a schema under a key (ajv.addSchema); an empty key is undefined. A []any
// adds each of its schemas, ignoring the key.
func (a *Ajv) AddSchema(schema any, key string) (err error) {
	defer catch(&err)
	a.addSchema(schema, key, false, a.opts.validateSchema)
	return nil
}

func (a *Ajv) addSchema(schema any, key string, meta bool, validateSchema any) {
	if arr, ok := schema.([]any); ok {
		for _, sch := range arr {
			a.addSchema(sch, "", meta, validateSchema)
		}
		return
	}
	id := undefined
	if typeOf(schema) == "object" {
		if schema == nil {
			panic(newError("Cannot read properties of null (reading '$id')"))
		}
		id = getProp(schema, "$id")
		if !isUndefined(id) {
			if _, ok := id.(string); !ok {
				panic(newError("schema $id must be string"))
			}
		}
	}
	if key == "" && truthy(id) {
		key = toString(id)
	}
	key = normalizeID(key)
	a.checkUnique(key)
	a.schemas[key] = a.addSchemaEnv(schema, meta, key, validateSchema, true)
}

// AddMetaSchema adds a meta-schema (ajv.addMetaSchema).
func (a *Ajv) AddMetaSchema(schema any, key string) (err error) {
	defer catch(&err)
	a.addMetaSchema(schema, key, a.opts.validateSchema)
	return nil
}

func (a *Ajv) addMetaSchema(schema any, key string, validateSchema any) {
	a.addSchema(schema, key, true, validateSchema)
}

// ValidateSchema validates a schema against its meta-schema (ajv.validateSchema).
func (a *Ajv) ValidateSchema(schema any) (valid bool, err error) {
	defer catch(&err)
	return a.validateSchema(schema, false), nil
}

// validateSchemaThrow is validateSchema(schema, true).
func (a *Ajv) validateSchemaThrow(schema any) { a.validateSchema(schema, true) }

func (a *Ajv) validateSchema(schema any, throwOrLogError bool) bool {
	if _, ok := schema.(bool); ok {
		return true
	}
	if schema == nil {
		panic(newError("Cannot read properties of null (reading '$schema')"))
	}
	schemaRef := getProp(schema, "$schema")
	if !isUndefined(schemaRef) {
		if _, ok := schemaRef.(string); !ok {
			panic(newError("$schema must be a string"))
		}
	}
	if !truthy(schemaRef) {
		schemaRef = a.opts.defaultMeta
		if !truthy(schemaRef) {
			schemaRef = a.defaultMeta()
		}
	}
	if !truthy(schemaRef) {
		a.logger.Warn("meta-schema not available")
		a.errors = nil
		return true
	}
	valid := a.validate(schemaRef, schema)
	if !valid && throwOrLogError {
		message := "schema is invalid: " + a.errorsText(errorsOf(a.errors), ", ", "data")
		if a.opts.validateSchema == "log" {
			a.logger.Error(message)
		} else {
			panic(newError(message))
		}
	}
	return valid
}

// GetSchema returns the compiled schema with the given key or reference (ajv.getSchema); it
// returns nil, nil when there is none.
func (a *Ajv) GetSchema(keyRef string) (v *ValidateFunction, err error) {
	defer catch(&err)
	return a.getSchema(keyRef), nil
}

func (a *Ajv) getSchema(keyRef string) *ValidateFunction {
	var sch any
	for {
		sch = a.getSchEnv(keyRef)
		s, ok := sch.(string)
		if !ok {
			break
		}
		keyRef = s
	}
	env, _ := sch.(*schemaEnv)
	if env == nil {
		root := newSchemaEnv(schemaEnvArgs{schema: NewObject()})
		env = a.resolveSchema(root, keyRef)
		if env == nil {
			return nil
		}
		a.refs[keyRef] = env
	}
	if env.validate != nil {
		return env.validate
	}
	return a.compileSchemaEnv(env)
}

func (a *Ajv) getSchEnv(keyRef string) any {
	keyRef = normalizeID(keyRef)
	if env := a.schemas[keyRef]; env != nil {
		return env
	}
	if r := a.refs[keyRef]; truthy(r) {
		return r
	}
	return nil
}

// RemoveSchema removes schemas (ajv.removeSchema): nil removes all of them except the
// meta-schemas, a string the schema with that key or reference, and an object or array the
// schema itself.
func (a *Ajv) RemoveSchema(schemaKeyRef any) (err error) {
	defer catch(&err)
	switch x := schemaKeyRef.(type) {
	case nil:
		removeAllSchemas(a, a.schemas)
		removeAllRefs(a)
		a.cache = map[any]*schemaEnv{}
		a.arrayCache = nil
	case string:
		if env, ok := a.getSchEnv(x).(*schemaEnv); ok {
			a.cacheDelete(env.schema)
		}
		delete(a.schemas, x)
		delete(a.refs, x)
	case *Object, []any:
		a.cacheDelete(x)
		if id := getProp(x, "$id"); truthy(id) {
			key := normalizeID(toString(id))
			delete(a.schemas, key)
			delete(a.refs, key)
		}
	default:
		panic(newError("ajv.removeSchema: invalid parameter"))
	}
	return nil
}

func removeAllSchemas(a *Ajv, schemas map[string]*schemaEnv) {
	for key, sch := range schemas {
		if sch != nil && !sch.meta {
			a.cacheDelete(sch.schema)
			delete(schemas, key)
		}
	}
}

func removeAllRefs(a *Ajv) {
	for key, r := range a.refs {
		switch sch := r.(type) {
		case string:
			delete(a.refs, key)
		case *schemaEnv:
			if !sch.meta {
				a.cacheDelete(sch.schema)
				delete(a.refs, key)
			}
		}
	}
}

func (a *Ajv) cacheGet(schema any) *schemaEnv {
	if arr, ok := schema.([]any); ok {
		for _, env := range a.arrayCache {
			if strictEquals(env.schema, arr) {
				return env
			}
		}
		return nil
	}
	return a.cache[schema]
}

func (a *Ajv) cacheSet(env *schemaEnv) {
	if _, ok := env.schema.([]any); ok {
		a.arrayCache = append(a.arrayCache, env)
		return
	}
	a.cache[env.schema] = env
}

func (a *Ajv) cacheDelete(schema any) {
	if arr, ok := schema.([]any); ok {
		for i, env := range a.arrayCache {
			if strictEquals(env.schema, arr) {
				a.arrayCache = append(a.arrayCache[:i], a.arrayCache[i+1:]...)
				return
			}
		}
		return
	}
	switch schema.(type) {
	case *Object, bool:
		delete(a.cache, schema)
	}
}

// AddVocabulary adds keywords (ajv.addVocabulary).
func (a *Ajv) AddVocabulary(defs []KeywordDefinition) (err error) {
	defer catch(&err)
	for _, def := range defs {
		a.addKeywordDef(fromKeywordDefinition(def))
	}
	return nil
}

// AddKeyword adds a keyword (ajv.addKeyword).
func (a *Ajv) AddKeyword(def KeywordDefinition) (err error) {
	defer catch(&err)
	a.addKeywordDef(fromKeywordDefinition(def))
	return nil
}

// userKeyword is a keyword definition passed to AddKeyword, before its types are checked.
type userKeyword struct {
	def        *keywordDef
	typ        any
	schemaType any
}

func fromKeywordDefinition(d KeywordDefinition) userKeyword {
	return userKeyword{
		def: &keywordDef{
			keywords:       []string{d.Keyword},
			data:           d.Data,
			allowUndefined: d.AllowUndefined,
			implements:     d.Implements,
			before:         d.Before,
			post:           d.Post,
			metaSchema:     d.MetaSchema,
			dependencies:   d.Dependencies,
			validate:       d.Validate,
			compile:        d.Compile,
			macro:          d.Macro,
			modifying:      d.Modifying,
			valid:          jsOption(d.Valid),
			errors:         jsOption(d.Errors),
		},
		typ:        d.Type,
		schemaType: d.SchemaType,
	}
}

func (a *Ajv) addVocabulary(defs []any) {
	for _, def := range defs {
		switch d := def.(type) {
		case string:
			a.addKeywordName(d)
		case *keywordDef:
			a.addKeywordDef(userKeyword{def: d, typ: d.typ, schemaType: d.schemaType})
		}
	}
}

var keywordNameRe = regexp.MustCompile(`^(?i)[a-z_$][a-z0-9_$:-]*$`)

// addKeywordName is ajv.addKeyword(name) without a definition.
func (a *Ajv) addKeywordName(keyword string) {
	a.checkKeyword([]string{keyword}, nil)
	a.addRule(keyword, nil, "")
}

// addKeywordDef is ajv.addKeyword(definition).
func (a *Ajv) addKeywordDef(uk userKeyword) {
	def := uk.def
	a.checkKeyword(def.keywords, def)
	if !isUndefined(def.metaSchema) && def.metaSchema != nil {
		metaSchema := def.metaSchema
		if def.data && a.opts.data {
			metaSchema = schemaOrData(metaSchema)
		}
		def.validateSchema = a.compile(metaSchema, true)
	}
	def.typ = getJSONTypes(uk.typ)
	def.schemaType = getJSONTypes(uk.schemaType)
	for _, k := range def.keywords {
		if len(def.typ) == 0 {
			a.addRule(k, def, "")
		} else {
			for _, t := range def.typ {
				a.addRule(k, def, t)
			}
		}
	}
}

func (a *Ajv) checkKeyword(keywords []string, def *keywordDef) {
	for _, kwd := range keywords {
		if a.rules.hasKeyword(kwd) {
			panic(newError("Keyword " + kwd + " is already defined"))
		}
		if !keywordNameRe.MatchString(kwd) {
			panic(newError("Keyword " + kwd + " has invalid name"))
		}
	}
	if def != nil && def.data && def.code == nil && def.validate == nil {
		panic(newError(`$data keyword must have "code" or "validate" function`))
	}
}

func (a *Ajv) addRule(keyword string, def *keywordDef, dataType string) {
	post := def != nil && def.post
	if dataType != "" && post {
		panic(newError(`keyword with "post" flag cannot have "type"`))
	}
	rules := a.rules
	var group *ruleGroup
	if post {
		group = rules.post
	} else {
		for _, g := range rules.rules {
			if g.typ == dataType {
				group = g
				break
			}
		}
	}
	if group == nil {
		group = &ruleGroup{typ: dataType}
		rules.rules = append(rules.rules, group)
	}
	rules.keywords[keyword] = true
	if def == nil {
		return
	}
	ru := &rule{keyword: keyword, definition: def}
	if def.before != "" {
		a.addBeforeRule(group, ru, def.before)
	} else {
		group.rules = append(group.rules, ru)
	}
	rules.setRule(keyword, ru)
	for _, kwd := range def.implements {
		a.addKeywordName(kwd)
	}
}

func (a *Ajv) addBeforeRule(group *ruleGroup, ru *rule, before string) {
	for i, r := range group.rules {
		if r.keyword == before {
			group.rules = append(group.rules[:i], append([]*rule{ru}, group.rules[i:]...)...)
			return
		}
	}
	group.rules = append(group.rules, ru)
	a.logger.Warn("rule " + before + " is not defined")
}

// ErrorsText renders errors as text (ajv.errorsText).
func (a *Ajv) ErrorsText(errors []*ErrorObject, opts ErrorsTextOptions) string {
	sep, dataVar := opts.Separator, opts.DataVar
	if sep == "" {
		sep = ", "
	}
	if dataVar == "" {
		dataVar = "data"
	}
	return a.errorsText(errors, sep, dataVar)
}

func (a *Ajv) errorsText(errors []*ErrorObject, separator, dataVar string) string {
	if len(errors) == 0 {
		return "No errors"
	}
	parts := make([]string, len(errors))
	for i, e := range errors {
		msg := e.Message
		if msg == "" {
			msg = "undefined"
		}
		parts[i] = dataVar + e.InstancePath + " " + msg
	}
	return strings.Join(parts, separator)
}

// dataMetaSchema is ajv.$dataMetaSchema.
func (a *Ajv) dataMetaSchema(metaSchema any, keywordsJSONPointers []string) any {
	metaSchema = deepCopyJSON(metaSchema)
	for _, ptr := range keywordsJSONPointers {
		keywords := metaSchema
		for _, seg := range strings.Split(ptr, "/")[1:] {
			keywords = getProp(keywords, seg)
		}
		for _, key := range a.rules.allOrder {
			ru := a.rules.all[key]
			if ru == nil {
				continue
			}
			schema := getProp(keywords, key)
			if ru.definition.data && truthy(schema) {
				setProp(keywords, key, schemaOrData(schema))
			}
		}
	}
	return metaSchema
}

const dataRefURL = "https://raw.githubusercontent.com/ajv-validator/ajv/master/lib/refs/data.json#"

var dataRef = newObjectKV("$ref", dataRefURL)

func schemaOrData(schema any) any {
	return newObjectKV("anyOf", []any{schema, dataRef})
}

// addSchemaEnv is ajv._addSchema.
func (a *Ajv) addSchemaEnv(schema any, meta bool, baseID string, validateSchema, addSchema any) *schemaEnv {
	id := undefined
	switch typeOf(schema) {
	case "object":
		if schema == nil {
			panic(newError("Cannot read properties of null (reading '$id')"))
		}
		id = getProp(schema, "$id")
	case "boolean":
	default:
		panic(newError("schema must be object or boolean"))
	}
	if sch := a.cacheGet(schema); sch != nil {
		return sch
	}
	if truthy(id) {
		s, ok := id.(string)
		if !ok {
			panic(newError("id.replace is not a function"))
		}
		baseID = s
	}
	baseID = normalizeID(baseID)
	localRefs := a.getSchemaRefs(schema, baseID)
	sch := newSchemaEnv(schemaEnvArgs{schema: schema, meta: meta, baseID: &baseID, localRefs: localRefs})
	a.cacheSet(sch)
	if truthy(addSchema) && !strings.HasPrefix(baseID, "#") {
		if baseID != "" {
			a.checkUnique(baseID)
		}
		a.refs[baseID] = sch
	}
	if truthy(validateSchema) {
		a.validateSchema(schema, true)
	}
	return sch
}

func (a *Ajv) checkUnique(id string) {
	if a.schemas[id] != nil || truthy(a.refs[id]) {
		panic(newError(`schema with key or id "` + id + `" already exists`))
	}
}

func (a *Ajv) compileSchemaEnv(sch *schemaEnv) *ValidateFunction {
	if sch.meta {
		a.compileMetaSchema(sch)
	} else {
		a.compileSchema(sch)
	}
	if sch.validate == nil {
		panic(newError("ajv implementation error"))
	}
	return sch.validate
}

func (a *Ajv) compileMetaSchema(sch *schemaEnv) {
	current := a.opts
	a.opts = a.metaOpts
	defer func() { a.opts = current }()
	a.compileSchema(sch)
}

// consoleLogger is the default logger: console.log, console.warn and console.error, all
// writing to standard error.
type consoleLogger struct{}

func (consoleLogger) Log(args ...any)   { writeConsole(args) }
func (consoleLogger) Warn(args ...any)  { writeConsole(args) }
func (consoleLogger) Error(args ...any) { writeConsole(args) }

func writeConsole(args []any) {
	parts := make([]string, len(args))
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			parts[i] = s
		} else {
			parts[i] = fmt.Sprint(arg)
		}
	}
	if _, err := fmt.Fprintln(os.Stderr, strings.Join(parts, " ")); err != nil {
		return
	}
}
