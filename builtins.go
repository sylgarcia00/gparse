package gparse

import (
	"math"
	"strconv"
	"strings"
)

// builtinFunctions is the registry of built-in functions, keyed by the name
// used to call them in an expression (e.g. `len(x)`, `type(x)`); the parser
// turns a matching identifier into the mapped Function token (see parse).
//
// len and type are single-argument; min and max are variadic and exercise the
// multi-argument call path that the "," (tuple) executor unblocks (see commaOp
// in operators.go): a call like `min(a, b, c)` arrives here as a spread of one
// arg per element.
var builtinFunctions = map[string]Function{
	"len":   builtinLen,
	"type":  builtinType,
	"min":   builtinMin,
	"max":   builtinMax,
	"abs":   builtinAbs,
	"floor": builtinFloor,
	"ceil":  builtinCeil,
	"round": builtinRound,
	"sqrt":  builtinSqrt,
	"str":   builtinStr,
	"int":   builtinInt,
	"float": builtinFloat,
	"lower": builtinLower,
	"upper": builtinUpper,
	"strip": builtinStrip,
	"split": builtinSplit,
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

// builtinAbs implements `abs(x)`: the absolute value of a single numeral
// argument, preserving its type — abs(-3) stays an intToken, abs(-3.5) a
// floatToken (mirroring how min/max return the original token kind). A
// non-numeral argument is a RuntimeErr. Like Go's own arithmetic, the abs of
// the most-negative intToken overflows back to itself; callers needing that
// edge should use a float.
func builtinAbs(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("abs", args)
	if err != nil {
		return nil, err
	}

	switch v := arg.(type) {
	case intToken:
		if v < 0 {
			return -v, nil
		}
		return v, nil
	case floatToken:
		if v < 0 {
			return -v, nil
		}
		return v, nil
	default:
		return nil, RuntimeErr("abs() argument is not a number", map[string]any{
			"argument": arg,
		})
	}
}

// builtinFloor implements `floor(x)`: the largest integer <= x. It takes a
// single numeral and returns an intToken — floor/ceil/round always yield whole
// numbers, and an int is the more useful type for a filter predicate (e.g.
// `floor(x) == 3`). An intToken argument is already whole and is returned
// unchanged; a floatToken is rounded down via math.Floor. A non-numeral
// argument is a RuntimeErr.
func builtinFloor(args []Token, scope mapToken) (Token, error) {
	return numeralToInt("floor", args, math.Floor)
}

// builtinCeil implements `ceil(x)`: the smallest integer >= x. Same return-type
// contract as floor (intToken out; int in stays that int); a floatToken is
// rounded up via math.Ceil. A non-numeral argument is a RuntimeErr.
func builtinCeil(args []Token, scope mapToken) (Token, error) {
	return numeralToInt("ceil", args, math.Ceil)
}

// builtinRound implements `round(x)`: the nearest integer, rounding halves away
// from zero (Go's math.Round semantics). Same return-type contract as floor
// (intToken out; int in stays that int). A non-numeral argument is a
// RuntimeErr.
func builtinRound(args []Token, scope mapToken) (Token, error) {
	return numeralToInt("round", args, math.Round)
}

// builtinSqrt implements `sqrt(x)`: the square root of a single numeral. Unlike
// floor/ceil/round the result is generally irrational, so it always returns a
// floatToken (even for a perfect square like sqrt(9) -> 3.0). A negative
// operand is a RuntimeErr rather than a NaN; a non-numeral argument is a
// RuntimeErr too.
func builtinSqrt(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("sqrt", args)
	if err != nil {
		return nil, err
	}

	val, ok := asFloat(arg)
	if !ok {
		return nil, RuntimeErr("sqrt() argument is not a number", map[string]any{
			"argument": arg,
		})
	}
	if val < 0 {
		return nil, RuntimeErr("sqrt of negative number", map[string]any{
			"argument": arg,
		})
	}

	return floatToken(math.Sqrt(val)), nil
}

// numeralToInt is the shared body of floor/ceil/round: it validates a single
// numeral argument and returns an intToken. An intToken is already whole and is
// returned unchanged (int in -> same int out); a floatToken is passed through
// round (math.Floor/Ceil/Round) before the intToken conversion. A non-numeral
// argument is a RuntimeErr naming the calling function.
func numeralToInt(name string, args []Token, round func(float64) float64) (Token, error) {
	arg, err := singleArg(name, args)
	if err != nil {
		return nil, err
	}

	switch v := arg.(type) {
	case intToken:
		return v, nil
	case floatToken:
		return intToken(round(float64(v))), nil
	default:
		return nil, RuntimeErr(name+"() argument is not a number", map[string]any{
			"argument": arg,
		})
	}
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

// builtinStr implements `str(x)`: a strToken holding the string rendering of its
// single argument, mirroring cparse's default_str (tok.str()). A strToken is
// returned unchanged; any other token is rendered via its String() method. Note
// strToken.String() JSON-quotes, so passing through the original (rather than
// re-rendering) is what makes str("a") == "a" instead of "\"a\"".
func builtinStr(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("str", args)
	if err != nil {
		return nil, err
	}

	if s, ok := arg.(strToken); ok {
		return s, nil
	}
	return strToken(arg.String()), nil
}

// builtinInt implements `int(x)`: always an intToken. An intToken passes
// through; a floatToken truncates toward zero (Go int64 conversion); a strToken
// parses as a base-10 integer (mirroring cparse's default_int strtol base 10). A
// boolToken converts to 1/0 — cparse treats bool as a numeric type (the NUM bit
// is set), so this stays faithful. Any other type, or a non-numeric string, is a
// RuntimeErr. Deliberate divergence from cparse: strtol partial-parses ("3abc"
// -> 3); we parse the whole string and error on malformed input (Go-idiomatic).
func builtinInt(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("int", args)
	if err != nil {
		return nil, err
	}

	switch v := arg.(type) {
	case intToken:
		return v, nil
	case floatToken:
		return intToken(int64(v)), nil
	case boolToken:
		if v {
			return intToken(1), nil
		}
		return intToken(0), nil
	case strToken:
		n, parseErr := strconv.ParseInt(string(v), 10, 64)
		if parseErr != nil {
			return nil, RuntimeErr("int() cannot convert string to integer", map[string]any{
				"argument": arg,
				"error":    parseErr,
			})
		}
		return intToken(n), nil
	default:
		return nil, RuntimeErr("int() argument cannot be converted to an integer", map[string]any{
			"argument": arg,
		})
	}
}

// builtinFloat implements `float(x)`: always a floatToken. int/float pass
// through as a float; a strToken parses via strconv.ParseFloat (mirroring
// cparse's default_float strtod). A boolToken converts to 1.0/0.0 — cparse
// treats bool as numeric (the NUM bit is set), so this stays faithful. Any other
// type, or a non-numeric string, is a RuntimeErr. Deliberate divergence from
// cparse: strtod partial-parses ("3abc" -> 3); we parse the whole string and
// error on malformed input (Go-idiomatic).
func builtinFloat(args []Token, scope mapToken) (Token, error) {
	arg, err := singleArg("float", args)
	if err != nil {
		return nil, err
	}

	switch v := arg.(type) {
	case floatToken:
		return v, nil
	case intToken:
		return floatToken(v), nil
	case boolToken:
		if v {
			return floatToken(1), nil
		}
		return floatToken(0), nil
	case strToken:
		f, parseErr := strconv.ParseFloat(string(v), 64)
		if parseErr != nil {
			return nil, RuntimeErr("float() cannot convert string to float", map[string]any{
				"argument": arg,
				"error":    parseErr,
			})
		}
		return floatToken(f), nil
	default:
		return nil, RuntimeErr("float() argument cannot be converted to a float", map[string]any{
			"argument": arg,
		})
	}
}

// builtinLower implements `lower(s)` and builtinUpper `upper(s)`: the single
// strToken argument with ASCII/Unicode case folded down or up (strings.ToLower/
// ToUpper). These exist for case-insensitive JSON-field filtering, e.g.
// `lower(user.email) == "a@b.com"`; only a string is meaningful, so any other
// type is a RuntimeErr rather than a silent pass-through.
func builtinLower(args []Token, scope mapToken) (Token, error) {
	return strTransform("lower", args, strings.ToLower)
}

func builtinUpper(args []Token, scope mapToken) (Token, error) {
	return strTransform("upper", args, strings.ToUpper)
}

// builtinStrip implements `strip(s)`: the single strToken argument with leading
// and trailing whitespace removed (strings.TrimSpace). Like lower/upper it
// targets JSON-field filtering, where values often carry stray padding, e.g.
// `strip(user.name) == "Vini"`; a non-string argument is a RuntimeErr.
func builtinStrip(args []Token, scope mapToken) (Token, error) {
	return strTransform("strip", args, strings.TrimSpace)
}

// builtinSplit implements `split(s, sep)`: a listToken of the substrings of s
// separated by each non-overlapping sep (strings.Split). Both arguments must be
// strToken; anything else is a RuntimeErr. It complements lower/upper/strip for
// working with JSON string fields — e.g. `split(tags, ",")` fans a delimited
// field into a list that len/indexing can then work on. An empty sep splits s
// into its UTF-8 runes, matching Go's own strings.Split semantics.
func builtinSplit(args []Token, scope mapToken) (Token, error) {
	if len(args) != 2 {
		return nil, SyntaxErr("built-in function expects exactly two arguments", map[string]any{
			"function": "split",
			"gotArgs":  len(args),
		})
	}

	s, ok := args[0].(strToken)
	if !ok {
		return nil, RuntimeErr("split() first argument is not a string", map[string]any{
			"argument": args[0],
		})
	}
	sep, ok := args[1].(strToken)
	if !ok {
		return nil, RuntimeErr("split() second argument is not a string", map[string]any{
			"argument": args[1],
		})
	}

	parts := strings.Split(string(s), string(sep))
	out := make(listToken, len(parts))
	for i, p := range parts {
		out[i] = strToken(p)
	}
	return out, nil
}

func strTransform(name string, args []Token, fn func(string) string) (Token, error) {
	arg, err := singleArg(name, args)
	if err != nil {
		return nil, err
	}

	v, ok := arg.(strToken)
	if !ok {
		return nil, RuntimeErr(name+"() argument is not a string", map[string]any{
			"argument": arg,
		})
	}
	return strToken(fn(string(v))), nil
}
