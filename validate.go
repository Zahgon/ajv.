// Ported from Ajv 8.20.0, lib/compile/validate/index.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ajv-validator/ajv-go/internal/js"
)

// schemaCxt is SchemaCxt: the compilation context of a schema (or subschema).
type schemaCxt struct {
	gen                *codeGen
	allErrors          bool
	data               *jsName
	parentData         *jsName
	parentDataProperty jsCode
	dataNames          []*jsName
	dataPathArr        []jsCode
	dataLevel          int
	dataTypes          []string
	definedProperties  *stringSet
	validateFn         *ValidateFunction
	evaluated          *jsName
	schema             any
	schemaEnv          *schemaEnv
	rootID             string
	baseID             string
	errSchemaPath      string
	errorPath          *pathCode
	propertyName       *jsName
	compositeRule      bool
	props              any // nil (undefined), true, *propSet or *jsName
	items              any // nil (undefined), true, float64 or *jsName
	createErrorsFalse  bool
	opts               *instanceOptions
	self               *Ajv
}

// validateFunctionCode generates the body of a compiled schema's validation function.
func validateFunctionCode(it *schemaCxt) {
	if isSchemaObj(it) {
		checkKeywords(it)
		if schemaCxtHasRules(it) {
			topSchemaObjCode(it)
			return
		}
	}
	topBoolOrEmptySchema(it)
}

func topSchemaObjCode(it *schemaCxt) {
	gen := it.gen
	checkNoDefault(it)
	gen.let(names.vErrors, nil)
	gen.let(names.errors, 0.0)
	if it.opts.unevaluated {
		resetEvaluated(it)
	}
	typeAndKeywords(it, nil)
	returnResults(it)
}

func resetEvaluated(it *schemaCxt) {
	gen := it.gen
	fn := it.validateFn
	it.evaluated = gen.constDef("evaluated", exprFn(func(*frame) any { return fn.evaluated }))
	gen.code(func(*frame) {
		ev := fn.evaluated
		if ev.dynamicProps {
			ev.props = nil
		}
		if ev.dynamicItems {
			ev.items = nil
		}
	})
}

// subschemaCode generates the code of a subschema, assigning its validity to valid.
func subschemaCode(it *schemaCxt, valid *jsName) {
	if isSchemaObj(it) {
		checkKeywords(it)
		if schemaCxtHasRules(it) {
			subSchemaObjCode(it, valid)
			return
		}
	}
	boolOrEmptySchema(it, valid)
}

func schemaCxtHasRules(it *schemaCxt) bool {
	if b, ok := it.schema.(bool); ok {
		return !b
	}
	for _, key := range objectKeys(it.schema) {
		if it.self.rules.hasRule(key) {
			return true
		}
	}
	return false
}

func isSchemaObj(it *schemaCxt) bool {
	_, ok := it.schema.(bool)
	return !ok
}

func subSchemaObjCode(it *schemaCxt, valid *jsName) {
	gen := it.gen
	updateContext(it)
	checkAsyncSchema(it)
	errsCount := gen.constDef("_errs", names.errors)
	typeAndKeywords(it, errsCount)
	gen.varDef(valid, exprFn(func(f *frame) any { return strictEquals(f.get(errsCount), f.vars[slotErrors]) }))
}

func checkKeywords(it *schemaCxt) {
	checkUnknownRules(it, it.schema)
}

func typeAndKeywords(it *schemaCxt, errsCount *jsName) {
	types := getSchemaTypes(it.schema)
	checkedTypes := coerceAndCheckDataType(it, types)
	schemaKeywords(it, types, !checkedTypes, errsCount)
}

func checkNoDefault(it *schemaCxt) {
	if !isUndefined(getProp(it.schema, "default")) && truthy(it.opts.useDefaults) && truthy(it.opts.strictSchema) {
		checkStrictMode(it, "default is ignored in the schema root", it.opts.strictSchema)
	}
}

func updateContext(it *schemaCxt) {
	schID := getProp(it.schema, "$id")
	if truthy(schID) {
		it.baseID = resolveURL(it.baseID, toString(schID))
	}
}

func checkAsyncSchema(it *schemaCxt) {
	if truthy(getProp(it.schema, "$async")) && !it.schemaEnv.async {
		panic(newError("async schema in sync schema"))
	}
}

func returnResults(it *schemaCxt) {
	gen := it.gen
	gen.code(func(f *frame) { f.fn.errors = vErrorsOf(f) })
	if it.opts.unevaluated {
		assignEvaluated(it)
	}
	gen.returnStmt(exprFn(func(f *frame) any { return toNumber(f.vars[slotErrors]) == 0 }))
}

func assignEvaluated(it *schemaCxt) {
	gen, evaluated := it.gen, it.evaluated
	if props, ok := it.props.(*jsName); ok {
		gen.code(func(f *frame) { f.get(evaluated).(*evaluatedInfo).props = f.get(props) })
	}
	if items, ok := it.items.(*jsName); ok {
		gen.code(func(f *frame) { f.get(evaluated).(*evaluatedInfo).items = f.get(items) })
	}
}

func errorsEqual(errsCount *jsName) jsCode {
	if errsCount == nil {
		return exprFn(func(f *frame) any { return toNumber(f.vars[slotErrors]) == 0 })
	}
	return exprFn(func(f *frame) any { return strictEquals(f.vars[slotErrors], f.get(errsCount)) })
}

func schemaKeywords(it *schemaCxt, types []string, typeErrors bool, errsCount *jsName) {
	gen, schema, rules := it.gen, it.schema, it.self.rules
	if truthy(getProp(schema, "$ref")) && !schemaHasRulesButRef(schema, rules) {
		gen.block(func() { keywordCode(it, "$ref", rules.all["$ref"].definition, "") })
		return
	}
	checkStrictTypes(it, types)
	groupKeywords := func(group *ruleGroup) {
		if !shouldUseGroup(schema, group) {
			return
		}
		if group.typ != "" {
			gen.ifThen(checkDataType(group.typ, it.data, it.opts.strictNumbers, false))
			iterateKeywords(it, group)
			if len(types) == 1 && types[0] == group.typ && typeErrors {
				gen.elseBranch()
				reportTypeError(it)
			}
			gen.endIf()
		} else {
			iterateKeywords(it, group)
		}
		if !it.allErrors {
			gen.ifThen(errorsEqual(errsCount))
		}
	}
	gen.block(func() {
		for _, group := range rules.rules {
			groupKeywords(group)
		}
		groupKeywords(rules.post)
	})
}

func iterateKeywords(it *schemaCxt, group *ruleGroup) {
	if truthy(it.opts.useDefaults) {
		assignDefaults(it, group.typ)
	}
	it.gen.block(func() {
		for _, ru := range group.rules {
			if shouldUseRule(it.schema, ru) {
				keywordCode(it, ru.keyword, ru.definition, group.typ)
			}
		}
	})
}

func checkStrictTypes(it *schemaCxt, types []string) {
	if it.schemaEnv.meta || !truthy(it.opts.strictTypes) {
		return
	}
	checkContextTypes(it, types)
	if !it.opts.allowUnionTypes {
		checkMultipleTypes(it, types)
	}
	checkKeywordTypes(it, it.dataTypes)
}

func checkContextTypes(it *schemaCxt, types []string) {
	if len(types) == 0 {
		return
	}
	if len(it.dataTypes) == 0 {
		it.dataTypes = types
		return
	}
	for _, t := range types {
		if !includesType(it.dataTypes, t) {
			strictTypesError(it, `type "`+t+`" not allowed by context "`+strings.Join(it.dataTypes, ",")+`"`)
		}
	}
	narrowSchemaTypes(it, types)
}

func checkMultipleTypes(it *schemaCxt, ts []string) {
	if len(ts) > 1 && !(len(ts) == 2 && contains(ts, "null")) {
		strictTypesError(it, "use allowUnionTypes to allow union type keyword")
	}
}

func checkKeywordTypes(it *schemaCxt, ts []string) {
	rules := it.self.rules
	for _, keyword := range rules.allOrder {
		ru := rules.all[keyword]
		if ru != nil && shouldUseRule(it.schema, ru) {
			typ := ru.definition.typ
			if len(typ) > 0 {
				applicable := false
				for _, t := range typ {
					if hasApplicableType(ts, t) {
						applicable = true
						break
					}
				}
				if !applicable {
					strictTypesError(it, `missing type "`+strings.Join(typ, ",")+`" for keyword "`+keyword+`"`)
				}
			}
		}
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func hasApplicableType(schTs []string, kwdT string) bool {
	return contains(schTs, kwdT) || (kwdT == "number" && contains(schTs, "integer"))
}

func includesType(ts []string, t string) bool {
	return contains(ts, t) || (t == "integer" && contains(ts, "number"))
}

func narrowSchemaTypes(it *schemaCxt, withTypes []string) {
	ts := []string{}
	for _, t := range it.dataTypes {
		if includesType(withTypes, t) {
			ts = append(ts, t)
		} else if contains(withTypes, "integer") && t == "number" {
			ts = append(ts, "integer")
		}
	}
	it.dataTypes = ts
}

func strictTypesError(it *schemaCxt, msg string) {
	schemaPath := it.schemaEnv.baseID + it.errSchemaPath
	msg += ` at "` + schemaPath + `" (strictTypes)`
	checkStrictMode(it, msg, it.opts.strictTypes)
}

// keywordCxt is KeywordCxt: the context in which a keyword's code is generated.
type keywordCxt struct {
	gen          *codeGen
	allErrors    bool
	keyword      string
	data         *jsName
	dataRef      string // the $data pointer, "" when the keyword does not use $data
	schema       any
	schemaValue  jsCode
	schemaCode   jsCode
	schemaType   []string
	parentSchema any
	errsCount    *jsName
	params       map[string]jsCode
	it           *schemaCxt
	def          *keywordDef
}

func newKeywordCxt(it *schemaCxt, def *keywordDef, keyword string) *keywordCxt {
	validateKeywordUsage(it, def, keyword)
	cxt := &keywordCxt{
		gen:          it.gen,
		allErrors:    it.allErrors,
		keyword:      keyword,
		data:         it.data,
		schema:       getProp(it.schema, keyword),
		schemaType:   def.schemaType,
		parentSchema: it.schema,
		params:       map[string]jsCode{},
		it:           it,
		def:          def,
	}
	if def.data && it.opts.data && truthy(cxt.schema) {
		if ref := getProp(cxt.schema, "$data"); truthy(ref) {
			cxt.dataRef = toString(ref)
		}
	}
	cxt.schemaValue = cxt.schema
	if cxt.dataRef != "" {
		cxt.schemaCode = it.gen.constDef("vSchema", getData(cxt.dataRef, it))
	} else {
		cxt.schemaCode = cxt.schemaValue
		if !validSchemaType(cxt.schema, def.schemaType, def.allowUndefined) {
			panic(newError(keyword + " value must be " + Stringify(stringsToAny(def.schemaType))))
		}
	}
	track := def.errors != false
	if def.code != nil {
		track = def.trackErrors
	}
	if track {
		cxt.errsCount = it.gen.constDef("_errs", names.errors)
	}
	return cxt
}

func stringsToAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func (cxt *keywordCxt) result(cond jsCode, successAction, failAction func()) {
	cxt.failResult(not(cond), successAction, failAction)
}

func (cxt *keywordCxt) failResult(cond jsCode, successAction, failAction func()) {
	cxt.gen.ifThen(cond)
	if failAction != nil {
		failAction()
	} else {
		cxt.error(false)
	}
	if successAction != nil {
		cxt.gen.elseBranch()
		successAction()
		if cxt.allErrors {
			cxt.gen.endIf()
		}
	} else if cxt.allErrors {
		cxt.gen.endIf()
	} else {
		cxt.gen.elseBranch()
	}
}

func (cxt *keywordCxt) pass(cond jsCode, failAction func()) {
	cxt.failResult(not(cond), nil, failAction)
}

// failAlways is cxt.fail() without a condition.
func (cxt *keywordCxt) failAlways() {
	cxt.error(false)
	if !cxt.allErrors {
		cxt.gen.ifThen(false)
	}
}

func (cxt *keywordCxt) fail(cond jsCode) {
	cxt.gen.ifThen(cond)
	cxt.error(false)
	if cxt.allErrors {
		cxt.gen.endIf()
	} else {
		cxt.gen.elseBranch()
	}
}

func (cxt *keywordCxt) failData(cond jsCode) {
	if cxt.dataRef == "" {
		cxt.fail(cond)
		return
	}
	schemaCode := cxt.schemaCode
	inner := or(cxt.invalidData(), cond)
	cxt.fail(exprFn(func(f *frame) any {
		return !isUndefined(eval(schemaCode, f)) && truthy(eval(inner, f))
	}))
}

func (cxt *keywordCxt) error(appendErr bool) {
	if appendErr {
		reportExtraError(cxt, cxt.def.err)
	} else {
		reportError(cxt, cxt.def.err, nil)
	}
}

// errorWithParams is cxt.error(append, errorParams).
func (cxt *keywordCxt) errorWithParams(appendErr bool, params map[string]jsCode) {
	cxt.setParams(params, false)
	cxt.error(appendErr)
	cxt.setParams(map[string]jsCode{}, false)
}

func (cxt *keywordCxt) dataError() {
	reportError(cxt, keywordDataError, nil)
}

func (cxt *keywordCxt) reset() {
	if cxt.errsCount == nil {
		panic(newError(`add "trackErrors" to keyword definition`))
	}
	resetErrorsCount(cxt.gen, cxt.errsCount)
}

func (cxt *keywordCxt) ok(cond jsCode) {
	if !cxt.allErrors {
		cxt.gen.ifThen(cond)
	}
}

func (cxt *keywordCxt) setParams(obj map[string]jsCode, assign bool) {
	if assign {
		merged := make(map[string]jsCode, len(cxt.params)+len(obj))
		for k, v := range cxt.params {
			merged[k] = v
		}
		for k, v := range obj {
			merged[k] = v
		}
		cxt.params = merged
		return
	}
	cxt.params = obj
}

func (cxt *keywordCxt) blockData(valid *jsName, codeBlock func(), dataValid jsCode) {
	cxt.gen.block(func() {
		cxt.checkData(valid, dataValid)
		codeBlock()
	})
}

func (cxt *keywordCxt) checkData(valid *jsName, dataValid jsCode) {
	if cxt.dataRef == "" {
		return
	}
	gen, schemaCode := cxt.gen, cxt.schemaCode
	gen.ifThen(or(exprFn(func(f *frame) any { return isUndefined(eval(schemaCode, f)) }), dataValid))
	if valid != nil {
		gen.assign(valid, true)
	}
	if len(cxt.schemaType) > 0 || cxt.def.validateSchema != nil {
		gen.elseIf(cxt.invalidData())
		cxt.dataError()
		if valid != nil {
			gen.assign(valid, false)
		}
	}
	gen.elseBranch()
}

func (cxt *keywordCxt) invalidData() jsCode {
	wrongType := nilCode
	if len(cxt.schemaType) > 0 {
		sc, ok := cxt.schemaCode.(*jsName)
		if !ok {
			panic(newError("ajv implementation error"))
		}
		wrongType = checkDataTypes(cxt.schemaType, sc, cxt.it.opts.strictNumbers, true)
	}
	invalidSchema := nilCode
	if vs := cxt.def.validateSchema; vs != nil {
		schemaCode := cxt.schemaCode
		invalidSchema = exprFn(func(f *frame) any { return !vs.call(eval(schemaCode, f), nil) })
	}
	return or(wrongType, invalidSchema)
}

// maxSubschemaDepth bounds the nesting of subschemas: inlining a schema that refers to itself
// recurses until JavaScript's call stack overflows.
const maxSubschemaDepth = 2000

func (cxt *keywordCxt) subschema(a *subschemaArgs, valid *jsName) *schemaCxt {
	sub := getSubschema(cxt.it, a)
	extendSubschemaData(sub, cxt.it, a)
	next := *cxt.it
	next.schema = sub.schema
	next.errSchemaPath = sub.errSchemaPath
	if sub.hasData {
		next.data = sub.data
		next.dataLevel = sub.dataLevel
		next.parentData = sub.parentData
		next.dataNames = sub.dataNames
	}
	if sub.hasDataTypes {
		next.dataTypes = sub.dataTypes
	}
	if sub.errorPath != nil {
		next.errorPath = sub.errorPath
	}
	if sub.hasDataProp {
		next.parentDataProperty = sub.parentDataProperty
		next.dataPathArr = sub.dataPathArr
	}
	if sub.propertyName != nil {
		next.propertyName = sub.propertyName
	}
	next.definedProperties = cxt.it.definedProperties
	if a.compositeRule {
		next.compositeRule = true
	}
	if a.createErrors != nil {
		next.createErrorsFalse = !*a.createErrors
	}
	if a.allErrors != nil {
		next.allErrors = *a.allErrors
	}
	next.items = nil
	next.props = nil
	self := cxt.it.self
	self.depth++
	defer func() { self.depth-- }()
	if self.depth > maxSubschemaDepth {
		panic(newError("Maximum call stack size exceeded"))
	}
	subschemaCode(&next, valid)
	return &next
}

func (cxt *keywordCxt) mergeEvaluated(sc *schemaCxt, toName bool) {
	it, gen := cxt.it, cxt.gen
	if !it.opts.unevaluated {
		return
	}
	if it.props != true && sc.props != nil {
		it.props = mergeEvaluatedProps(gen, sc.props, it.props, toName)
	}
	if it.items != true && sc.items != nil {
		it.items = mergeEvaluatedItems(gen, sc.items, it.items, toName)
	}
}

func (cxt *keywordCxt) mergeValidEvaluated(sc *schemaCxt, valid *jsName) bool {
	it, gen := cxt.it, cxt.gen
	if it.opts.unevaluated && (it.props != true || it.items != true) {
		gen.ifThen(valid)
		cxt.mergeEvaluated(sc, true)
		gen.endIf()
		return true
	}
	return false
}

func keywordCode(it *schemaCxt, keyword string, def *keywordDef, ruleType string) {
	cxt := newKeywordCxt(it, def, keyword)
	switch {
	case def.code != nil:
		def.code(cxt, ruleType)
	case cxt.dataRef != "" && def.validate != nil:
		funcKeywordCode(cxt, def)
	case def.macro != nil:
		macroKeywordCode(cxt, def)
	case def.compile != nil || def.validate != nil:
		funcKeywordCode(cxt, def)
	}
}

var (
	jsonPointerRe         = regexp.MustCompile(`^/(?:[^~]|~0|~1)*$`)
	relativeJSONPointerRe = regexp.MustCompile(`^([0-9]+)(#|/(?:[^~]|~0|~1)*)?$`)
)

// getData is validate.getData: the expression of the data a $data pointer refers to.
func getData(dataRef string, it *schemaCxt) jsCode {
	if dataRef == "" {
		return names.rootData
	}
	var jsonPointer string
	var base *jsName
	if dataRef[0] == '/' {
		if !jsonPointerRe.MatchString(dataRef) {
			panic(newError("Invalid JSON-pointer: " + dataRef))
		}
		jsonPointer = dataRef
		base = names.rootData
	} else {
		m := relativeJSONPointerRe.FindStringSubmatch(dataRef)
		if m == nil {
			panic(newError("Invalid JSON-pointer: " + dataRef))
		}
		up := js.ToNumber(m[1])
		jsonPointer = m[2]
		level := float64(it.dataLevel)
		errorMsg := func(pointerType string) string {
			return "Cannot access " + pointerType + " " + js.NumberToString(up) + " levels up, current level is " + strconv.Itoa(it.dataLevel)
		}
		if jsonPointer == "#" {
			if up >= level {
				panic(newError(errorMsg("property/index")))
			}
			return it.dataPathArr[it.dataLevel-int(up)]
		}
		if up > level {
			panic(newError(errorMsg("data")))
		}
		base = it.dataNames[it.dataLevel-int(up)]
		if jsonPointer == "" {
			return base
		}
	}
	var segments []string
	for _, seg := range strings.Split(jsonPointer, "/") {
		if seg != "" {
			segments = append(segments, unescapeJSONPointer(seg))
		}
	}
	return exprFn(func(f *frame) any {
		v := f.get(base)
		for _, seg := range segments {
			if !truthy(v) {
				return v
			}
			v = getProp(v, seg)
		}
		return v
	})
}
