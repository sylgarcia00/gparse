package gparse

// builtinFunctions is the registry of built-in functions, keyed by the name
// used to call them in an expression (e.g. `len(x)`, `type(x)`); the parser
// turns a matching identifier into the mapped Function token (see parse).
//
// Only single-argument built-ins exist so far: multi-arg calls need a ","
// (tuple) executor, which does not exist yet.
var builtinFunctions = map[string]Function{
	"len":  builtinLen,
	"type": builtinType,
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
