package gparse

import (
	"encoding/json"
	"testing"
)

// jsonScope decodes a JSON object into a Scope for the in-package evaluation
// tests. It intentionally does NOT depend on the jsonscope sub-package (which
// would create an import cycle, since jsonscope imports gparse); instead it
// builds resolved core tokens directly. The lazy jsonscope decoder and the
// Resolver-unwrap path are covered by the jsonscope package's own tests.
func jsonScope(tb testing.TB, b []byte) Scope {
	tb.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		tb.Fatalf("invalid test JSON %q: %v", string(b), err)
	}

	scope := MapScope{}
	for k, raw := range m {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			tb.Fatalf("invalid test JSON value for %q: %v", k, err)
		}
		scope[k] = toToken(v)
	}
	return scope
}

// toToken maps a decoded encoding/json value onto the matching core Token,
// mirroring how jsonscope resolves values (all JSON numbers become floatToken).
func toToken(v any) Token {
	switch x := v.(type) {
	case string:
		return strToken(x)
	case float64:
		return floatToken(x)
	case bool:
		return boolToken(x)
	case map[string]any:
		m := mapToken{}
		for k, val := range x {
			m[k] = toToken(val)
		}
		return m
	case []any:
		l := listToken{}
		for _, val := range x {
			l = append(l, toToken(val))
		}
		return l
	case nil:
		return noneToken{}
	default:
		return strToken("")
	}
}
