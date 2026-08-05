package gparse

import (
	"fmt"
	"strings"
)

type Err struct {
	Code  string
	Title string
	Data  map[string]any
}

func (e Err) Error() string {
	fields := []string{
		e.Code + ": " + e.Title,
	}
	for k, v := range e.Data {
		if err, ok := v.(error); ok {
			v = err.Error()
		}

		fields = append(fields, fmt.Sprintf("%s = %+v", k, v))
	}

	return strings.Join(fields, "; ")
}

func ErrIs(err error, code string) bool {
	e, ok := err.(Err)
	if !ok {
		return false
	}

	return e.Code == code
}

func RuntimeErr(title string, data map[string]any) error {
	return Err{
		Code:  "RuntimeErr",
		Title: title,
		Data:  data,
	}
}

func SyntaxErr(title string, data map[string]any) error {
	return Err{
		Code:  "SyntaxErr",
		Title: title,
		Data:  data,
	}
}

func ParserErr(title string, data map[string]any) error {
	return Err{
		Code:  "ParserErr",
		Title: title,
		Data:  data,
	}
}

func InternalErr(title string, data map[string]any) error {
	return Err{
		Code:  "InternalErr",
		Title: title,
		Data:  data,
	}
}
