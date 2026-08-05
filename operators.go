package cparse

import (
	"fmt"
	"reflect"
)

// Operator represents all types of operators including
// left and right unary operators.
//
// For left and right unary operators one of the input
// tokens will be a unaryPlaceholderToken.
type Operator func(Token, Token, opToken, *EvaluationData) (Token, error)

// Create the operator precedence map based on C++ default
// precedence order as described on cppreference website:
// http://en.cppreference.com/w/cpp/language/operator_precedence
var opPrecedence = map[string]int{
	"[]": 2, "()": 2, ".": 2,
	"**": 3,
	"*":  5, "/": 5, "%": 5,
	"+": 6, "-": 6,
	"<<": 7, ">>": 7,
	"<": 9, "<=": 9, ">=": 9, ">": 9,
	"==": 10, "!=": 10,
	"&":  11,
	"^":  12,
	"|":  13,
	"&&": 14,
	"||": 15,
	"=":  16,
	":":  16,
	",":  17,

	// Unary operators' precence is prefixed by L or R implying
	// they operate on the left or on the right side of the token.
	// E.g. ++ in Go is a right side unary operator, ! is a left side.
	"L-": 3, "L+": 3, "L!": 3,

	// TODO(vingarcia): Check if we really need this one:
	"!": 3,
}

type opTypePair struct {
	left  reflect.Type
	right reflect.Type
}

func newOpTypePair(t1 any, t2 any) opTypePair {
	return opTypePair{
		left:  reflect.TypeOf(t1),
		right: reflect.TypeOf(t2),
	}
}

var operators = map[opToken]map[opTypePair]Operator{
	// ">":  greaterThanOp,
	// "<":  lesserThanOp,
	"==": map[opTypePair]Operator{
		newOpTypePair(floatToken(0), floatToken(0)): equalsFloatOp,
		newOpTypePair(intToken(0), intToken(0)):     equalsIntOp,
		newOpTypePair(floatToken(0), intToken(0)):   equalsFloatIntOp,
		newOpTypePair(intToken(0), floatToken(0)):   equalsIntFloatOp,
	},
	"!=": map[opTypePair]Operator{
		newOpTypePair(floatToken(0), floatToken(0)): differsOp,
		newOpTypePair(intToken(0), intToken(0)):     differsOp,
		newOpTypePair(floatToken(0), intToken(0)):   differsFloatIntOp,
		newOpTypePair(intToken(0), floatToken(0)):   differsIntFloatOp,
	},
}

// opRunes contains the list of runes used
// on the currently registered operators so
// so we can differentiate op characters from
// other types of characters
var opRunesSet = func() (runes map[rune]bool) {
	runeSet := map[rune]bool{}
	for k := range operators {
		for _, c := range k {
			runeSet[c] = true
		}
	}

	return runeSet
}()

func greaterThanOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	f1, ok := t1.(floatToken)
	if !ok {
		return nil, fmt.Errorf("expected numeral as the left operand for operator '>', but got: %v", t1)
	}

	f2, ok := t2.(floatToken)
	if !ok {
		return nil, fmt.Errorf("expected numeral as the right operand for operator '>', but got: %v", t2)
	}

	return boolToken(f1 > f2), nil
}

func lesserThanOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	f1, ok := t1.(floatToken)
	if !ok {
		return nil, fmt.Errorf("expected numeral as the left operand for operator '<', but got: %v", t1)
	}

	f2, ok := t2.(floatToken)
	if !ok {
		return nil, fmt.Errorf("expected numeral as the right operand for operator '<', but got: %v", t2)
	}

	return boolToken(f1 < f2), nil
}

func equalsIntOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(t1 == t2), nil
}

func equalsFloatOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(t1 == t2), nil
}

func equalsIntFloatOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(floatToken(float64(t1.(intToken))) == t2), nil
}

func equalsFloatIntOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(t1 == floatToken(float64(t2.(intToken)))), nil
}

func differsOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(t1 != t2), nil
}

func differsIntFloatOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(floatToken(float64(t1.(intToken))) != t2), nil
}

func differsFloatIntOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	return boolToken(t1 != floatToken(float64(t2.(intToken)))), nil
}
