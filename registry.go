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

	// builtinsCopied, opsCopied and precCopied track whether the matching map
	// is a copy this registry owns. defaultRegistry aliases the package-level
	// maps by reference; the first option that mutates one copies it (see the
	// copy* helpers) so custom entries never leak into the globals.
	builtinsCopied bool
	opsCopied      bool
	precCopied     bool

	// opRunes is the set of runes that may appear in an operator symbol,
	// derived from ops. The lexer uses it to tell operator characters apart
	// from other characters. It is derived here (rather than from a separate
	// global) so a future custom op introducing a novel rune extends this set.
	// It is always owned by the registry (built fresh in defaultRegistry), so
	// it can be written to directly when a custom op adds a novel rune.
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

// copyOps ensures reg.ops is a copy owned by this registry before it is
// written to. defaultRegistry aliases the package-level operators map by
// reference, so writing to it directly would leak custom operators into every
// other caller; the first mutating option copies it lazily instead.
func (reg *registry) copyOps() {
	if reg.opsCopied {
		return
	}

	dup := make(map[opToken]Operator, len(reg.ops)+1)
	for sym, op := range reg.ops {
		dup[sym] = op
	}
	reg.ops = dup
	reg.opsCopied = true
}

// copyPrec ensures reg.prec is a copy owned by this registry before it is
// written to, for the same reason as copyOps.
func (reg *registry) copyPrec() {
	if reg.precCopied {
		return
	}

	dup := make(map[string]int, len(reg.prec)+1)
	for sym, prec := range reg.prec {
		dup[sym] = prec
	}
	reg.prec = dup
	reg.precCopied = true
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
