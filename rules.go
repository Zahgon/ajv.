// Ported from Ajv 8.20.0, lib/compile/rules.ts, lib/compile/validate/applicability.ts and the
// keyword definition types of lib/types/index.ts, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

var jsonTypes = toSet("string", "number", "integer", "boolean", "null", "object", "array")

func isJSONType(x any) bool {
	s, ok := x.(string)
	return ok && jsonTypes[s]
}

// errorDef is a KeywordErrorDefinition: message and params build, when an error is reported
// at generation time, the runtime expressions of the error message and params.
type errorDef struct {
	message func(cxt *keywordCxt) func(f *frame) string
	params  func(cxt *keywordCxt) func(f *frame) *Object
}

// keywordDef is an added keyword definition (AddedKeywordDefinition).
type keywordDef struct {
	keywords       []string
	typ            []string
	schemaType     []string
	data           bool
	allowUndefined bool
	implements     []string
	before         string
	post           bool
	metaSchema     any
	validateSchema *ValidateFunction
	dependencies   []string
	err            *errorDef

	// jsCode keywords (the built-in vocabularies).
	code        func(cxt *keywordCxt, ruleType string)
	trackErrors bool

	// Function keywords (AddKeyword).
	validate  func(schema, data, parentSchema any, cxt DataValidationCxt) (bool, []*ErrorObject)
	compile   func(schema, parentSchema any) (func(data any, cxt DataValidationCxt) (bool, []*ErrorObject), error)
	macro     func(schema, parentSchema any) any
	modifying bool
	valid     any
	errors    any
}

// rule is a keyword with its definition.
type rule struct {
	keyword    string
	definition *keywordDef
}

// ruleGroup holds the rules applying to one data type (or to any type when typ is "").
type ruleGroup struct {
	typ   string
	rules []*rule
}

// validationRules is RULES.
type validationRules struct {
	types    map[string]*ruleGroup
	rules    []*ruleGroup
	post     *ruleGroup
	all      map[string]*rule
	allOrder []string
	keywords map[string]bool
}

func getRules() *validationRules {
	number := &ruleGroup{typ: "number"}
	str := &ruleGroup{typ: "string"}
	array := &ruleGroup{typ: "array"}
	object := &ruleGroup{typ: "object"}
	return &validationRules{
		types:    map[string]*ruleGroup{"number": number, "string": str, "array": array, "object": object},
		rules:    []*ruleGroup{{}, number, str, array, object},
		post:     &ruleGroup{},
		all:      map[string]*rule{},
		keywords: map[string]bool{},
	}
}

// hasKeyword is RULES.keywords[key] as a truth value: the dictionary is a plain object, so the
// names of Object.prototype's properties are always "defined".
func (r *validationRules) hasKeyword(key string) bool { return r.keywords[key] || inObjectProto(key) }

// hasRule is RULES.all[key] as a truth value.
func (r *validationRules) hasRule(key string) bool { return r.all[key] != nil || inObjectProto(key) }

func (r *validationRules) setRule(key string, ru *rule) {
	if _, ok := r.all[key]; !ok {
		r.allOrder = append(r.allOrder, key)
	}
	r.all[key] = ru
}

// schemaHasRulesForType is applicability.schemaHasRulesForType.
func schemaHasRulesForType(it *schemaCxt, typ string) bool {
	group := it.self.rules.types[typ]
	return group != nil && shouldUseGroup(it.schema, group)
}

func shouldUseGroup(schema any, group *ruleGroup) bool {
	for _, ru := range group.rules {
		if shouldUseRule(schema, ru) {
			return true
		}
	}
	return false
}

func shouldUseRule(schema any, ru *rule) bool {
	if !isUndefined(getProp(schema, ru.keyword)) {
		return true
	}
	for _, kwd := range ru.definition.implements {
		if !isUndefined(getProp(schema, kwd)) {
			return true
		}
	}
	return false
}
