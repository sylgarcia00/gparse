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
