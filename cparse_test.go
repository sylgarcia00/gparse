package gparse

import (
	"encoding/json"
	"strings"
	"testing"
)

// Test cases ported from the insights evaluator interface spec.
// The C++ test-shunting-yard.cpp suite will be ported here section
// by section as execution support grows.
func TestParse(t *testing.T) {
	tests := []struct {
		expr               string
		vars               map[string]any
		expectedResult     bool
		expectErrToContain []string
	}{
		{
			expr: "a == 1",
			vars: map[string]any{
				"a": 1,
			},
			expectedResult: true,
		},
		{
			expr: "a != 0",
			vars: map[string]any{
				"a": 1,
			},
			expectedResult: true,
		},
		{
			expr: "a != 1",
			vars: map[string]any{
				"a": 1,
			},
			expectedResult: false,
		},
		{
			expr: "a == 0b1010",
			vars: map[string]any{
				"a": 10,
			},
			expectedResult: true,
		},
		{
			expr: "a == 012",
			vars: map[string]any{
				"a": 10,
			},
			expectedResult: true,
		},
		{
			expr: "a == 0xA",
			vars: map[string]any{
				"a": 10,
			},
			expectedResult: true,
		},
		{
			expr:           "true",
			vars:           map[string]any{},
			expectedResult: true,
		},
		{
			expr:           "false",
			vars:           map[string]any{},
			expectedResult: false,
		},
		{
			expr:           "true == true",
			vars:           map[string]any{},
			expectedResult: true,
		},
		{
			expr:           "true == false",
			vars:           map[string]any{},
			expectedResult: false,
		},
		{
			expr:           "true != false",
			vars:           map[string]any{},
			expectedResult: true,
		},
		{
			expr: "a == true",
			vars: map[string]any{
				"a": true,
			},
			expectedResult: true,
		},
		{
			expr: "a == false",
			vars: map[string]any{
				"a": true,
			},
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			rawJSON, err := json.Marshal(test.vars)
			assertNoErr(t, err)

			result, err := expr.Evaluate(jsonScope(t, rawJSON))
			if test.expectErrToContain != nil {
				assertErrContains(t, err, test.expectErrToContain...)
				return
			}
			assertNoErr(t, err)

			if result != test.expectedResult {
				t.Fatalf("expected %v, got %v", test.expectedResult, result)
			}
		})
	}
}

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertErrContains(t *testing.T, err error, substrs ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	for _, s := range substrs {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("expected error %q to contain %q", err.Error(), s)
		}
	}
}
