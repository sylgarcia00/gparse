package gparse

import "testing"

func TestVarTokenResolveScopeChain(t *testing.T) {
	tests := map[string]struct {
		vars     mapToken
		variable varToken
		expected Token
	}{
		"resolves from the immediate scope": {
			vars:     mapToken{"a": intToken(1)},
			variable: varToken{"a"},
			expected: intToken(1),
		},
		"climbs one parent scope on a miss": {
			vars:     mapToken{"a": intToken(1)}.getChildMap(),
			variable: varToken{"a"},
			expected: intToken(1),
		},
		"climbs several parent scopes": {
			vars:     mapToken{"a": intToken(7)}.getChildMap().getChildMap(),
			variable: varToken{"a"},
			expected: intToken(7),
		},
		"inner scope shadows the parent": {
			vars: func() mapToken {
				child := mapToken{"a": intToken(1)}.getChildMap()
				child["a"] = intToken(2)
				return child
			}(),
			variable: varToken{"a"},
			expected: intToken(2),
		},
		"unknown variable falls back to its string form": {
			vars:     mapToken{"a": intToken(1)}.getChildMap(),
			variable: varToken{"missing"},
			expected: strToken("missing"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := test.variable.Resolve(test.vars, nil)
			if got != test.expected {
				t.Errorf("expected %v, got %v", test.expected, got)
			}
		})
	}
}
