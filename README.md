# gparse

A Go port of [cparse](https://github.com/cparse/cparse), a shunting-yard
expression parser and evaluator — the kind of small language you reach for
when you want users to write `age >= 18 && plan == "pro"` against JSON data
and get a boolean back, without embedding a scripting engine.

I'm porting it section by section, using cparse's own C++ test suite
(`test-shunting-yard.cpp`) as the behavior spec: each section gets ported,
made to pass, and only then do I move on. I like this way of working — the
spec already exists, it just lives in another language, and every green
section is ground that stays won.

## Status

Further along than "work in progress" suggests. Working today:

- Tokenizer, RPN/shunting-yard builder, and the operator-precedence table.
- Container literals (lists, maps) and lazy evaluation of JSON values —
  fields are only unmarshaled when an expression actually touches them.
- Comparison (`<` `>` `<=` `>=` `==` `!=`), arithmetic (`+ - * / % **`),
  unary operators, and boolean logic with `None` behaving as falsy.
- Built-ins for numbers (`len`, `min`, `max`, `abs`, `floor`, `ceil`, `round`,
  `sqrt`), conversions (`str`, `int`, `float`), and strings (`lower`, `upper`,
  `strip`, `split`, `join`, `replace`, `contains`, `startswith`, `endswith`,
  `find`).
- A registry for user-defined builtins and operators (see *Extending gparse*).

Deliberately absent (mirroring cparse): ordering comparisons between strings.

Open design questions, not yet decided: first-class boolean literals
(`true`/`false` currently lex as strings, as cparse leaves room to do).

## Extending gparse

`Parse` takes an `Args` struct, so a caller can add its own builtins and
operators without forking the package — and godoc shows the whole extension
surface in one place, the way [krest](https://github.com/vingarcia/krest) does
it. The zero value `gparse.Args{}` means "defaults only". Entries build a
per-call registry that starts as a copy of the defaults and overlays the
custom ones — nothing process-global is mutated, so concurrent `Parse` calls
never leak operators into each other. A registered name that collides with an
existing one is an error, surfaced from `Parse`: silent shadowing is a worse
surprise than a loud failure.

```go
expr, err := gparse.Parse(`geodist(a, b) < 100 && tag ~= "^v"`, gparse.Args{
    Builtins: map[string]func(args ...any) (any, error){
        "geodist": geodist, // geodist(a, b)
    },
    Operators: map[string]gparse.BinaryOperator{
        "~=": {Prec: gparse.SamePrecAs("=="), Fn: regexMatch}, // a ~= b
    },
    LeftUnary:  map[string]func(a any) (any, error){"√": sqrtFloor}, // √x
    RightUnary: map[string]func(a any) (any, error){"°": factorial}, // x°
})
```

(Once the accepted Go proposal
[golang/go#12854](https://github.com/golang/go/issues/12854) lands, the struct
type can be elided at the call site: `gparse.Parse(expr, {Builtins: ...})`.)

Custom functions speak native Go values, not the internal `Token` type —
gparse boxes/unboxes across the boundary. Supported types are any Go integer
kind (`int`, `int8`…`int64`, `uint`…`uint64` — boxed as `int`, erroring if the
value overflows the platform `int`), any float (`float32`/`float64` — boxed as
`float64`), `string`, `bool`, `[]any`, `map[string]any`, and `nil` (for
`None`). Values handed back to your function are always `int`/`float64`:

| Args field | Signature | Adds |
| --- | --- | --- |
| `Builtins` | `func(args ...any) (any, error)` | a callable `name(...)` |
| `Operators` | `BinaryOperator{Prec, Fn func(a, b any) (any, error)}` | a binary infix `a op b` |
| `LeftUnary` | `func(a any) (any, error)` | a prefix `op a` |
| `RightUnary` | `func(a any) (any, error)` | a postfix `a op` |

A `BinaryOperator` carries its precedence in `Prec`: `gparse.Level(n)` for a
raw level, or `gparse.SamePrecAs("*")` to borrow an existing symbol's level
without knowing its number — the referenced symbol may be a built-in or
another operator of the same `Args`, in any order. Unary operators bind at the
built-in unary levels (prefix looser than postfix, as in C). A symbol may take
one unary role, not both — the two would share a single precedence and be told
apart only by position, which is incoherent, so registering the same symbol as
left- and right-unary is an error.

### Parsing many expressions with one config

`Parse(expr, args)` is the one-shot door: it applies and validates the args on
every call. When one configuration parses many expressions, build a `Parser`
once with `NewParser` and reuse it — the args are applied and validated a
single time and the registry is frozen, so each `Parse` skips that work:

```go
p, err := gparse.NewParser(gparse.Args{
    Builtins: map[string]func(args ...any) (any, error){"geodist": geodist},
    Operators: map[string]gparse.BinaryOperator{
        "~=": {Prec: gparse.SamePrecAs("=="), Fn: regexMatch},
    },
})
if err != nil {
    return err
}

for _, rule := range rules {
    expr, err := p.Parse(rule)
    // ...
}
```

A `Parser` is read-only after `NewParser` returns — the parse and evaluate
paths only read the frozen registry — so the same `*Parser` is safe to share
across goroutines.

## Design notes

Unlike the C++ original, this port has no "smart" wrapper type like
`packToken`. Values are the `Token` interface over plain concrete types, and
operators use type switches. That's a real trade — cparse's wrapper buys
convenience at call sites — but wrapping every value to make Go feel like
C++ fights the language. I'd rather the port read like Go that happens to
implement cparse's behavior than like C++ transliterated.
