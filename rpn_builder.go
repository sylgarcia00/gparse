package cparse

import (
)

// RPNBuilder ties together the logic necessary to correctly
// build an RPN given input tokens and operators.
//
// this struct doesn't handle parsing of these tokens, just
// the creation of the RPN.
//
// Note: RPN or Reverse Polish Notation is a way of representing
// mathematical expressions in a way that makes it particularly
// efficient/simple for executing the expression for the computer afterwards.
type RPNBuilder struct {
	rpn     []Token
	opStack []string

	// lastTokenWasOp will contain the last operator
	// when the last token was not an operator it will be set to "no"
	//
	// Note that the default zero value of "" counts as an operator, this was
	// intentional because it simplifies the handling of left unary operators
	// on the very start of the expression
	lastTokenWasOp    string
	lastTokenWasUnary bool

	// Used to make sure the expression won't
	// end inside a bracket evaluation just because
	// found a delimiter like '\n' or ')'
	bracketLevel int
}

// Find out if op is a binary or unary operator and handle it:
func (r *RPNBuilder) handleOp(op string) error {
	// If it's a left unary operator:
	if r.lastTokenWasOp != "no" {
		if _, exists := opPrecedence["L"+op]; exists {
			r.handleLeftUnary("L" + op)
			r.lastTokenWasUnary = true
			r.lastTokenWasOp = op
		} else {
			return SyntaxErr("unrecognized unary operator", map[string]any{
				"op": op,
			})
		}

		// If its a right unary operator:
	} else if _, exists := opPrecedence["R"+op]; exists {
		r.handleRightUnary("R" + op)

		// Set it to false, since we have already added
		// an unary token and operand to the stack:
		r.lastTokenWasUnary = false
		r.lastTokenWasOp = "no"

		// If it is a binary operator:
	} else {
		if _, exists := opPrecedence[op]; exists {
			r.handleBinaryOp(op)
		} else {
			return SyntaxErr("unrecognized binary operator", map[string]any{
				"op": op,
			})
		}

		r.lastTokenWasUnary = false
		r.lastTokenWasOp = op
	}

	return nil
}

func (r *RPNBuilder) handleBinaryOp(op string) {
	r.handleOpStack(op)
	r.opStack = append(r.opStack, op)
}

// Convert left unary operators to binary and handle them:
func (r *RPNBuilder) handleLeftUnary(unaryOp string) {
	r.rpn = append(r.rpn, unaryPlaceholderToken{})
	r.opStack = append(r.opStack, unaryOp)
}

// Convert right unary operators to binary and handle them:
func (r *RPNBuilder) handleRightUnary(unaryOp string) {
	r.handleOpStack(unaryOp)
	r.rpn = append(r.rpn,
		unaryPlaceholderToken{},
		opToken(normalizeOp(unaryOp)),
	)
}

// handleOpStack handles the most important part of building
// the expression so it can be parsed efficiently
// in the reverse polish notation RPN, which is the part
// of moving operators from the opStack to the token list
// in the right order and preserving the precedence.
//
// E.g.
// The expression: "a + b * c" would become the RPN: ["a", "b", "c", "*", "+"]
// While: "a * b + c" would instead become: ["a", "b", "*", "c", "+"]
//
// So this function decides when is the right time to move operators to the token list.
func (r *RPNBuilder) handleOpStack(op string) {
	var currentOp string

	l := len(r.opStack)
	for ; l > 0 && opPrecedence[op] >= opPrecedence[r.opStack[l-1]]; l-- {
		currentOp = normalizeOp(r.opStack[l-1])
		r.rpn = append(r.rpn, opToken(currentOp))
	}

	// Drop all the tokens we removed from the stack:
	r.opStack = r.opStack[:l]
}

func (r *RPNBuilder) FinishAndReturnRPN(expr []rune, index int, parsingCtx ParsingCtx) (rpn []Token, _ error) {
	l := len(r.opStack)

	// Check for syntax errors (excess of operators i.e. 10 + + -1):
	if r.lastTokenWasUnary {
		return nil, SyntaxErr("expected operand after unary operator", map[string]any{
			"operator": normalizeOp(r.opStack[l-1]),
			"pos":      parsingCtx.FormatLineCol(index),
		})
	}

	var currentOp string

	for ; l > 0; l-- {
		currentOp = normalizeOp(r.opStack[l-1])
		r.rpn = append(r.rpn, opToken(currentOp))
	}

	// Drop all the tokens from the stack:
	r.opStack = r.opStack[:0]

	// In case one of the custom parsers left an empty expression:
	if len(r.rpn) == 0 {
		return nil, ParserErr("invalid state: the final rpn ended up empty", map[string]any{
			"expression": string(expr),
		})
	}

	return r.rpn, nil
}

func (r *RPNBuilder) handleToken(token Token) error {
	if r.lastTokenWasOp == "no" {
		return SyntaxErr("expected token to be an operator or bracket", map[string]any{
			"token": token,
		})
	}

	r.rpn = append(r.rpn, token)
	r.lastTokenWasOp = "no"
	r.lastTokenWasUnary = false

	return nil
}

func (r *RPNBuilder) openBracket(bracket string) {
	r.opStack = append(r.opStack, bracket)
	r.lastTokenWasOp = bracket
	r.lastTokenWasUnary = false
	r.bracketLevel++
}

func (r *RPNBuilder) closeBracket(bracket string) error {
	if r.lastTokenWasOp == bracket {
		return SyntaxErr("bracket unexpectedly closed with no elements", map[string]any{
			"bracketType": bracket,
		})
	}

	// Find the open matching open bracket on the stack:
	var currentOp string
	l := len(r.opStack)
	for ; l > 0 && r.opStack[l-1] != bracket; l-- {
		currentOp = normalizeOp(r.opStack[l-1])
		r.rpn = append(r.rpn, opToken(currentOp))
	}

	if l == 0 {
		return SyntaxErr("extra closing bracket on the expression", map[string]any{
			"bracketType": bracket,
		})
	}

	// Drop the open bracket:
	l = l - 1
	r.lastTokenWasOp = "no"
	r.lastTokenWasUnary = false
	r.bracketLevel--

	// Drop all the tokens we removed from the stack:
	r.opStack = r.opStack[:l]

	return nil
}

// * * * * * Static parsing helpers: * * * * * //

func normalizeOp(op string) string {
	// The prefix L and R is used for denoting left and right unary operators
	if op[0] == 'L' || op[0] == 'R' {
		return op[1:]
	} else {
		return op
	}
}
