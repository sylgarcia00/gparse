# Design proposal — custom-operator registry

Status: **IMPLEMENTED** (Option A shipped, slices 6b–6c-c, 2026-08-17). Written
by Sylphie 2026-08-14, flagged CORE by Vini. Goal: let a caller extend gparse
with their own operators / builtins / precedence without forking the package.
See the *Extending gparse* section of the README for the shipped public API
(`WithBuiltin`/`WithOperator`/`WithLeftUnary`/`WithRightUnary`/`SamePrecAs`);
Option B (a reusable `Parser` value) remains a future add-on.

## Problem

Everything extensible is a **package-global map**, read directly by the parser:

- `operators map[opToken]Operator` (operators.go)
- `opPrecedence map[string]int` (operators.go)
- `builtinFunctions map[string]Function` (builtins.go)

And the public entry point takes no config:

```go
func Parse(strExpr string) (BoolExpr, error)
```

Two consequences:

1. **No extension point.** A caller who wants a `~=` regex-match operator or a
   `geodist(a,b)` builtin must edit the package. Builtins keep growing in-tree
   (13 today) partly because there is nowhere else to put them.
2. **Mutating the globals is unsafe.** They are shared across every `Parse`
   call in the process; runtime mutation races the RPN builder and leaks
   operators between unrelated callers. So "just add to the map" is not a real
   option.

## Constraint that shapes the design

The RPN builder (`rpn_builder.go`) reads `opPrecedence` **at parse time** to
decide shunting; `evaluate` reads `operators`/`builtinFunctions` **at eval
time**. So a custom op needs to be visible to *both* phases. The clean cut is
to thread one registry object from `Parse` into `parse` → `rpn_builder` and
into the compiled `BoolExpr` so `Evaluate` sees the same funcs.

## Options

### A — Functional-options on `Parse` (recommended)

```go
func Parse(expr string, opts ...Option) (BoolExpr, error)

gparse.WithOperator("~=", 10 /*prec*/, regexMatchOp)
gparse.WithLeftUnary("√", floorSqrtOp)
gparse.WithBuiltin("geodist", geodistFn)
```

- Options build a per-call `registry{ops, prec, builtins}` that **starts as a
  copy of the defaults**, then overlays custom entries. Immutable after Parse.
- `BoolExpr` carries a pointer to its registry so `Evaluate` resolves the same
  funcs. No globals mutated → concurrency-safe by construction.
- Back-compat: `Parse(expr)` with no opts = today's behavior (defaults only).
- Cost: thread `*registry` through `parse`/`rpn_builder`/`evaluate` (they read
  globals directly today — the mechanical part of the change).

### B — Explicit `Parser` value

```go
p := gparse.NewParser().WithOperator(...).WithBuiltin(...)
expr, _ := p.Parse("a ~= b")
```

Same registry mechanics as A; better when one config parses many expressions
(compile the registry once, reuse). More API surface. **A and B compose** — B
is "A with the registry named and reused"; can ship A first, add `Parser`
later without breaking callers.

### C — Keep globals, expose `Register*()` funcs

`gparse.RegisterOperator(...)` mutating the globals. **Rejected:** re-introduces
the process-wide-shared-mutable-state race; no isolation between callers.

## Recommendation

Ship **A** (functional options) first — smallest public surface, fully
back-compatible, kills the global-mutation problem. Add **B** later if a
parse-many-with-one-config need shows up. The bulk of the work is identical
either way: introduce an internal `registry` struct and thread it through the
three read sites.

## Open questions — RESOLVED

Decided in `scope-decoupling.md` (plan of record, 2026-08-15): (1) `SamePrecAs`
alias primary, raw int still accepted; (2) narrow `func(a, b any) (any, error)`
public, full `Token` signature internal; (3) builtins first, operators same v1;
(4) collision is an error. Original phrasing kept below for provenance.

## Open questions for Vini (superseded — see above)

1. **Precedence for custom ops — explicit int, or a "bind like `*`" alias?**
   Raw ints leak the C++ table (operators.go). An alias (`SamePrecAs("*")`) is
   friendlier but needs a name→level lookup. Which API?
2. **Custom operator funcs get the full `Operator` signature**
   (`func(Token, Token, opToken, *EvaluationData)`) — do we expose `Token` and
   the token constructors as public API, or keep them internal and offer a
   narrower typed-helper (e.g. `func(a, b any) (any, error)`) that boxes/unboxes?
   This decides how much of the token layer becomes public contract.
3. **Scope of v1: operators + builtins both, or builtins only first?** Builtins
   are lower-risk (no precedence/shunting interaction) and would already let the
   in-tree builtin list stop growing.
4. Do custom entries **override** a default of the same symbol, or is collision
   an error? (Recommend: error — surprising silent shadowing is worse.)
