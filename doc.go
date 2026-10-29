// Package ajv is a Go port of Ajv 8.20.0, the JSON Schema validator (draft-07, 2019-09 and
// 2020-12), that reproduces the original's observable behaviour exactly: the validation
// result, the error objects and their order, error messages, compilation errors, logged
// warnings and the changes made to validated data.
//
// # Values
//
// JSON values are represented as nil (null), bool, float64, string, []any and *Object (an
// object keeping JavaScript's property order). ParseJSON and Stringify convert them from and
// to JSON text as JSON.parse and JSON.stringify do.
//
// # Design
//
// Ajv compiles a schema into the source of a JavaScript function. This port walks the schema
// in the same order Ajv's code generator does, so compile-time errors, strict mode warnings and
// references are produced in the same order, and builds the same statement tree, which is then
// executed directly. Expressions of the generated code are Go closures that implement the
// JavaScript semantics the original relies on (typeof, ==, ===, String(), property lookup
// through prototypes, ECMAScript regular expressions).
//
// Ported from Ajv 8.20.0 (https://github.com/ajv-validator/ajv), MIT License,
// Copyright (c) 2015-2021 Evgeny Poberezkin.
package ajv
