// Package jsonscope binds a JSON payload as a gparse.Scope, lazily decoding
// fields only when the evaluated expression reads them.
//
// It is gparse's batteries-included JSON door: the core language stays
// source-agnostic (it evaluates over a gparse.Scope), and this package adapts
// JSON to that interface without the core ever depending on encoding/json.
package jsonscope

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/vingarcia/gparse"
)

// New validates the top-level JSON object and returns a gparse.Scope whose
// fields are decoded lazily on first use. Validating up front means the lazy
// decoders further down never have to surface a parse error mid-evaluation.
func New(b []byte) (gparse.Scope, error) {
	var m map[string]json.RawMessage
	err := json.Unmarshal(b, &m)
	if err != nil {
		return nil, gparse.ParserErr("bad input json received", map[string]any{
			"invalidJson": string(b),
			"error":       err.Error(),
		})
	}

	return object(m), nil
}

// object is a lazily-decoded JSON object. It satisfies gparse.Scope (top-level
// name lookup) and gparse.Indexable (nested field access via a.b or a["b"]),
// wrapping each field's raw bytes in a lazyValue resolved on demand.
type object map[string]json.RawMessage

// Get implements gparse.Scope and gparse.Indexable.
func (o object) Get(key string) (gparse.Token, bool) {
	raw, ok := o[key]
	if !ok {
		return nil, false
	}
	return &lazyValue{json: raw}, true
}

// Clone implements gparse.Token. The underlying map is immutable after New, so
// a shallow clone is safe.
func (o object) Clone() gparse.Token {
	return o
}

func (o object) String() string {
	b, _ := json.Marshal(map[string]json.RawMessage(o))
	return string(b)
}

// array is a lazily-decoded JSON array. It satisfies gparse.Sequence so the
// indexing operator can read elements by position; each element stays raw until
// read.
type array []json.RawMessage

// Len implements gparse.Sequence.
func (a array) Len() int {
	return len(a)
}

// At implements gparse.Sequence.
func (a array) At(i int) gparse.Token {
	return &lazyValue{json: a[i]}
}

// Clone implements gparse.Token.
func (a array) Clone() gparse.Token {
	return a
}

func (a array) String() string {
	b, _ := json.Marshal([]json.RawMessage(a))
	return string(b)
}

// lazyValue holds a JSON field as raw bytes until first use, then caches the
// decoded token. It satisfies gparse.Resolver so the evaluator unwraps it
// through the interface, never naming this concrete type.
type lazyValue struct {
	value gparse.Token
	json  json.RawMessage
}

var (
	_ gparse.Resolver  = (*lazyValue)(nil)
	_ gparse.Scope     = object(nil)
	_ gparse.Indexable = object(nil)
	_ gparse.Sequence  = array(nil)
)

// Resolve implements gparse.Resolver, decoding the raw JSON on first call and
// caching the result. The input JSON was validated by New, so a decode error
// here is a bug and panics rather than being silently swallowed.
func (l *lazyValue) Resolve() gparse.Token {
	if l.value == nil {
		v, err := unmarshalLazyValue(l.json)
		if err != nil {
			panic(fmt.Sprintf(
				"invalid JSON received for jsonscope value, this should have been validated by New: %s",
				err,
			))
		}
		l.value = v
	}
	return l.value
}

// Clone implements gparse.Token.
func (l *lazyValue) Clone() gparse.Token {
	return l
}

func (l *lazyValue) String() string {
	if l.value != nil {
		return l.value.String()
	}
	return string(l.json)
}

// unmarshalLazyValue decodes a single JSON value into a gparse.Token, keeping
// objects and arrays lazy (their children stay raw until read). Scalars are
// boxed through the core's exported constructors so this package never depends
// on gparse's unexported token types.
func unmarshalLazyValue(rawJSON []byte) (gparse.Token, error) {
	rawJSON = bytes.TrimSpace(rawJSON)
	switch rawJSON[0] {
	case
		byte('-'), // JSON numbers may carry a leading minus (leading '+' is not valid JSON)
		byte('0'), byte('1'), byte('2'), byte('3'), byte('4'),
		byte('5'), byte('6'), byte('7'), byte('8'), byte('9'):

		var f float64
		err := json.Unmarshal(rawJSON, &f)
		return gparse.NewFloat(f), err

	case byte('"'):
		var s string
		err := json.Unmarshal(rawJSON, &s)
		return gparse.NewString(s), err

	case byte('f'), byte('t'):
		var b bool
		err := json.Unmarshal(rawJSON, &b)
		return gparse.NewBool(b), err

	case byte('{'):
		var m map[string]json.RawMessage
		err := json.Unmarshal(rawJSON, &m)
		if err != nil {
			return nil, err
		}
		return object(m), nil

	case byte('['):
		var l []json.RawMessage
		err := json.Unmarshal(rawJSON, &l)
		if err != nil {
			return nil, err
		}
		return array(l), nil

	default:
		return nil, gparse.InternalErr("unrecognized JSON value received on unmarshalLazyValue", map[string]any{
			"value": string(rawJSON),
		})
	}
}
