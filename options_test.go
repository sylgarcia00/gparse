package gparse

import (
	"math"
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

	t.Run("a zero-argument builtin is callable as f()", func(t *testing.T) {
		// A custom builtin recognized at parse time (via the registry) lexes as a
		// Function token, so an empty `f()` routes through the empty-call path and
		// invokes with no arguments — the same fix that makes len()/[]/{} parse.
		called := false
		expr, err := Parse("now()", WithBuiltin("now", func(args ...any) (any, error) {
			called = true
			if len(args) != 0 {
				return nil, ParserErr("expected no arguments", map[string]any{"got": len(args)})
			}
			return true, nil
		}))
		assertNoErr(t, err)

		got, err := expr.Evaluate(MapScope{})
		assertNoErr(t, err)
		if !called {
			t.Fatalf("the zero-argument builtin was never invoked")
		}
		if !got {
			t.Fatalf("expected now() to evaluate true")
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
		expr, err := Parse("a ~= b", WithOperator("~=", Level(10), approxEqual))
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
		expr, err := Parse("a ~= b + c", WithOperator("~=", Level(10), approxEqual))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{
			"a": floatToken(3.0), "b": floatToken(1.0), "c": floatToken(2.0),
		})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected 3 ~= (1 + 2) to be true")
		}
	})

	t.Run("SamePrecAs borrows an existing symbol's precedence", func(t *testing.T) {
		// SamePrecAs("==") resolves to prec 10, so ~= binds looser than + (prec
		// 6): a ~= b + c groups as a ~= (b + c), 3 ~= (1 + 2) -> 3 ~= 3 -> true.
		expr, err := Parse("a ~= b + c", WithOperator("~=", SamePrecAs("=="), approxEqual))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{
			"a": floatToken(3.0), "b": floatToken(1.0), "c": floatToken(2.0),
		})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected 3 ~= (1 + 2) to be true")
		}
	})

	t.Run("SamePrecAs referencing an unknown symbol is rejected", func(t *testing.T) {
		_, err := Parse("a ~= b", WithOperator("~=", SamePrecAs("<nope>"), approxEqual))
		assertErrContains(t, err, "SamePrecAs references an unknown symbol", "<nope>")
	})

	t.Run("collision with an existing operator surfaces an error from Parse", func(t *testing.T) {
		_, err := Parse("a == b", WithOperator("==", Level(10), approxEqual))
		assertErrContains(t, err, "operator already registered", "==")
	})

	t.Run("collision with a precedence-only builtin symbol is rejected", func(t *testing.T) {
		// "=" has a precedence entry but no operators entry, so a check keyed on
		// operators alone would silently overwrite its precedence. Keying on the
		// precedence map catches it.
		_, err := Parse("a", WithOperator("=", Level(10), approxEqual))
		assertErrContains(t, err, "operator already registered", "=")
	})

	t.Run("an invalid symbol character is rejected", func(t *testing.T) {
		_, err := Parse("a", WithOperator("a=", Level(10), approxEqual))
		assertErrContains(t, err, "invalid character", "a=")

		_, err = Parse("a", WithOperator("", Level(10), approxEqual))
		assertErrContains(t, err, "operator symbol is empty")
	})

	t.Run("a registered operator does not leak into a later default Parse", func(t *testing.T) {
		withOpt, err := Parse("a ~= b", WithOperator("~=", Level(10), approxEqual))
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

func TestWithLeftUnary(t *testing.T) {
	// negate: unary minus over an int operand. Used to prove a custom prefix
	// operator both lexes and evaluates end-to-end. The '¬' rune is novel — no
	// built-in operator uses it — so it lexes only via the opRunes overlay.
	negate := func(a any) (any, error) {
		n, ok := a.(int)
		if !ok {
			return nil, RuntimeErr("¬ needs an int", nil)
		}
		return -n, nil
	}

	t.Run("a novel-rune prefix operator lexes and evaluates end-to-end", func(t *testing.T) {
		expr, err := Parse("¬a == b", WithLeftUnary("¬", negate))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{"a": intToken(5), "b": intToken(-5)})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected ¬5 == -5 to be true")
		}
	})

	t.Run("prefix binds tighter than the binary operator it precedes", func(t *testing.T) {
		// ¬ has prec 3, tighter than + (prec 6), so ¬a + b groups as (¬a) + b:
		// (¬5) + 2 -> -3.
		expr, err := Parse("¬a + b == c", WithLeftUnary("¬", negate))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{
			"a": intToken(5), "b": intToken(2), "c": intToken(-3),
		})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected (¬5) + 2 == -3 to be true")
		}
	})

	t.Run("collision with a built-in unary operator surfaces an error from Parse", func(t *testing.T) {
		// "!" is the built-in left-unary logical negation, keyed under "L!".
		_, err := Parse("!a", WithLeftUnary("!", negate))
		assertErrContains(t, err, "unary operator already registered", "!")
	})

	t.Run("an invalid symbol character is rejected", func(t *testing.T) {
		_, err := Parse("a", WithLeftUnary("a¬", negate))
		assertErrContains(t, err, "invalid character", "a¬")

		_, err = Parse("a", WithLeftUnary("", negate))
		assertErrContains(t, err, "operator symbol is empty")
	})

	t.Run("a registered prefix operator does not leak into a later default Parse", func(t *testing.T) {
		withOpt, err := Parse("¬a == b", WithLeftUnary("¬", negate))
		assertNoErr(t, err)
		got, err := withOpt.Evaluate(MapScope{"a": intToken(1), "b": intToken(-1)})
		assertNoErr(t, err)
		if !got {
			t.Fatalf("expected ¬1 == -1 to evaluate true when registered")
		}

		// A plain Parse with no options must NOT see the custom prefix operator
		// nor its novel rune: if the option had mutated the shared globals (ops,
		// prec or opRunes), `¬a` would lex and evaluate; instead it must fail to
		// parse because '¬' is an unknown operator.
		if _, err = Parse("¬a == b"); err == nil {
			t.Fatalf("custom prefix operator leaked into a default Parse")
		}
	})
}

func TestWithRightUnary(t *testing.T) {
	// fact: postfix factorial over an int operand. Proves a custom postfix
	// operator lexes and evaluates end-to-end. The '°' rune is novel — no
	// built-in operator uses it — so it lexes only via the opRunes overlay.
	fact := func(a any) (any, error) {
		n, ok := a.(int)
		if !ok || n < 0 {
			return nil, RuntimeErr("° needs a non-negative int", nil)
		}
		result := 1
		for i := 2; i <= n; i++ {
			result *= i
		}
		return result, nil
	}

	t.Run("a novel-rune postfix operator lexes and evaluates end-to-end", func(t *testing.T) {
		expr, err := Parse("a° == b", WithRightUnary("°", fact))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{"a": intToken(4), "b": intToken(24)})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected 4° == 24 to be true")
		}
	})

	t.Run("postfix binds tighter than the binary operator it follows", func(t *testing.T) {
		// ° has prec 2, tighter than + (prec 6), so a + b° groups as a + (b°):
		// 2 + (3°) -> 2 + 6 -> 8.
		expr, err := Parse("a + b° == c", WithRightUnary("°", fact))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{
			"a": intToken(2), "b": intToken(3), "c": intToken(8),
		})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected 2 + (3°) == 8 to be true")
		}
	})

	t.Run("postfix binds tighter than a prefix operator", func(t *testing.T) {
		// ° has prec 2, tighter than the built-in prefix minus (prec 3), so -a°
		// groups as -(a°): -(3°) -> -6.
		expr, err := Parse("-a° == b", WithRightUnary("°", fact))
		assertNoErr(t, err)

		result, err := expr.Evaluate(MapScope{"a": intToken(3), "b": intToken(-6)})
		assertNoErr(t, err)
		if !result {
			t.Fatalf("expected -(3°) == -6 to be true")
		}
	})

	t.Run("a symbol cannot be both left- and right-unary", func(t *testing.T) {
		negate := func(a any) (any, error) { return a, nil }

		// Registering ~ as left-unary then attempting right-unary must fail: the
		// two roles share the bare-sym precedence and handleOp resolves the role
		// by position, so a dual registration is incoherent.
		_, err := Parse("a", WithLeftUnary("~", negate), WithRightUnary("~", fact))
		assertErrContains(t, err, "opposite unary", "~")

		// And the reverse direction: right-unary first, then left-unary.
		_, err = Parse("a", WithRightUnary("~", fact), WithLeftUnary("~", negate))
		assertErrContains(t, err, "opposite unary", "~")
	})

	t.Run("collision with a built-in binary operator surfaces an error from Parse", func(t *testing.T) {
		// "==" is a built-in binary operator, so it already owns the bare-sym key.
		_, err := Parse("a", WithRightUnary("==", fact))
		assertErrContains(t, err, "operator already registered", "==")
	})

	t.Run("collision with a built-in unary operator surfaces an error from Parse", func(t *testing.T) {
		// "!" is the built-in left-unary logical negation, keyed under "L!"; its
		// reciprocal must block a right-unary registration too.
		_, err := Parse("a", WithRightUnary("!", fact))
		assertErrContains(t, err, "opposite unary", "!")
	})

	t.Run("an invalid symbol character is rejected", func(t *testing.T) {
		_, err := Parse("a", WithRightUnary("a°", fact))
		assertErrContains(t, err, "invalid character", "a°")

		_, err = Parse("a", WithRightUnary("", fact))
		assertErrContains(t, err, "operator symbol is empty")
	})

	t.Run("a registered postfix operator does not leak into a later default Parse", func(t *testing.T) {
		withOpt, err := Parse("a° == b", WithRightUnary("°", fact))
		assertNoErr(t, err)
		got, err := withOpt.Evaluate(MapScope{"a": intToken(3), "b": intToken(6)})
		assertNoErr(t, err)
		if !got {
			t.Fatalf("expected 3° == 6 to evaluate true when registered")
		}

		// A plain Parse with no options must NOT see the custom postfix operator
		// nor its novel rune: if the option had mutated the shared globals (ops,
		// prec or opRunes), `a°` would lex and evaluate; instead it must fail to
		// parse because '°' is an unknown operator.
		if _, err = Parse("a° == b"); err == nil {
			t.Fatalf("custom postfix operator leaked into a default Parse")
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

// box widens every Go integer/float kind a user builtin might return into
// intToken/floatToken (unbox normalizes back to int/float64, so this is
// one-directional and not part of the round-trip table above).
func TestBoxWidensNumericTypes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		token Token
	}{
		{"int8", int8(-5), intToken(-5)},
		{"int16", int16(-5), intToken(-5)},
		{"int32", int32(-5), intToken(-5)},
		{"int64", int64(-5), intToken(-5)},
		{"uint8", uint8(5), intToken(5)},
		{"uint16", uint16(5), intToken(5)},
		{"uint32", uint32(5), intToken(5)},
		{"uint", uint(5), intToken(5)},
		{"uint64", uint64(5), intToken(5)},
		{"float32", float32(2.5), floatToken(2.5)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := box(test.value)
			assertNoErr(t, err)
			if !reflect.DeepEqual(token, test.token) {
				t.Fatalf("box(%#v) = %#v, want %#v", test.value, token, test.token)
			}
		})
	}
}

func TestBoxIntegerOverflow(t *testing.T) {
	overflowing := []any{uint64(math.MaxUint64), ^uint(0)}
	for _, value := range overflowing {
		_, err := box(value)
		assertErrContains(t, err, "overflows gparse's platform int")
	}
}
