package cparse

type ReservedWordParser func(expr []rune, parsingCtx *ParsingCtx, rpnBuilder *RPNBuilder, index int) (newIndex int, err error)

var reservedWordParsers = map[string]ReservedWordParser{}
