package gparse

import "unicode"

// Option overlays a per-call custom entry onto the registry a single Parse call
// resolves against. Options run in order before parsing; returning an error
// aborts the Parse (see Parse). They never touch the package-level defaults —
// an option that writes to a shared map copies it first (see copyBuiltins), so
// custom entries stay isolated to the call that registered them.
type Option func(*registry) error

// copyBuiltins ensures reg.builtins is a copy owned by this registry before it
// is written to. defaultRegistry aliases the package-level builtinFunctions map
// by reference, so writing to it directly would leak custom entries into every
// other caller; the first mutating option copies it lazily instead.
func (reg *registry) copyBuiltins() {
	if reg.builtinsCopied {
		return
	}

	dup := make(map[string]Function, len(reg.builtins)+1)
	for name, fn := range reg.builtins {
		dup[name] = fn
	}
	reg.builtins = dup
	reg.builtinsCopied = true
}

// WithBuiltin registers a variadic builtin callable by name inside expressions,
// e.g. WithBuiltin("geodist", geodist) enables geodist(a, b). The function takes
// its arguments as native Go values (int/float/string/bool/[]any/map[string]any/
// nil) and returns one; gparse boxes/unboxes across the token boundary (see box
// and unbox). Registering a name that already exists (a default like len, or one
// added by an earlier option) is an error, surfaced from Parse — shadowing is a
// footgun, so collisions fail loudly.
func WithBuiltin(name string, fn func(args ...any) (any, error)) Option {
	return func(reg *registry) error {
		if _, exists := reg.builtins[name]; exists {
			return ParserErr("builtin already registered", map[string]any{
				"name": name,
			})
		}

		// A name that lexes as a reserved literal (e.g. true/false) or a
		// reserved-word parser can never resolve to a builtin, so accepting it
		// would register a dead function — reject it to keep the collision
		// promise honest instead of failing silently at eval time.
		if _, reserved := reservedKeywords[name]; reserved {
			return ParserErr("builtin name is a reserved keyword", map[string]any{
				"name": name,
			})
		}
		if _, reserved := reservedWordParsers[name]; reserved {
			return ParserErr("builtin name is a reserved word", map[string]any{
				"name": name,
			})
		}

		reg.copyBuiltins()
		reg.builtins[name] = wrapBuiltin(name, fn)
		return nil
	}
}

// Prec is a custom operator's precedence, passed to WithOperator. Build one
// with Level for a raw numeric level or with SamePrecAs to borrow an existing
// symbol's level. It is resolved against the registry at option-application
// time (see resolve) so SamePrecAs can name a built-in — or an operator added
// by an earlier option — without the caller knowing its numeric level.
type Prec struct {
	level int
	// ref, when non-empty, names the symbol whose level this precedence borrows;
	// it takes priority over level and is resolved against the registry.
	ref string
}

// Level is the precedence of a custom operator as a raw numeric level (lower
// binds tighter; see the built-in levels in opPrecedence, e.g. == is 10, + is
// 6), e.g. WithOperator("~=", Level(10), approxEqual).
func Level(level int) Prec {
	return Prec{level: level}
}

// SamePrecAs is the precedence of a custom operator borrowed from an existing
// symbol, so a caller can say "bind like ==" instead of hard-coding a level,
// e.g. WithOperator("~=", SamePrecAs("=="), approxEqual). existingSym may be a
// built-in or an operator registered by an earlier option; an unknown symbol is
// an error, surfaced from Parse.
func SamePrecAs(existingSym string) Prec {
	return Prec{ref: existingSym}
}

// resolve returns the numeric precedence level, looking ref up in reg.prec when
// SamePrecAs was used. An unknown ref is an error rather than a silent default,
// so a typo'd symbol fails loudly at option-application time.
func (p Prec) resolve(reg *registry) (int, error) {
	if p.ref == "" {
		return p.level, nil
	}

	level, exists := reg.prec[p.ref]
	if !exists {
		return 0, ParserErr("SamePrecAs references an unknown symbol", map[string]any{
			"symbol": p.ref,
		})
	}
	return level, nil
}

// WithOperator registers a binary infix operator symbol callable inside
// expressions, e.g. WithOperator("~=", Level(10), approxEqual) enables a ~= b.
// prec sets the operator's precedence, either a raw level (Level) or one
// borrowed from an existing symbol (SamePrecAs). fn takes both operands as
// native Go values (int/float/string/bool/[]any/map[string]any/nil) and returns
// one; gparse boxes/unboxes across the token boundary (see box and unbox).
//
// The symbol may use a novel rune (e.g. ~): the rune set the lexer scans
// against is derived per registry, so registering the operator makes it lex.
// Every rune of the symbol must be a legal operator character — it may not be a
// digit, letter, space, or one of the token-starting characters (see
// opStartingChars) the lexer treats as an operator boundary; otherwise the
// symbol could never be scanned back out and registration fails.
//
// Registering a symbol that collides with an existing operator (a built-in like
// == or one added by an earlier option) is an error, surfaced from Parse.
func WithOperator(sym string, prec Prec, fn func(a, b any) (any, error)) Option {
	return func(reg *registry) error {
		if err := validateOpSymbol(sym); err != nil {
			return err
		}

		// Check prec, not ops: every built-in symbol has a precedence entry, but
		// some (=, (), the L-/L+/L! unaries) are precedence-only with no ops
		// entry. Keying the collision check on prec — the same map we write to
		// below — makes any built-in symbol fail loudly instead of silently
		// overwriting its precedence.
		if _, exists := reg.prec[sym]; exists {
			return ParserErr("operator already registered", map[string]any{
				"symbol": sym,
			})
		}

		level, err := prec.resolve(reg)
		if err != nil {
			return err
		}

		reg.copyOps()
		reg.copyPrec()
		reg.ops[opToken(sym)] = wrapOperator(sym, fn)
		reg.prec[sym] = level
		registerOpRunes(reg, sym)
		return nil
	}
}

// WithLeftUnary registers a left-unary (prefix) operator symbol callable inside
// expressions, e.g. WithLeftUnary("¬", logicalNot) enables ¬a. fn takes the
// single operand as a native Go value (int/float/string/bool/[]any/
// map[string]any/nil) and returns one; gparse boxes/unboxes across the token
// boundary (see box and unbox).
//
// Per the rpn_builder convention, a left-unary operator is keyed under "L"+sym
// in the precedence table (the built-ins L-, L+ and L! do the same), while its
// Operator is keyed under the bare sym in ops — the RPN builder normalizes the
// "L" prefix away before dispatch (see normalizeOp). The novel-rune, copy-on-
// write and validation discipline mirrors WithOperator.
//
// Registering a symbol whose "L"+sym key collides with an existing unary
// operator is an error, surfaced from Parse.
func WithLeftUnary(sym string, fn func(a any) (any, error)) Option {
	return func(reg *registry) error {
		if err := validateOpSymbol(sym); err != nil {
			return err
		}

		// A left-unary needs two precedence entries, mirroring the built-in
		// unaries (!, -, + all carry both a bare and an "L"-prefixed level): the
		// bare sym is what the lexer checks to recognize the symbol as a known
		// operator (see the reg.prec[op] gate in parse), and "L"+sym is what the
		// RPN builder checks to dispatch it as a prefix (see handleOp). Reject if
		// either key is taken so a collision fails loudly instead of silently
		// overwriting a built-in's precedence.
		unaryKey := "L" + sym
		if _, exists := reg.prec[unaryKey]; exists {
			return ParserErr("unary operator already registered", map[string]any{
				"symbol": sym,
			})
		}
		if _, exists := reg.prec[sym]; exists {
			return ParserErr("operator already registered", map[string]any{
				"symbol": sym,
			})
		}

		reg.copyOps()
		reg.copyPrec()
		// The Operator is keyed under the bare sym: handleLeftUnary pushes
		// "L"+sym onto the op stack, and normalizeOp strips the "L" before the
		// RPN op token is emitted, so eval resolves ops[sym].
		reg.ops[opToken(sym)] = wrapLeftUnary(sym, fn)
		reg.prec[sym] = leftUnaryPrec
		reg.prec[unaryKey] = leftUnaryPrec
		registerOpRunes(reg, sym)
		return nil
	}
}

// leftUnaryPrec is the precedence shared by every custom left-unary operator,
// matching the built-in unaries L-, L+ and L! (opPrecedence). Prefix operators
// bind tighter than the arithmetic and comparison operators they precede, so a
// single fixed level is enough; a symbolic override would only invite a custom
// prefix to bind looser than its operand, which prefix semantics never want.
const leftUnaryPrec = 3

// validateOpSymbol reports whether sym is a usable custom operator symbol: it
// must be non-empty and every rune must be a legal operator character (see
// isValidOpRune). Shared by WithOperator and WithLeftUnary.
func validateOpSymbol(sym string) error {
	if sym == "" {
		return ParserErr("operator symbol is empty", nil)
	}
	for _, c := range sym {
		if !isValidOpRune(c) {
			return ParserErr("operator symbol has an invalid character", map[string]any{
				"symbol": sym,
				"char":   string(c),
			})
		}
	}
	return nil
}

// registerOpRunes adds every rune of sym to the registry's opRunes overlay so a
// novel rune lexes. opRunes is owned by the registry (see defaultRegistry), so
// the runes can be added directly; without this the lexer would not scan the
// custom symbol back out of the expression.
func registerOpRunes(reg *registry, sym string) {
	for _, c := range sym {
		reg.opRunes[c] = true
	}
}

// isValidOpRune reports whether c may appear in a custom operator symbol. It
// must not be a token character the lexer routes elsewhere (digits, letters,
// quotes, brackets, whitespace) nor an operator-boundary character
// (opStartingChars) that would prevent the multi-rune symbol from being scanned
// as a single operator.
func isValidOpRune(c rune) bool {
	if opStartingChars[c] {
		return false
	}
	if unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.IsSpace(c) {
		return false
	}
	// '.' is member access (a built-in operator with special lexer handling in
	// parseNumber and the '.' case of parse); quotes start string literals.
	// A custom symbol using either could never be scanned back as one operator.
	switch c {
	case '\'', '"', '.':
		return false
	}
	return true
}

// wrapOperator adapts a user-facing binary func(a, b any) into the internal
// Operator shape: it unboxes both operand Tokens to native values, calls fn,
// then boxes the result back into a Token. A box failure on the result names
// the offending operator so the error is traceable.
func wrapOperator(sym string, fn func(a, b any) (any, error)) Operator {
	return func(left Token, right Token, op opToken, data *EvaluationData) (Token, error) {
		result, err := fn(unbox(left), unbox(right))
		if err != nil {
			return nil, err
		}

		token, err := box(result)
		if err != nil {
			return nil, RuntimeErr("operator returned an unsupported value", map[string]any{
				"operator": sym,
				"error":    err,
			})
		}
		return token, nil
	}
}

// wrapLeftUnary adapts a user-facing prefix func(a any) into the internal
// Operator shape. A left-unary operator is dispatched with a
// unaryPlaceholderToken as its left operand (see handleLeftUnary), so only the
// right operand carries the value: it is unboxed, passed to fn, and the result
// boxed back into a Token. A box failure on the result names the offending
// operator so the error is traceable.
func wrapLeftUnary(sym string, fn func(a any) (any, error)) Operator {
	return func(left Token, right Token, op opToken, data *EvaluationData) (Token, error) {
		result, err := fn(unbox(right))
		if err != nil {
			return nil, err
		}

		token, err := box(result)
		if err != nil {
			return nil, RuntimeErr("operator returned an unsupported value", map[string]any{
				"operator": sym,
				"error":    err,
			})
		}
		return token, nil
	}
}

// wrapBuiltin adapts a user-facing variadic func(...any) into the internal
// Function shape: it unboxes each argument Token to a native value, calls fn,
// then boxes the result back into a Token. A box failure on the result names
// the offending builtin so the error is traceable.
func wrapBuiltin(name string, fn func(args ...any) (any, error)) Function {
	return func(args []Token, scope mapToken) (Token, error) {
		values := make([]any, len(args))
		for i, arg := range args {
			values[i] = unbox(arg)
		}

		result, err := fn(values...)
		if err != nil {
			return nil, err
		}

		token, err := box(result)
		if err != nil {
			return nil, RuntimeErr("builtin returned an unsupported value", map[string]any{
				"builtin": name,
				"error":   err,
			})
		}
		return token, nil
	}
}

// box converts a native Go value produced by a user builtin into the equivalent
// internal Token. It covers the value kinds gparse exposes to hosts: int, float,
// string, bool, list ([]any), map (map[string]any) and none (nil). An
// unsupported type is an error rather than a panic, so a misbehaving builtin
// fails the evaluation instead of crashing the process.
func box(value any) (Token, error) {
	switch v := value.(type) {
	case nil:
		return noneToken{}, nil
	case int:
		return intToken(v), nil
	case float64:
		return floatToken(v), nil
	case string:
		return strToken(v), nil
	case bool:
		return boolToken(v), nil
	case []any:
		list := make(listToken, len(v))
		for i, elem := range v {
			token, err := box(elem)
			if err != nil {
				return nil, err
			}
			list[i] = token
		}
		return list, nil
	case map[string]any:
		m := make(mapToken, len(v))
		for key, elem := range v {
			token, err := box(elem)
			if err != nil {
				return nil, err
			}
			m[key] = token
		}
		return m, nil
	default:
		return nil, RuntimeErr("cannot box value into a token", map[string]any{
			"value": value,
		})
	}
}

// unbox converts an internal Token into the native Go value handed to a user
// builtin, inverting box. Any token kind box does not produce (or a lazily
// resolved one) falls through as the Token itself, so a builtin can still
// inspect it if it wants; nil maps to noneToken via box on the way back.
func unbox(token Token) any {
	if lazy, ok := token.(Resolver); ok {
		token = lazy.Resolve()
	}

	switch v := token.(type) {
	case noneToken:
		return nil
	case intToken:
		return int(v)
	case floatToken:
		return float64(v)
	case strToken:
		return string(v)
	case boolToken:
		return bool(v)
	case listToken:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = unbox(elem)
		}
		return out
	case mapToken:
		out := make(map[string]any, len(v))
		for key, elem := range v {
			out[key] = unbox(elem)
		}
		return out
	default:
		return token
	}
}
