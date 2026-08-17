package gparse

import (
	"fmt"
	"strconv"
	"unicode"
)

func Parse(strExpr string) (_ BoolExpr, err error) {
	reg := defaultRegistry()
	rpn, err := parse(strExpr, nil, reg)

	return BoolExpr{expr: Expr{rpn: rpn, reg: reg}}, err
}

// Expr is a compiled expression that evaluates over a source-agnostic Scope and
// returns the resulting Token. It is the core language surface: enforcing a
// particular result type (e.g. bool) is the host DSL's decision, layered on top
// via a wrapper like BoolExpr.
//
// It carries the registry it was compiled against so Eval resolves operators
// and builtins against the same set the parser used.
type Expr struct {
	rpn []Token
	reg *registry
}

// Eval evaluates the expression against scope and returns the resulting Token.
// The core never learns where the scope's values came from — a host binds them
// from a Go map (MapScope), JSON (jsonscope), or any other source.
func (e Expr) Eval(scope Scope) (Token, error) {
	return evaluate(e.rpn, scope, e.reg)
}

// BoolExpr is the thin bool-enforcing wrapper over Expr: it evaluates the
// expression and asserts the result is a boolean, which is what a filter
// predicate needs.
type BoolExpr struct {
	expr Expr
}

// Evaluate evaluates the expression against scope and asserts a boolean result.
// Bind a JSON payload with jsonscope.New, or any map with gparse.MapScope.
func (b BoolExpr) Evaluate(scope Scope) (bool, error) {
	token, err := b.expr.Eval(scope)
	if err != nil {
		return false, err
	}

	bToken, ok := token.(boolToken)
	if !ok {
		return false, InternalErr("expression should evaluate to a boolean", map[string]any{
			"actualValue": token.String(),
		})
	}

	return bool(bToken), nil
}

type ParsingCtx struct {
	currentLine   int
	lastLineStart int
}

func (p *ParsingCtx) HandleNewLine(newLineRuneIdx int) {
	p.currentLine++
	p.lastLineStart = newLineRuneIdx + 1
}

func (p ParsingCtx) FormatLineCol(i int) string {
	return strconv.Itoa(p.currentLine) + ":" + strconv.Itoa(i-p.lastLineStart)
}

// parse will decode the input expression into a Reverse
// Polish notation for easy future evaluation.
func parse(strExpr string, vars map[string]Token, reg *registry) (_ []Token, err error) {
	if len(strExpr) == 0 {
		return nil, fmt.Errorf("cannot build an expression from an empty string")
	}

	expr := []rune(strExpr)

	rpnBuilder := RPNBuilder{reg: reg}

	parsingCtx := ParsingCtx{
		currentLine:   0,
		lastLineStart: 0,
	}

	i := consumeSpaces(expr, 0, &parsingCtx)

	// Each iteration of this loop should produce a token or an operator
	for i < len(expr) && expr[i] != ';' {
		switch {
		case unicode.IsNumber(expr[i]):
			var num Token
			i, num, err = parseNumber(expr, i)
			if err != nil {
				return nil, err
			}

			rpnBuilder.handleToken(num)

		case isVarChar(expr[i]):
			var varName string
			i, varName = parseVar(expr, i)

			if builtin, isBuiltin := reg.builtins[varName]; isBuiltin {
				// A registered built-in name (e.g. len, type) becomes a Function
				// token so a following "(" is dispatched as a call. This lookup
				// runs first, so built-ins take precedence over reserved words
				// and over a payload variable of the same name (a field named
				// "len" can only be a call, never read as a value).
				rpnBuilder.handleToken(builtin)
				i = consumeSpaces(expr, i, &parsingCtx)
				continue
			}

			if keyword, isKeyword := reservedKeywords[varName]; isKeyword {
				// A reserved literal (true/false) is emitted as its constant Token
				// so it lexes as e.g. boolToken instead of being resolved as a
				// scope variable. Builtins are checked first and win a name clash.
				rpnBuilder.handleToken(keyword.Clone())
				i = consumeSpaces(expr, i, &parsingCtx)
				continue
			}

			rpnBuilder.handleToken(varToken([]string{varName}))

			parser := reservedWordParsers[varName]
			if parser != nil {
				i, err = parser(expr, &parsingCtx, &rpnBuilder, i)
				if err != nil {
					return nil, err
				}
			} else {
				token := vars[varName]
				if token != nil {
					// Save a reference token:
					// TODO(vingarcia): Consider cloning the token here
					err := rpnBuilder.handleToken(refToken{
						key:           varToken{varName},
						originalValue: token,
					})
					if err != nil {
						return nil, err
					}
				} else {
					// Save the variable name:
					rpnBuilder.handleToken(varToken([]string{varName}))
				}
			}

		case expr[i] == '\'' || expr[i] == '"':
			// If it is a string literal, parse it and
			// add to the output queue.
			quote := expr[i]
			formattedPos := parsingCtx.FormatLineCol(i)

			i++
			str := []rune{}
			for i < len(expr) && expr[i] != quote && expr[i] != '\n' {
				if expr[i] == '\\' {
					switch expr[i+1] {
					case 'n':
						i += 2
						str = append(str, '\n')

					case 't':
						i += 2
						str = append(str, '\t')

					default:
						switch expr[i+1] {
						case '"', '\'':
							i++
						case '\n':
							i++
							parsingCtx.HandleNewLine(i)
						}
						str = append(str, expr[i])
						i++
					}
				} else {
					str = append(str, expr[i])
					i++
				}
			}

			if expr[i] != quote {
				return nil, SyntaxErr("string literal not terminated", map[string]any{
					"startedAt": formattedPos,
				})
			}
			i++
			rpnBuilder.handleToken(strToken(string(str)))
		default:
			// Otherwise, the variable is an operator or parenthesis.
			switch expr[i] {
			case '(':
				// If it is a function call:
				if rpnBuilder.lastTokenWasOp == "no" {
					// An empty call `foo()` invokes with no arguments: emit the
					// zero-argument call instead of opening a bracket that would
					// close with no operand (same root cause as empty `[]`/`{}`).
					if isEmptyBracket(expr, i, ')') {
						i = consumeSpaces(expr, i+1, &parsingCtx) + 1
						err = rpnBuilder.handleEmptyConstructor()
						if err != nil {
							return nil, err
						}
						break
					}
					// This counts as a bracket and as an operator:
					rpnBuilder.handleOp("()")
					// Add it as a bracket to the op stack:
				}
				rpnBuilder.openBracket("(")
				i++
			case '[':
				if rpnBuilder.lastTokenWasOp == "no" {
					// If it is an operator:
					rpnBuilder.handleOp("[]")
				} else {
					// If it is the list constructor:
					// Add the list constructor to the rpn:
					rpnBuilder.handleToken(Function(NewListToken))

					// An empty literal `[]` constructs an empty list: emit the
					// zero-argument call instead of opening a bracket that would
					// close with no operand.
					if isEmptyBracket(expr, i, ']') {
						i = consumeSpaces(expr, i+1, &parsingCtx) + 1
						err = rpnBuilder.handleEmptyConstructor()
						if err != nil {
							return nil, err
						}
						break
					}

					// We make the program see it as a normal function call:
					rpnBuilder.handleOp("()")
				}
				// Add it as a bracket to the op stack:
				rpnBuilder.openBracket("[")
				i++
			case '{':
				// Add a map constructor call to the rpn:
				rpnBuilder.handleToken(Function(NewMapToken))

				// An empty literal `{}` constructs an empty map: emit the
				// zero-argument call instead of opening a bracket that would
				// close with no operand.
				if isEmptyBracket(expr, i, '}') {
					i = consumeSpaces(expr, i+1, &parsingCtx) + 1
					err = rpnBuilder.handleEmptyConstructor()
					if err != nil {
						return nil, err
					}
					break
				}

				// We make the program see it as a normal function call:
				rpnBuilder.handleOp("()")
				rpnBuilder.openBracket("{")
				i++
			case '.':
				// Attribute access: `m.foo` is identical to `m["foo"]` for a
				// map (see the "." entry in the operators map, routed to
				// indexOp). We emit the "." operator and then the following
				// identifier as a string literal operand, so the map-index path
				// handles it. A "." here (in the default operator branch) can
				// never be part of a float literal: parseNumber owns any "."
				// inside a number and only fires when the token starts with a
				// digit, so a bare `.5` is not lexed as a number.
				rpnBuilder.handleOp(".")
				i++
				if i >= len(expr) || !isVarChar(expr[i]) {
					return nil, SyntaxErr("expected an attribute name after '.'", map[string]any{
						"pos": parsingCtx.FormatLineCol(i),
					})
				}
				var attrName string
				i, attrName = parseVar(expr, i)
				rpnBuilder.handleToken(strToken(attrName))
			case ')':
				rpnBuilder.closeBracket("(")
				i++
			case ']':
				rpnBuilder.closeBracket("[")
				i++
			case '}':
				rpnBuilder.closeBracket("{")
				i++
			default:
				{
					// Then the token is an operator

					start := i
					opRunes := []rune{expr[i]}
					i++
					// These ops are here to serve as ending characters so that expressions
					// such as: `10 *-3` don't interpret *- as a single operator when its actually 2.
					opStartingChars := map[rune]bool{
						'+': true, '-': true, '\'': true, '"': true,
						'(': true, ')': true, '[': true, ']': true, '{': true, '}': true,
						'_': true,
					}
					for i < len(expr) && reg.opRunes[expr[i]] && !opStartingChars[expr[i]] {
						opRunes = append(opRunes, expr[i])
						i++
					}
					op := string(opRunes)

					// Evaluate the meaning of this operator in the following order:
					// 1. Is it a reserved word?
					// 2. Is it a valid operator?
					// 3. Is there a character parser for its first character?
					parser, isReservedWord := reservedWordParsers[op]
					if isReservedWord {
						// Parse reserved operators:
						i, err = parser(expr, &parsingCtx, &rpnBuilder, i)
						if err != nil {
							return nil, err
						}
					} else if _, isKnownOp := reg.prec[op]; isKnownOp {
						rpnBuilder.handleOp(op)
						// Maybe just the first character is an operator:
					} else if parser, isReservedWord := reservedWordParsers[op[0:1]]; isReservedWord {
						i = start + 1
						i, err = parser(expr, &parsingCtx, &rpnBuilder, i)
						if err != nil {
							return nil, err
						}
					} else {
						return nil, SyntaxErr("unrecognized operator", map[string]any{
							"op":  op,
							"pos": parsingCtx.FormatLineCol(i),
						})
					}
				}
			}
		}

		i = consumeSpaces(expr, i, &parsingCtx)
	}

	rpn, err := rpnBuilder.FinishAndReturnRPN(expr, i, parsingCtx)
	if err != nil {
		return nil, err
	}

	return rpn, nil
}

// EvaluationData contains the context used during
// evaluation and is passed as argument to all
// operator and custom operator functions, which
// allows the operators to take advantage of this info
type EvaluationData struct {
	// Vars holds function-local variable scopes ($parent-chained maps built by
	// getChildMap/execFunc). Top-level names not found here fall back to Scope.
	Vars mapToken

	// Scope is the source-agnostic top-level binding the expression evaluates
	// over; it is consulted after the local Vars chain is exhausted.
	Scope Scope

	LeftRef  refToken
	RightRef refToken

	reg *registry
}

// evaluate will copy the input rpn and then process it until it gets a resulting response
func evaluate(originalRpn []Token, scope Scope, reg *registry) (_ Token, err error) {
	var left, right Token
	data := EvaluationData{
		Scope: scope,
		reg:   reg,
	}

	rpn := copyRPN(originalRpn)

	evalStack := []Token{}

	l := len(rpn)
	for i := 0; i < l; i++ {
		token := rpn[i]
		op, isOperator := token.(opToken)
		if !isOperator {
			if v, isVar := token.(varToken); isVar {
				token = v.Resolve(data.Vars, data.Scope)
			}

			evalStack = append(evalStack, token)
			continue
		}

		// If it got here it's an operator:
		evalStack, left, right, err = popLeftAndRightOperands(evalStack)
		if err != nil {
			return nil, err
		}

		switch v := right.(type) {
		case refToken:
			data.RightRef = v
			right = v.Resolve(data.Vars)
		case varToken:
			data.RightRef = refToken{key: v}
		default:
			data.RightRef = refToken{}
		}

		switch v := left.(type) {
		case refToken:
			data.LeftRef = v
			left = v.Resolve(data.Vars)
		case varToken:
			data.LeftRef = refToken{key: v}
		default:
			data.LeftRef = refToken{}
		}

		if fn, ok := left.(Function); ok && op == "()" {
			var args tupleToken
			if tuple, ok := right.(tupleToken); ok {
				args = tuple
			} else {
				// A tuple with a single element, which might be a unaryPlaceholder:
				args = tupleToken{right}
			}

			var fnReceiver = data.Vars
			if data.LeftRef.origin != nil {
				fnReceiver = data.LeftRef.origin
			}

			resp, err := execFunc(fnReceiver, fn, args, data.Vars)
			if err != nil {
				return nil, RuntimeErr("error parsing function", map[string]any{
					"error": err,
				})
			}

			evalStack = append(evalStack, resp)
		} else {
			// * * * * * All other operations * * * * * //

			// TODO(vingarcia): Copy the exec_operator func from cparse (it's more complex than this):
			resp, err := findAndRunOperator(op, left, right, &data)
			if err != nil {
				return nil, RuntimeErr("operation error", map[string]any{
					"error": err,
				})
			}

			evalStack = append(evalStack, resp)
		}
	}

	if len(evalStack) != 1 {
		return nil, InternalErr("the evalStack should contains a single element at the end", map[string]any{
			"evalStack": evalStack,
		})
	}

	return evalStack[0], nil
}

func findAndRunOperator(op opToken, left Token, right Token, data *EvaluationData) (Token, error) {
	opFunc := data.reg.ops[op]
	if opFunc == nil {
		return nil, SyntaxErr("unrecognized operator", map[string]any{
			"op": op,
		})
	}

	return opFunc(left, right, op, data)
}

func popLeftAndRightOperands(evalStack []Token) (updatedStack []Token, left Token, right Token, _ error) {
	if len(evalStack) < 2 {
		return nil, nil, nil, InternalErr("missing operands for operator", map[string]any{
			"evalStack": evalStack,
		})
	}

	l := len(evalStack)
	right = (evalStack)[l-1]
	left = (evalStack)[l-2]
	return (evalStack)[:l-2], left, right, nil
}

func copyRPN(rpn []Token) (copy []Token) {
	for _, token := range rpn {
		copy = append(copy, token.Clone())
	}

	return copy
}

func consumeSpaces(expr []rune, index int, parsingCtx *ParsingCtx) (newIndex int) {
	for i := index; i < len(expr); i++ {
		if expr[i] == '\n' {
			parsingCtx.HandleNewLine(i)
		}
		if !unicode.IsSpace(expr[i]) {
			return i
		}
	}

	return index
}

// isEmptyBracket reports whether the open bracket at openIdx is immediately
// followed (ignoring spaces) by its matching close rune, i.e. an empty `[]` or
// `{}` literal. It does not advance parsingCtx; the caller re-consumes the
// spaces via consumeSpaces so newline bookkeeping happens exactly once.
func isEmptyBracket(expr []rune, openIdx int, closeRune rune) bool {
	i := openIdx + 1
	for i < len(expr) && unicode.IsSpace(expr[i]) {
		i++
	}
	return i < len(expr) && expr[i] == closeRune
}

// isVarChar checks if a character is the first character of a variable:
func isVarChar(c rune) bool {
	return unicode.IsLetter(c) || c == '_'
}

func parseVar(expr []rune, index int) (newIndex int, varName string) {
	// parseVar assumes the first character is already a valid starting
	// character for a varname, so we skip it:
	for i := index + 1; i < len(expr); i++ {
		if !isVarChar(expr[i]) && !unicode.IsNumber(expr[i]) && expr[i] != '_' {
			return i, string(expr[index:i])
		}
	}

	return len(expr), string(expr[index:])
}

var hexValidChars = map[rune]bool{
	'0': true, '1': true, '2': true, '3': true, '4': true,
	'5': true, '6': true, '7': true, '8': true, '9': true,
	'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true,
	'A': true, 'B': true, 'C': true, 'D': true, 'E': true, 'F': true,
}

func isValidHexDigit(r rune) bool {
	return hexValidChars[r]
}

func parseNumber(expr []rune, index int) (newIndex int, token Token, err error) {
	base := 10

	i := index
	if expr[i] == '0' {
		if i+1 < len(expr) {
			switch expr[i+1] {
			// Handle hexadecimal numbers such as 0x10:
			case 'x':
				base = 16
				// Skip the '0x' characters
				i += 2
				// Handle binary numbers such as 010:
			case 'b':
				base = 2
				// Skip the '0x' characters
				i += 2
				// Handle octal numbers such as 010:
			default:
				if unicode.IsNumber(expr[i+1]) {
					base = 8
					// Skip the '0' character
					i++
				}
			}
		}
	}

	isNumberFn := unicode.IsNumber
	if base == 16 {
		isNumberFn = isValidHexDigit
	}

	isFloat := false
	sawExponent := false

	// Find the end of the numerical literal. A single '.' switches to the
	// float path; an 'e'/'E' exponent (base 10 only) also switches to float.
	// The base != 10 check below rejects e.g. `0x1.5` with a clear message
	// rather than silently splitting the token.
	for ; i < len(expr); i++ {
		if expr[i] == '.' && !isFloat {
			isFloat = true
			continue
		}

		// Scientific notation: `2e5`, `1.5e-3`, `2E+10`. Base 10 only, once,
		// and only when an optional sign is followed by at least one digit — so
		// a trailing `e` (or hex digit 'e') is not swallowed here.
		if base == 10 && !sawExponent && (expr[i] == 'e' || expr[i] == 'E') {
			j := i + 1
			if j < len(expr) && (expr[j] == '+' || expr[j] == '-') {
				j++
			}
			if j < len(expr) && unicode.IsNumber(expr[j]) {
				isFloat = true
				sawExponent = true
				i = j // consume 'e' and optional sign; loop's i++ passes the first exponent digit
				continue
			}
		}

		if !isNumberFn(expr[i]) {
			break
		}
	}

	if isFloat {
		if base != 10 {
			return 0, nil, SyntaxErr("only base 10 literals can have decimals", map[string]any{
				"literal": string(expr[index:i]),
			})
		}

		num, err := strconv.ParseFloat(string(expr[index:i]), 64)
		if err != nil {
			panic(
				fmt.Errorf("unexpected error parsing a previously validated number '%s': %w",
					string(expr[index:i]), err,
				),
			)
		}

		return i, floatToken(num), nil
	}

	num, err := strconv.ParseInt(string(expr[index:i]), 0, 64)
	if err != nil {
		return 0, nil, SyntaxErr("error parsing numeric literal", map[string]any{
			"literal": string(expr[index:i]),
			"error":   err,
		})
	}

	return i, intToken(num), nil
}

func execFunc(this mapToken, fn Function, args tupleToken, vars mapToken) (Token, error) {
	vars = vars.getChildMap()
	return fn(args, mapToken{
		"$parent": vars,
		"this":    this,
	})
}
