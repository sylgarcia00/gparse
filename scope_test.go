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

func TestNilMapScopeGet(t *testing.T) {
	// A nil MapScope must behave as an empty scope, never panic.
	var scope MapScope
	if got, ok := scope.Get("anything"); ok || got != nil {
		t.Fatalf("nil MapScope Get = (%v, %v), want (nil, false)", got, ok)
	}
}
