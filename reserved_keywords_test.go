package gparse

import (
	"encoding/json"
	"testing"
)

func TestLineComment(t *testing.T) {
	tests := []struct {
		name           string
		expr           string
		vars           map[string]any
		expectedResult bool
	}{
		{
			name:           "trailing comment is ignored",
			expr:           "1 == 1 # this is ignored",
			expectedResult: true,
		},
		{
			name:           "comment with no space after the hash",
			expr:           "1 == 1 #ignored",
			expectedResult: true,
		},
		{
			name:           "leading full-line comment before the expression",
			expr:           "# header\n1 == 1",
			expectedResult: true,
		},
		{
			name:           "comment between an operator and its operand",
			expr:           "1 + # add one\n 1 == 2",
			expectedResult: true,
		},
		{
			name:           "operator characters inside a comment are not evaluated",
			expr:           "1 == 1 # a > b && c || d",
			expectedResult: true,
		},
		{
			// A banner comment overscans as one op token (#=====); the first-char
			// fallback in parse still routes it to the line-comment parser.
			name:           "banner comment starting with an operator rune",
			expr:           "true #===== section =====",
			expectedResult: true,
		},
		{
			name:           "a hash inside a string literal is not a comment",
			expr:           `"a#b" == "a#b"`,
			expectedResult: true,
		},
		{
			name:           "comment after a scope-variable comparison",
			expr:           "a == 1 # check a",
			vars:           map[string]any{"a": 1},
			expectedResult: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expr, err := Parse(test.expr, Args{})
			assertNoErr(t, err)

			rawJSON, err := json.Marshal(test.vars)
			assertNoErr(t, err)

			result, err := expr.Evaluate(jsonScope(t, rawJSON))
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

func TestSlashSlashComment(t *testing.T) {
	tests := []struct {
		name               string
		expr               string
		vars               map[string]any
		expectErrToContain []string
		expectedResult     bool
	}{
		{
			name:           "trailing comment is ignored",
			expr:           "1 == 1 // this is ignored",
			expectedResult: true,
		},
		{
			name:           "comment with no space after the slashes",
			expr:           "1 == 1 //ignored",
			expectedResult: true,
		},
		{
			name:           "leading full-line comment before the expression",
			expr:           "// header\n1 == 1",
			expectedResult: true,
		},
		{
			name:           "comment between an operator and its operand",
			expr:           "1 + // add one\n 1 == 2",
			expectedResult: true,
		},
		{
			name:           "operator characters inside a comment are not evaluated",
			expr:           "1 == 1 // a > b && c || d",
			expectedResult: true,
		},
		{
			name:           "double slash inside a string literal is not a comment",
			expr:           `"a//b" == "a//b"`,
			expectedResult: true,
		},
		{
			name:           "comment after a scope-variable comparison",
			expr:           "a == 1 // check a",
			vars:           map[string]any{"a": 1},
			expectedResult: true,
		},
		{
			// Operator runes directly after "//" (no space) overscan into one
			// unknown operator token and fail, matching cparse. See the note on
			// reservedWordParsers in reserved_keywords.go.
			name:               "operator runes abutting the slashes are not a comment (cparse parity)",
			expr:               "1 == 1 //====",
			expectErrToContain: []string{"unrecognized operator"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expr, err := Parse(test.expr, Args{})
			if test.expectErrToContain != nil {
				assertErrContains(t, err, test.expectErrToContain...)
				return
			}
			assertNoErr(t, err)

			rawJSON, err := json.Marshal(test.vars)
			assertNoErr(t, err)

			result, err := expr.Evaluate(jsonScope(t, rawJSON))
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

func TestBlockComment(t *testing.T) {
	tests := []struct {
		name               string
		expr               string
		vars               map[string]any
		expectErrToContain []string
		expectedResult     bool
	}{
		{
			name:           "inline block comment mid-expression is ignored",
			expr:           "1 + /* add one */ 1 == 2",
			expectedResult: true,
		},
		{
			name:           "block comment spanning newlines",
			expr:           "1 + /* add\n one\n more */ 1 == 2",
			expectedResult: true,
		},
		{
			name:           "block comment at the start of the expression",
			expr:           "/* preamble */ 1 == 1",
			expectedResult: true,
		},
		{
			name:           "operator characters inside a block comment are not evaluated",
			expr:           "1 == 1 /* a > b && c || d */",
			expectedResult: true,
		},
		{
			name:           "slash-star inside a string literal is not a comment",
			expr:           `"a/*b" == "a/*b"`,
			expectedResult: true,
		},
		{
			name:           "block comment after a scope-variable comparison",
			expr:           "a == 1 /* check a */",
			vars:           map[string]any{"a": 1},
			expectedResult: true,
		},
		{
			name:               "unterminated block comment is an error",
			expr:               "1 + /* never closed",
			expectErrToContain: []string{"unterminated block comment"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expr, err := Parse(test.expr, Args{})
			if test.expectErrToContain != nil {
				assertErrContains(t, err, test.expectErrToContain...)
				return
			}
			assertNoErr(t, err)

			rawJSON, err := json.Marshal(test.vars)
			assertNoErr(t, err)

			result, err := expr.Evaluate(jsonScope(t, rawJSON))
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

// TestDivisionNotShadowed guards against registering a bare "/" reserved-word
// parser: only the two-rune "//" and "/*" keys may be registered, so plain
// division must keep working.
func TestDivisionNotShadowed(t *testing.T) {
	expr, err := Parse("6 / 2 == 3", Args{})
	assertNoErr(t, err)

	result, err := expr.Evaluate(jsonScope(t, []byte("null")))
	assertNoErr(t, err)

	if result != true {
		t.Fatalf("expected division 6 / 2 == 3 to be true, got %v", result)
	}
}
