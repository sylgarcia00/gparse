package gparse

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
