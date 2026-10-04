package gparse

type ReservedWordParser func(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error)

var reservedWordParsers = map[string]ReservedWordParser{
	"#":  lineComment,
	"//": lineComment,
	"/*": blockComment,
}

// Only the full two-rune keys "//" and "/*" are registered, never a bare "/":
// "/" is the division operator, and registering it would shadow `a / b`. As a
// consequence, a comment opener immediately followed by operator runes (e.g.
// "//====" or "/*=="), with no separating space, overscans into a single
// unknown operator token and fails to parse, because the operator dispatcher's
// first-rune fallback (eparser.go) looks up "/", which is intentionally absent.
// This matches cparse, whose dispatcher likewise only falls back on a
// single-rune parser and registers none for '/', so "//====" is an error there
// too. (The "#" line comment escapes this because "#" is itself the one-rune
// fallback key.) In practice a comment opener is followed by a space or text,
// so this edge is benign; it is documented here rather than worked around to
// preserve both cparse parity and division.

// lineComment consumes the rest of the current line, emitting no token so the
// comment is transparent to the surrounding expression. It stops at (without
// consuming) the newline, which the main parse loop then skips as whitespace.
// Mirrors cparse's LineComment, registered for both the hash and double-slash
// forms (builtin-features/reservedWords.inc, `parser.add("#", &LineComment)`
// and `parser.add("//", &LineComment)`).
func lineComment(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error) {
	i := index
	for i < len(expr) && expr[i] != '\n' {
		i++
	}
	return i, nil
}

// blockComment consumes everything up to and including the terminating `*/`,
// emitting no token so the comment is transparent to the surrounding
// expression. index points just past the opening `/*`; the returned index is
// just past the closing `*/`. An unterminated comment (no `*/` before the end
// of the input) is a syntax error. Mirrors cparse's SlashStarComment
// (builtin-features/reservedWords.inc, `parser.add("/*", &SlashStarComment)`).
func blockComment(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error) {
	i := index
	for i+1 < len(expr) && !(expr[i] == '*' && expr[i+1] == '/') {
		i++
	}
	if i+1 >= len(expr) {
		// Report the position of the opening `/*` (two runes before the body)
		// so the error points at where the unterminated comment began.
		return 0, SyntaxErr("unterminated block comment", map[string]any{
			"pos": parsingCtx.FormatLineCol(index - 2),
		})
	}
	// Skip past the closing `*/`.
	return i + 2, nil
}

// reservedKeywords maps identifier-shaped literals to the constant Token they
// stand for. Unlike reservedWordParsers (which run custom parsing logic), these
// are fixed values emitted in place of the identifier, so `true`/`false` lex as
// boolToken rather than being resolved as scope variables.
var reservedKeywords = map[string]Token{
	"true":  boolToken(true),
	"false": boolToken(false),
}
