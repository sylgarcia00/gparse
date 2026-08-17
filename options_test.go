package gparse

import (
	"reflect"
	"testing"
)

func TestWithBuiltin(t *testing.T) {
	t.Run("registered builtin is callable and its result is boxed", func(t *testing.T) {
		double := func(args ...any) (any, error) {
			n := args[0].(int)
			return n * 2, nil
		}

		expr, err := Parse("double(a) == 42", WithBuiltin("double", double))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{"a": intToken(21)})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected double(21) == 42 to be true")
		}
	})

	t.Run("collision with an existing builtin surfaces an error from Parse", func(t *testing.T) {
		_, err := Parse("len(a) == 1", WithBuiltin("len", func(args ...any) (any, error) {
			return 0, nil
		}))
		assertErrContains(t, err, "builtin already registered", "len")
	})

	t.Run("a reserved keyword name is rejected", func(t *testing.T) {
		_, err := Parse("true", WithBuiltin("true", func(args ...any) (any, error) {
			return 0, nil
		}))
		assertErrContains(t, err, "reserved keyword", "true")
	})

	t.Run("a registered builtin does not leak into a later default Parse", func(t *testing.T) {
		withOpt, err := Parse("custom(1) == 1", WithBuiltin("custom", func(args ...any) (any, error) {
			return 1, nil
		}))
		assertNoErr(t, err)
		got, err := withOpt.Evaluate(MapScope{})
		assertNoErr(t, err)
		if !got {
			t.Fatalf("expected custom(1) == 1 to evaluate true when registered")
		}

		// A plain Parse with no options must NOT see the custom builtin: if the
		// option had mutated the shared globals, `custom(1)` would resolve to the
		// registered function and evaluate; instead the call must fail at eval.
		def, err := Parse("custom(1) == 1")
		assertNoErr(t, err)
		if _, err = def.Evaluate(MapScope{}); err == nil {
			t.Fatalf("custom builtin leaked into a default Parse")
		}
	})
}

func TestBoxUnboxRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value any
		token Token
	}{
		{"none", nil, noneToken{}},
		{"int", 7, intToken(7)},
		{"float", 3.5, floatToken(3.5)},
		{"string", "hi", strToken("hi")},
		{"bool", true, boolToken(true)},
		{"list", []any{1, "a", true}, listToken{intToken(1), strToken("a"), boolToken(true)}},
		{"map", map[string]any{"k": 2}, mapToken{"k": intToken(2)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := box(test.value)
			assertNoErr(t, err)
			if !reflect.DeepEqual(token, test.token) {
				t.Fatalf("box(%#v) = %#v, want %#v", test.value, token, test.token)
			}

			back := unbox(token)
			if !reflect.DeepEqual(back, test.value) {
				t.Fatalf("unbox(box(%#v)) = %#v, want %#v", test.value, back, test.value)
			}
		})
	}
}

func TestBoxUnsupportedType(t *testing.T) {
	_, err := box(struct{ X int }{X: 1})
	assertErrContains(t, err, "cannot box value into a token")
}
