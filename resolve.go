// Ported from Ajv 8.20.0, lib/compile/resolve.ts, and json-schema-traverse 1.0.0 (MIT,
// Copyright (c) 2017 Evgeny Poberezkin), MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"errors"
	"math"
	"regexp"
	"strings"

	"github.com/ajv-validator/ajv-go/internal/js"
	"github.com/ajv-validator/ajv-go/internal/uri"
)

// throwJS rethrows an error of the URI library as the exception fast-uri throws.
func throwJS(err error) {
	var e *js.Error
	if errors.As(err, &e) {
		panic(newError(e.Message))
	}
	panic(newError(err.Error()))
}

func uriSerialize(c *uri.Components) string {
	s, err := uri.TrySerialize(c)
	if err != nil {
		throwJS(err)
	}
	return s
}

func uriResolve(base, ref string) string {
	s, err := uri.TryResolve(base, ref)
	if err != nil {
		throwJS(err)
	}
	return s
}

var simpleInlined = toSet("type", "format", "pattern", "maxLength", "minLength", "maxProperties",
	"minProperties", "maxItems", "minItems", "maximum", "minimum", "uniqueItems", "multipleOf",
	"required", "enum", "const")

// inlineRef is resolve.inlineRef; limit is the inlineRefs option.
func inlineRef(schema any, limit any) bool {
	if _, ok := schema.(bool); ok {
		return true
	}
	if limit == true {
		return !hasRef(schema)
	}
	if !truthy(limit) {
		return false
	}
	return compare(countKeys(schema), opLTE, limit)
}

var refKeywords = toSet("$ref", "$recursiveRef", "$recursiveAnchor", "$dynamicRef", "$dynamicAnchor")

func hasRef(schema any) bool {
	for _, key := range objectKeys(schema) {
		if refKeywords[key] {
			return true
		}
		sch := getProp(schema, key)
		if arr, ok := sch.([]any); ok {
			for _, s := range arr {
				if hasRef(s) {
					return true
				}
			}
		}
		if typeOf(sch) == "object" && hasRef(sch) {
			return true
		}
	}
	return false
}

func countKeys(schema any) float64 {
	count := 0.0
	for _, key := range objectKeys(schema) {
		if key == "$ref" {
			return math.Inf(1)
		}
		count++
		if simpleInlined[key] {
			continue
		}
		sch := getProp(schema, key)
		if typeOf(sch) == "object" {
			if arr, ok := sch.([]any); ok {
				for _, s := range arr {
					count += countKeys(s)
				}
			} else {
				count += countKeys(sch)
			}
		}
		if math.IsInf(count, 1) {
			return count
		}
	}
	return count
}

// getFullPath is resolve.getFullPath.
func getFullPath(id string, normalize bool) string {
	if normalize {
		id = normalizeID(id)
	}
	return getFullPathOf(uri.Parse(id))
}

// getFullPathOf is resolve._getFullPath.
func getFullPathOf(p *uri.Components) string {
	serialized := uriSerialize(p)
	return strings.SplitN(serialized, "#", 2)[0] + "#"
}

var trailingSlashHash = regexp.MustCompile(`#/?$`)

// normalizeID is resolve.normalizeId.
func normalizeID(id string) string {
	if id == "" {
		return ""
	}
	return trailingSlashHash.ReplaceAllLiteralString(id, "")
}

// resolveURL is resolve.resolveUrl.
func resolveURL(baseID, id string) string {
	return uriResolve(baseID, normalizeID(id))
}

var anchorRe = regexp.MustCompile(`^(?i)[a-z_][-a-z0-9._]*$`)

// getSchemaRefs is resolve.getSchemaRefs: it collects the $id and anchor references inside a
// schema, registering absolute ones in a.refs and returning the local ones.
func (a *Ajv) getSchemaRefs(schema any, baseID string) map[string]any {
	localRefs := map[string]any{}
	if _, ok := schema.(bool); ok {
		return localRefs
	}
	idVal := getProp(schema, "$id")
	schID := baseID
	if truthy(idVal) {
		schID = toString(idVal)
	}
	schID = normalizeID(schID)
	baseIDs := map[string]string{"": schID}
	pathPrefix := getFullPath(schID, false)
	schemaRefs := map[string]bool{}

	ambiguos := func(ref string) error {
		return newError(`reference "` + ref + `" resolves to more than one schema`)
	}
	checkAmbiguosRef := func(sch1, sch2 any, ref string) {
		if sch2 != nil && !equal(sch1, sch2) {
			panic(ambiguos(ref))
		}
	}

	traverse(schema, func(sch any, jsonPtr string, parentJSONPtr *string) {
		if parentJSONPtr == nil {
			return
		}
		fullPath := pathPrefix + jsonPtr
		innerBaseID := baseIDs[*parentJSONPtr]
		addRef := func(ref string) string {
			if innerBaseID != "" {
				ref = uriResolve(innerBaseID, ref)
			}
			ref = normalizeID(ref)
			if schemaRefs[ref] {
				panic(ambiguos(ref))
			}
			schemaRefs[ref] = true
			schOrRef := a.refs[ref]
			if s, ok := schOrRef.(string); ok {
				schOrRef = a.refs[s]
			}
			if env, ok := schOrRef.(*schemaEnv); ok {
				checkAmbiguosRef(sch, env.schema, ref)
			} else if ref != normalizeID(fullPath) {
				if ref != "" && ref[0] == '#' {
					checkAmbiguosRef(sch, localRefs[ref], ref)
					localRefs[ref] = sch
				} else {
					a.refs[ref] = fullPath
				}
			}
			return ref
		}
		addAnchor := func(anchor any) {
			if s, ok := anchor.(string); ok {
				if !anchorRe.MatchString(s) {
					panic(newError(`invalid anchor "` + s + `"`))
				}
				addRef("#" + s)
			}
		}
		if id, ok := getProp(sch, "$id").(string); ok {
			innerBaseID = addRef(id)
		}
		addAnchor(getProp(sch, "$anchor"))
		addAnchor(getProp(sch, "$dynamicAnchor"))
		baseIDs[jsonPtr] = innerBaseID
	})
	return localRefs
}

var (
	traverseKeywords = toSet("additionalItems", "items", "contains", "additionalProperties",
		"propertyNames", "not", "if", "then", "else")
	traverseArrayKeywords = toSet("items", "allOf", "anyOf", "oneOf")
	traversePropsKeywords = toSet("$defs", "definitions", "properties", "patternProperties", "dependencies")
	traverseSkipKeywords  = toSet("default", "enum", "const", "required", "maximum", "minimum",
		"exclusiveMaximum", "exclusiveMinimum", "multipleOf", "maxLength", "minLength", "pattern",
		"format", "maxItems", "minItems", "uniqueItems", "maxProperties", "minProperties")
)

// inDict is the `key in dict` test against a plain object of json-schema-traverse.
func inDict(dict map[string]bool, key string) bool { return dict[key] || inObjectProto(key) }

// traverse is json-schema-traverse with {allKeys: true} and a pre callback.
func traverse(schema any, pre func(sch any, jsonPtr string, parentJSONPtr *string)) {
	traverseNode(schema, "", nil, pre)
}

func traverseNode(schema any, jsonPtr string, parentJSONPtr *string, pre func(any, string, *string)) {
	if !isPlainObject(schema) {
		return
	}
	pre(schema, jsonPtr, parentJSONPtr)
	ptr := jsonPtr
	for _, key := range objectKeys(schema) {
		sch := getProp(schema, key)
		if arr, ok := sch.([]any); ok {
			if inDict(traverseArrayKeywords, key) {
				for i, item := range arr {
					traverseNode(item, jsonPtr+"/"+key+"/"+js.NumberToString(float64(i)), &ptr, pre)
				}
			}
		} else if inDict(traversePropsKeywords, key) {
			if truthy(sch) && typeOf(sch) == "object" {
				for _, prop := range objectKeys(sch) {
					traverseNode(getProp(sch, prop), jsonPtr+"/"+key+"/"+escapeJSONPointer(prop), &ptr, pre)
				}
			}
		} else if inDict(traverseKeywords, key) || !inDict(traverseSkipKeywords, key) {
			traverseNode(sch, jsonPtr+"/"+key, &ptr, pre)
		}
	}
}
