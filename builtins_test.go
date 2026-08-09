package gparse

import (
	"encoding/json"
	"testing"
)

// TestBuiltinLen exercises len() directly on operands built programmatically:
// byte length for strings and element count for lists and maps.
func TestBuiltinLen(t *testing.T) {
	tests := []struct {
		name     string
		arg      Token
		expected intToken
	}{
		{name: "empty string", arg: strToken(""), expected: 0},
		{name: "ascii string", arg: strToken("hello"), expected: 5},
		// len counts bytes, matching how str[i] indexes by byte: "é" is 2 bytes.
		{name: "multibyte string is byte length", arg: strToken("é"), expected: 2},
		{name: "list", arg: listToken{intToken(1), intToken(2), intToken(3)}, expected: 3},
		{name: "empty list", arg: listToken{}, expected: 0},
		{name: "map", arg: mapToken{"a": intToken(1), "b": intToken(2)}, expected: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinLen([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

// TestBuiltinLenErrors covers a non-sizable argument and the wrong argument
// count, both of which must return an error rather than panic.
func TestBuiltinLenErrors(t *testing.T) {
	_, err := builtinLen([]Token{intToken(5)}, nil)
	assertErrContains(t, err, "not sizable")

	_, err = builtinLen([]Token{boolToken(true)}, nil)
	assertErrContains(t, err, "not sizable")

	_, err = builtinLen([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")
}

// TestBuiltinType exercises type() directly, checking the name returned for each
// token kind.
func TestBuiltinType(t *testing.T) {
	tests := []struct {
		name     string
		arg      Token
		expected strToken
	}{
		{name: "int", arg: intToken(1), expected: "int"},
		{name: "float", arg: floatToken(1.5), expected: "float"},
		{name: "string", arg: strToken("x"), expected: "string"},
		{name: "bool", arg: boolToken(true), expected: "bool"},
		{name: "list", arg: listToken{intToken(1)}, expected: "list"},
		{name: "map", arg: mapToken{"a": intToken(1)}, expected: "map"},
		{name: "none", arg: noneToken{}, expected: "none"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinType([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

// TestBuiltinTypeError covers the wrong argument count.
func TestBuiltinTypeError(t *testing.T) {
	_, err := builtinType([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")
}

// TestBuiltinsThroughParse exercises len() and type() end-to-end via the public
// Parse API. The value-returning call is wrapped in a comparison because the
// bool-only Evaluate entry point returns bool.
func TestBuiltinsThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		payload        json.RawMessage
		expectedResult bool
	}{
		{expr: `len("hello") == 5`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `len("") == 0`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `len("hello") == 4`, payload: json.RawMessage("{}"), expectedResult: false},
		// len() on a JSON list and map read from the payload.
		{expr: `len(items) == 3`, payload: json.RawMessage(`{"items":[1,2,3]}`), expectedResult: true},
		{expr: `len(obj) == 2`, payload: json.RawMessage(`{"obj":{"a":1,"b":2}}`), expectedResult: true},

		{expr: `type("hello") == "string"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(1) == "int"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(1.5) == "float"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(items) == "list"`, payload: json.RawMessage(`{"items":[1,2,3]}`), expectedResult: true},
		{expr: `type(obj) == "map"`, payload: json.RawMessage(`{"obj":{"a":1}}`), expectedResult: true},
		// type() of a len() result: an int.
		{expr: `type(len("ab")) == "int"`, payload: json.RawMessage("{}"), expectedResult: true},

		// abs() preserves numeral type; the negative operand comes from a unary
		// minus (abs(-5)) and from arithmetic (abs(3 - 10)) — reading a negative
		// literal straight from the JSON payload is a separate lexer gap.
		{expr: `abs(-5) == 5`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `abs(3 - 10) == 7`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(abs(-3)) == "int"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(abs(-3.5)) == "float"`, payload: json.RawMessage("{}"), expectedResult: true},

		// floor/ceil/round return an int (whole numbers); an int argument is
		// returned unchanged. sqrt always returns a float.
		{expr: `floor(3.7) == 3`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `ceil(2.1) == 3`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `round(2.5) == 3`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `floor(5) == 5`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(floor(3.7)) == "int"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(ceil(2.1)) == "int"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `sqrt(9.0) == 3.0`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(sqrt(9.0)) == "float"`, payload: json.RawMessage("{}"), expectedResult: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			result, err := expr.Evaluate(test.payload)
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

// TestBuiltinNameShadowsPayloadField documents the chosen precedence: a
// built-in name always lexes as a function, so a payload field of the same name
// (here "len") can never be read as a value. Used uncalled it is not a boolean
// and fails to evaluate rather than resolving to the field.
func TestBuiltinNameShadowsPayloadField(t *testing.T) {
	expr, err := Parse(`len == 5`)
	assertNoErr(t, err)

	_, err = expr.Evaluate(json.RawMessage(`{"len":5}`))
	if err == nil {
		t.Fatalf("expected an error: a built-in name must not resolve to a payload field")
	}
}

// TestBuiltinLenNonSizableThroughParse covers a non-sizable argument surfacing
// as an error through the public API (wrapped by evaluate's "error parsing
// function").
func TestBuiltinLenNonSizableThroughParse(t *testing.T) {
	expr, err := Parse(`len(5) == 1`)
	assertNoErr(t, err)

	_, err = expr.Evaluate(json.RawMessage("{}"))
	assertErrContains(t, err, "not sizable")
}

// TestBuiltinMinMax exercises min/max directly on programmatically built
// argument slices (the multi-argument shape the "," executor produces),
// including int/float mixing and the single-argument edge case.
func TestBuiltinMinMax(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]Token, mapToken) (Token, error)
		args []Token
		want Token
	}{
		{name: "min ints", fn: builtinMin, args: []Token{intToken(3), intToken(1), intToken(2)}, want: intToken(1)},
		{name: "max ints", fn: builtinMax, args: []Token{intToken(3), intToken(1), intToken(2)}, want: intToken(3)},
		{name: "min single arg", fn: builtinMin, args: []Token{intToken(7)}, want: intToken(7)},
		{name: "max single arg", fn: builtinMax, args: []Token{floatToken(2.5)}, want: floatToken(2.5)},

		// Mixed operands compare on the numeral value but return the original
		// token, so the winning type is preserved.
		{name: "min keeps float", fn: builtinMin, args: []Token{intToken(2), floatToken(1.5)}, want: floatToken(1.5)},
		{name: "min keeps int", fn: builtinMin, args: []Token{intToken(1), floatToken(2.5)}, want: intToken(1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.fn(test.args, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v (%T), got %v (%T)", test.want, test.want, got, got)
			}
		})
	}
}

// TestBuiltinMinMaxErrors covers an empty call and a non-numeral argument, both
// of which must return an error rather than panic.
func TestBuiltinMinMaxErrors(t *testing.T) {
	_, err := builtinMin([]Token{}, nil)
	assertErrContains(t, err, "at least one argument")

	_, err = builtinMax([]Token{}, nil)
	assertErrContains(t, err, "at least one argument")

	_, err = builtinMin([]Token{intToken(1), strToken("x")}, nil)
	assertErrContains(t, err, "not a number")
}

// TestBuiltinAbs covers the absolute value of both numeral kinds, the type
// being preserved (abs of an int stays an int) and the non-negative pass-through
// path.
func TestBuiltinAbs(t *testing.T) {
	tests := []struct {
		name string
		arg  Token
		want Token
	}{
		{name: "negative int", arg: intToken(-3), want: intToken(3)},
		{name: "positive int", arg: intToken(3), want: intToken(3)},
		{name: "zero int", arg: intToken(0), want: intToken(0)},
		{name: "negative float keeps float", arg: floatToken(-3.5), want: floatToken(3.5)},
		{name: "positive float", arg: floatToken(3.5), want: floatToken(3.5)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinAbs([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v (%T), got %v (%T)", test.want, test.want, got, got)
			}
		})
	}
}

// TestBuiltinAbsErrors covers the wrong-arity and non-numeral paths, both of
// which must return an error rather than panic.
func TestBuiltinAbsErrors(t *testing.T) {
	_, err := builtinAbs([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinAbs([]Token{intToken(1), intToken(2)}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinAbs([]Token{strToken("x")}, nil)
	assertErrContains(t, err, "not a number")
}

// TestBuiltinFloorCeilRound covers floor/ceil/round on both numeral kinds: a
// float is rounded to a whole intToken (verified via the exact type, not just
// the value), and an int argument is returned unchanged as the same intToken.
func TestBuiltinFloorCeilRound(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]Token, mapToken) (Token, error)
		arg  Token
		want Token
	}{
		{name: "floor float down", fn: builtinFloor, arg: floatToken(3.7), want: intToken(3)},
		{name: "floor negative float", fn: builtinFloor, arg: floatToken(-1.2), want: intToken(-2)},
		{name: "floor int unchanged", fn: builtinFloor, arg: intToken(5), want: intToken(5)},
		{name: "ceil float up", fn: builtinCeil, arg: floatToken(2.1), want: intToken(3)},
		{name: "ceil negative float", fn: builtinCeil, arg: floatToken(-1.2), want: intToken(-1)},
		{name: "ceil int unchanged", fn: builtinCeil, arg: intToken(5), want: intToken(5)},
		{name: "round half away from zero", fn: builtinRound, arg: floatToken(2.5), want: intToken(3)},
		{name: "round negative half", fn: builtinRound, arg: floatToken(-2.5), want: intToken(-3)},
		{name: "round int unchanged", fn: builtinRound, arg: intToken(7), want: intToken(7)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.fn([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v (%T), got %v (%T)", test.want, test.want, got, got)
			}
		})
	}
}

// TestBuiltinFloorCeilRoundErrors covers the wrong-arity and non-numeral paths
// for all three whole-number functions, each of which must return an error
// rather than panic.
func TestBuiltinFloorCeilRoundErrors(t *testing.T) {
	fns := map[string]func([]Token, mapToken) (Token, error){
		"floor": builtinFloor,
		"ceil":  builtinCeil,
		"round": builtinRound,
	}

	for name, fn := range fns {
		t.Run(name, func(t *testing.T) {
			_, err := fn([]Token{}, nil)
			assertErrContains(t, err, "exactly one argument")

			_, err = fn([]Token{intToken(1), intToken(2)}, nil)
			assertErrContains(t, err, "exactly one argument")

			_, err = fn([]Token{strToken("x")}, nil)
			assertErrContains(t, err, "not a number")
		})
	}
}

// TestBuiltinSqrt covers sqrt returning a floatToken for both int and float
// operands, including a perfect square (sqrt(9) -> 3.0, still a float).
func TestBuiltinSqrt(t *testing.T) {
	tests := []struct {
		name string
		arg  Token
		want Token
	}{
		{name: "float operand", arg: floatToken(9.0), want: floatToken(3.0)},
		{name: "int operand stays float", arg: intToken(16), want: floatToken(4.0)},
		{name: "zero", arg: intToken(0), want: floatToken(0.0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinSqrt([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v (%T), got %v (%T)", test.want, test.want, got, got)
			}
		})
	}
}

// TestBuiltinSqrtErrors covers wrong-arity, non-numeral, and the negative-input
// guard (a RuntimeErr rather than a NaN).
func TestBuiltinSqrtErrors(t *testing.T) {
	_, err := builtinSqrt([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinSqrt([]Token{intToken(1), intToken(2)}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinSqrt([]Token{strToken("x")}, nil)
	assertErrContains(t, err, "not a number")

	_, err = builtinSqrt([]Token{intToken(-4)}, nil)
	assertErrContains(t, err, "sqrt of negative number")
}
