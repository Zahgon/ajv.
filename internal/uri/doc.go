// Package uri is a Go port of the npm package fast-uri 3.1.9, the URI library Ajv 8.20.0 uses
// to parse, serialize and resolve schema identifiers.
//
// Ported from fast-uri 3.1.9 (BSD-3-Clause, Copyright (c) 2011-2021, Gary Court until
// https://github.com/garycourt/uri-js/commit/a1acf730b4bba3f1097c9f52e7d9d3aba8cdcaae;
// Copyright (c) 2021-present The Fastify team <https://github.com/fastify/fastify#team>).
// The full licence text follows the package clause below. (The package.json of 3.1.9 declares BSD-3-Clause;
// its LICENSE file is the BSD 3-clause text.)
//
// The port reproduces fast-uri's behaviour exactly for the calls Ajv makes, all without
// options: parse(uri), serialize(components) and resolve(base, ref), including the built-in
// http, https, ws, wss, urn and urn:uuid scheme handlers. fast-uri hands the hosts of the
// domain-host schemes (http, https, ws, wss) that are not plain lowercase ASCII names to
// `new URL`; that WHATWG host parsing (percent-decoding, UTS #46 IDNA processing with
// punycode, and IPv4 number parsing) is reproduced from ada-url 4.0.0, the URL parser of
// Node.js 24.22.1 and 26.10, with its Unicode 17 tables.
//
// Strings follow the model of package internal/js: a Go string holds the WTF-8 form of a
// JavaScript string, so a lone surrogate is a three-byte sequence; bytes that are not WTF-8
// are read as U+FFFD. Where fast-uri throws, TrySerialize and TryResolve return the exception
// as a *js.Error. No function panics.
//
// Behaviour was verified differentially against fast-uri running in Node.js 26.10 and
// 24.22.1, with the vectors in testdata (see vectors_test.go).
package uri

// The licence of fast-uri 3.1.9, which this package is ported from, reproduced as its BSD
// 3-clause terms require for redistributions of source code:
//
// Copyright (c) 2011-2021, Gary Court until https://github.com/garycourt/uri-js/commit/a1acf730b4bba3f1097c9f52e7d9d3aba8cdcaae
// Copyright (c) 2021-present The Fastify team <https://github.com/fastify/fastify#team>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//     * Redistributions of source code must retain the above copyright
//       notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above copyright
//       notice, this list of conditions and the following disclaimer in the
//       documentation and/or other materials provided with the distribution.
//     * The names of any contributors may not be used to endorse or promote
//       products derived from this software without specific prior written
//       permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDERS AND CONTRIBUTORS BE LIABLE FOR ANY
// DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//
//                                   *   *   *
//
// The complete list of contributors can be found at:
// - https://github.com/garycourt/uri-js/graphs/contributors
