package ajv

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// nodeCase is a case of testdata/node_cases.json, whose expected values were recorded with the
// original Ajv 8.20.0 under Node.js.
type nodeCase struct {
	Name     string         `json:"name"`
	Cls      string         `json:"cls"`
	Opts     map[string]any `json:"opts"`
	Schema   string         `json:"schema"`
	Data     []string       `json:"data"`
	Compile  string         `json:"compile"`
	Warnings []string       `json:"warnings"`
	Results  []struct {
		Valid  bool   `json:"valid"`
		Errors string `json:"errors"`
		Data   string `json:"data"`
	} `json:"results"`
}

// recordingLogger joins the arguments of each call with spaces, as the recorder did.
type recordingLogger struct{ lines []string }

func (l *recordingLogger) Log(...any) {}
func (l *recordingLogger) Warn(args ...any) {
	l.lines = append(l.lines, fmt.Sprint(args...))
}
func (l *recordingLogger) Error(args ...any) {
	l.lines = append(l.lines, fmt.Sprint(args...))
}

func optionsFrom(t *testing.T, m map[string]any, logger Logger) Options {
	o := Options{Logger: logger}
	for k, v := range m {
		switch k {
		case "allErrors":
			o.AllErrors = v.(bool)
		case "allowUnionTypes":
			o.AllowUnionTypes = v.(bool)
		case "strictNumbers":
			o.StrictNumbers = v
		case "strictTuples":
			o.StrictTuples = v
		case "strictTypes":
			o.StrictTypes = v
		case "strictRequired":
			o.StrictRequired = v
		case "coerceTypes":
			o.CoerceTypes = v
		case "multipleOfPrecision":
			o.MultipleOfPrecision = int(v.(float64))
		case "ownProperties":
			o.OwnProperties = v.(bool)
		case "inlineRefs":
			o.InlineRefs = v
		case "useDefaults":
			o.UseDefaults = v
		case "removeAdditional":
			o.RemoveAdditional = v
		case "discriminator":
			o.Discriminator = v.(bool)
		case "$data":
			o.Data = v.(bool)
		case "verbose":
			o.Verbose = v.(bool)
		case "messages":
			o.Messages = v
		default:
			t.Fatalf("unsupported option %s", k)
		}
	}
	return o
}

// errorsJSON renders errors as the recorder normalized them.
func errorsJSON(errs []*ErrorObject) string {
	if errs == nil {
		return "null"
	}
	parts := make([]string, len(errs))
	for i, e := range errs {
		o := NewObject()
		o.Set("instancePath", e.InstancePath)
		o.Set("schemaPath", e.SchemaPath)
		o.Set("keyword", e.Keyword)
		if e.Params != nil {
			o.Set("params", e.Params)
		}
		if e.Message != "" {
			o.Set("message", e.Message)
		}
		if e.PropertyName != nil {
			o.Set("propertyName", *e.PropertyName)
		}
		parts[i] = Stringify(o)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func newInstance(cls string, opts Options) *Ajv {
	switch cls {
	case "Ajv2019":
		return New2019(opts)
	case "Ajv2020":
		return New2020(opts)
	}
	return New(opts)
}

func TestAgainstNode(t *testing.T) {
	raw, err := os.ReadFile("testdata/node_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []nodeCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			logger := &recordingLogger{}
			a := newInstance(c.Cls, optionsFrom(t, c.Opts, logger))
			schema, err := ParseJSON(c.Schema)
			if err != nil {
				t.Fatal(err)
			}
			v, err := a.Compile(schema)
			got := "ok"
			if err != nil {
				got = err.Error()
			}
			if got != c.Compile {
				t.Fatalf("compile: expected %q, got %q", c.Compile, got)
			}
			if v != nil {
				for i, d := range c.Data {
					data, err := ParseJSON(d)
					if err != nil {
						t.Fatal(err)
					}
					valid := v.Validate(data)
					want := c.Results[i]
					if valid != want.Valid {
						t.Errorf("data %s: valid: expected %v, got %v", d, want.Valid, valid)
					}
					if e := errorsJSON(v.Errors()); e != want.Errors {
						t.Errorf("data %s: errors:\nexpected %s\ngot      %s", d, want.Errors, e)
					}
					if s := Stringify(data); s != want.Data {
						t.Errorf("data %s: data after validation: expected %s, got %s", d, want.Data, s)
					}
				}
			}
			if strings.Join(logger.lines, "\n") != strings.Join(c.Warnings, "\n") {
				t.Errorf("logged:\nexpected %q\ngot      %q", c.Warnings, logger.lines)
			}
		})
	}
}
