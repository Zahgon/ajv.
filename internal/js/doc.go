// Package js reproduces the JavaScript semantics of V8 (as shipped in Node.js 24, ICU 78.3,
// Unicode 17.0) that the validator relies on: String(number), Number(string) and parseInt,
// Math.round, JSON.parse and the JSON quoting of strings, String.prototype.toLowerCase, trim,
// charAt and string comparison, and objects with JavaScript's property order.
//
// # String model
//
// A JavaScript string is represented by a Go string holding its WTF-8 encoding: UTF-8 in which a
// lone (unpaired) surrogate code unit is stored as its three-byte generalised UTF-8 form
// (0xED 0xA0..0xBF 0x80..0xBF). Every index, length and position used by this package counts
// UTF-16 code units, as JavaScript does. Functions that build strings always produce canonical
// WTF-8, where a high surrogate followed by a low surrogate is encoded as the four-byte form of
// the supplementary code point. Inputs that join a lone high and a lone low surrogate byte form
// (for example with Go's + operator) are still read as the code unit sequence they spell, and
// bytes that are not WTF-8 at all are read as U+FFFD.
//
// # Errors
//
// JSON parsing returns its SyntaxError as an *Error carrying the JavaScript error name and
// message.
package js
