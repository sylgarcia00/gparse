package gparse

import (
	"math"
)

// Operator represents all types of operators including
// left and right unary operators.
//
// For left and right unary operators one of the input
// tokens will be a unaryPlaceholderToken.
//
// Each operator does a type switch on its operands internally to
// pick behavior, returning a SyntaxErr for unsupported operand types.
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

// operators maps each operator symbol to a single Operator that
// type-switches on its operands. There is exactly one Operator per
// op symbol; operand-type dispatch lives inside each function.
var operators = map[opToken]Operator{
	"==": equalsOp,
	"!=": differsOp,
	"+":  arithmeticOp,
	"-":  arithmeticOp,
	"*":  arithmeticOp,
	"/":  arithmeticOp,
	"%":  arithmeticOp,
	"**": arithmeticOp,
	"<":  comparisonOp,
	">":  comparisonOp,
	"<=": comparisonOp,
	">=": comparisonOp,
}

// opRunes contains the list of runes used
// on the currently registered operators so
// we can differentiate op characters from
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

// asFloat returns the numeric value of a numeral Token (intToken or
// floatToken) as a float64. The second return is false for any other type.
func asFloat(t Token) (float64, bool) {
	switch v := t.(type) {
	case intToken:
		return float64(v), true
	case floatToken:
		return float64(v), true
	default:
		return 0, false
	}
}

// asInt returns the integer value of an intToken. The second return is
// false for any other type (including floatToken).
func asInt(t Token) (int, bool) {
	v, ok := t.(intToken)
	return int(v), ok
}

// equalsOp implements the "==" operator for the numeral combinations
// int/int, float/float, int/float and float/int, returning a boolToken.
func equalsOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	// Same concrete type: direct comparison keeps int/int and float/float exact.
	switch t1.(type) {
	case intToken:
		if _, ok := t2.(intToken); ok {
			return boolToken(t1 == t2), nil
		}
	case floatToken:
		if _, ok := t2.(floatToken); ok {
			return boolToken(t1 == t2), nil
		}
	}

	f1, ok1 := asFloat(t1)
	f2, ok2 := asFloat(t2)
	if ok1 && ok2 {
		return boolToken(f1 == f2), nil
	}

	return nil, unsupportedTypesErr(op, t1, t2)
}

// differsOp implements the "!=" operator for the numeral combinations
// int/int, float/float, int/float and float/int, returning a boolToken.
func differsOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	result, err := equalsOp(t1, t2, "==", data)
	if err != nil {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	return boolToken(!result.(boolToken)), nil
}

// arithmeticOp implements +, -, *, /, % and ** for intToken and
// floatToken operands.
//
// Following cparse semantics (shunting-yard.cpp NumeralOperation):
//   - "/" and "**" always evaluate as float division/power (floatToken).
//   - "%" is integer modulo and requires both operands to be intToken.
//   - the remaining ops (+, -, *) preserve intToken when both operands
//     are intToken and promote to floatToken when either is a floatToken.
//
// Division and modulo by zero return a SyntaxErr instead of panicking.
func arithmeticOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	if op == "%" {
		return moduloOp(t1, t2, op)
	}

	f1, ok1 := asFloat(t1)
	f2, ok2 := asFloat(t2)
	if !ok1 || !ok2 {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	// "/" and "**" always produce a float, matching cparse's use of
	// double division and pow().
	switch op {
	case "/":
		if f2 == 0 {
			return nil, divByZeroErr(op, t1, t2)
		}
		return floatToken(f1 / f2), nil
	case "**":
		return floatToken(math.Pow(f1, f2)), nil
	}

	// For +, - and * we keep integer results when both operands are ints.
	i1, isInt1 := asInt(t1)
	i2, isInt2 := asInt(t2)
	if isInt1 && isInt2 {
		switch op {
		case "+":
			return intToken(i1 + i2), nil
		case "-":
			return intToken(i1 - i2), nil
		case "*":
			return intToken(i1 * i2), nil
		}
	}

	switch op {
	case "+":
		return floatToken(f1 + f2), nil
	case "-":
		return floatToken(f1 - f2), nil
	case "*":
		return floatToken(f1 * f2), nil
	}

	return nil, unsupportedTypesErr(op, t1, t2)
}

// comparisonOp implements <, >, <= and >= for the numeral combinations
// int/int, float/float, int/float and float/int, returning a boolToken.
//
// Following cparse (shunting-yard.cpp NumeralOperation), comparisons are
// computed on the double value of both operands; non-numeral operands are
// unsupported. String ordering is not implemented yet.
func comparisonOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	f1, ok1 := asFloat(t1)
	f2, ok2 := asFloat(t2)
	if !ok1 || !ok2 {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	switch op {
	case "<":
		return boolToken(f1 < f2), nil
	case ">":
		return boolToken(f1 > f2), nil
	case "<=":
		return boolToken(f1 <= f2), nil
	case ">=":
		return boolToken(f1 >= f2), nil
	}

	return nil, unsupportedTypesErr(op, t1, t2)
}

// moduloOp implements the integer modulo operator "%". Both operands
// must be intToken; a floatToken operand is unsupported (mirroring
// cparse, which computes int64 % int64). Modulo by zero returns an error.
func moduloOp(t1 Token, t2 Token, op opToken) (Token, error) {
	i1, ok1 := asInt(t1)
	i2, ok2 := asInt(t2)
	if !ok1 || !ok2 {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	if i2 == 0 {
		return nil, divByZeroErr(op, t1, t2)
	}

	return intToken(i1 % i2), nil
}

func unsupportedTypesErr(op opToken, left Token, right Token) error {
	return SyntaxErr("unsupported types for operator", map[string]any{
		"op":         op,
		"leftToken":  left,
		"rightToken": right,
	})
}

func divByZeroErr(op opToken, left Token, right Token) error {
	return SyntaxErr("division by zero", map[string]any{
		"op":         op,
		"leftToken":  left,
		"rightToken": right,
	})
}
