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
			// String + numeral now concatenates (see TestStringConcatOps);
			// string + bool remains an unsupported combination.
			name:               "add string and bool is unsupported",
			op:                 "+",
			left:               strToken("a"),
			right:              boolToken(true),
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

// TestBitwiseOps exercises &, |, ^, << and >> directly on intToken operands.
func TestBitwiseOps(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		left     Token
		right    Token
		expected intToken
	}{
		{name: "5&3", op: "&", left: intToken(5), right: intToken(3), expected: 1},
		{name: "5|2", op: "|", left: intToken(5), right: intToken(2), expected: 7},
		{name: "5^1", op: "^", left: intToken(5), right: intToken(1), expected: 4},
		{name: "1<<3", op: "<<", left: intToken(1), right: intToken(3), expected: 8},
		{name: "16>>2", op: ">>", left: intToken(16), right: intToken(2), expected: 4},
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

// TestBitwiseNonIntError checks that bitwise operators reject float (or other
// non-int) operands, and that a negative shift count is rejected instead of
// panicking.
func TestBitwiseNonIntError(t *testing.T) {
	and := operators["&"]
	_, err := and(intToken(1), floatToken(2.0), "&", nil)
	assertErrContains(t, err, "unsupported types")

	shl := operators["<<"]
	_, err = shl(intToken(1), intToken(-1), "<<", nil)
	assertErrContains(t, err, "negative shift count")
}

// TestBitwiseThroughParse exercises the bitwise operators end-to-end via the
// public Parse API. Results are wrapped in a comparison because the bool-only
// Evaluate entry point returns bool; the grouping parens are also required
// since &, | and ^ bind looser than == (the classic C precedence).
func TestBitwiseThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: "(5 & 3) == 1", expectedResult: true},
		{expr: "(5 | 2) == 7", expectedResult: true},
		{expr: "(5 ^ 1) == 4", expectedResult: true},
		{expr: "(1 << 3) == 8", expectedResult: true},
		{expr: "(16 >> 2) == 4", expectedResult: true},
		// << binds tighter than |: (1 << 2 | 1) -> (4 | 1) -> 5.
		{expr: "(1 << 2 | 1) == 5", expectedResult: true},
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

// TestLogicalOps exercises the &&, || and ! operators directly on boolToken
// operands. The ! operator is left-unary, so its left operand is the
// unaryPlaceholderToken the RPN builder emits.
func TestLogicalOps(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		left     Token
		right    Token
		expected boolToken
	}{
		{name: "true&&true", op: "&&", left: boolToken(true), right: boolToken(true), expected: true},
		{name: "true&&false", op: "&&", left: boolToken(true), right: boolToken(false), expected: false},
		{name: "false||true", op: "||", left: boolToken(false), right: boolToken(true), expected: true},
		{name: "false||false", op: "||", left: boolToken(false), right: boolToken(false), expected: false},
		{name: "!true", op: "!", left: unaryPlaceholderToken{}, right: boolToken(true), expected: false},
		{name: "!false", op: "!", left: unaryPlaceholderToken{}, right: boolToken(false), expected: true},
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

// TestLogicalNonBoolError checks that logical operators reject non-boolean
// operands with a SyntaxErr instead of coercing them.
func TestLogicalNonBoolError(t *testing.T) {
	andOp := operators["&&"]
	_, err := andOp(boolToken(true), intToken(1), "&&", nil)
	assertErrContains(t, err, "unsupported types")

	not := operators["!"]
	_, err = not(unaryPlaceholderToken{}, intToken(0), "!", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestLogicalThroughParse exercises &&, || and ! end-to-end via the public
// Parse API, including precedence against comparison (comparison binds
// tighter than && which binds tighter than ||) and unary ! grouping.
func TestLogicalThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: "1 < 2 && 3 > 2", expectedResult: true},
		{expr: "1 < 2 && 3 < 2", expectedResult: false},
		{expr: "1 > 2 || 3 > 2", expectedResult: true},
		{expr: "1 > 2 || 3 < 2", expectedResult: false},
		// NOTE: unary ! through Parse needs parenthesis grouping (e.g. !(1>2)),
		// which is a separate pre-existing lexer/RPN gap — the next blocker after
		// this. Direct ! coverage lives in TestLogicalOps.
		// && binds tighter than ||: false || (true && true) -> true.
		{expr: "1 > 2 || 1 < 2 && 3 > 2", expectedResult: true},
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

// TestParenGroupingThroughParse exercises parenthesis grouping end-to-end
// through the public Parse/Evaluate API: grouping that overrides precedence,
// grouping that unblocks the left-unary "!" operator, and nested groups.
func TestParenGroupingThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		// Grouping overrides arithmetic-vs-comparison precedence.
		{expr: "(1 + 2) < 4", expectedResult: true},
		{expr: "(1 + 2) < 3", expectedResult: false},
		// Grouping around a full comparison.
		{expr: "(1 < 2)", expectedResult: true},
		// "!" is only reachable through Parse via a grouped operand.
		{expr: "!(1 > 2)", expectedResult: true},
		{expr: "!(1 < 2)", expectedResult: false},
		// Grouping changes which logical op binds first: the || is evaluated
		// before the && because of the parens.
		{expr: "(1 > 2 || 1 < 2) && 3 < 2", expectedResult: false},
		{expr: "(1 > 2 || 1 < 2) && 3 > 2", expectedResult: true},
		// Nested grouping.
		{expr: "((1 + 1) * 2) == 4", expectedResult: true},
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

// TestUnmatchedOpenBracketError checks that an unbalanced open bracket is
// reported as a syntax error rather than leaking a "(" into the RPN.
func TestUnmatchedOpenBracketError(t *testing.T) {
	_, err := Parse("(1 < 2")
	if err == nil {
		t.Fatalf("expected an error for an unmatched open bracket")
	}
}

// TestStringConcatOps exercises "+" concatenation for the string/string,
// string/number and number/string combinations. A numeral operand is rendered
// through its double value (matching cparse), so both int and float produce a
// plain decimal with no trailing zeros.
func TestStringConcatOps(t *testing.T) {
	tests := []struct {
		name     string
		left     Token
		right    Token
		expected strToken
	}{
		{name: "str+str", left: strToken("foo"), right: strToken("bar"), expected: "foobar"},
		{name: "str+int", left: strToken("x"), right: intToken(5), expected: "x5"},
		{name: "int+str", left: intToken(5), right: strToken("x"), expected: "5x"},
		{name: "str+float", left: strToken("v"), right: floatToken(2.5), expected: "v2.5"},
		{name: "float+str", left: floatToken(2.5), right: strToken("v"), expected: "2.5v"},
		// A float with an integral value drops the trailing zero: 3.0 -> "3".
		{name: "str+float-integral", left: strToken("n"), right: floatToken(3), expected: "n3"},
		{name: "str+str-empty", left: strToken(""), right: strToken("a"), expected: "a"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := arithmeticOp(test.left, test.right, "+", nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, got)
			}
		})
	}
}

// TestStringEqualityOps checks that "==" and "!=" compare two strings and that
// a string compared against a numeral is an unsupported operation (cparse only
// defines string-on-string equality), not a silent false.
func TestStringEqualityOps(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		left     Token
		right    Token
		expected boolToken
	}{
		{name: "str==str true", op: "==", left: strToken("a"), right: strToken("a"), expected: true},
		{name: "str==str false", op: "==", left: strToken("a"), right: strToken("b"), expected: false},
		{name: "str!=str true", op: "!=", left: strToken("a"), right: strToken("b"), expected: true},
		{name: "str!=str false", op: "!=", left: strToken("a"), right: strToken("a"), expected: false},
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

// TestStringOpErrors checks the unsupported string-operand combinations:
// concatenating a string with a bool, and comparing a string with a numeral.
func TestStringOpErrors(t *testing.T) {
	_, err := arithmeticOp(strToken("a"), boolToken(true), "+", nil)
	assertErrContains(t, err, "unsupported types")

	_, err = equalsOp(strToken("1"), intToken(1), "==", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestStringThroughParse exercises string concatenation and equality end-to-end
// via the public Parse API. Results are wrapped in a string comparison because
// the bool-only Evaluate entry point returns bool.
func TestStringThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: `"foo" + "bar" == "foobar"`, expectedResult: true},
		{expr: `"a" == "a"`, expectedResult: true},
		{expr: `"a" != "b"`, expectedResult: true},
		{expr: `"x" + 5 == "x5"`, expectedResult: true},
		// Concatenation binds looser than nothing here, but chains left-to-right.
		{expr: `"a" + "b" + "c" == "abc"`, expectedResult: true},
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

// TestIndexOps exercises the "[]" indexing operator directly for strings and
// lists, including negative (from-the-end) indices.
func TestIndexOps(t *testing.T) {
	list := listToken{intToken(10), intToken(20), intToken(30)}

	tests := []struct {
		name     string
		left     Token
		right    Token
		expected Token
	}{
		{name: `"hello"[0]`, left: strToken("hello"), right: intToken(0), expected: strToken("h")},
		{name: `"hello"[1]`, left: strToken("hello"), right: intToken(1), expected: strToken("e")},
		{name: `"hello"[-1]`, left: strToken("hello"), right: intToken(-1), expected: strToken("o")},
		{name: `list[0]`, left: list, right: intToken(0), expected: intToken(10)},
		{name: `list[2]`, left: list, right: intToken(2), expected: intToken(30)},
		{name: `list[-1]`, left: list, right: intToken(-1), expected: intToken(30)},
	}

	index := operators["[]"]
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := index(test.left, test.right, "[]", nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

// TestIndexOpErrors covers out-of-range indices, non-integer indices and
// unindexable left operands.
func TestIndexOpErrors(t *testing.T) {
	list := listToken{intToken(10), intToken(20)}
	index := operators["[]"]

	_, err := index(strToken("ab"), intToken(2), "[]", nil)
	assertErrContains(t, err, "index out of range")

	_, err = index(strToken("ab"), intToken(-3), "[]", nil)
	assertErrContains(t, err, "index out of range")

	_, err = index(list, intToken(2), "[]", nil)
	assertErrContains(t, err, "index out of range")

	_, err = index(strToken("ab"), floatToken(1.0), "[]", nil)
	assertErrContains(t, err, "unsupported types")

	_, err = index(intToken(5), intToken(0), "[]", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestMapIndexOps exercises "[]" indexing on a mapToken: a present string key
// returns its value, a missing key returns a noneToken (mirroring cparse's
// MapIndex returning packToken::None()), and a non-string key is unsupported.
func TestMapIndexOps(t *testing.T) {
	m := mapToken{"a": intToken(10), "b": strToken("x")}
	index := operators["[]"]

	got, err := index(m, strToken("a"), "[]", nil)
	assertNoErr(t, err)
	if got != intToken(10) {
		t.Fatalf(`expected m["a"] == 10, got %v`, got)
	}

	got, err = index(m, strToken("b"), "[]", nil)
	assertNoErr(t, err)
	if got != strToken("x") {
		t.Fatalf(`expected m["b"] == "x", got %v`, got)
	}

	got, err = index(m, strToken("missing"), "[]", nil)
	assertNoErr(t, err)
	if _, ok := got.(noneToken); !ok {
		t.Fatalf("expected noneToken for a missing key, got %T (%v)", got, got)
	}

	_, err = index(m, intToken(0), "[]", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestIndexThroughParse exercises "[]" indexing end-to-end via the public Parse
// API, wrapped in a comparison because the bool-only Evaluate returns bool. It
// also covers that "[]" binds tighter than "==" (indexing happens first).
//
// Only string literals with non-negative literal indices are reachable here:
// list literals need the "," operator executed and negative literal indices
// need unary-minus execution, neither of which exists yet (both are covered by
// the direct TestIndexOps above, which builds the operands programmatically).
func TestIndexThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: `"hello"[0] == "h"`, expectedResult: true},
		{expr: `"hello"[1] == "e"`, expectedResult: true},
		{expr: `"hello"[4] == "o"`, expectedResult: true},
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

// TestUnaryArithmeticOps exercises the left-unary "-" and "+" operators
// directly: the left operand is a unaryPlaceholderToken, mirroring how the RPN
// builder feeds a normalized "-"/"+" symbol for a left-unary use.
func TestUnaryArithmeticOps(t *testing.T) {
	tests := []struct {
		name     string
		op       opToken
		operand  Token
		expected Token
	}{
		{name: "-int", op: "-", operand: intToken(5), expected: intToken(-5)},
		{name: "-negative int", op: "-", operand: intToken(-3), expected: intToken(3)},
		{name: "-float", op: "-", operand: floatToken(2.5), expected: floatToken(-2.5)},
		{name: "+int identity", op: "+", operand: intToken(7), expected: intToken(7)},
		{name: "+float identity", op: "+", operand: floatToken(1.5), expected: floatToken(1.5)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			op := operators[test.op]
			got, err := op(unaryPlaceholderToken{}, test.operand, test.op, nil)
			assertNoErr(t, err)

			if got != test.expected {
				t.Fatalf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

// TestUnaryArithmeticNonNumeralError checks that a left-unary "-"/"+" on a
// non-numeral operand returns a SyntaxErr rather than a bogus result.
func TestUnaryArithmeticNonNumeralError(t *testing.T) {
	op := operators["-"]
	_, err := op(unaryPlaceholderToken{}, boolToken(true), "-", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestUnaryArithmeticThroughParse exercises unary minus/plus end-to-end via the
// public Parse API, including its precedence against binary arithmetic and its
// use inside grouped sub-expressions.
func TestUnaryArithmeticThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: "-5 == 0 - 5", expectedResult: true},
		{expr: "-2 < 0", expectedResult: true},
		// Unary minus binds tighter than binary "+": -2 + 3 -> 1.
		{expr: "-2 + 3 == 1", expectedResult: true},
		{expr: "+7 == 7", expectedResult: true},
		{expr: "3 * -2 == -6", expectedResult: true},
		{expr: "-(1 + 2) == -3", expectedResult: true},
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

// TestDotOps exercises the "." attribute operator directly. For a map it is
// identical to "[]": a present key returns its value and a missing key returns a
// noneToken (the lexer feeds the attribute name as a strToken, so at this level
// it is the same call as index(m, strToken(...))).
func TestDotOps(t *testing.T) {
	m := mapToken{"a": intToken(10), "b": strToken("x")}
	dot := operators["."]

	got, err := dot(m, strToken("a"), ".", nil)
	assertNoErr(t, err)
	if got != intToken(10) {
		t.Fatalf(`expected m.a == 10, got %v`, got)
	}

	got, err = dot(m, strToken("missing"), ".", nil)
	assertNoErr(t, err)
	if _, ok := got.(noneToken); !ok {
		t.Fatalf("expected noneToken for a missing attribute, got %T (%v)", got, got)
	}

	_, err = dot(intToken(5), strToken("a"), ".", nil)
	assertErrContains(t, err, "unsupported types")
}

// TestDotThroughParse exercises "." attribute access end-to-end via the public
// Parse API against a JSON payload, wrapped in a comparison because the bool-only
// Evaluate returns bool. It covers a present key, a nested/chained access and a
// missing key (which resolves to None and thus compares unequal), plus that "."
// binds tighter than "==".
func TestDotThroughParse(t *testing.T) {
	payload := json.RawMessage(`{"user":{"name":"bob","age":30,"addr":{"city":"nyc"}}}`)

	tests := []struct {
		expr           string
		expectedResult bool
	}{
		{expr: `user.name == "bob"`, expectedResult: true},
		{expr: `user.name == "alice"`, expectedResult: false},
		{expr: `user.age == 30`, expectedResult: true},
		{expr: `user.addr.city == "nyc"`, expectedResult: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			result, err := expr.Evaluate(payload)
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

// TestDotThroughParseErrors covers "." applied to a non-map value (a runtime
// unsupported-types error) and a dangling "." with no attribute name after it (a
// parse-time syntax error).
func TestDotThroughParseErrors(t *testing.T) {
	// "." on a non-map operand: user.name is a string, so .foo on it fails.
	expr, err := Parse(`user.name.foo == "x"`)
	assertNoErr(t, err)
	_, err = expr.Evaluate(json.RawMessage(`{"user":{"name":"bob"}}`))
	assertErrContains(t, err, "unsupported types")

	// A dangling "." with no attribute name is a syntax error at parse time.
	_, err = Parse(`user. == 1`)
	assertErrContains(t, err, "expected an attribute name")

	_, err = Parse(`user.`)
	assertErrContains(t, err, "expected an attribute name")

	// A missing attribute resolves to None (verified directly in TestDotOps);
	// comparing None with "==" is not a supported operation yet, so end-to-end
	// it surfaces as an unsupported-types error rather than false.
	expr, err = Parse(`user.missing == "x"`)
	assertNoErr(t, err)
	_, err = expr.Evaluate(json.RawMessage(`{"user":{"name":"bob"}}`))
	assertErrContains(t, err, "unsupported types")
}

// TestCommaOp exercises the "," executor directly: a non-tuple left operand
// starts a new two-element tuple, while a tupleToken left operand is extended
// in place. Left-folding a comma chain (comma is left-associative) therefore
// yields a single flat tuple.
func TestCommaOp(t *testing.T) {
	comma := operators[","]

	// Fresh pair from two scalars.
	got, err := comma(intToken(1), intToken(2), ",", nil)
	assertNoErr(t, err)
	assertTupleEquals(t, got, tupleToken{intToken(1), intToken(2)})

	// Extending an existing tuple appends one element (mirrors the left-fold
	// the RPN produces for `1, 2, 3`).
	got, err = comma(got, intToken(3), ",", nil)
	assertNoErr(t, err)
	assertTupleEquals(t, got, tupleToken{intToken(1), intToken(2), intToken(3)})

	// Tuples are heterogeneous, like Python tuples.
	got, err = comma(strToken("a"), boolToken(true), ",", nil)
	assertNoErr(t, err)
	assertTupleEquals(t, got, tupleToken{strToken("a"), boolToken(true)})
}

func assertTupleEquals(t *testing.T, got Token, want tupleToken) {
	t.Helper()
	tuple, ok := got.(tupleToken)
	if !ok {
		t.Fatalf("expected a tupleToken, got %T (%v)", got, got)
	}
	if len(tuple) != len(want) {
		t.Fatalf("expected tuple %v, got %v", want, tuple)
	}
	for i := range want {
		if tuple[i] != want[i] {
			t.Fatalf("expected tuple %v, got %v", want, tuple)
		}
	}
}

// TestCommaThroughParse proves the comma executor end-to-end: multi-element
// list/tuple building and multi-argument built-in calls (the path the comma
// executor unblocks). It also pins comma precedence: comma binds looser than
// every other operator, so `min(1+1, 3)` groups as `min((1+1), 3)`.
func TestCommaThroughParse(t *testing.T) {
	tests := []struct {
		expr           string
		expectedResult bool
	}{
		// Multi-element list literal: the "," builds the argument tuple that the
		// list constructor spreads into elements.
		{expr: "len([1, 2, 3]) == 3", expectedResult: true},
		{expr: "[10, 20, 30][1] == 20", expectedResult: true},

		// Multi-argument built-in call reaching min/max.
		{expr: "min(3, 1, 2) == 1", expectedResult: true},
		{expr: "max(3, 1, 2) == 3", expectedResult: true},

		// Comma binds looser than the inner operators, so sub-expressions are
		// fully evaluated before becoming tuple elements.
		{expr: "min(1 + 1, 3) == 2", expectedResult: true},
		{expr: "max(2 * 2, 3) == 4", expectedResult: true},

		// A nested list literal is a single element of the outer list.
		{expr: "len([[1, 2], [3, 4], [5, 6]]) == 3", expectedResult: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			got, err := expr.Evaluate(json.RawMessage("{}"))
			assertNoErr(t, err)

			if got != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, got)
			}
		})
	}
}

// TestCommaMalformedTuple covers malformed tuple syntax. A trailing comma
// leaves a dangling "," operator with no right-hand operand, which surfaces as
// a missing-operands error at evaluation time; a doubled comma is rejected at
// parse time as an unrecognized operator. Either way a malformed tuple never
// silently builds a short/wrong tuple.
func TestCommaMalformedTuple(t *testing.T) {
	expr, err := Parse("[1, 2,]")
	assertNoErr(t, err)
	_, err = expr.Evaluate(json.RawMessage("{}"))
	assertErrContains(t, err, "missing operands")

	_, err = Parse("[1,,2]")
	assertErrContains(t, err, "unrecognized operator")
}
