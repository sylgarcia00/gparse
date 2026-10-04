package gparse

type ReservedWordParser func(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error)

var reservedWordParsers = map[string]ReservedWordParser{
	"#": lineComment,
}

// lineComment consumes the rest of the current line, emitting no token so the
// comment is transparent to the surrounding expression. It stops at (without
// consuming) the newline, which the main parse loop then skips as whitespace.
// Mirrors cparse's LineComment (builtin-features/reservedWords.inc,
// `parser.add("#", &LineComment)`).
func lineComment(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error) {
	i := index
	for i < len(expr) && expr[i] != '\n' {
		i++
	}
	return i, nil
}

// reservedKeywords maps identifier-shaped literals to the constant Token they
// stand for. Unlike reservedWordParsers (which run custom parsing logic), these
// are fixed values emitted in place of the identifier, so `true`/`false` lex as
// boolToken rather than being resolved as scope variables.
var reservedKeywords = map[string]Token{
	"true":  boolToken(true),
	"false": boolToken(false),
}
