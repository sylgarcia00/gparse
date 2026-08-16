package gparse

// Scope is a source-agnostic name→Token binding that the core evaluator reads.
//
// It decouples the language core from any particular input shape: a host binds
// values from whatever source it has (a Go map, JSON, a struct, a database row)
// by implementing Get, and the core never learns where the tokens came from.
//
// Get reports whether name is bound; an unbound name returns (nil, false) so the
// evaluator can distinguish "absent" from a bound None/zero value.
type Scope interface {
	Get(name string) (Token, bool)
}

// MapScope adapts a plain map[string]Token to Scope, keeping the common case a
// single line: expr.Eval(gparse.MapScope{"x": ...}).
type MapScope map[string]Token

// Get looks name up in the map, returning (value, true) when present.
func (m MapScope) Get(name string) (Token, bool) {
	t, ok := m[name]
	return t, ok
}
