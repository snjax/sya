package query

import (
	"strconv"
	"strings"
	"unicode"
)

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenIdent
	tokenString
	tokenOp
	tokenLParen
	tokenRParen
	tokenComma
)

type token struct {
	typ    tokenType
	text   string
	pos    int
	quoted bool
}

func lex(input string) []token {
	var tokens []token
	for i := 0; i < len(input); {
		r := rune(input[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		switch input[i] {
		case '(':
			tokens = append(tokens, token{typ: tokenLParen, text: "(", pos: i})
			i++
		case ')':
			tokens = append(tokens, token{typ: tokenRParen, text: ")", pos: i})
			i++
		case ',':
			tokens = append(tokens, token{typ: tokenComma, text: ",", pos: i})
			i++
		case '"':
			text, next, ok := scanString(input, i)
			if !ok {
				tokens = append(tokens, token{typ: tokenString, text: input[i+1:], pos: i, quoted: true})
				i = len(input)
				continue
			}
			tokens = append(tokens, token{typ: tokenString, text: text, pos: i, quoted: true})
			i = next
		case '=', '!', '~', '>', '<':
			start := i
			i++
			if i < len(input) && input[i] == '=' && (input[start] == '!' || input[start] == '>' || input[start] == '<') {
				i++
			}
			tokens = append(tokens, token{typ: tokenOp, text: input[start:i], pos: start})
		default:
			start := i
			for i < len(input) && !unicode.IsSpace(rune(input[i])) && !strings.ContainsRune("(),=!~<>", rune(input[i])) {
				i++
			}
			tokens = append(tokens, token{typ: tokenIdent, text: input[start:i], pos: start})
		}
	}
	tokens = append(tokens, token{typ: tokenEOF, pos: len(input)})
	return tokens
}

func scanString(input string, start int) (string, int, bool) {
	for i := start + 1; i < len(input); i++ {
		if input[i] == '\\' {
			i++
			continue
		}
		if input[i] != '"' {
			continue
		}
		unquoted, err := strconv.Unquote(input[start : i+1])
		if err != nil {
			return "", i + 1, false
		}
		return unquoted, i + 1, true
	}
	return "", len(input), false
}
