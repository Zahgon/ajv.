// Ported from Ajv 8.20.0, lib/vocabularies/draft7.ts, lib/vocabularies/draft2020.ts,
// lib/vocabularies/next.ts, lib/vocabularies/metadata.ts and lib/refs, MIT,
// Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"embed"
	"sync"
)

func formatVocabulary() []any { return []any{formatKeyword()} }

func metadataVocabulary() []any {
	return []any{"title", "description", "default", "deprecated", "readOnly", "writeOnly", "examples"}
}

func contentVocabulary() []any {
	return []any{"contentMediaType", "contentEncoding", "contentSchema"}
}

func nextVocabulary() []any {
	return []any{dependentRequiredKeyword(), dependentSchemasKeyword(), limitContainsKeyword()}
}

func draft7Vocabularies() [][]any {
	return [][]any{
		coreVocabulary(),
		validationVocabulary(),
		applicatorVocabulary(false),
		formatVocabulary(),
		metadataVocabulary(),
		contentVocabulary(),
	}
}

func draft2020Vocabularies() [][]any {
	return [][]any{
		dynamicVocabulary(),
		coreVocabulary(),
		validationVocabulary(),
		applicatorVocabulary(true),
		formatVocabulary(),
		metadataVocabulary(),
		contentVocabulary(),
		nextVocabulary(),
		unevaluatedVocabulary(),
	}
}

const (
	draft7MetaSchemaID    = "http://json-schema.org/draft-07/schema"
	draft2019MetaSchemaID = "https://json-schema.org/draft/2019-09/schema"
	draft2020MetaSchemaID = "https://json-schema.org/draft/2020-12/schema"
)

//go:embed refs
var refsFS embed.FS

var (
	refsOnce   sync.Once
	refsParsed map[string]any
)

// refSchema returns the parsed meta-schema file at path under refs. The documents are shared,
// as the modules Ajv requires them from are.
func refSchema(path string) any {
	refsOnce.Do(func() {
		refsParsed = map[string]any{}
		for _, p := range []string{
			"data.json",
			"json-schema-draft-07.json",
			"json-schema-2019-09/schema.json",
			"json-schema-2019-09/meta/applicator.json",
			"json-schema-2019-09/meta/content.json",
			"json-schema-2019-09/meta/core.json",
			"json-schema-2019-09/meta/format.json",
			"json-schema-2019-09/meta/meta-data.json",
			"json-schema-2019-09/meta/validation.json",
			"json-schema-2020-12/schema.json",
			"json-schema-2020-12/meta/applicator.json",
			"json-schema-2020-12/meta/unevaluated.json",
			"json-schema-2020-12/meta/content.json",
			"json-schema-2020-12/meta/core.json",
			"json-schema-2020-12/meta/format-annotation.json",
			"json-schema-2020-12/meta/meta-data.json",
			"json-schema-2020-12/meta/validation.json",
		} {
			b, err := refsFS.ReadFile("refs/" + p)
			if err != nil {
				panic("ajv: missing embedded meta-schema " + p)
			}
			refsParsed[p] = mustParseJSON(string(b))
		}
	})
	return refsParsed[path]
}

func dataRefSchema() any    { return refSchema("data.json") }
func draft7MetaSchema() any { return refSchema("json-schema-draft-07.json") }

func metaSchemas2019() []any {
	return []any{
		refSchema("json-schema-2019-09/schema.json"),
		refSchema("json-schema-2019-09/meta/applicator.json"),
		refSchema("json-schema-2019-09/meta/content.json"),
		refSchema("json-schema-2019-09/meta/core.json"),
		refSchema("json-schema-2019-09/meta/format.json"),
		refSchema("json-schema-2019-09/meta/meta-data.json"),
		refSchema("json-schema-2019-09/meta/validation.json"),
	}
}

func metaSchemas2020() []any {
	return []any{
		refSchema("json-schema-2020-12/schema.json"),
		refSchema("json-schema-2020-12/meta/applicator.json"),
		refSchema("json-schema-2020-12/meta/unevaluated.json"),
		refSchema("json-schema-2020-12/meta/content.json"),
		refSchema("json-schema-2020-12/meta/core.json"),
		refSchema("json-schema-2020-12/meta/format-annotation.json"),
		refSchema("json-schema-2020-12/meta/meta-data.json"),
		refSchema("json-schema-2020-12/meta/validation.json"),
	}
}
