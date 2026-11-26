package metrics

import (
	"strconv"

	"mvdan.cc/sh/v3/syntax"
)

// walker visits the statements of one function (or the top level) and
// accumulates its measures. Function declarations it meets start a walker of
// their own.
type walker struct {
	file *File
	fn   *Function
	// condition is set while visiting the condition of an if, while or
	// until: a test there is the structure's own, not a hidden if.
	condition bool
}

func (w *walker) add(pos syntax.Pos, kind string, nesting int, nests bool) {
	cost := 1
	if nests {
		cost += nesting
	} else {
		nesting = 0
	}
	w.fn.Cognitive += cost
	w.fn.Increments = append(w.fn.Increments, Increment{
		Line: int(pos.Line()), Col: int(pos.Col()), Kind: kind, Nesting: nesting, Cost: cost,
	})
}

// enter records that the control structure at pos opens a body at depth
// nesting+1.
func (w *walker) enter(pos syntax.Pos, nesting int) {
	if nesting+1 > w.fn.MaxNesting {
		w.fn.MaxNesting = nesting + 1
		w.fn.MaxNestingLine = int(pos.Line())
	}
}

func (w *walker) stmts(stmts []*syntax.Stmt, nesting int) {
	for _, s := range stmts {
		w.stmt(s, nesting)
	}
}

func (w *walker) stmt(s *syntax.Stmt, nesting int) {
	if s == nil {
		return
	}
	for _, r := range s.Redirs {
		w.nested(r, nesting)
	}
	w.command(s.Cmd, nesting)
}

func (w *walker) command(cmd syntax.Command, nesting int) {
	switch c := cmd.(type) {
	case *syntax.IfClause:
		w.ifClause(c, nesting)
	case *syntax.WhileClause:
		kind := "while"
		if c.Until {
			kind = "until"
		}
		w.add(c.WhilePos, kind, nesting, true)
		w.fn.Cyclomatic++
		if unbounded(c) {
			w.fn.UnboundedLoops = append(w.fn.UnboundedLoops, Position{Line: int(c.WhilePos.Line()), Col: int(c.WhilePos.Col())})
		}
		w.conditions(c.Cond, nesting)
		w.enter(c.WhilePos, nesting)
		w.stmts(c.Do, nesting+1)
	case *syntax.ForClause:
		kind := "for"
		if c.Select {
			kind = "select"
		}
		w.add(c.ForPos, kind, nesting, true)
		w.fn.Cyclomatic++
		w.nested(c.Loop, nesting)
		w.enter(c.ForPos, nesting)
		w.stmts(c.Do, nesting+1)
	case *syntax.CaseClause:
		w.add(c.Case, "case", nesting, true)
		w.nested(c.Word, nesting)
		w.enter(c.Case, nesting)
		for _, item := range c.Items {
			if !isCatchAll(item) {
				w.fn.Cyclomatic++
			}
			w.stmts(item.Stmts, nesting+1)
		}
	case *syntax.BinaryCmd:
		if c.Op == syntax.AndStmt || c.Op == syntax.OrStmt {
			w.list(c, nesting)
			return
		}
		w.stmt(c.X, nesting)
		w.stmt(c.Y, nesting)
	case *syntax.Block:
		w.stmts(c.Stmts, nesting)
	case *syntax.Subshell:
		w.stmts(c.Stmts, nesting)
	case *syntax.FuncDecl:
		w.funcDecl(c)
	case *syntax.TestClause:
		w.test(c.X, nesting)
	case *syntax.ArithmCmd:
		w.arithm(c.X, nesting)
	case *syntax.LetClause:
		for _, x := range c.Exprs {
			w.arithm(x, nesting)
		}
	case *syntax.TimeClause:
		w.stmt(c.Stmt, nesting)
	case *syntax.CoprocClause:
		w.stmt(c.Stmt, nesting)
	case *syntax.CallExpr:
		w.call(c, nesting)
	case nil:
	default:
		w.nested(c, nesting)
	}
}

// ifClause scores an if and its elif / else chain: the if is a structure
// (+1 + nesting), each elif and the else a hybrid increment (+1).
func (w *walker) ifClause(c *syntax.IfClause, nesting int) {
	w.add(c.Position, "if", nesting, true)
	w.fn.Cyclomatic++
	w.conditions(c.Cond, nesting)
	w.enter(c.Position, nesting)
	w.stmts(c.Then, nesting+1)
	for e := c.Else; e != nil; e = e.Else {
		if e.ThenPos.IsValid() {
			w.add(e.Position, "elif", nesting, false)
			w.fn.Cyclomatic++
			w.conditions(e.Cond, nesting)
		} else {
			w.add(e.Position, "else", nesting, false)
		}
		w.stmts(e.Then, nesting+1)
	}
}

func (w *walker) conditions(stmts []*syntax.Stmt, nesting int) {
	saved := w.condition
	w.condition = true
	w.stmts(stmts, nesting)
	w.condition = saved
}

// list scores a command list (a && b || c).
//
//   - A list that starts with a test ([[ ]], [ ], test, (( ))) is an if in
//     disguise: +1 + nesting, and the commands after the test are nested.
//   - A command followed by `|| return` / `|| exit` (also break, continue)
//     is error handling: the trailing || is free. After a test it is not:
//     [[ -n $1 ]] || return 1 is an if.
//   - In the condition of an if, while or until, the list is that
//     structure's condition: no hidden if, only the operator sequences.
//   - Anything else costs +1 per sequence of like operators: a && b && c
//     costs 1, a && b || c costs 2.
//
// Every operator is a decision point for the cyclomatic complexity.
func (w *walker) list(c *syntax.BinaryCmd, nesting int) {
	operands, ops := flattenList(c)
	w.fn.Cyclomatic += len(ops)

	conditional := !w.condition && isTest(operands[0])
	guard := !conditional && ops[len(ops)-1].Op == syntax.OrStmt && isExit(operands[len(operands)-1])
	scored := ops
	if guard {
		scored = ops[:len(ops)-1]
	}

	inner := nesting
	if conditional {
		w.add(scored[0].OpPos, "conditional", nesting, true)
		w.enter(scored[0].OpPos, nesting)
		inner = nesting + 1
		// The operators of the same kind right after the test belong to the
		// conditional: [[ a ]] && b && c is one if.
		i := 1
		for i < len(scored) && scored[i].Op == scored[0].Op {
			i++
		}
		scored = scored[i:]
	}
	var prev syntax.BinCmdOperator
	for i, op := range scored {
		if i == 0 || op.Op != prev {
			w.add(op.OpPos, op.Op.String(), nesting, false)
		}
		prev = op.Op
	}

	w.stmt(operands[0], nesting)
	saved := w.condition
	w.condition = false
	for _, s := range operands[1:] {
		w.stmt(s, inner)
	}
	w.condition = saved
}

type listOp struct {
	Op    syntax.BinCmdOperator
	OpPos syntax.Pos
}

// flattenList turns the left-nested tree of a && b || c into its operands and
// operators, in source order.
func flattenList(c *syntax.BinaryCmd) ([]*syntax.Stmt, []listOp) {
	var operands []*syntax.Stmt
	var ops []listOp
	var visit func(s *syntax.Stmt)
	visit = func(s *syntax.Stmt) {
		if b, ok := s.Cmd.(*syntax.BinaryCmd); ok && isListOp(b.Op) && plain(s) {
			visit(b.X)
			ops = append(ops, listOp{Op: b.Op, OpPos: b.OpPos})
			visit(b.Y)
			return
		}
		operands = append(operands, s)
	}
	visit(c.X)
	ops = append(ops, listOp{Op: c.Op, OpPos: c.OpPos})
	visit(c.Y)
	return operands, ops
}

func isListOp(op syntax.BinCmdOperator) bool {
	return op == syntax.AndStmt || op == syntax.OrStmt
}

// plain reports whether a statement is nothing but its command: no !, &, or
// redirection that would make it a unit of its own.
func plain(s *syntax.Stmt) bool {
	return !s.Negated && !s.Background && len(s.Redirs) == 0
}

func isTest(s *syntax.Stmt) bool {
	switch c := s.Cmd.(type) {
	case *syntax.TestClause, *syntax.ArithmCmd:
		return true
	case *syntax.CallExpr:
		name := callName(c)
		return name == "[" || name == "test"
	}
	return false
}

func isExit(s *syntax.Stmt) bool {
	switch c := s.Cmd.(type) {
	case *syntax.CallExpr:
		switch callName(c) {
		case "return", "exit", "break", "continue":
			return true
		}
	case *syntax.Block:
		// { echo "error" >&2; return 1; }
		if n := len(c.Stmts); n > 0 {
			return isExit(c.Stmts[n-1])
		}
	}
	return false
}

func callName(c *syntax.CallExpr) string {
	if len(c.Args) == 0 {
		return ""
	}
	return c.Args[0].Lit()
}

// call scores a simple command: a call to the function itself (recursion),
// and break N / continue N, which jump out of several loops at once.
func (w *walker) call(c *syntax.CallExpr, nesting int) {
	name := callName(c)
	switch {
	case name != "" && name == w.fn.Name && w.fn.Name != TopLevel:
		w.add(c.Pos(), "recursion", nesting, false)
	case (name == "break" || name == "continue") && len(c.Args) > 1:
		if n := c.Args[1].Lit(); n != "" && n != "1" {
			w.add(c.Pos(), name+" "+n, nesting, false)
		}
	}
	w.nested(c, nesting)
}

func (w *walker) funcDecl(c *syntax.FuncDecl) {
	name := ""
	if c.Name != nil {
		name = c.Name.Value
	}
	fn := &Function{
		Name:    name,
		Line:    int(c.Pos().Line()),
		EndLine: int(c.End().Line()),
	}
	fn.Lines = fn.EndLine - fn.Line + 1
	// Register before walking the body, so functions appear in source order
	// even when they declare others.
	w.file.Functions = append(w.file.Functions, fn)
	inner := &walker{file: w.file, fn: fn}
	inner.stmt(c.Body, 0)
	finish(fn)
}

// test scores a [[ ]] expression: +1 per sequence of like logical operators,
// read left to right (a && b || c costs 2, a && (b && c) costs 1).
func (w *walker) test(x syntax.TestExpr, nesting int) {
	var prev syntax.BinTestOperator
	first := true
	var visit func(x syntax.TestExpr)
	visit = func(x syntax.TestExpr) {
		switch t := x.(type) {
		case *syntax.BinaryTest:
			if t.Op == syntax.AndTest || t.Op == syntax.OrTest {
				visit(t.X)
				w.fn.Cyclomatic++
				if first || t.Op != prev {
					w.add(t.OpPos, t.Op.String(), nesting, false)
				}
				first, prev = false, t.Op
				visit(t.Y)
				return
			}
			visit(t.X)
			visit(t.Y)
		case *syntax.ParenTest:
			visit(t.X)
		case *syntax.UnaryTest:
			visit(t.X)
		case *syntax.Word:
			w.nested(t, nesting)
		}
	}
	visit(x)
}

// arithm scores an arithmetic expression: && and || like in [[ ]], and the
// ternary operator as a structure (+1 + nesting).
func (w *walker) arithm(x syntax.ArithmExpr, nesting int) {
	var prev syntax.BinAritOperator
	first := true
	var visit func(x syntax.ArithmExpr)
	visit = func(x syntax.ArithmExpr) {
		switch a := x.(type) {
		case *syntax.BinaryArithm:
			switch a.Op {
			case syntax.AndArit, syntax.OrArit:
				visit(a.X)
				w.fn.Cyclomatic++
				if first || a.Op != prev {
					w.add(a.OpPos, a.Op.String(), nesting, false)
				}
				first, prev = false, a.Op
				visit(a.Y)
				return
			case syntax.TernQuest:
				w.add(a.OpPos, "?:", nesting, true)
				w.fn.Cyclomatic++
			}
			visit(a.X)
			visit(a.Y)
		case *syntax.UnaryArithm:
			visit(a.X)
		case *syntax.ParenArithm:
			visit(a.X)
		case *syntax.Word:
			w.nested(a, nesting)
		}
	}
	visit(x)
}

// nested finds the code hidden in words and other nodes ($(…), <(…),
// $((…))) and scores it at the current nesting.
func (w *walker) nested(node syntax.Node, nesting int) {
	if node == nil {
		return
	}
	syntax.Walk(node, func(n syntax.Node) bool {
		switch c := n.(type) {
		case *syntax.CmdSubst:
			w.stmts(c.Stmts, nesting)
			return false
		case *syntax.ProcSubst:
			w.stmts(c.Stmts, nesting)
			return false
		case *syntax.ArithmExp:
			w.arithm(c.X, nesting)
			return false
		case *syntax.CStyleLoop:
			for _, x := range []syntax.ArithmExpr{c.Init, c.Cond, c.Post} {
				if x != nil {
					w.arithm(x, nesting)
				}
			}
			return false
		}
		return true
	})
}

// unbounded reports whether a while or until loop never ends through its
// condition: the condition is one constant command (true, :, false, or an
// arithmetic test of a number) whose status keeps the loop going. Anything
// else, a variable or a command that may fail, is a bound bashq cannot judge.
func unbounded(c *syntax.WhileClause) bool {
	if len(c.Cond) != 1 {
		return false
	}
	s := c.Cond[0]
	if s.Background || s.Coprocess || len(s.Redirs) > 0 {
		return false
	}
	succeeds, constant := constantStatus(s.Cmd)
	if !constant {
		return false
	}
	if s.Negated {
		succeeds = !succeeds
	}
	// while runs while the condition succeeds, until while it fails.
	return succeeds != c.Until
}

// constantStatus tells whether a command always succeeds or always fails,
// when it is one of the constant forms: true, :, false, (( n )).
func constantStatus(cmd syntax.Command) (succeeds, constant bool) {
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		if len(c.Assigns) > 0 || len(c.Args) != 1 {
			return false, false
		}
		switch c.Args[0].Lit() {
		case "true", ":":
			return true, true
		case "false":
			return false, true
		}
	case *syntax.ArithmCmd:
		w, ok := c.X.(*syntax.Word)
		if !ok {
			return false, false
		}
		n, err := strconv.ParseInt(w.Lit(), 10, 64)
		if err != nil {
			return false, false
		}
		return n != 0, true
	}
	return false, false
}

// isCatchAll reports whether a case item matches anything: `*)`.
func isCatchAll(item *syntax.CaseItem) bool {
	for _, p := range item.Patterns {
		if p.Lit() == "*" {
			return true
		}
	}
	return false
}
