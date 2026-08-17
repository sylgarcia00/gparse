package gparse

// registry is the immutable, per-call set of operators, precedences and
// builtins the eval pipeline resolves against. It replaces the direct reads of
// the package-level globals (operators, opPrecedence, builtinFunctions) so a
// later slice can overlay per-call custom entries without mutating shared
// process state. It is built once per Parse call and never mutated afterwards.
type registry struct {
	ops      map[opToken]Operator
	prec     map[string]int
	builtins map[string]Function

	// builtinsCopied tracks whether builtins is a copy this registry owns.
	// defaultRegistry aliases the package-level builtinFunctions map by
	// reference; the first option that registers a builtin copies it (see
	// registry.copyBuiltins) so custom entries never leak into the globals.
	builtinsCopied bool

	// opRunes is the set of runes that may appear in an operator symbol,
	// derived from ops. The lexer uses it to tell operator characters apart
	// from other characters. It is derived here (rather than from a separate
	// global) so a future custom op introducing a novel rune extends this set.
	opRunes map[rune]bool
}

// defaultRegistry builds a registry seeded from the package-level defaults.
func defaultRegistry() *registry {
	reg := &registry{
		ops:      operators,
		prec:     opPrecedence,
		builtins: builtinFunctions,
	}
	reg.opRunes = deriveOpRunes(reg.ops)
	return reg
}

// deriveOpRunes collects every rune used by any operator symbol in ops.
func deriveOpRunes(ops map[opToken]Operator) map[rune]bool {
	runeSet := map[rune]bool{}
	for op := range ops {
		for _, c := range op {
			runeSet[c] = true
		}
	}
	return runeSet
}
