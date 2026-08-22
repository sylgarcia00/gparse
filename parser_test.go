package gparse

import (
	"sync"
	"testing"
)

func TestParser(t *testing.T) {
	t.Run("one Parser parses many expressions", func(t *testing.T) {
		p, err := NewParser()
		assertNoErr(t, err)

		cases := []struct {
			expr string
			vars MapScope
			want bool
		}{
			{"a == 1", MapScope{"a": intToken(1)}, true},
			{"a != 1", MapScope{"a": intToken(1)}, false},
			{"a > b", MapScope{"a": intToken(3), "b": intToken(2)}, true},
		}
		for _, c := range cases {
			expr, err := p.Parse(c.expr)
			assertNoErr(t, err)

			got, err := expr.Evaluate(c.vars)
			assertNoErr(t, err)
			if got != c.want {
				t.Fatalf("%q: expected %v, got %v", c.expr, c.want, got)
			}
		}
	})

	t.Run("a custom builtin applied once works across many parses", func(t *testing.T) {
		double := func(args ...any) (any, error) {
			return args[0].(int) * 2, nil
		}

		p, err := NewParser(WithBuiltin("double", double))
		assertNoErr(t, err)

		// Parse twice: proves the option is not consumed by the first parse.
		for range 2 {
			expr, err := p.Parse("double(a) == 42")
			assertNoErr(t, err)

			got, err := expr.Evaluate(MapScope{"a": intToken(21)})
			assertNoErr(t, err)
			if !got {
				t.Fatalf("expected double(21) == 42 to be true")
			}
		}
	})

	t.Run("an option error surfaces from NewParser and yields no Parser", func(t *testing.T) {
		p, err := NewParser(WithBuiltin("true", func(args ...any) (any, error) {
			return true, nil
		}))
		if err == nil {
			t.Fatalf("expected a name-collision error from NewParser")
		}
		if p != nil {
			t.Fatalf("expected a nil Parser on option error, got %v", p)
		}
	})

	t.Run("ParseExpr exposes the type-agnostic core surface", func(t *testing.T) {
		p, err := NewParser()
		assertNoErr(t, err)

		expr, err := p.ParseExpr("a + b")
		assertNoErr(t, err)

		tok, err := expr.Eval(MapScope{"a": intToken(2), "b": intToken(3)})
		assertNoErr(t, err)
		if got := tok.(intToken); got != 5 {
			t.Fatalf("expected 2 + 3 == 5, got %v", got)
		}
	})

	// Pins the concurrency invariant: after NewParser freezes the registry, the
	// parse path only reads it, so many goroutines may share one *Parser. Run
	// with -race to make a data race a test failure.
	t.Run("concurrent parses on one Parser are race-free", func(t *testing.T) {
		p, err := NewParser(WithBuiltin("double", func(args ...any) (any, error) {
			return args[0].(int) * 2, nil
		}))
		assertNoErr(t, err)

		var wg sync.WaitGroup
		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				expr, err := p.Parse("double(a) == 42")
				if err != nil {
					t.Errorf("unexpected parse error: %v", err)
					return
				}
				got, err := expr.Evaluate(MapScope{"a": intToken(21)})
				if err != nil {
					t.Errorf("unexpected eval error: %v", err)
					return
				}
				if !got {
					t.Errorf("expected double(21) == 42 to be true")
				}
			}()
		}
		wg.Wait()
	})
}
