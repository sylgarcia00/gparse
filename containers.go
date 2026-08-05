package cparse

import (
	"strings"

)

// listToken represents a list data type
type listToken []Token

// NewListToken is an internal constructor that matches the signature
// of the type `Function`
func NewListToken(args []Token, scope mapToken) (Token, error) {
	return listToken(args), nil
}

func (t listToken) Clone() Token {
	return t
}

// TODO(vingarcia): Consider how to handle an infinite loop
// in case the list contains itself
func (t listToken) String() string {
	tokens := []string{}
	for _, token := range t {
		tokens = append(tokens, token.String())
	}
	return "[" + strings.Join(tokens, ",") + "]"
}

// mapToken represents a map data type
type mapToken map[string]Token

// NewMapToken is an internal constructor that matches the signature
// of the type `Function`
func NewMapToken(args []Token, scope mapToken) (Token, error) {
	m := mapToken{}
	for _, v := range args {
		kv, notAKVPair := v.(KeyValuePair)
		if !notAKVPair {
			return nil, SyntaxErr("map constructor expects only `key: value` pairs", map[string]any{
				"invalidArgument": v,
			})
		}

		_, alreadyExists := m[kv.Key]
		if alreadyExists {
			return nil, SyntaxErr("duplicate key in map literal", map[string]any{
				"key": kv.Key,
			})
		}

		m[kv.Key] = kv.Value
	}

	return m, nil
}

func (t mapToken) Clone() Token {
	return t
}

// TODO(vingarcia): Consider how to handle an infinite loop
// in case the map contains itself
func (t mapToken) String() string {
	kvPairs := []string{}
	for k, v := range t {
		kvPairs = append(kvPairs, k+":"+v.String())
	}
	return "{" + strings.Join(kvPairs, ",") + "}"
}

func (m mapToken) getChildMap() mapToken {
	return mapToken{
		"$parent": m,
	}
}

type KeyValuePair struct {
	Key   string
	Value Token
}

func (k KeyValuePair) Clone() Token {
	return k
}

func (k KeyValuePair) String() string {
	return k.Key + ":" + k.Value.String()
}

// tupleToken represents tuples like in Python: (1, "foo", false)
type tupleToken []Token

func (t tupleToken) Clone() Token {
	return t
}

func (t tupleToken) String() string {
	items := []string{}
	for _, token := range t {
		items = append(items, token.String())
	}

	return "(" + strings.Join(items, ",") + ")"
}
