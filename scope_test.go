package gparse

import "testing"

func TestMapScopeGet(t *testing.T) {
	// MapScope must satisfy Scope.
	var _ Scope = MapScope{}

	scope := MapScope{
		"x":    intToken(7),
		"name": strToken("vini"),
	}

	tests := []struct {
		desc    string
		name    string
		wantOk  bool
		wantStr string
	}{
		{desc: "present int", name: "x", wantOk: true, wantStr: "7"},
		{desc: "present string", name: "name", wantOk: true, wantStr: `"vini"`},
		{desc: "absent name", name: "missing", wantOk: false},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, ok := scope.Get(test.name)
			if ok != test.wantOk {
				t.Fatalf("Get(%q) ok = %v, want %v", test.name, ok, test.wantOk)
			}
			if !test.wantOk {
				if got != nil {
					t.Fatalf("Get(%q) returned %v, want nil for absent name", test.name, got)
				}
				return
			}
			if got.String() != test.wantStr {
				t.Fatalf("Get(%q) = %q, want %q", test.name, got.String(), test.wantStr)
			}
		})
	}
}

// TestEvalThroughMapScope is the core decoupling proof: an expression must
// evaluate against a non-JSON Scope (a plain MapScope, including nested map and
// list containers) through the same Eval/Evaluate path used for JSON.
func TestEvalThroughMapScope(t *testing.T) {
	scope := MapScope{
		"n":    intToken(7),
		"name": strToken("vini"),
		"user": mapToken{"email": strToken("a@b.com")},
		"tags": listToken{strToken("a"), strToken("b"), strToken("c")},
	}

	tests := []struct {
		expr string
		want bool
	}{
		{expr: `n == 7`, want: true},
		{expr: `n > 10`, want: false},
		{expr: `name == "vini"`, want: true},
		{expr: `user.email == "a@b.com"`, want: true},
		{expr: `tags[1] == "b"`, want: true},
		{expr: `user.email == user.phone`, want: false}, // present != absent(None)
		{expr: `user.phone == user.other`, want: true},  // None == None (both absent)
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := Parse(test.expr)
			assertNoErr(t, err)

			got, err := expr.Evaluate(scope)
			assertNoErr(t, err)
			if got != test.want {
				t.Fatalf("Evaluate(%q) = %v, want %v", test.expr, got, test.want)
			}
		})
	}
}

// TestExprEvalReturnsToken checks the core Expr.Eval returns the raw Token
// (not bool-coerced), so a host can build result types other than bool on it.
func TestExprEvalReturnsToken(t *testing.T) {
	boolExpr, err := Parse(`n + 1`)
	assertNoErr(t, err)

	got, err := boolExpr.expr.Eval(MapScope{"n": intToken(41)})
	assertNoErr(t, err)
	if got.String() != "42" {
		t.Fatalf("Eval returned %q, want %q", got.String(), "42")
	}
}

func TestNilMapScopeGet(t *testing.T) {
	// A nil MapScope must behave as an empty scope, never panic.
	var scope MapScope
	if got, ok := scope.Get("anything"); ok || got != nil {
		t.Fatalf("nil MapScope Get = (%v, %v), want (nil, false)", got, ok)
	}
}
