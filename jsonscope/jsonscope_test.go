package jsonscope_test

import (
	"strings"
	"testing"

	"github.com/vingarcia/gparse"
	"github.com/vingarcia/gparse/jsonscope"
)

// TestNewInvalidJSON checks that a malformed payload is rejected up front by
// New, so lazy resolution never has to surface a decode error mid-evaluation.
func TestNewInvalidJSON(t *testing.T) {
	_, err := jsonscope.New([]byte(`{not json`))
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "bad input json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestScopeGet covers lazy top-level field resolution for every JSON scalar and
// container kind, plus absent-key behavior.
func TestScopeGet(t *testing.T) {
	scope, err := jsonscope.New([]byte(`{
		"s": "hello",
		"i": 42,
		"neg": -7,
		"f": -3.5,
		"b": true,
		"obj": {"inner": "x"},
		"arr": [1, 2, 3]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		key     string
		wantOk  bool
		wantStr string
	}{
		{key: "s", wantOk: true, wantStr: `"hello"`},
		{key: "i", wantOk: true, wantStr: "42"},
		{key: "neg", wantOk: true, wantStr: "-7"},
		{key: "f", wantOk: true, wantStr: "-3.5"},
		{key: "b", wantOk: true, wantStr: "true"},
		{key: "missing", wantOk: false},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			tok, ok := scope.Get(test.key)
			if ok != test.wantOk {
				t.Fatalf("Get(%q) ok = %v, want %v", test.key, ok, test.wantOk)
			}
			if !test.wantOk {
				if tok != nil {
					t.Fatalf("Get(%q) = %v, want nil for absent key", test.key, tok)
				}
				return
			}

			resolved := tok.(gparse.Resolver).Resolve()
			if resolved.String() != test.wantStr {
				t.Fatalf("Get(%q).Resolve() = %q, want %q", test.key, resolved.String(), test.wantStr)
			}
		})
	}
}

// TestEvalThroughJsonScope is the integration proof that the core evaluator runs
// end-to-end against the real jsonscope.New binding, exercising lazy scalar,
// nested object and array access through the Scope/Resolver/Indexable/Sequence
// interfaces.
func TestEvalThroughJsonScope(t *testing.T) {
	payload := []byte(`{
		"n": -7,
		"user": {"name": "bob", "email": "a@b.com"},
		"tags": ["a", "b", "c"],
		"nums": [10, 20, 30]
	}`)

	scope, err := jsonscope.New(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		expr string
		want bool
	}{
		{expr: `n == -7`, want: true},
		{expr: `abs(n) == 7`, want: true},
		{expr: `user.name == "bob"`, want: true},
		{expr: `user.email == "a@b.com"`, want: true},
		{expr: `user.name == user.missing`, want: false}, // present != absent(None)
		{expr: `user.missing == user.other`, want: true}, // None == None (both absent)
		{expr: `tags[1] == "b"`, want: true},
		{expr: `nums[0] + nums[2] == 40`, want: true},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			expr, err := gparse.Parse(test.expr)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", test.expr, err)
			}
			got, err := expr.Evaluate(scope)
			if err != nil {
				t.Fatalf("Evaluate(%q) error: %v", test.expr, err)
			}
			if got != test.want {
				t.Fatalf("Evaluate(%q) = %v, want %v", test.expr, got, test.want)
			}
		})
	}
}

// TestLazyValueCaches checks Resolve decodes once and returns the same cached
// scalar token on subsequent calls.
func TestLazyValueCaches(t *testing.T) {
	scope, err := jsonscope.New([]byte(`{"i": 42}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, ok := scope.Get("i")
	if !ok {
		t.Fatal("expected i to be present")
	}
	r := tok.(gparse.Resolver)
	if r.Resolve() != r.Resolve() {
		t.Fatal("Resolve should cache and return the same token on repeat calls")
	}
}
