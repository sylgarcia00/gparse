package gparse

import (
	"strings"
)

// Indexable is a map-like container the evaluator can index by string key.
//
// Any Token implementing it is treated as a map by the indexing operator and by
// nested variable field access, so a host can supply its own (possibly lazy)
// container from any source without the core knowing its concrete type. Get
// reports (nil, false) when the key is absent.
type Indexable interface {
	Token
	Get(key string) (Token, bool)
}

// Sequence is a list-like container the evaluator can index by integer
// position. Any Token implementing it is treated as a list by the indexing
// operator, letting a host supply its own (possibly lazy) sequence from any
// source. At is only called with an index already normalized into [0, Len()).
type Sequence interface {
	Token
	Len() int
	At(i int) Token
}

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

// Len implements Sequence.
func (t listToken) Len() int {
	return len(t)
}

// At implements Sequence.
func (t listToken) At(i int) Token {
	return t[i]
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

// Get implements Indexable.
func (m mapToken) Get(key string) (Token, bool) {
	t, ok := m[key]
	return t, ok
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

// Clone is intentionally shallow (returns the same slice) because a tupleToken
// is never stored as an RPN literal: it is only ever built at evaluation time
// by commaOp (which appends to it in place) and immediately consumed by a call
// or list/map constructor within the same evaluation. copyRPN therefore never
// needs to deep-copy one. If tuples ever become RPN literals, commaOp's in-place
// append would alias shared state and this must become a deep copy.
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
