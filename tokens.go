package gparse

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

type Token interface {
	Clone() Token
	String() string
}

// Resolver is implemented by tokens whose concrete value is produced lazily,
// such as a JSON field kept as raw bytes until first use. The evaluator unwraps
// through this interface so it never depends on a concrete lazy type — letting
// a host supply its own lazy-backed tokens from any source.
type Resolver interface {
	Resolve() Token
}

// opToken represents operators
type opToken string

func (o opToken) Clone() Token {
	return o
}

func (o opToken) String() string {
	return string(o)
}

// unaryPlaceholderToken is only used for making it easier
// to handle unary operators as if they were binary.
//
// When parsing the expression it will just be dropped
type unaryPlaceholderToken struct{}

func (u unaryPlaceholderToken) Clone() Token {
	return u
}

func (unaryPlaceholderToken) String() string {
	return "UnaryToken"
}

// Function represents a custom function for our parser
type Function func(args []Token, scope mapToken) (Token, error)

// Type implements the Token interface
func (f Function) Clone() Token {
	return f
}

func (f Function) String() string {
	return "[function]"
}

// strToken represent string tokens
type strToken string

func (s strToken) Clone() Token {
	return s
}

func (s strToken) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// intToken represent real numerical values
type intToken int

func (i intToken) Clone() Token {
	return i
}

func (i intToken) String() string {
	return strconv.Itoa(int(i))
}

// floatToken represent real numerical values
type floatToken float64

func (f floatToken) Clone() Token {
	return f
}

func (f floatToken) String() string {
	return strconv.FormatFloat(float64(f), 'f', -1, 64)
}

// noneToken represents the absence of a value, mirroring cparse's NONE type.
// It is produced by map indexing on a missing key (see indexOp) and matches
// cparse's packToken::None(). There is no literal syntax for it yet.
type noneToken struct{}

func (n noneToken) Clone() Token {
	return n
}

func (noneToken) String() string {
	return "None"
}

// boolToken represent boolean values
type boolToken bool

func (b boolToken) Clone() Token {
	return b
}

func (b boolToken) String() string {
	if bool(b) {
		return "true"
	}
	return "false"
}

// refToken is used to keep references
type refToken struct {
	// The value found at compilation time
	originalValue Token

	// The key used to reference this token at compilation time
	key varToken

	// The scope of variables available at compilation time:
	origin mapToken
}

func (r refToken) Clone() Token {
	return r
}

func (r refToken) String() string {
	return "&" + strings.Join(r.key, ".")
}

func (r refToken) Resolve(localScope map[string]Token) Token {
	// Local variables have no map of origin,
	// thus, require a localScope to be resolved:
	if r.origin == nil && localScope != nil {
		// Get the most recent value from the local scope. A local ref resolves
		// purely against the function-local scope; the host Scope is not
		// consulted here (nil).
		refValue := r.key.Resolve(localScope, nil)
		if refValue != nil {
			// TODO(vingarcia): Consider cloning this value first
			return refValue
		}
	}

	// In last case return the compilation-time value:
	// TODO(vingarcia): Consider cloning this value first
	return r.originalValue
}

// varToken represent variable references
//
// A variable such as: `a.b['and c']`
// would be stored here as: []string{"a", "b", "and c"}
type varToken []string

func (v varToken) Clone() Token {
	return append(varToken{}, v...)
}

func (v varToken) String() string {
	if len(v) == 0 {
		return ""
	}

	out := v[0]
	for _, str := range v[1:] {
		onlyVarChars := isVarChar(rune(str[0]))
		if onlyVarChars {
			for _, c := range str[1:] {
				if !isVarChar(c) && !unicode.IsNumber(c) {
					onlyVarChars = false
					break
				}
			}
		}

		if onlyVarChars {
			out += "." + str
		} else {
			b, _ := json.Marshal(str)
			out += "[" + string(b) + "]"
		}
	}
	return out
}

// resolveInScopeChain looks up key in vars, following the "$parent" link to
// enclosing scopes on a miss. When the chain is exhausted it falls back to the
// source-agnostic scope, so top-level names bound by the host (e.g. a JSON
// payload) resolve after any function-local variables that shadow them.
// Returns nil if not found in any scope.
func resolveInScopeChain(vars map[string]Token, scope Scope, key string) Token {
	for vars != nil {
		if value, ok := vars[key]; ok {
			return value
		}

		parent, ok := vars["$parent"].(mapToken)
		if !ok {
			break
		}
		vars = parent
	}

	if scope != nil {
		if value, ok := scope.Get(key); ok {
			return value
		}
	}

	return nil
}

func (v varToken) Resolve(vars map[string]Token, scope Scope) Token {
	// The first identifier climbs the scope chain via "$parent"
	// (see mapToken.getChildMap), mirroring cparse's parent-scope
	// resolution, then falls back to the host scope. Nested field access
	// below stays within resolved Indexable containers.
	value := resolveInScopeChain(vars, scope, v[0])
	if lazy, ok := value.(Resolver); ok {
		value = lazy.Resolve()
	}

	for _, str := range v[1:] {
		m, ok := value.(Indexable)
		if !ok {
			return strToken(v.String())
		}

		value, _ = m.Get(str)
		if lazy, ok := value.(Resolver); ok {
			value = lazy.Resolve()
		}
	}

	if value == nil {
		return strToken(v.String())
	}

	return value
}
