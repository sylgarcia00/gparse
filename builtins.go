package gparse

// builtinFunctions is the registry of built-in functions, keyed by the name
// used to call them in an expression (e.g. `len(x)`, `type(x)`); the parser
// turns a matching identifier into the mapped Function token (see parse).
//
// len and type are single-argument; min and max are variadic and exercise the
// multi-argument call path that the "," (tuple) executor unblocks (see commaOp
// in operators.go): a call like `min(a, b, c)` arrives here as a spread of one
// arg per element.
var builtinFunctions = map[string]Function{
	"len":  builtinLen,
	"type": builtinType,
	"min":  builtinMin,
	"max":  builtinMax,
}

// builtinLen implements `len(x)`: the number of elements of a list or map, or
// the byte length of a string (matching how str[i] indexes by byte, see
// indexOp). Any other argument type returns a RuntimeErr. It requires exactly
// one argument.
func builtinLen(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("len", args)
	if err != nil {
		return nil, err
	}

	switch v := arg.(type) {
	case strToken:
		return intToken(len(v)), nil
	case listToken:
		return intToken(len(v)), nil
	case mapToken:
		return intToken(len(v)), nil
	default:
		return nil, RuntimeErr("len() argument is not sizable", map[string]any{
			"argument": arg,
		})
	}
}

// builtinType implements `type(x)`: a strToken naming the runtime type of its
// single argument. The names are Go-idiomatic and consistent with the token
// kinds used across the package ("int", "float", "string", "bool", "list",
// "map", "none"); an unrecognized token kind returns a RuntimeErr.
func builtinType(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("type", args)
	if err != nil {
		return nil, err
	}

	switch arg.(type) {
	case intToken:
		return strToken("int"), nil
	case floatToken:
		return strToken("float"), nil
	case strToken:
		return strToken("string"), nil
	case boolToken:
		return strToken("bool"), nil
	case listToken:
		return strToken("list"), nil
	case mapToken:
		return strToken("map"), nil
	case noneToken:
		return strToken("none"), nil
	default:
		return nil, RuntimeErr("type() argument has an unknown type", map[string]any{
			"argument": arg,
		})
	}
}

// builtinMin implements `min(a, b, ...)`: the smallest of its numeral
// arguments, and builtinMax the largest. Both require at least one argument and
// accept any mix of intToken and floatToken; the original token (not its
// float64 form) is returned so `min(1, 2)` stays an intToken while
// `min(1, 2.5)` returns the floatToken. A non-numeral argument is a RuntimeErr.
func builtinMin(args []Token, scope mapToken) (Token, error) {
	return reduceNumeral("min", args, func(candidate float64, best float64) bool {
		return candidate < best
	})
}

func builtinMax(args []Token, scope mapToken) (Token, error) {
	return reduceNumeral("max", args, func(candidate float64, best float64) bool {
		return candidate > best
	})
}

// reduceNumeral returns the argument for which better(candidate, currentBest)
// first holds, comparing on the numeral (float64) value while returning the
// original token. It requires at least one argument and rejects non-numerals.
func reduceNumeral(name string, args []Token, better func(candidate float64, best float64) bool) (Token, error) {
	if len(args) == 0 {
		return nil, SyntaxErr("built-in function expects at least one argument", map[string]any{
			"function": name,
		})
	}

	best := args[0]
	bestVal, ok := asFloat(best)
	if !ok {
		return nil, RuntimeErr("argument is not a number", map[string]any{
			"function": name,
			"argument": best,
		})
	}

	for _, arg := range args[1:] {
		val, ok := asFloat(arg)
		if !ok {
			return nil, RuntimeErr("argument is not a number", map[string]any{
				"function": name,
				"argument": arg,
			})
		}
		if better(val, bestVal) {
			best, bestVal = arg, val
		}
	}

	return best, nil
}

// singleArg validates that a built-in was called with exactly one argument and
// returns it. A tupleToken with a different arity (built by a "," call) or an
// empty call is a SyntaxErr naming the offending function.
func singleArg(name string, args []Token) (Token, error) {
	if len(args) != 1 {
		return nil, SyntaxErr("built-in function expects exactly one argument", map[string]any{
			"function": name,
			"gotArgs":  len(args),
		})
	}
	return args[0], nil
}
