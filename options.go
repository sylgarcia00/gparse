package gparse

import (
	"maps"
	"math"
	"slices"
	"unicode"
)

// Args declares the custom builtins and operators a parse resolves against, on
// top of the package defaults. The zero value Args{} means "defaults only", so
// a caller with nothing to register passes it as-is. Every field is a map
// keyed by the name or symbol it registers, which gives godoc one obvious
// place listing everything that can be passed.
//
// Entries overlay a registry seeded from the package defaults; nothing
// process-global is mutated — a field that writes to a shared map copies it
// first (see copyBuiltins) — so custom entries stay isolated to the
// Parser/Parse call that registered them. A key that collides with a default
// (or with a key from another Args field) is an error, surfaced from
// NewParser/Parse: shadowing is a footgun, so collisions fail loudly.
type Args struct {
	// Builtins registers variadic callables by name inside expressions, e.g.
	// {"geodist": geodist} enables geodist(a, b). Each function takes its
	// arguments as native Go values (int/float/string/bool/[]any/
	// map[string]any/nil) and returns one; gparse boxes/unboxes across the
	// token boundary (see box and unbox). A name that collides with a default
	// builtin (like len) or with a reserved keyword is an error.
	Builtins map[string]func(args ...any) (any, error)

	// Operators registers binary infix operator symbols, e.g.
	// {"~=": {Prec: SamePrecAs("=="), Fn: approxEqual}} enables a ~= b.
	//
	// A symbol may use a novel rune (e.g. ~): the rune set the lexer scans
	// against is derived per registry, so registering the operator makes it
	// lex. Every rune of the symbol must be a legal operator character — it
	// may not be a digit, letter, space, or one of the token-starting
	// characters (see opStartingChars) the lexer treats as an operator
	// boundary; otherwise the symbol could never be scanned back out and
	// registration fails. A symbol that collides with an existing operator is
	// an error.
	Operators map[string]BinaryOperator

	// LeftUnary registers prefix operator symbols, e.g. {"¬": logicalNot}
	// enables ¬a. Operand and result cross the same native-value boundary as
	// Builtins, and symbols obey the same rune rules as Operators. All custom
	// prefix operators bind at the built-in prefix level (see leftUnaryPrec).
	LeftUnary map[string]func(a any) (any, error)

	// RightUnary registers postfix operator symbols, e.g. {"!": factorial}
	// enables a!, under the same boundary and rune rules as LeftUnary. All
	// custom postfix operators bind at the built-in postfix level (see
	// rightUnaryPrec). A symbol may not appear in both LeftUnary and
	// RightUnary: the two roles share the bare-sym precedence entry the lexer
	// keys on, and handleOp resolves prefix vs postfix purely by position, so
	// a dual registration would give one role an incoherent precedence.
	RightUnary map[string]func(a any) (any, error)
}

// BinaryOperator is one Args.Operators entry: a binary infix operator's
// precedence and implementation. (It is not named Operator because that name
// is taken by the internal Token-level operator shape in operators.go.)
type BinaryOperator struct {
	// Prec is the operator's precedence: Level for a raw numeric level, or
	// SamePrecAs to borrow an existing symbol's level. The zero value is
	// Level(0), which binds tighter than every built-in — set it explicitly.
	Prec Prec
	// Fn implements the operator over both operands as native Go values
	// (int/float/string/bool/[]any/map[string]any/nil), returning one; gparse
	// boxes/unboxes across the token boundary (see box and unbox).
	Fn func(a, b any) (any, error)
}

// applyArgs overlays args onto reg, validating every entry. Registration
// order is fixed — builtins, then binary operators, then prefix, then postfix
// — and each map's keys are walked in sorted order, so which error surfaces
// first is deterministic even though Go maps iterate in random order.
func (reg *registry) applyArgs(args Args) error {
	for _, name := range slices.Sorted(maps.Keys(args.Builtins)) {
		if err := reg.registerBuiltin(name, args.Builtins[name]); err != nil {
			return err
		}
	}

	if err := reg.registerBinaryOps(args.Operators); err != nil {
		return err
	}

	for _, sym := range slices.Sorted(maps.Keys(args.LeftUnary)) {
		if err := reg.registerLeftUnary(sym, args.LeftUnary[sym]); err != nil {
			return err
		}
	}

	for _, sym := range slices.Sorted(maps.Keys(args.RightUnary)) {
		if err := reg.registerRightUnary(sym, args.RightUnary[sym]); err != nil {
			return err
		}
	}
	return nil
}

// copyBuiltins ensures reg.builtins is a copy owned by this registry before it
// is written to. defaultRegistry aliases the package-level builtinFunctions map
// by reference, so writing to it directly would leak custom entries into every
// other caller; the first mutating registration copies it lazily instead.
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

// registerBuiltin registers one Args.Builtins entry. Registering a name that
// already exists (a default like len) is an error, surfaced from
// NewParser/Parse — shadowing is a footgun, so collisions fail loudly.
func (reg *registry) registerBuiltin(name string, fn func(args ...any) (any, error)) error {
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

// Prec is a custom operator's precedence, set on BinaryOperator.Prec. Build
// one with Level for a raw numeric level or with SamePrecAs to borrow an
// existing symbol's level. It is resolved against the registry when the Args
// are applied (see registerBinaryOps) so SamePrecAs can name a built-in — or
// another operator in the same Args.Operators map — without the caller
// knowing its numeric level.
type Prec struct {
	level int
	// ref, when non-empty, names the symbol whose level this precedence borrows;
	// it takes priority over level and is resolved against the registry.
	ref string
}

// Level is the precedence of a custom operator as a raw numeric level (lower
// binds tighter; see the built-in levels in opPrecedence, e.g. == is 10, + is
// 6), e.g. BinaryOperator{Prec: Level(10), Fn: approxEqual}.
func Level(level int) Prec {
	return Prec{level: level}
}

// SamePrecAs is the precedence of a custom operator borrowed from an existing
// symbol, so a caller can say "bind like ==" instead of hard-coding a level,
// e.g. BinaryOperator{Prec: SamePrecAs("=="), Fn: approxEqual}. existingSym
// may be a built-in or another operator in the same Args.Operators map; an
// unknown symbol (or a cycle of references) is an error, surfaced from
// NewParser/Parse.
func SamePrecAs(existingSym string) Prec {
	return Prec{ref: existingSym}
}

// registerBinaryOps registers every Args.Operators entry. Symbols and
// collisions are validated up front, then Level-based entries register
// immediately while SamePrecAs entries are set aside and resolved in as many
// passes as it takes, so a reference to another entry of the same map — even
// through a chain — works regardless of Go's random map order. When a full
// pass resolves nothing, the stuck references can never resolve: each one
// either names another stuck symbol (a reference cycle) or nothing at all,
// and erroring loudly beats a silent default.
func (reg *registry) registerBinaryOps(ops map[string]BinaryOperator) error {
	var pending []string
	for _, sym := range slices.Sorted(maps.Keys(ops)) {
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

		if ops[sym].Prec.ref != "" {
			pending = append(pending, sym)
			continue
		}
		reg.registerBinaryOp(sym, ops[sym].Prec.level, ops[sym].Fn)
	}

	for len(pending) > 0 {
		var unresolved []string
		for _, sym := range pending {
			level, exists := reg.prec[ops[sym].Prec.ref]
			if !exists {
				unresolved = append(unresolved, sym)
				continue
			}
			reg.registerBinaryOp(sym, level, ops[sym].Fn)
		}

		if len(unresolved) == len(pending) {
			sym := unresolved[0]
			ref := ops[sym].Prec.ref
			if slices.Contains(unresolved, ref) {
				return ParserErr("SamePrecAs references form a cycle", map[string]any{
					"symbol": sym,
					"ref":    ref,
				})
			}
			return ParserErr("SamePrecAs references an unknown symbol", map[string]any{
				"symbol": ref,
			})
		}
		pending = unresolved
	}
	return nil
}

// registerBinaryOp writes one validated binary operator into the registry,
// copying the shared default maps first so custom entries never leak into the
// package-level globals.
func (reg *registry) registerBinaryOp(sym string, level int, fn func(a, b any) (any, error)) {
	reg.copyOps()
	reg.copyPrec()
	reg.ops[opToken(sym)] = wrapOperator(sym, fn)
	reg.prec[sym] = level
	registerOpRunes(reg, sym)
}

// registerLeftUnary registers one Args.LeftUnary entry.
//
// Per the rpn_builder convention, a left-unary operator is keyed under "L"+sym
// in the precedence table (the built-ins L-, L+ and L! do the same), while its
// Operator is keyed under the bare sym in ops — the RPN builder normalizes the
// "L" prefix away before dispatch (see normalizeOp). The novel-rune, copy-on-
// write and validation discipline mirrors registerBinaryOps.
//
// Registering a symbol whose "L"+sym key collides with an existing unary
// operator is an error, surfaced from NewParser/Parse.
func (reg *registry) registerLeftUnary(sym string, fn func(a any) (any, error)) error {
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
	if err := validateUnarySymbol(reg, sym, "L"); err != nil {
		return err
	}

	reg.copyOps()
	reg.copyPrec()
	// The Operator is keyed under the bare sym: handleLeftUnary pushes
	// "L"+sym onto the op stack, and normalizeOp strips the "L" before the
	// RPN op token is emitted, so eval resolves ops[sym].
	reg.ops[opToken(sym)] = wrapLeftUnary(sym, fn)
	reg.prec[sym] = leftUnaryPrec
	reg.prec["L"+sym] = leftUnaryPrec
	registerOpRunes(reg, sym)
	return nil
}

// registerRightUnary registers one Args.RightUnary entry.
//
// Per the rpn_builder convention, a right-unary operator is keyed under "R"+sym
// in the precedence table, while its Operator is keyed under the bare sym in
// ops. The RPN builder dispatches a symbol as postfix only when it follows an
// operand (handleOp's else-if branch), pushing "R"+sym; normalizeOp strips the
// "R" prefix before dispatch, and handleRightUnary places the operand as the
// left token with a unaryPlaceholderToken on the right (mirror of the prefix
// case). The novel-rune, copy-on-write and validation discipline mirrors
// registerLeftUnary.
//
// Registering a symbol whose "R"+sym key (or its reciprocal "L"+sym / bare-sym
// binary key) collides with an existing operator is an error, surfaced from
// NewParser/Parse.
func (reg *registry) registerRightUnary(sym string, fn func(a any) (any, error)) error {
	if err := validateOpSymbol(sym); err != nil {
		return err
	}

	if err := validateUnarySymbol(reg, sym, "R"); err != nil {
		return err
	}

	reg.copyOps()
	reg.copyPrec()
	// The Operator is keyed under the bare sym: handleRightUnary pushes
	// "R"+sym onto the op stack, and normalizeOp strips the "R" before the
	// RPN op token is emitted, so eval resolves ops[sym].
	reg.ops[opToken(sym)] = wrapRightUnary(sym, fn)
	reg.prec[sym] = rightUnaryPrec
	reg.prec["R"+sym] = rightUnaryPrec
	registerOpRunes(reg, sym)
	return nil
}

// validateUnarySymbol rejects a unary operator registration whose keys collide
// with an existing operator. dir is "L" for prefix or "R" for postfix; the
// check guards three keys: the primary "dir"+sym (this role), the reciprocal
// unary key (the opposite role — a symbol may not be both prefix and postfix,
// since the two share the bare-sym precedence the lexer keys on and handleOp
// picks the role by position), and the bare sym (a binary operator of the same
// name). Any collision fails loudly instead of silently overwriting precedence.
func validateUnarySymbol(reg *registry, sym string, dir string) error {
	if _, exists := reg.prec[dir+sym]; exists {
		return ParserErr("unary operator already registered", map[string]any{
			"symbol": sym,
		})
	}

	reciprocal := "L"
	if dir == "L" {
		reciprocal = "R"
	}
	if _, exists := reg.prec[reciprocal+sym]; exists {
		return ParserErr("symbol already registered as the opposite unary", map[string]any{
			"symbol": sym,
		})
	}

	if _, exists := reg.prec[sym]; exists {
		return ParserErr("operator already registered", map[string]any{
			"symbol": sym,
		})
	}
	return nil
}

// leftUnaryPrec is the precedence shared by every custom left-unary operator,
// matching the built-in unaries L-, L+ and L! (opPrecedence). Prefix operators
// bind tighter than the arithmetic and comparison operators they precede, so a
// single fixed level is enough; a symbolic override would only invite a custom
// prefix to bind looser than its operand, which prefix semantics never want.
const leftUnaryPrec = 3

// rightUnaryPrec is the precedence shared by every custom right-unary operator.
// Postfix operators bind tighter than prefix ones in C-family precedence (the
// built-in postfix (), [] and . sit at level 2, tighter than the prefix unaries
// at 3), so a! parses as (a)! and -a! as -(a!). A single fixed level suffices
// for the same reason it does for prefix operators.
const rightUnaryPrec = 2

// validateOpSymbol reports whether sym is a usable custom operator symbol: it
// must be non-empty and every rune must be a legal operator character (see
// isValidOpRune). Shared by the binary, prefix and postfix registrations.
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

// lexerRoutesElsewhere reports whether the main lexer loop (parse) would route c
// to a non-operator branch rather than the generic-operator branch: a number
// (unicode.IsNumber), a variable/identifier (isVarChar), a string literal
// (quotes), or one of the bracket / member-access characters handled by their
// own switch cases. It is the single source of truth for that routing, shared
// by parse's dispatch intent and by isValidOpRune, so the two cannot drift.
func lexerRoutesElsewhere(c rune) bool {
	switch c {
	case '\'', '"': // string-literal openers
		return true
	case '(', ')', '[', ']', '{', '}', '.': // bracket + member-access cases
		return true
	}
	return unicode.IsNumber(c) || isVarChar(c)
}

// isValidOpRune reports whether c may appear in a custom operator symbol. It
// must not be a rune the lexer routes elsewhere (lexerRoutesElsewhere), a
// whitespace token separator, nor an operator-boundary character
// (opStartingChars) that would prevent the multi-rune symbol from being scanned
// as a single operator.
func isValidOpRune(c rune) bool {
	if lexerRoutesElsewhere(c) {
		return false
	}
	if opStartingChars[c] {
		return false
	}
	return !unicode.IsSpace(c)
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

// wrapRightUnary adapts a user-facing postfix func(a any) into the internal
// Operator shape. A right-unary operator is dispatched with its operand as the
// left token and a unaryPlaceholderToken as the right (see handleRightUnary),
// the mirror of the prefix case, so only the left operand carries the value: it
// is unboxed, passed to fn, and the result boxed back into a Token. A box
// failure on the result names the offending operator so the error is traceable.
func wrapRightUnary(sym string, fn func(a any) (any, error)) Operator {
	return func(left Token, right Token, op opToken, data *EvaluationData) (Token, error) {
		result, err := fn(unbox(left))
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
// intOverflowErr reports an integer that does not fit gparse's platform int
// (the underlying type of intToken). Boxing it as int would silently wrap, so
// box errors instead of losing precision.
func intOverflowErr(value any) error {
	return RuntimeErr("integer value overflows gparse's platform int", map[string]any{
		"value": value,
	})
}

func box(value any) (Token, error) {
	switch v := value.(type) {
	case nil:
		return noneToken{}, nil
	case int:
		return intToken(v), nil
	case int8:
		return intToken(v), nil
	case int16:
		return intToken(v), nil
	case int32:
		return intToken(v), nil
	case int64:
		if v > math.MaxInt || v < math.MinInt {
			return nil, intOverflowErr(value)
		}
		return intToken(v), nil
	case uint8:
		return intToken(v), nil
	case uint16:
		return intToken(v), nil
	case uint32:
		if uint64(v) > math.MaxInt {
			return nil, intOverflowErr(value)
		}
		return intToken(v), nil
	case uint:
		if uint64(v) > math.MaxInt {
			return nil, intOverflowErr(value)
		}
		return intToken(v), nil
	case uint64:
		if v > math.MaxInt {
			return nil, intOverflowErr(value)
		}
		return intToken(v), nil
	case float64:
		return floatToken(v), nil
	case float32:
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
