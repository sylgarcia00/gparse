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

func TestWithOperator(t *testing.T) {
	// approxEqual: true when two numbers are within 0.5 of each other. Used to
	// prove a custom binary operator both lexes and evaluates end-to-end.
	approxEqual := func(a, b any) (any, error) {
		af, aok := a.(float64)
		bf, bok := b.(float64)
		if !aok || !bok {
			return nil, RuntimeErr("~= needs two floats", nil)
		}
		diff := af - bf
		if diff < 0 {
			diff = -diff
		}
		return diff <= 0.5, nil
	}

	t.Run("a novel-rune custom operator lexes and evaluates end-to-end", func(t *testing.T) {
		// ~= uses the novel rune '~', absent from every built-in operator. It
		// can only lex if opRunes is derived per registry (divergence #2): this
		// asserts both the lexing and the evaluation.
		expr, err := Parse("a ~= b", WithOperator("~=", 10, approxEqual))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{"a": floatToken(1.2), "b": floatToken(1.0)})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected 1.2 ~= 1.0 to be true")
		}

		result, err = expr.Evaluate(MapScope{"a": floatToken(1.2), "b": floatToken(5.0)})
		assertNoErr(t, err)
		if result {
			t.Fatalf("expected 1.2 ~= 5.0 to be false")
		}
	})

	t.Run("precedence is honored relative to built-in operators", func(t *testing.T) {
		// plus binds tighter (prec 6) than ~= (prec 10), so a ~= b + c groups as
		// a ~= (b + c): 3 ~= (1 + 2) -> 3 ~= 3 -> true.
		expr, err := Parse("a ~= b + c", WithOperator("~=", 10, approxEqual))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{
			"a": floatToken(3.0), "b": floatToken(1.0), "c": floatToken(2.0),
		})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected 3 ~= (1 + 2) to be true")
		}
	})

	t.Run("collision with an existing operator surfaces an error from Parse", func(t *testing.T) {
		_, err := Parse("a == b", WithOperator("==", 10, approxEqual))
		assertErrContains(t, err, "operator already registered", "==")
	})

	t.Run("collision with a precedence-only builtin symbol is rejected", func(t *testing.T) {
		// "=" has a precedence entry but no operators entry, so a check keyed on
		// operators alone would silently overwrite its precedence. Keying on the
		// precedence map catches it.
		_, err := Parse("a", WithOperator("=", 10, approxEqual))
		assertErrContains(t, err, "operator already registered", "=")
	})

	t.Run("an invalid symbol character is rejected", func(t *testing.T) {
		_, err := Parse("a", WithOperator("a=", 10, approxEqual))
		assertErrContains(t, err, "invalid character", "a=")

		_, err = Parse("a", WithOperator("", 10, approxEqual))
		assertErrContains(t, err, "operator symbol is empty")
	})

	t.Run("a registered operator does not leak into a later default Parse", func(t *testing.T) {
		withOpt, err := Parse("a ~= b", WithOperator("~=", 10, approxEqual))
		assertNoErr(t, err)
		got, err := withOpt.Evaluate(MapScope{"a": floatToken(1.0), "b": floatToken(1.0)})
		assertNoErr(t, err)
		if !got {
			t.Fatalf("expected 1.0 ~= 1.0 to evaluate true when registered")
		}

		// A plain Parse with no options must NOT see the custom operator nor its
		// novel rune: if the option had mutated the shared globals (ops, prec or
		// opRunes), `a ~= b` would lex and evaluate; instead it must fail to
		// parse because '~=' is an unknown operator.
		if _, err = Parse("a ~= b"); err == nil {
			t.Fatalf("custom operator leaked into a default Parse")
		}
	})
}

func TestIsValidOpRune(t *testing.T) {
	tests := []struct {
		name string
		c    rune
		want bool
	}{
		{"tilde is a valid operator rune", '~', true},
		{"equals is a valid operator rune", '=', true},
		{"at sign is a valid operator rune", '@', true},
		{"letter is not", 'a', false},
		{"digit is not", '1', false},
		{"space is not", ' ', false},
		{"open paren is a token-boundary char", '(', false},
		{"minus is a token-boundary char", '-', false},
		{"underscore is a token-boundary char", '_', false},
		{"dot is member access", '.', false},
		{"double quote starts a string", '"', false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isValidOpRune(test.c); got != test.want {
				t.Fatalf("isValidOpRune(%q) = %v, want %v", test.c, got, test.want)
			}
		})
	}
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
