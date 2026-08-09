package gparse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type Token interface {
	Clone() Token
	String() string
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
		// Get the most recent value from the local scope:
		refValue := r.key.Resolve(localScope)
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

// resolveInScopeChain looks up key in vars, following the "$parent"
// link to enclosing scopes on a miss. Returns nil if not found in any scope.
func resolveInScopeChain(vars map[string]Token, key string) Token {
	for vars != nil {
		if value, ok := vars[key]; ok {
			return value
		}

		parent, ok := vars["$parent"].(mapToken)
		if !ok {
			return nil
		}
		vars = parent
	}
	return nil
}

func (v varToken) Resolve(vars map[string]Token) Token {
	// The first identifier climbs the scope chain via "$parent"
	// (see mapToken.getChildMap), mirroring cparse's parent-scope
	// resolution. Nested field access below stays within one map.
	value := resolveInScopeChain(vars, v[0])
	if lazy, ok := value.(lazyJsonToken); ok {
		value = lazy.Value()
	}

	for _, str := range v[1:] {
		m, ok := value.(mapToken)
		if !ok {
			return strToken(v.String())
		}

		value = m[str]
		if lazy, ok := value.(lazyJsonToken); ok {
			value = lazy.Value()
		}
	}

	if value == nil {
		return strToken(v.String())
	}

	return value
}

// lazyJsonToken will unmarshal from
// JSON in a lazy way, i.e. it will keep
// most of the json as a json.RawMessage
// and only unmarshal further if/when required.
type lazyJsonToken struct {
	value Token
	json  json.RawMessage
}

// NewLazyJsonMap will parse the map in an lazy way so we don't
// unmarshal anything we don't need to at first
// It also validates the input JSON so we don't need to handle
// issues with invalid JSON later on
func NewLazyJsonMap(b []byte) (mapToken, error) {
	var m map[string]json.RawMessage
	err := json.Unmarshal(b, &m)
	if err != nil {
		return mapToken{}, ParserErr("bad input json received", map[string]any{
			"invalidJson": string(b),
			"error":       err.Error(),
		})
	}

	token := mapToken{}
	for k, v := range m {
		token[k] = lazyJsonToken{
			json: v,
		}
	}

	return token, nil
}

func (l lazyJsonToken) Clone() Token {
	return l
}

func (l lazyJsonToken) String() string {
	if l.value != nil {
		return l.value.String()
	}
	return string(l.json)
}

func (l lazyJsonToken) Value() Token {
	if l.value == nil {
		var err error
		l.value, err = unmarshalLazyValue(l.json)
		if err != nil {
			panic(fmt.Sprintf(
				`invalid JSON received for lazyJsonToken, this should have been validated before calling Evaluate!: %s`,
				err,
			))
		}
	}

	return l.value
}

func unmarshalLazyValue(rawJSON []byte) (Token, error) {
	rawJSON = bytes.TrimSpace(rawJSON)
	switch rawJSON[0] {
	case
		byte('-'), // JSON numbers may carry a leading minus (leading '+' is not valid JSON)
		byte('0'), byte('1'), byte('2'), byte('3'), byte('4'),
		byte('5'), byte('6'), byte('7'), byte('8'), byte('9'):

		var f float64
		return floatToken(f), json.Unmarshal(rawJSON, &f)

	case byte('"'):
		var s string
		return strToken(s), json.Unmarshal(rawJSON, &s)

	case byte('f'), byte('t'):
		var b bool
		return boolToken(b), json.Unmarshal(rawJSON, &b)

	case byte('{'):
		var m map[string]json.RawMessage
		err := json.Unmarshal(rawJSON, &m)
		if err != nil {
			return nil, err
		}

		token := mapToken{}
		for k, v := range m {
			token[k] = lazyJsonToken{
				json: v,
			}
		}
		return token, nil

	case byte('['):
		var l []json.RawMessage
		err := json.Unmarshal(rawJSON, &l)
		if err != nil {
			return nil, err
		}

		token := listToken{}
		for _, v := range l {
			token = append(token, lazyJsonToken{
				json: v,
			})
		}
		return token, nil

	default:
		return nil, InternalErr("unrecognized JSON value received on unmarshalLazyValue", map[string]any{
			"value": string(rawJSON),
		})
	}
}
