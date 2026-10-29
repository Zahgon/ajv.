package js

import (
	"sort"
	"strconv"
)

type undefinedType struct{}

// Undefined is the JavaScript undefined value.
var Undefined any = undefinedType{}

// IsUndefined reports whether v is Undefined.
func IsUndefined(v any) bool {
	_, ok := v.(undefinedType)
	return ok
}

// Array is a JavaScript array.
type Array []any

// Object is a plain JavaScript object with string keys, iterating in own-property order: keys
// that are canonical array indices ("0" .. "4294967294") first in ascending numeric order, then
// the other keys in insertion order. Setting an existing key keeps its position; deleting and
// setting it again moves a non-index key to the end. The zero value is an empty object.
type Object struct {
	vals    map[string]any
	indices []uint32
	names   []string
}

// NewObject returns an empty object.
func NewObject() *Object { return &Object{} }

// ArrayIndex reports whether key is a canonical array index and returns its value.
func ArrayIndex(key string) (uint32, bool) {
	if key == "" || len(key) > 10 || (key[0] == '0' && len(key) > 1) {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	if n >= 4294967295 {
		return 0, false
	}
	return uint32(n), true
}

// Set sets the property key to v.
func (o *Object) Set(key string, v any) {
	if o.vals == nil {
		o.vals = map[string]any{}
	}
	if _, ok := o.vals[key]; !ok {
		if n, isIndex := ArrayIndex(key); isIndex {
			i := sort.Search(len(o.indices), func(i int) bool { return o.indices[i] >= n })
			o.indices = append(o.indices, 0)
			copy(o.indices[i+1:], o.indices[i:])
			o.indices[i] = n
		} else {
			o.names = append(o.names, key)
		}
	}
	o.vals[key] = v
}

// Get returns the value of the property key and whether it exists.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.vals[key]
	return v, ok
}

// Delete removes the property key, if present.
func (o *Object) Delete(key string) {
	if o == nil {
		return
	}
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	if n, isIndex := ArrayIndex(key); isIndex {
		i := sort.Search(len(o.indices), func(i int) bool { return o.indices[i] >= n })
		o.indices = append(o.indices[:i], o.indices[i+1:]...)
		return
	}
	for i, k := range o.names {
		if k == key {
			o.names = append(o.names[:i], o.names[i+1:]...)
			return
		}
	}
}

// Len returns the number of properties.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.vals)
}

// Keys returns the property keys in own-property order (Object.keys).
func (o *Object) Keys() []string {
	if o == nil {
		return []string{}
	}
	out := make([]string, 0, len(o.vals))
	for _, n := range o.indices {
		out = append(out, strconv.FormatUint(uint64(n), 10))
	}
	return append(out, o.names...)
}

// Range calls fn for each property in own-property order until fn returns false.
func (o *Object) Range(fn func(key string, v any) bool) {
	for _, k := range o.Keys() {
		if !fn(k, o.vals[k]) {
			return
		}
	}
}
