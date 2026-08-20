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

		// str/int/float type conversions: int truncates toward zero, float widens
		// an int, str renders unquoted. type() confirms the resulting kind.
		{expr: `int(3.7) == 3`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `int("42") == 42`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(int(3.7)) == "int"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `float(5) == 5.0`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `float("2.5") == 2.5`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(float(5)) == "float"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `str(42) == "42"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `str("a") == "a"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(str(1)) == "string"`, payload: json.RawMessage("{}"), expectedResult: true},

		// lower/upper case-fold a string; the motivating use is case-insensitive
		// comparison of a JSON field (here read straight from the payload).
		{expr: `lower(email) == "a@b.com"`, payload: json.RawMessage(`{"email":"A@B.com"}`), expectedResult: true},
		{expr: `upper("aBc") == "ABC"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `lower("ÁÉ") == "áé"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(lower("X")) == "string"`, payload: json.RawMessage("{}"), expectedResult: true},
		// strip trims surrounding whitespace off a JSON field before comparing.
		{expr: `strip(name) == "Vini"`, payload: json.RawMessage(`{"name":"  Vini  "}`), expectedResult: true},

		// split fans a delimited string into a list; len/type/indexing then work
		// on the result. The motivating use is a delimited JSON field (tags).
		{expr: `len(split("a,b,c", ",")) == 3`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `type(split("a", ",")) == "list"`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `split(tags, ",")[1] == "b"`, payload: json.RawMessage(`{"tags":"a,b,c"}`), expectedResult: true},

		// replace normalizes a JSON field before a filter compares it — e.g.
		// strip separators out of a phone number, or blank out a substring.
		{expr: `replace(phone, "-", "") == "11999998888"`, payload: json.RawMessage(`{"phone":"11-99999-8888"}`), expectedResult: true},
		{expr: `replace("banana", "a", "o") == "bonono"`, payload: json.RawMessage("{}"), expectedResult: true},

		// contains/startswith/endswith test a JSON field directly in a filter,
		// without normalizing it first.
		{expr: `contains(tags, "urgent")`, payload: json.RawMessage(`{"tags":"a,urgent,b"}`), expectedResult: true},
		{expr: `startswith("abc", "ab")`, payload: json.RawMessage("{}"), expectedResult: true},
		{expr: `endswith(email, "@acme.com")`, payload: json.RawMessage(`{"email":"vini@acme.com"}`), expectedResult: true},
		{expr: `endswith(email, "@other.com")`, payload: json.RawMessage(`{"email":"vini@acme.com"}`), expectedResult: false},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			result, err := expr.Evaluate(jsonScope(t, test.payload))
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

	_, err = expr.Evaluate(jsonScope(t, json.RawMessage(`{"len":5}`)))
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

	_, err = expr.Evaluate(jsonScope(t, json.RawMessage("{}")))
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

// TestBuiltinStr covers str() rendering each token kind. A strToken passes
// through unquoted (str("a") == "a"), while other kinds use their String().
func TestBuiltinStr(t *testing.T) {
	tests := []struct {
		name string
		arg  Token
		want strToken
	}{
		{name: "string passthrough", arg: strToken("a"), want: "a"},
		{name: "int", arg: intToken(42), want: "42"},
		{name: "negative int", arg: intToken(-7), want: "-7"},
		{name: "float", arg: floatToken(3.5), want: "3.5"},
		{name: "bool true", arg: boolToken(true), want: "true"},
		{name: "none", arg: noneToken{}, want: "None"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinStr([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v (%T), got %v (%T)", test.want, test.want, got, got)
			}
		})
	}
}

func TestBuiltinStrError(t *testing.T) {
	_, err := builtinStr([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")
}

// TestBuiltinInt covers int() on each convertible kind: int passthrough, float
// truncation toward zero, bool to 1/0, and base-10 string parsing.
func TestBuiltinInt(t *testing.T) {
	tests := []struct {
		name string
		arg  Token
		want intToken
	}{
		{name: "int passthrough", arg: intToken(5), want: 5},
		{name: "float truncates down", arg: floatToken(3.7), want: 3},
		{name: "float truncates toward zero", arg: floatToken(-3.7), want: -3},
		{name: "string base 10", arg: strToken("42"), want: 42},
		{name: "negative string", arg: strToken("-8"), want: -8},
		{name: "bool true", arg: boolToken(true), want: 1},
		{name: "bool false", arg: boolToken(false), want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinInt([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBuiltinIntErrors(t *testing.T) {
	_, err := builtinInt([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinInt([]Token{strToken("abc")}, nil)
	assertErrContains(t, err, "cannot convert string to integer")

	// Deliberate divergence from cparse strtol partial-parse: "3abc" is an error.
	_, err = builtinInt([]Token{strToken("3abc")}, nil)
	assertErrContains(t, err, "cannot convert string to integer")

	_, err = builtinInt([]Token{listToken{intToken(1)}}, nil)
	assertErrContains(t, err, "cannot be converted to an integer")
}

// TestBuiltinFloat covers float() on each convertible kind: float passthrough,
// int widening, bool to 1.0/0.0, and string parsing.
func TestBuiltinFloat(t *testing.T) {
	tests := []struct {
		name string
		arg  Token
		want floatToken
	}{
		{name: "float passthrough", arg: floatToken(3.5), want: 3.5},
		{name: "int widens", arg: intToken(5), want: 5.0},
		{name: "string", arg: strToken("2.5"), want: 2.5},
		{name: "string integer", arg: strToken("42"), want: 42.0},
		{name: "bool true", arg: boolToken(true), want: 1.0},
		{name: "bool false", arg: boolToken(false), want: 0.0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinFloat([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBuiltinFloatErrors(t *testing.T) {
	_, err := builtinFloat([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinFloat([]Token{strToken("abc")}, nil)
	assertErrContains(t, err, "cannot convert string to float")

	// Deliberate divergence from cparse strtod partial-parse: "3abc" is an error.
	_, err = builtinFloat([]Token{strToken("3abc")}, nil)
	assertErrContains(t, err, "cannot convert string to float")

	_, err = builtinFloat([]Token{listToken{intToken(1)}}, nil)
	assertErrContains(t, err, "cannot be converted to a float")
}

// TestBuiltinLowerUpper covers case folding, including a non-ASCII (Unicode)
// letter and the empty-string edge, for both lower() and upper().
func TestBuiltinLowerUpper(t *testing.T) {
	tests := []struct {
		name string
		fn   Function
		arg  strToken
		want strToken
	}{
		{name: "lower ascii", fn: builtinLower, arg: "AbC", want: "abc"},
		{name: "lower already lower", fn: builtinLower, arg: "abc", want: "abc"},
		{name: "lower unicode", fn: builtinLower, arg: "ÁÉ", want: "áé"},
		{name: "lower empty", fn: builtinLower, arg: "", want: ""},
		{name: "upper ascii", fn: builtinUpper, arg: "aBc", want: "ABC"},
		{name: "upper unicode", fn: builtinUpper, arg: "áé", want: "ÁÉ"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.fn([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBuiltinLowerUpperErrors(t *testing.T) {
	_, err := builtinLower([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinLower([]Token{intToken(1)}, nil)
	assertErrContains(t, err, "lower() argument is not a string")

	_, err = builtinUpper([]Token{listToken{intToken(1)}}, nil)
	assertErrContains(t, err, "upper() argument is not a string")
}

// TestBuiltinStrip covers whitespace trimming, including inner whitespace that
// must be preserved, a Unicode-space case, and the empty/all-space edges.
func TestBuiltinStrip(t *testing.T) {
	tests := []struct {
		name string
		arg  strToken
		want strToken
	}{
		{name: "both sides", arg: "  hi  ", want: "hi"},
		{name: "tabs and newline", arg: "\t hi\n", want: "hi"},
		{name: "inner space kept", arg: "  a b  ", want: "a b"},
		{name: "unicode space", arg: " hi ", want: "hi"},
		{name: "nothing to trim", arg: "hi", want: "hi"},
		{name: "all whitespace", arg: "   ", want: ""},
		{name: "empty", arg: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinStrip([]Token{test.arg}, nil)
			assertNoErr(t, err)

			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestBuiltinStripErrors(t *testing.T) {
	_, err := builtinStrip([]Token{}, nil)
	assertErrContains(t, err, "exactly one argument")

	_, err = builtinStrip([]Token{intToken(1)}, nil)
	assertErrContains(t, err, "strip() argument is not a string")
}

// TestBuiltinSplit covers the common separator case, the empty-separator
// (per-rune) case, a separator absent from the input (single-element list), and
// the empty-input edge (one empty string).
func TestBuiltinSplit(t *testing.T) {
	tests := []struct {
		name string
		s    strToken
		sep  strToken
		want listToken
	}{
		{name: "comma", s: "a,b,c", sep: ",", want: listToken{strToken("a"), strToken("b"), strToken("c")}},
		{name: "empty sep runes", s: "áb", sep: "", want: listToken{strToken("á"), strToken("b")}},
		{name: "sep absent", s: "abc", sep: ",", want: listToken{strToken("abc")}},
		{name: "empty input", s: "", sep: ",", want: listToken{strToken("")}},
		{name: "trailing sep keeps empty", s: "a,", sep: ",", want: listToken{strToken("a"), strToken("")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinSplit([]Token{test.s, test.sep}, nil)
			assertNoErr(t, err)

			list, ok := got.(listToken)
			if !ok {
				t.Fatalf("expected a listToken, got %T", got)
			}
			if len(list) != len(test.want) {
				t.Fatalf("expected %v, got %v", test.want, list)
			}
			for i := range test.want {
				if list[i] != test.want[i] {
					t.Fatalf("at %d: expected %v, got %v", i, test.want[i], list[i])
				}
			}
		})
	}
}

func TestBuiltinSplitErrors(t *testing.T) {
	_, err := builtinSplit([]Token{strToken("a")}, nil)
	assertErrContains(t, err, "exactly two arguments")

	_, err = builtinSplit([]Token{intToken(1), strToken(",")}, nil)
	assertErrContains(t, err, "split() first argument is not a string")

	_, err = builtinSplit([]Token{strToken("a"), intToken(1)}, nil)
	assertErrContains(t, err, "split() second argument is not a string")
}

func TestBuiltinReplace(t *testing.T) {
	tests := []struct {
		name           string
		s, old, newStr strToken
		want           strToken
	}{
		{name: "all occurrences", s: "banana", old: "a", newStr: "o", want: "bonono"},
		{name: "multi-char old", s: "11-99999-8888", old: "-", newStr: "", want: "11999998888"},
		{name: "old absent", s: "abc", old: "x", newStr: "y", want: "abc"},
		{name: "empty old inserts between runes", s: "ab", old: "", newStr: "-", want: "-a-b-"},
		{name: "unicode", s: "café", old: "é", newStr: "e", want: "cafe"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := builtinReplace([]Token{test.s, test.old, test.newStr}, nil)
			assertNoErr(t, err)
			if got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBuiltinReplaceErrors(t *testing.T) {
	_, err := builtinReplace([]Token{strToken("a"), strToken("b")}, nil)
	assertErrContains(t, err, "exactly three arguments")

	_, err = builtinReplace([]Token{intToken(1), strToken("a"), strToken("b")}, nil)
	assertErrContains(t, err, "replace() first argument is not a string")

	_, err = builtinReplace([]Token{strToken("a"), intToken(1), strToken("b")}, nil)
	assertErrContains(t, err, "replace() second argument is not a string")

	_, err = builtinReplace([]Token{strToken("a"), strToken("b"), intToken(1)}, nil)
	assertErrContains(t, err, "replace() third argument is not a string")
}

// TestBuiltinStrPredicates covers contains/startswith/endswith over the match,
// no-match, empty-needle (always true) and unicode cases.
func TestBuiltinStrPredicates(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]Token, mapToken) (Token, error)
		s, q strToken
		want boolToken
	}{
		{name: "contains match", fn: builtinContains, s: "abcabc", q: "bca", want: true},
		{name: "contains absent", fn: builtinContains, s: "abc", q: "xyz", want: false},
		{name: "contains empty needle", fn: builtinContains, s: "abc", q: "", want: true},
		{name: "contains unicode", fn: builtinContains, s: "café", q: "é", want: true},
		{name: "startswith match", fn: builtinStartsWith, s: "abcdef", q: "abc", want: true},
		{name: "startswith absent", fn: builtinStartsWith, s: "abcdef", q: "bcd", want: false},
		{name: "endswith match", fn: builtinEndsWith, s: "abcdef", q: "def", want: true},
		{name: "endswith absent", fn: builtinEndsWith, s: "abcdef", q: "de", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.fn([]Token{test.s, test.q}, nil)
			assertNoErr(t, err)
			if got != test.want {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBuiltinStrPredicateErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func([]Token, mapToken) (Token, error)
	}{
		{name: "contains", fn: builtinContains},
		{name: "startswith", fn: builtinStartsWith},
		{name: "endswith", fn: builtinEndsWith},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.fn([]Token{strToken("a")}, nil)
			assertErrContains(t, err, "exactly two arguments")

			_, err = tc.fn([]Token{intToken(1), strToken("a")}, nil)
			assertErrContains(t, err, tc.name+"() first argument is not a string")

			_, err = tc.fn([]Token{strToken("a"), intToken(1)}, nil)
			assertErrContains(t, err, tc.name+"() second argument is not a string")
		})
	}
}
