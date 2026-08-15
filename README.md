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
- First built-ins: `strip()`, `split()`.

Deliberately absent (mirroring cparse): ordering comparisons between strings.

Open design questions, not yet decided: first-class boolean literals
(`true`/`false` currently lex as strings, as cparse leaves room to do) and a
registry for user-defined operators.

## Design notes

Unlike the C++ original, this port has no "smart" wrapper type like
`packToken`. Values are the `Token` interface over plain concrete types, and
operators use type switches. That's a real trade — cparse's wrapper buys
convenience at call sites — but wrapping every value to make Go feel like
C++ fights the language. I'd rather the port read like Go that happens to
implement cparse's behavior than like C++ transliterated.
