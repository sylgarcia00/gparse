package gparse

import (
	"math"
	"strconv"
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
	"&&": logicalOp,
	"||": logicalOp,
	"!":  notOp,
	"&":  bitwiseOp,
	"|":  bitwiseOp,
	"^":  bitwiseOp,
	"<<": bitwiseOp,
	">>": bitwiseOp,
	"[]": indexOp,
	// "." is attribute access; for maps it is identical to "[]" (m.foo ==
	// m["foo"]), so it reuses indexOp. The lexer pushes the identifier after
	// the "." as a strToken operand (see the '.' case in parse).
	".": indexOp,
	",": commaOp,
	":": colonOp,
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

// asBool returns the boolean value of a boolToken. The second return is
// false for any other type.
func asBool(t Token) (bool, bool) {
	v, ok := t.(boolToken)
	return bool(v), ok
}

// asStr returns the string value of a strToken. The second return is false for
// any other type.
func asStr(t Token) (string, bool) {
	v, ok := t.(strToken)
	return string(v), ok
}

// equalsOp implements the "==" operator for the numeral combinations
// int/int, float/float, int/float and float/int, returning a boolToken.
func equalsOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	// None equals only None. Comparing None against a present value is false
	// (not an error), so a filter predicate can test key presence directly:
	// `user.email == None` is true only when the field is absent, and
	// `field != None` is true when it is present.
	_, none1 := t1.(noneToken)
	_, none2 := t2.(noneToken)
	if none1 || none2 {
		return boolToken(none1 && none2), nil
	}

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
	case strToken:
		// cparse compares a string only against another string; a string vs
		// numeral comparison is an undefined operation, not simply false.
		if _, ok := t2.(strToken); ok {
			return boolToken(t1 == t2), nil
		}
		return nil, unsupportedTypesErr(op, t1, t2)
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
//
// A unaryPlaceholderToken as the left operand marks a left-unary use of "+" or
// "-" (e.g. -5, +x): the RPN builder normalizes "L-"/"L+" to the same "-"/"+"
// symbol as their binary form, so unary negation/identity is handled here.
func arithmeticOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	if _, ok := t1.(unaryPlaceholderToken); ok {
		return unaryArithmeticOp(t2, op)
	}

	if op == "%" {
		return moduloOp(t1, t2, op)
	}

	// "+" concatenates when either operand is a string, matching cparse's
	// string-on-string, string-on-number and number-on-string operations (a
	// numeral operand is rendered through its double value: "x" + 5 -> "x5").
	if op == "+" {
		if _, ok := t1.(strToken); ok {
			return concatOp(t1, t2, op)
		}
		if _, ok := t2.(strToken); ok {
			return concatOp(t1, t2, op)
		}
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

// unaryArithmeticOp implements the left-unary "-" and "+" operators for a
// numeral operand, preserving intToken/floatToken. Unary minus negates the
// value; unary plus is the identity. A non-numeral operand is unsupported.
func unaryArithmeticOp(t Token, op opToken) (Token, error) {
	switch v := t.(type) {
	case intToken:
		if op == "-" {
			return -v, nil
		}
		return v, nil
	case floatToken:
		if op == "-" {
			return -v, nil
		}
		return v, nil
	default:
		return nil, unsupportedTypesErr(op, unaryPlaceholderToken{}, t)
	}
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

// concatOp implements "+" concatenation for a string operand paired with
// another string or a numeral, returning a strToken. Any other operand type
// (e.g. bool) is unsupported.
func concatOp(t1 Token, t2 Token, op opToken) (Token, error) {
	s1, ok1 := concatString(t1)
	s2, ok2 := concatString(t2)
	if !ok1 || !ok2 {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	return strToken(s1 + s2), nil
}

// concatString returns the concatenation form of a token: the raw text of a
// strToken (no surrounding quotes) or the double-value rendering of a numeral,
// mirroring cparse which streams numbers through their double value. The
// second return is false for any other type.
func concatString(t Token) (string, bool) {
	if s, ok := t.(strToken); ok {
		return string(s), true
	}
	if f, ok := asFloat(t); ok {
		return strconv.FormatFloat(f, 'f', -1, 64), true
	}
	return "", false
}

// bitwiseOp implements the integer bitwise operators &, |, ^, << and >>.
// Both operands must be intToken; a floatToken (or any non-int) operand is
// unsupported, mirroring cparse which computes these on int64. A negative
// shift count returns a SyntaxErr instead of panicking (Go panics on a
// negative shift; C++ would be undefined behavior).
func bitwiseOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	i1, ok1 := asInt(t1)
	i2, ok2 := asInt(t2)
	if !ok1 || !ok2 {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	switch op {
	case "&":
		return intToken(i1 & i2), nil
	case "|":
		return intToken(i1 | i2), nil
	case "^":
		return intToken(i1 ^ i2), nil
	case "<<", ">>":
		if i2 < 0 {
			return nil, negativeShiftErr(op, t1, t2)
		}
		if op == "<<" {
			return intToken(i1 << uint(i2)), nil
		}
		return intToken(i1 >> uint(i2)), nil
	}

	return nil, unsupportedTypesErr(op, t1, t2)
}

// logicalOp implements the binary logical operators "&&" and "||" for
// boolToken operands, returning a boolToken. Non-boolean operands are
// unsupported.
//
// Note: unlike C, evaluation is NOT short-circuiting — the RPN is evaluated
// post-order, so both operands are already computed before this runs. For a
// side-effect-free filter predicate (insights' use case) this is equivalent.
func logicalOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	b1, ok1 := asBool(t1)
	b2, ok2 := asBool(t2)
	if !ok1 || !ok2 {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	switch op {
	case "&&":
		return boolToken(b1 && b2), nil
	case "||":
		return boolToken(b1 || b2), nil
	}

	return nil, unsupportedTypesErr(op, t1, t2)
}

// notOp implements the left-unary logical negation operator "!". The left
// operand is a unaryPlaceholderToken; the right operand must be a boolToken.
func notOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	b, ok := asBool(t2)
	if !ok {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	return boolToken(!b), nil
}

// commaOp implements the "," operator, which builds a tuple from its operands,
// mirroring cparse's Comma (builtin-features/operations.inc): when the left
// operand is already a tupleToken it is extended in place with the right
// operand; otherwise a new two-element tuple is created. Left-folding a chain
// of commas (comma is left-associative) therefore yields a single flat tuple,
// e.g. `1, 2, 3` -> tupleToken{1, 2, 3}. This is what makes multi-argument
// function calls reachable: the "()" call path in evaluate() spreads a
// tupleToken argument into one arg per element (see execFunc / singleArg).
func commaOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	if tuple, ok := t1.(tupleToken); ok {
		return append(tuple, t2), nil
	}
	return tupleToken{t1, t2}, nil
}

// colonOp implements the ":" operator, which pairs a key with a value to form
// a KeyValuePair for a map literal, mirroring cparse's Colon
// (builtin-features/operations.inc), the structural analog of Comma.
//
// A map literal `{"a": 1, "b": 2}` is lexed as a call to the map constructor
// (NewMapToken) whose argument list is a comma-separated series of colon pairs:
// the "," executor (commaOp) folds the pairs into a single tupleToken and the
// "()" call path spreads that tuple into one KeyValuePair per element (see
// NewMapToken, which rejects any argument that is not a KeyValuePair).
//
// The left operand is the key and must be a strToken, since map keys are
// strings (NewMapToken indexes by string); a non-string key is an
// unsupported-types error. Unlike cparse's chained STuple, each ":" produces
// exactly one KeyValuePair — chaining is not needed because a KeyValuePair
// already holds a single key and value, and ":" binds tighter than "," so
// `"a": 1, "b": 2` groups as `("a": 1), ("b": 2)`.
func colonOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	key, ok := asStr(t1)
	if !ok {
		return nil, unsupportedTypesErr(op, t1, t2)
	}
	return KeyValuePair{Key: key, Value: t2}, nil
}

// indexOp implements the "[]" indexing operator for a string or list on the
// left and an integer index on the right, mirroring cparse's
// StringOnNumberOperation / ListOnNumberOperation:
//   - str[i] returns the byte at index i as a single-character strToken
//     (cparse indexes the underlying std::string by byte, so we match that;
//     multi-byte runes are not decoded).
//   - list[i] returns the element Token at index i.
//
// Negative indices count from the end (list[-1] == last element), matching
// cparse. An out-of-range index returns a RuntimeErr; a non-integer index or an
// unindexable left operand returns a SyntaxErr (unsupported types).
//
//   - map[key] returns the value Token for a string key, or a noneToken when
//     the key is absent, mirroring cparse's MapIndex (which returns
//     packToken::None() for a missing key).
//
// The "." attribute operator is also routed here (see the operators map): for a
// map, m.foo is identical to m["foo"], so the lexer pushes the identifier after
// the "." as a strToken and this same map path handles it. A "." applied to a
// non-map left operand is an unsupported-types error (unlike "[]", a "." never
// makes sense on a string or list index here).
func indexOp(t1 Token, t2 Token, op opToken, data *EvaluationData) (Token, error) {
	// Maps are keyed by string; sequences are indexed by integer.
	if container, ok := t1.(mapToken); ok {
		key, ok := asStr(t2)
		if !ok {
			return nil, unsupportedTypesErr(op, t1, t2)
		}
		value, found := container[key]
		if !found {
			return noneToken{}, nil
		}
		// Values read from JSON input are stored lazily; unwrap so downstream
		// operators (and chained access like a.b.c) see the concrete token,
		// matching varToken.Resolve which also unwraps lazyJsonToken.
		if lazy, ok := value.(lazyJsonToken); ok {
			value = lazy.Value()
		}
		return value, nil
	}

	idx, ok := asInt(t2)
	if !ok {
		return nil, unsupportedTypesErr(op, t1, t2)
	}

	switch container := t1.(type) {
	case strToken:
		i, err := resolveIndex(idx, len(container), op, t1, t2)
		if err != nil {
			return nil, err
		}
		return strToken(container[i]), nil
	case listToken:
		i, err := resolveIndex(idx, len(container), op, t1, t2)
		if err != nil {
			return nil, err
		}
		return container[i], nil
	default:
		return nil, unsupportedTypesErr(op, t1, t2)
	}
}

// resolveIndex normalizes a possibly-negative index against a container of the
// given length (idx == -1 maps to length-1, as in cparse/Python) and returns a
// RuntimeErr when the resulting index falls outside [0, length).
func resolveIndex(idx int, length int, op opToken, left Token, right Token) (int, error) {
	if idx < 0 {
		idx += length
	}
	if idx < 0 || idx >= length {
		return 0, indexOutOfRangeErr(op, left, right)
	}
	return idx, nil
}

func indexOutOfRangeErr(op opToken, left Token, right Token) error {
	return RuntimeErr("index out of range", map[string]any{
		"op":         op,
		"leftToken":  left,
		"rightToken": right,
	})
}

func unsupportedTypesErr(op opToken, left Token, right Token) error {
	return SyntaxErr("unsupported types for operator", map[string]any{
		"op":         op,
		"leftToken":  left,
		"rightToken": right,
	})
}

func negativeShiftErr(op opToken, left Token, right Token) error {
	return SyntaxErr("negative shift count", map[string]any{
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
