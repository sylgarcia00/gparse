# gparse

A Go port of [cparse](https://github.com/cparse/cparse), a shunting-yard
expression parser and evaluator.

**Status: early port, work in progress.** The parsing infrastructure
(tokenizer, RPN/shunting-yard builder, operator-precedence table, container
literals) is functional; the execution engine currently supports only a small
set of operators. The C++ `test-shunting-yard.cpp` suite is being ported
section by section as the behavior spec.

## Design notes

Unlike the C++ original, this port intentionally avoids a "smart" wrapper
type like `packToken`. Values are represented by the `Token` interface with
plain concrete types, and operators use type switches — idiomatic Go over
C++-style abstraction.
