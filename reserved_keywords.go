package gparse

type ReservedWordParser func(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error)

var reservedWordParsers = map[string]ReservedWordParser{}

// reservedKeywords maps identifier-shaped literals to the constant Token they
// stand for. Unlike reservedWordParsers (which run custom parsing logic), these
// are fixed values emitted in place of the identifier, so `true`/`false` lex as
// boolToken rather than being resolved as scope variables.
var reservedKeywords = map[string]Token{
	"true":  boolToken(true),
	"false": boolToken(false),
}
