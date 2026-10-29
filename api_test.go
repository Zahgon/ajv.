package ajv

import (
	"errors"
	"testing"
)

// The expected values of these tests were recorded with the original Ajv 8.20.0.

func mustParse(t *testing.T, text string) any {
	t.Helper()
	v, err := ParseJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func expectErr(t *testing.T, what string, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Errorf("%s: expected error %q, got %v", what, want, err)
	}
}

func TestSchemaManagement(t *testing.T) {
	a := New(Options{Logger: &recordingLogger{}})
	if err := a.AddSchema(mustParse(t, `{"$id":"http://x/s","definitions":{"n":{"type":"number"}}}`), ""); err != nil {
		t.Fatal(err)
	}
	expectErr(t, "duplicate id", a.AddSchema(mustParse(t, `{"$id":"http://x/s"}`), ""),
		`schema with key or id "http://x/s" already exists`)

	valid, err := a.Validate("http://x/s#/definitions/n", "a")
	if err != nil || valid {
		t.Fatalf("validate by reference: got %v, %v", valid, err)
	}
	if got, want := errorsJSON(a.Errors()), `[{"instancePath":"","schemaPath":"#/type","keyword":"type","params":{"type":"number"},"message":"must be number"}]`; got != want {
		t.Errorf("errors: expected %s, got %s", want, got)
	}
	if got := a.ErrorsText(a.Errors(), ErrorsTextOptions{Separator: "|", DataVar: "d"}); got != "d must be number" {
		t.Errorf("errors text: got %q", got)
	}
	if got := a.ErrorsText(nil, ErrorsTextOptions{}); got != "No errors" {
		t.Errorf("errors text of nil: got %q", got)
	}

	_, err = a.Validate("nope", 1.0)
	expectErr(t, "unknown key", err, `no schema with key or ref "nope"`)

	if v, err := a.GetSchema("http://x/s#/definitions/zz"); v != nil || err != nil {
		t.Errorf("missing fragment: got %v, %v", v, err)
	}

	_, err = a.Compile(mustParse(t, `{"$ref":"http://y/missing.json#/a"}`))
	var mre *MissingRefError
	if !errors.As(err, &mre) {
		t.Fatalf("expected a MissingRefError, got %v", err)
	}
	if mre.Message != "can't resolve reference http://y/missing.json#/a from id #" ||
		mre.MissingRef != "http://y/missing.json#/a" || mre.MissingSchema != "http://y/missing.json" {
		t.Errorf("missing ref error: got %+v", mre)
	}

	if err := a.RemoveSchema("http://x/s"); err != nil {
		t.Fatal(err)
	}
	if v, err := a.GetSchema("http://x/s"); v != nil || err != nil {
		t.Errorf("removed schema: got %v, %v", v, err)
	}
	expectErr(t, "remove by number", a.RemoveSchema(5.0), "ajv.removeSchema: invalid parameter")

	_, err = a.Compile(5.0)
	expectErr(t, "compile a number", err, "schema must be object or boolean")
}

func TestValidateSchemaErrors(t *testing.T) {
	a := New(Options{})
	valid, err := a.ValidateSchema(mustParse(t, `{"type":1}`))
	if err != nil || valid {
		t.Fatalf("got %v, %v", valid, err)
	}
	want := `[{"instancePath":"/type","schemaPath":"#/definitions/simpleTypes/enum","keyword":"enum","params":{"allowedValues":["array","boolean","integer","null","number","object","string"]},"message":"must be equal to one of the allowed values"},` +
		`{"instancePath":"/type","schemaPath":"#/properties/type/anyOf/1/type","keyword":"type","params":{"type":"array"},"message":"must be array"},` +
		`{"instancePath":"/type","schemaPath":"#/properties/type/anyOf","keyword":"anyOf","params":{},"message":"must match a schema in anyOf"}]`
	if got := errorsJSON(a.Errors()); got != want {
		t.Errorf("expected %s\ngot      %s", want, got)
	}
}

func TestCompileAsync(t *testing.T) {
	logger := &recordingLogger{}
	var loaded []string
	a := New(Options{Logger: logger, LoadSchema: func(uri string) (any, error) {
		loaded = append(loaded, uri)
		if uri == "http://l/b" {
			return mustParse(t, `{"type":"string"}`), nil
		}
		return nil, errors.New("no " + uri)
	}})
	_, err := a.CompileAsync(mustParse(t, `{"$id":"http://l/a","properties":{"b":{"$ref":"b"},"c":{"$ref":"c"}}}`))
	expectErr(t, "compileAsync", err, "no http://l/c")
	if len(loaded) != 2 || loaded[0] != "http://l/b" || loaded[1] != "http://l/c" {
		t.Errorf("loaded: got %v", loaded)
	}
	want := `strict mode: missing type "object" for keyword "properties" at "http://l/a#" (strictTypes)`
	if len(logger.lines) != 2 || logger.lines[0] != want || logger.lines[1] != want {
		t.Errorf("logged: got %q", logger.lines)
	}
	_, err = New(Options{}).CompileAsync(true)
	expectErr(t, "no loadSchema", err, "options.loadSchema should be a function")
}

func TestAddKeyword(t *testing.T) {
	logger := &recordingLogger{}
	a := New(Options{Logger: logger})
	err := a.AddKeyword(KeywordDefinition{
		Keyword: "even",
		Type:    "number",
		Validate: func(schema, data, parentSchema any, cxt DataValidationCxt) (bool, []*ErrorObject) {
			n, _ := data.(float64)
			return int(n)%2 == 0, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := a.Compile(mustParse(t, `{"even":true,"minimum":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if !v.Validate(4.0) || v.Validate(5.0) {
		t.Error("unexpected validation results")
	}
	if got, want := errorsJSON(v.Errors()), `[{"instancePath":"","schemaPath":"#/even","keyword":"even","params":{},"message":"must pass \"even\" keyword validation"}]`; got != want {
		t.Errorf("errors: expected %s, got %s", want, got)
	}
	if len(logger.lines) != 2 {
		t.Errorf("expected two strictTypes warnings, got %q", logger.lines)
	}
	expectErr(t, "redefinition", a.AddKeyword(KeywordDefinition{Keyword: "even"}), "Keyword even is already defined")
	expectErr(t, "prototype name", a.AddKeyword(KeywordDefinition{Keyword: "toString"}), "Keyword toString is already defined")
}

func TestUnevaluatedAllErrors(t *testing.T) {
	a := New2020(Options{AllErrors: true})
	v, err := a.Compile(mustParse(t, `{"type":"object","properties":{"a":{"type":"integer"}},"unevaluatedProperties":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Validate(mustParse(t, `{"a":1.5,"b":1}`)) {
		t.Error("expected invalid")
	}
	want := `[{"instancePath":"/a","schemaPath":"#/properties/a/type","keyword":"type","params":{"type":"integer"},"message":"must be integer"},` +
		`{"instancePath":"","schemaPath":"#/unevaluatedProperties","keyword":"unevaluatedProperties","params":{"unevaluatedProperty":"b"},"message":"must NOT have unevaluated properties"}]`
	if got := errorsJSON(v.Errors()); got != want {
		t.Errorf("expected %s\ngot      %s", want, got)
	}
}
