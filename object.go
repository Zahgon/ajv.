package ajv

import (
	"errors"
	"math"
	"strconv"

	"github.com/ajv-validator/ajv-go/internal/js"
)

// Object is a JSON object. Its keys keep JavaScript's property order: keys that are canonical
// array indices ("0" .. "4294967294") come first in ascending numeric order, then the other keys
// in insertion order. Setting an existing key keeps its position. The zero value is an empty
// object.
type Object struct{ m js.Object }

// NewObject returns an empty object.
func NewObject() *Object { return &Object{} }

// Set adds or replaces a property.
func (o *Object) Set(key string, value any) { o.m.Set(key, value) }

// Get returns a property and whether it exists.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	return o.m.Get(key)
}

// Delete removes a property.
func (o *Object) Delete(key string) {
	if o != nil {
		o.m.Delete(key)
	}
}

// Keys returns the property names in order.
func (o *Object) Keys() []string {
	if o == nil {
		return []string{}
	}
	return o.m.Keys()
}

// Len returns the number of properties.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return o.m.Len()
}

// has reports whether the object has the own property key.
func (o *Object) has(key string) bool {
	_, ok := o.Get(key)
	return ok
}

// value returns the own property key, or undefined.
func (o *Object) value(key string) any {
	if v, ok := o.Get(key); ok {
		return v
	}
	return undefined
}

// newObjectKV returns an object with the given key/value pairs, skipping undefined values.
func newObjectKV(kv ...any) *Object {
	o := NewObject()
	for i := 0; i+1 < len(kv); i += 2 {
		if !isUndefined(kv[i+1]) {
			o.Set(kv[i].(string), kv[i+1])
		}
	}
	return o
}

// ParseJSON parses JSON text the way JSON.parse does: objects become *Object, arrays []any,
// numbers float64. A malformed text returns an error carrying V8's SyntaxError message.
func ParseJSON(text string) (any, error) {
	v, err := js.Parse(text)
	if err != nil {
		var jsErr *js.Error
		if errors.As(err, &jsErr) {
			return nil, errors.New(jsErr.Message)
		}
		return nil, err
	}
	return fromJS(v), nil
}

// mustParseJSON parses an embedded JSON document.
func mustParseJSON(text string) any {
	v, err := ParseJSON(text)
	if err != nil {
		panic("ajv: invalid embedded JSON: " + err.Error())
	}
	return v
}

// fromJS converts a value produced by js.Parse into the package's value model.
func fromJS(v any) any {
	switch x := v.(type) {
	case *js.Object:
		o := NewObject()
		x.Range(func(k string, e any) bool {
			o.Set(k, fromJS(e))
			return true
		})
		return o
	case js.Array:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = fromJS(e)
		}
		return arr
	}
	return v
}

// Stringify renders a value the way JSON.stringify does. Undefined members of objects are
// omitted and undefined array elements are written as null; for an undefined value it returns
// the empty string.
func Stringify(value any) string {
	if isUndefined(value) || typeOf(value) == "function" {
		return ""
	}
	return string(appendJSON(nil, value))
}

func appendJSON(b []byte, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(b, "null"...)
	case bool:
		return strconv.AppendBool(b, x)
	case string:
		return js.AppendQuote(b, x)
	case *Object:
		if x == nil {
			return append(b, "null"...)
		}
		b = append(b, '{')
		first := true
		for _, k := range x.Keys() {
			e := x.value(k)
			if isUndefined(e) || typeOf(e) == "function" {
				continue
			}
			if !first {
				b = append(b, ',')
			}
			first = false
			b = js.AppendQuote(b, k)
			b = append(b, ':')
			b = appendJSON(b, e)
		}
		return append(b, '}')
	case []any:
		b = append(b, '[')
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			if isUndefined(e) || typeOf(e) == "function" {
				b = append(b, "null"...)
				continue
			}
			b = appendJSON(b, e)
		}
		return append(b, ']')
	}
	if n, ok := asNumber(v); ok {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return append(b, "null"...)
		}
		return append(b, js.NumberToString(n)...)
	}
	return append(b, "null"...)
}

// deepCopyJSON is JSON.parse(JSON.stringify(v)).
func deepCopyJSON(v any) any {
	switch x := v.(type) {
	case *Object:
		o := NewObject()
		for _, k := range x.Keys() {
			e := x.value(k)
			if isUndefined(e) || typeOf(e) == "function" {
				continue
			}
			o.Set(k, deepCopyJSON(e))
		}
		return o
	case []any:
		arr := make([]any, len(x))
		for i, e := range x {
			if isUndefined(e) || typeOf(e) == "function" {
				arr[i] = nil
				continue
			}
			arr[i] = deepCopyJSON(e)
		}
		return arr
	}
	if n, ok := asNumber(v); ok {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil
		}
		if n == 0 {
			return float64(0)
		}
		return n
	}
	return v
}
