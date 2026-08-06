package gparse

import (
	"encoding/json"
	"testing"
)

// TestArithmeticThroughParse exercises the arithmetic operators through the
// public Parse/Evaluate entry point by comparing the result against an
// expected value inside the expression.
//
// Float-literal lexing is exercised separately in TestFloatLiteralLexing;
// int/float promotion behavior is covered in TestArithmeticOpResultTypes.
func TestArithmeticThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		vars           map[string]any
		expectedResult bool
	}{
		{expr: "2 + 3 == 5", expectedResult: true},
		{expr: "5 - 3 == 2", expectedResult: true},
		{expr: "4 * 3 == 12", expectedResult: true},

		// Division always yields a float in cparse semantics; 6/3 == 2 holds
		// because floatToken(2) == intToken(2) compares equal numerically.
		{expr: "6 / 3 == 2", expectedResult: true},

		// Modulo (integer only)
		{expr: "7 % 3 == 1", expectedResult: true},
		{expr: "10 % 5 == 0", expectedResult: true},

		// Power (always float)
		{expr: "2 ** 3 == 8", expectedResult: true},

		// Regression: equality/difference still work.
		{expr: "1 == 1", expectedResult: true},
		{expr: "1 != 2", expectedResult: true},
		{expr: "1 != 1", expectedResult: false},

		// Compound expression exercising precedence + chained ops.
		{expr: "2 + 3 * 4 == 14", expectedResult: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			rawJSON, err := json.Marshal(test.vars)
			assertNoErr(t, err)

			result, err := expr.Evaluate(rawJSON)
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

// TestFloatLiteralLexing verifies decimal float literals now lex and flow
// through the arithmetic/comparison path via the public Parse entry point.
func TestFloatLiteralLexing(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: "2.5 == 2.5", expectedResult: true},
		{expr: "2.5 + 0.5 == 3", expectedResult: true},
		{expr: "0.5 == 0.5", expectedResult: true},
		{expr: "1.5 * 2 == 3", expectedResult: true},
		{expr: "2.5 == 2", expectedResult: false},
		// A trailing decimal point still lexes (ParseFloat accepts "3.").
		{expr: "3. == 3", expectedResult: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			result, err := expr.Evaluate(json.RawMessage("{}"))
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

// TestFloatLiteralBaseError verifies a decimal point on a non-base-10 literal
// is rejected with a clear message rather than silently splitting the token.
func TestFloatLiteralBaseError(t *testing.T) {
	_, err := Parse("0x1.5 == 0")
	if err == nil {
		t.Fatalf("expected an error for a hex literal with a decimal point")
	}
}

// TestArithmeticOpResultTypes checks the concrete Token type and value
// returned by arithmeticOp for each operand combination, which the boolean
// public entry point cannot distinguish (e.g. intToken vs floatToken).
func TestArithmeticOpResultTypes(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		left     Token
		right    Token
		expected Token
	}{
		{name: "int+int is int", op: "+", left: intToken(2), right: intToken(3), expected: intToken(5)},
		{name: "float+float is float", op: "+", left: floatToken(2), right: floatToken(3), expected: floatToken(5)},
		{name: "int+float is float", op: "+", left: intToken(2), right: floatToken(3), expected: floatToken(5)},
		{name: "float+int is float", op: "+", left: floatToken(2), right: intToken(3), expected: floatToken(5)},

		{name: "int-int is int", op: "-", left: intToken(5), right: intToken(3), expected: intToken(2)},
		{name: "float-int is float", op: "-", left: floatToken(5), right: intToken(3), expected: floatToken(2)},

		{name: "int*int is int", op: "*", left: intToken(4), right: intToken(3), expected: intToken(12)},
		{name: "int*float is float", op: "*", left: intToken(4), right: floatToken(3), expected: floatToken(12)},

		// Division always produces a float, matching cparse.
		{name: "int/int is float", op: "/", left: intToken(6), right: intToken(3), expected: floatToken(2)},
		{name: "int/int fractional", op: "/", left: intToken(7), right: intToken(2), expected: floatToken(3.5)},

		// Modulo always produces an int.
		{name: "int%int is int", op: "%", left: intToken(7), right: intToken(3), expected: intToken(1)},

		// Power always produces a float.
		{name: "int**int is float", op: "**", left: intToken(2), right: intToken(3), expected: floatToken(8)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := arithmeticOp(test.left, test.right, test.op, nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %T(%v), got %T(%v)", test.expected, test.expected, got, got)
			}
		})
	}
}

func TestArithmeticOpErrors(t *testing.T) {
	tests := []struct {
		name               string
		op                 opToken
		left               Token
		right              Token
		expectErrToContain []string
	}{
		{
			name:               "div by zero",
			op:                 "/",
			left:               intToken(1),
			right:              intToken(0),
			expectErrToContain: []string{"division by zero"},
		},
		{
			name:               "float div by zero",
			op:                 "/",
			left:               floatToken(1),
			right:              floatToken(0),
			expectErrToContain: []string{"division by zero"},
		},
		{
			name:               "mod by zero",
			op:                 "%",
			left:               intToken(1),
			right:              intToken(0),
			expectErrToContain: []string{"division by zero"},
		},
		{
			name:               "mod on float is unsupported",
			op:                 "%",
			left:               floatToken(1),
			right:              intToken(2),
			expectErrToContain: []string{"unsupported types for operator"},
		},
		{
			name:               "add on string is unsupported",
			op:                 "+",
			left:               strToken("a"),
			right:              intToken(1),
			expectErrToContain: []string{"unsupported types for operator"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := arithmeticOp(test.left, test.right, test.op, nil)
			assertErrContains(t, err, test.expectErrToContain...)
		})
	}
}

func TestEqualityOps(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		left     Token
		right    Token
		expected boolToken
	}{
		{name: "int==int true", op: "==", left: intToken(1), right: intToken(1), expected: true},
		{name: "float==float true", op: "==", left: floatToken(1), right: floatToken(1), expected: true},
		{name: "int==float true", op: "==", left: intToken(1), right: floatToken(1), expected: true},
		{name: "float==int true", op: "==", left: floatToken(1), right: intToken(1), expected: true},
		{name: "int==int false", op: "==", left: intToken(1), right: intToken(2), expected: false},

		{name: "int!=int true", op: "!=", left: intToken(1), right: intToken(2), expected: true},
		{name: "float!=float true", op: "!=", left: floatToken(1), right: floatToken(2), expected: true},
		{name: "int!=float true", op: "!=", left: intToken(1), right: floatToken(2), expected: true},
		{name: "float!=int true", op: "!=", left: floatToken(1), right: intToken(2), expected: true},
		{name: "int!=int false", op: "!=", left: intToken(1), right: intToken(1), expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			op := operators[test.op]
			got, err := op(test.left, test.right, test.op, nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

func TestComparisonOps(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		left     Token
		right    Token
		expected boolToken
	}{
		{name: "int<int true", op: "<", left: intToken(1), right: intToken(2), expected: true},
		{name: "int<int false", op: "<", left: intToken(2), right: intToken(2), expected: false},
		{name: "int>int true", op: ">", left: intToken(3), right: intToken(2), expected: true},
		{name: "int<=int equal", op: "<=", left: intToken(2), right: intToken(2), expected: true},
		{name: "int>=int equal", op: ">=", left: intToken(2), right: intToken(2), expected: true},
		{name: "float<float true", op: "<", left: floatToken(1.5), right: floatToken(2.5), expected: true},
		{name: "int<float true", op: "<", left: intToken(2), right: floatToken(2.5), expected: true},
		{name: "float>=int true", op: ">=", left: floatToken(2.0), right: intToken(2), expected: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			op := operators[test.op]
			got, err := op(test.left, test.right, test.op, nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

// TestComparisonNonNumeralError checks that comparing a non-numeral operand
// returns a SyntaxErr rather than a bogus result.
func TestComparisonNonNumeralError(t *testing.T) {
	op := operators["<"]
	_, err := op(intToken(1), boolToken(true), "<", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestComparisonThroughParse exercises <, >, <= and >= end-to-end via the
// public Parse API, including operator precedence against arithmetic.
func TestComparisonThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: "1 < 2", expectedResult: true},
		{expr: "2 < 2", expectedResult: false},
		{expr: "3 > 2", expectedResult: true},
		{expr: "2 <= 2", expectedResult: true},
		{expr: "2 >= 3", expectedResult: false},
		{expr: "2.5 < 3", expectedResult: true},
		// Arithmetic binds tighter than comparison: 2 + 3 < 6 -> 5 < 6.
		{expr: "2 + 3 < 6", expectedResult: true},
		{expr: "2 * 3 >= 6", expectedResult: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			result, err := expr.Evaluate(json.RawMessage("{}"))
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}
