# Design — scope decoupling & core-vs-host-DSL

Status: **PLAN OF RECORD** (Vini approved the decoupling shape 2026-08-15,
issue-thread #681). Written by Sylphie. Consolidates the scope-decoupling
redesign with the custom-operator-registry proposal
(`custom-operator-registry.md`) — they are the same underlying question
("what belongs to the core language vs. the host DSL that embeds it"), so
they're resolved together here. This doc's decisions supersede the four
"Open questions for Vini" in the registry doc.

## The single question

Today gparse is welded to one input shape (`json.RawMessage`) and one fixed
operator/builtin set (package globals). Both are the *host DSL's* concerns
leaking into the *core language*. A core expression language should:

- evaluate over an **abstract scope** (name → value), not know that values
  came from JSON;
- let the host **extend** operators/builtins without forking or mutating
  process globals.

## Part 1 — scope decoupling

### Current shape (welded)

```go
func Parse(strExpr string) (BoolExpr, error)
func (rpn BoolExpr) Evaluate(logLine json.RawMessage) (bool, error) // eparser.go:18
```

`Evaluate` calls `NewLazyJsonMap(logLine)` internally, and `operators.go:507`
type-asserts the concrete `lazyJsonToken` to unwrap it. JSON is not one binding
among many — it is the only door.

### Target shape (approved, from #681)

1. **Core evaluates over a source-agnostic scope.** A minimal interface:

   ```go
   type Scope interface {
       Get(name string) (Token, bool)
   }
   ```

   Lazy binding from any source; the core never learns where tokens came from.
   `map[string]Token` gets a trivial adapter so the common case stays one line.

2. **The `operators.go:507` unwrap becomes an interface, not a concrete
   type-assert.** Any lazily-resolved token implements:

   ```go
   type Resolver interface { Resolve() Token }
   ```

   Core calls `Resolve()` if the value implements it; it no longer names
   `lazyJsonToken`. Same fix applies to the two sites in `tokens.go` (211, 222).

3. **JSON binding moves to a sub-package** (`gparse/jsonscope`).
   `NewLazyJsonMap` relocates there and satisfies `Scope`. Callers do:

   ```go
   expr.Eval(jsonscope.New(line))
   ```

   JSON stays batteries-included — just not the only door.

4. **Generic `Eval` returning `Token` in core; `BoolExpr` becomes the thin
   bool-enforcing wrapper.** "Must return bool" is the host DSL's decision, not
   the core's:

   ```go
   func (rpn Expr) Eval(scope Scope) (Token, error)          // core
   func (rpn BoolExpr) Evaluate(scope Scope) (bool, error)   // thin wrapper: Eval + bool-assert
   ```

### Slice-3 note: narrow scalar-boxing seam is exported

Extracting `jsonscope` into a *separate package* means it can no longer name the
core's unexported scalar token types. Slice 3 therefore exports a minimal boxing
seam — `NewString`, `NewFloat`, `NewBool` — plus two container interfaces
(`Indexable`, `Sequence`) so any host container is indexed through an interface
rather than a concrete `mapToken`/`listToken` assert. This is a deliberate,
narrow reversal of Part 2's "token constructors stay internal" deferral (which
was about the custom-func *registry* surface, not a Scope binding in another
package). Kept as small as possible: no `NewInt` until a caller needs it.

### Back-compat

Pre-1.0, no external users → breaking changes are fine. Keep the old
`Evaluate(json.RawMessage)` shim **only if it costs nothing** (a one-line
wrapper delegating to `jsonscope.New`); if it drags the refactor, drop it.

## Part 2 — extension registry (resolves the 4 open questions)

Ships **functional-options on `Parse`** (option A in the registry doc):

```go
func Parse(expr string, opts ...Option) (BoolExpr, error)
```

Options build a per-call registry that starts as a copy of the defaults, then
overlays custom entries; `Expr`/`BoolExpr` carries a pointer to it so `Eval`
resolves the same funcs. No globals mutated → concurrency-safe by construction.
Explicit `Parser` value (option B) can follow later without breaking callers.

### Decisions on the registry doc's open questions

1. **Precedence API → named alias primary, raw int still accepted.**
   Raw ints leak the C++ precedence table as the *only* path, which is
   unfriendly and couples callers to internals. Ship a helper:

   ```go
   gparse.WithOperator("~=", gparse.SamePrecAs("=="), regexMatchOp)
   ```

   `SamePrecAs(symbol)` resolves to the level via the internal name→level
   lookup; a raw int is still valid for the rare caller who wants a novel level.
   Friendly default, escape hatch preserved.

2. **Custom funcs get the narrow typed helper, not the full `Token` signature.**
   Public v1 contract is `func(a, b any) (any, error)` (and the unary/builtin
   analogues); the registry boxes/unboxes to/from `Token` internally. The full
   `Operator` signature (`func(Token, Token, opToken, *EvaluationData)`) and the
   token constructors stay **internal**. Rationale: pre-1.0, freeze the smallest
   possible public surface — exposing the token layer makes every token type a
   compatibility contract forever. Promote to the full signature later only if a
   real caller needs `*EvaluationData` access; that's additive.

3. **Scope of v1 → builtins first, operators same release.** Builtins are
   lower-risk (no precedence/shunting interaction) and immediately stop the
   in-tree builtin list from growing (13 today). Land `WithBuiltin` first, then
   `WithOperator`/`WithLeftUnary` once the scope/`Eval` refactor below is in.
   Both in v1; builtins are the first slice.

4. **Collision → error, never silent override.** A custom entry whose symbol/
   name already exists in the defaults returns a `Parse` error. Silent shadowing
   of a built-in operator is a debugging trap worse than a loud failure.

## Build order (slices, tests green at every step)

Sequenced so each slice is independently shippable and testable:

1. `Scope` interface + adapter for `map[string]Token`.
2. `Resolver` interface; replace the three `lazyJsonToken` type-asserts
   (operators.go:507, tokens.go:211, tokens.go:222) with `Resolve()`.
3. Token-returning `Eval(scope)` in core; `BoolExpr.Evaluate` becomes the thin
   bool-asserting wrapper over it.
4. Extract `jsonscope` sub-package; move `NewLazyJsonMap`; wire the (optional)
   `Evaluate(json.RawMessage)` shim.
5. `Parse(expr, ...Option)` + internal `registry` threaded through
   `parse`/`rpn_builder`/`eval`; `WithBuiltin` first.
6. `WithOperator`/`WithLeftUnary` + `SamePrecAs`; collision-is-error. (Biggest;
   last.)

Slices 1–4 are the scope decoupling; 5–6 are the registry. They share the
registry-threading plumbing, which is why they're one plan.
