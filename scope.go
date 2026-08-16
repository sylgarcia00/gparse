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

// NewString, NewFloat and NewBool box a Go scalar into the matching core Token.
// They are the public seam a host binding (e.g. the jsonscope sub-package, or a
// MapScope caller) uses to build scope values from its own source without
// depending on the unexported concrete token types.
//
// Note: this intentionally diverges from the design doc's slice-5 note that
// "token constructors stay internal". That deferral was about the custom-func
// registry surface; a Scope binding living in a *separate package* has no other
// way to construct scalar Tokens, so a minimal boxing seam is exported here.
// Kept deliberately narrow (no NewInt until a caller needs it — additive later).
func NewString(s string) Token {
	return strToken(s)
}

func NewFloat(f float64) Token {
	return floatToken(f)
}

func NewBool(b bool) Token {
	return boolToken(b)
}

// MapScope adapts a plain map[string]Token to Scope, keeping the common case a
// single line: expr.Eval(gparse.MapScope{"x": ...}).
type MapScope map[string]Token

// Get looks name up in the map, returning (value, true) when present.
func (m MapScope) Get(name string) (Token, bool) {
	t, ok := m[name]
	return t, ok
}
