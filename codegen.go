// Ported from Ajv 8.20.0, lib/compile/codegen, MIT, Copyright (c) 2015-2021 Evgeny Poberezkin

package ajv

import (
	"strconv"
)

// Ajv generates the source of a JavaScript validation function with its CodeGen builder. This
// port keeps the builder's interface (if/else/endIf, block/endBlock, for loops, let/const/var,
// assign, break, return) and the shape of the tree it builds, but the tree's nodes are executed
// directly by a small interpreter instead of being rendered to source. Expressions are jsCode
// values: a *jsName (a variable of the function), an exprFn closure or a literal value.

// jsName is a variable of a generated function.
type jsName struct {
	str  string
	slot int
}

func (n *jsName) String() string { return n.str }

// jsCode is an expression of generated code: *jsName, exprFn, emptyCode or a literal JavaScript
// value (bool, float64, string, nil for null, undefined).
type jsCode = any

// exprFn is an expression evaluated in a frame.
type exprFn func(f *frame) any

// emptyCode is Ajv's `nil` code: the absence of an expression.
type emptyCode struct{}

var nilCode jsCode = emptyCode{}

func isNil(c jsCode) bool {
	_, ok := c.(emptyCode)
	return ok
}

// eval evaluates the expression c in frame f.
func eval(c jsCode, f *frame) any {
	switch x := c.(type) {
	case *jsName:
		return f.vars[x.slot]
	case exprFn:
		return x(f)
	case emptyCode:
		return undefined
	case int:
		return float64(x)
	}
	return c
}

// not is codegen's not(x).
func not(c jsCode) jsCode {
	switch x := c.(type) {
	case bool:
		return !x
	case float64:
		return !truthy(x)
	case int:
		return x == 0
	case nil:
		return true
	}
	return exprFn(func(f *frame) any { return !truthy(eval(c, f)) })
}

// and is codegen's and(...args): JavaScript && skipping nil codes.
func and(args ...jsCode) jsCode {
	res := nilCode
	for _, a := range args {
		res = mappend(res, a, true)
	}
	return res
}

// or is codegen's or(...args): JavaScript || skipping nil codes.
func or(args ...jsCode) jsCode {
	res := nilCode
	for _, a := range args {
		res = mappend(res, a, false)
	}
	return res
}

func mappend(x, y jsCode, isAnd bool) jsCode {
	if isNil(x) {
		return y
	}
	if isNil(y) {
		return x
	}
	return exprFn(func(f *frame) any {
		v := eval(x, f)
		if truthy(v) == isAnd {
			return eval(y, f)
		}
		return v
	})
}

// condition evaluates c as an if condition.
func condition(c jsCode, f *frame) bool { return truthy(eval(c, f)) }

// ctl is the completion of a statement.
type ctl uint8

const (
	ctlNormal ctl = iota
	ctlBreak
	ctlContinue
	ctlReturn
	// ctlBreakOuter is a labeled break leaving the loop it was executed in.
	ctlBreakOuter
)

// frame is one invocation of a generated function.
type frame struct {
	vars  []any
	fn    *ValidateFunction
	label *jsName // label of a pending labeled break
	depth int     // nesting of the function call
	ret   any
}

func (f *frame) get(n *jsName) any     { return f.vars[n.slot] }
func (f *frame) set(n *jsName, v any)  { f.vars[n.slot] = v }
func (f *frame) num(n *jsName) float64 { return toNumber(f.vars[n.slot]) }

// node is a statement of generated code.
type node interface {
	exec(f *frame) ctl
}

// parentNode is a statement holding a list of statements.
type parentNode interface {
	node
	append(n node)
}

type nodeList struct{ nodes []node }

func (l *nodeList) append(n node) { l.nodes = append(l.nodes, n) }

func (l *nodeList) exec(f *frame) ctl {
	for _, n := range l.nodes {
		if c := n.exec(f); c != ctlNormal {
			return c
		}
	}
	return ctlNormal
}

type rootNode struct{ nodeList }

type elseNode struct{ nodeList }

type ifNode struct {
	nodeList
	cond jsCode
	els  parentNode // *ifNode or *elseNode
}

func (n *ifNode) exec(f *frame) ctl {
	if condition(n.cond, f) {
		return n.nodeList.exec(f)
	}
	if n.els != nil {
		return n.els.exec(f)
	}
	return ctlNormal
}

// loopNode is a for statement: run implements its initialization, test and update around the
// body, which it runs through the callback.
type loopNode struct {
	nodeList
	label *jsName
	run   func(f *frame, body func() ctl) ctl
}

func (n *loopNode) exec(f *frame) ctl {
	return n.run(f, func() ctl {
		c := n.nodeList.exec(f)
		switch c {
		case ctlContinue:
			return ctlNormal
		case ctlBreak:
			if f.label == nil || f.label == n.label {
				f.label = nil
				return ctlBreak
			}
			return ctlBreakOuter
		}
		return c
	})
}

// loopResult maps the completion of a loop body to the completion of the loop: done reports
// whether the loop stops and the returned ctl is the loop statement's completion.
func loopResult(c ctl) (done bool, res ctl) {
	switch c {
	case ctlNormal:
		return false, ctlNormal
	case ctlBreak:
		return true, ctlNormal
	case ctlBreakOuter:
		return true, ctlBreak
	}
	return true, c
}

type defNode struct {
	name   *jsName
	rhs    jsCode
	hasRhs bool
	isVar  bool
}

func (n *defNode) exec(f *frame) ctl {
	if n.hasRhs {
		f.vars[n.name.slot] = eval(n.rhs, f)
	} else if !n.isVar {
		f.vars[n.name.slot] = undefined
	}
	return ctlNormal
}

type assignNode struct {
	name *jsName
	rhs  jsCode
}

func (n *assignNode) exec(f *frame) ctl {
	f.vars[n.name.slot] = eval(n.rhs, f)
	return ctlNormal
}

// codeNode is an expression statement or any other statement with side effects only.
type codeNode struct{ fn func(f *frame) }

func (n *codeNode) exec(f *frame) ctl {
	n.fn(f)
	return ctlNormal
}

type breakNode struct{ label *jsName }

func (n *breakNode) exec(f *frame) ctl {
	f.label = n.label
	return ctlBreak
}

type continueNode struct{}

func (continueNode) exec(*frame) ctl { return ctlContinue }

type returnNode struct{ value jsCode }

func (n *returnNode) exec(f *frame) ctl {
	f.ret = eval(n.value, f)
	return ctlReturn
}

// codeGen builds the statement tree of one generated function.
type codeGen struct {
	prefixes     map[string]int
	nslots       int
	root         *rootNode
	nodes        []parentNode
	blockStarts  []int
	pendingLabel *jsName
}

func newCodeGen() *codeGen {
	root := &rootNode{}
	g := &codeGen{prefixes: map[string]int{}, root: root, nodes: []parentNode{root}}
	g.nslots = numFixedNames
	return g
}

// Fixed names: the arguments and the function-scoped variables every function has.
const (
	slotData = iota
	slotInstancePath
	slotParentData
	slotParentDataProperty
	slotRootData
	slotDynamicAnchors
	slotVErrors
	slotErrors
	numFixedNames
)

var names = struct {
	data, instancePath, parentData, parentDataProperty, rootData, dynamicAnchors, vErrors, errors *jsName
}{
	data:               &jsName{"data", slotData},
	instancePath:       &jsName{"instancePath", slotInstancePath},
	parentData:         &jsName{"parentData", slotParentData},
	parentDataProperty: &jsName{"parentDataProperty", slotParentDataProperty},
	rootData:           &jsName{"rootData", slotRootData},
	dynamicAnchors:     &jsName{"dynamicAnchors", slotDynamicAnchors},
	vErrors:            &jsName{"vErrors", slotVErrors},
	errors:             &jsName{"errors", slotErrors},
}

// name returns a new variable named prefix followed by a counter.
func (g *codeGen) name(prefix string) *jsName {
	i := g.prefixes[prefix]
	g.prefixes[prefix] = i + 1
	n := &jsName{str: prefix + strconv.Itoa(i), slot: g.nslots}
	g.nslots++
	return n
}

func (g *codeGen) toName(nameOrPrefix any) *jsName {
	if n, ok := nameOrPrefix.(*jsName); ok {
		return n
	}
	return g.name(nameOrPrefix.(string))
}

func (g *codeGen) curr() parentNode { return g.nodes[len(g.nodes)-1] }

func (g *codeGen) leaf(n node) *codeGen {
	g.curr().append(n)
	return g
}

func (g *codeGen) blockNode(n parentNode) {
	g.curr().append(n)
	g.nodes = append(g.nodes, n)
}

func (g *codeGen) def(nameOrPrefix any, rhs jsCode, hasRhs, isVar bool) *jsName {
	n := g.toName(nameOrPrefix)
	g.leaf(&defNode{name: n, rhs: rhs, hasRhs: hasRhs, isVar: isVar})
	return n
}

// constDef is gen.const(nameOrPrefix, rhs).
func (g *codeGen) constDef(nameOrPrefix any, rhs jsCode) *jsName {
	return g.def(nameOrPrefix, rhs, true, false)
}

// let is gen.let(nameOrPrefix, rhs).
func (g *codeGen) let(nameOrPrefix any, rhs jsCode) *jsName {
	return g.def(nameOrPrefix, rhs, true, false)
}

// letNoInit is gen.let(nameOrPrefix) without an initializer.
func (g *codeGen) letNoInit(nameOrPrefix any) *jsName { return g.def(nameOrPrefix, nil, false, false) }

// varDef is gen.var(nameOrPrefix, rhs).
func (g *codeGen) varDef(nameOrPrefix any, rhs jsCode) *jsName {
	return g.def(nameOrPrefix, rhs, true, true)
}

// assign is gen.assign(name, rhs).
func (g *codeGen) assign(n *jsName, rhs jsCode) *codeGen {
	return g.leaf(&assignNode{name: n, rhs: rhs})
}

// code adds a statement with side effects.
func (g *codeGen) code(fn func(f *frame)) *codeGen { return g.leaf(&codeNode{fn: fn}) }

// block runs body in a self-balancing block: every node it leaves open is closed at its end.
func (g *codeGen) block(body func()) *codeGen {
	g.blockStarts = append(g.blockStarts, len(g.nodes))
	if body != nil {
		body()
		g.endBlock()
	}
	return g
}

func (g *codeGen) endBlock() *codeGen {
	l := g.blockStarts[len(g.blockStarts)-1]
	g.blockStarts = g.blockStarts[:len(g.blockStarts)-1]
	g.nodes = g.nodes[:l]
	return g
}

// ifThen is gen.if(condition): it opens an if block.
func (g *codeGen) ifThen(cond jsCode) *codeGen {
	g.blockNode(&ifNode{cond: cond})
	return g
}

// ifElse is gen.if(condition, thenBody, elseBody) with bodies; elseBody may be nil.
func (g *codeGen) ifElse(cond jsCode, thenBody, elseBody func()) *codeGen {
	g.ifThen(cond)
	thenBody()
	if elseBody != nil {
		g.elseBranch()
		elseBody()
	}
	return g.endIf()
}

func (g *codeGen) elseNode(n parentNode) *codeGen {
	in, ok := g.curr().(*ifNode)
	if !ok {
		panic(newError(`CodeGen: "else" without "if"`))
	}
	in.els = n
	g.nodes[len(g.nodes)-1] = n
	return g
}

// elseIf is gen.elseIf(condition).
func (g *codeGen) elseIf(cond jsCode) *codeGen { return g.elseNode(&ifNode{cond: cond}) }

// elseBranch is gen.else().
func (g *codeGen) elseBranch() *codeGen { return g.elseNode(&elseNode{}) }

// endIf is gen.endIf().
func (g *codeGen) endIf() *codeGen {
	switch g.curr().(type) {
	case *ifNode, *elseNode:
		g.nodes = g.nodes[:len(g.nodes)-1]
		return g
	}
	panic(newError(`CodeGen: not in block "if/else"`))
}

func (g *codeGen) forLoop(run func(f *frame, body func() ctl) ctl, body func()) *codeGen {
	n := &loopNode{label: g.pendingLabel, run: run}
	g.pendingLabel = nil
	g.blockNode(n)
	body()
	g.nodes = g.nodes[:len(g.nodes)-1]
	return g
}

// forRange is gen.forRange(prefix, from, to, body): for(let i=from; i<to; i++).
func (g *codeGen) forRange(prefix string, from, to jsCode, body func(i *jsName)) *codeGen {
	i := g.name(prefix)
	return g.forLoop(func(f *frame, run func() ctl) ctl {
		f.set(i, eval(from, f))
		for compare(f.get(i), opLT, eval(to, f)) {
			if done, res := loopResult(run()); done {
				return res
			}
			f.set(i, toNumber(f.get(i))+1)
		}
		return ctlNormal
	}, func() { body(i) })
}

// forOf is gen.forOf(nameOrPrefix, iterable, body): for(const x of iterable).
func (g *codeGen) forOf(nameOrPrefix any, iterable jsCode, body func(x *jsName)) *codeGen {
	x := g.toName(nameOrPrefix)
	return g.forLoop(func(f *frame, run func() ctl) ctl {
		for _, item := range iterate(eval(iterable, f)) {
			f.set(x, item)
			if done, res := loopResult(run()); done {
				return res
			}
		}
		return ctlNormal
	}, func() { body(x) })
}

// iterate returns the values a for-of loop visits.
func iterate(v any) []any {
	switch x := v.(type) {
	case []any:
		return append([]any(nil), x...)
	case string:
		var out []any
		for _, r := range x {
			out = append(out, string(r))
		}
		return out
	}
	return nil
}

// forIn is gen.forIn(prefix, obj, body): for(const key in obj), or for(const key of
// Object.keys(obj)) with the ownProperties option.
func (g *codeGen) forIn(prefix string, obj jsCode, ownProperties bool, body func(key *jsName)) *codeGen {
	key := g.name(prefix)
	return g.forLoop(func(f *frame, run func() ctl) ctl {
		o := eval(obj, f)
		for _, k := range objectKeys(o) {
			if !ownProperties {
				// for-in skips properties deleted before they are visited.
				if obj, ok := o.(*Object); ok && !obj.has(k) {
					continue
				}
			}
			f.set(key, k)
			if done, res := loopResult(run()); done {
				return res
			}
		}
		return ctlNormal
	}, func() { body(key) })
}

// label is gen.label(name): the next loop is labeled.
func (g *codeGen) label(n *jsName) *codeGen {
	g.pendingLabel = n
	return g
}

// breakStmt is gen.break(label).
func (g *codeGen) breakStmt(label *jsName) *codeGen { return g.leaf(&breakNode{label: label}) }

// continueStmt is the `continue` statement.
func (g *codeGen) continueStmt() *codeGen { return g.leaf(continueNode{}) }

// returnStmt is gen.return(value).
func (g *codeGen) returnStmt(value jsCode) *codeGen { return g.leaf(&returnNode{value: value}) }
