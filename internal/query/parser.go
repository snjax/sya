package query

import "strings"

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matchKeyword("or") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binaryExpr{op: "or", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.matchKeyword("and") {
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binaryExpr{op: "and", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if p.matchKeyword("not") {
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return notExpr{expr: expr}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	if p.match(tokenLParen) {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(tokenRParen) {
			tok := p.peek()
			return nil, ParseError{Pos: tok.pos, Expected: "')'", Got: tok.text}
		}
		return expr, nil
	}
	keyTok := p.peek()
	if keyTok.typ != tokenIdent {
		return nil, ParseError{Pos: keyTok.pos, Expected: "predicate key", Got: keyTok.text}
	}
	p.pos++
	key := keyTok.text
	if !validKey(key) {
		return nil, ParseError{Pos: keyTok.pos, Expected: "known key", Got: key}
	}
	if isBareBoolKey(key) || strings.HasPrefix(key, "rel.") {
		if !startsOperator(p.peek()) {
			return boolExpr{key: key}, nil
		}
	}
	opTok := p.peek()
	if opTok.typ == tokenIdent && opTok.text == "in" {
		p.pos++
		return p.parseIn(key, opTok.pos)
	}
	if opTok.typ != tokenOp || !validOp(opTok.text) {
		return nil, ParseError{Pos: opTok.pos, Expected: "operator", Got: opTok.text}
	}
	p.pos++
	parsedValue, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return compareExpr{key: key, op: opTok.text, values: []value{parsedValue}}, nil
}

func (p *parser) parseIn(key string, pos int) (Expr, error) {
	if !p.match(tokenLParen) {
		tok := p.peek()
		return nil, ParseError{Pos: tok.pos, Expected: "'(' after in", Got: tok.text}
	}
	var values []value
	if p.peek().typ == tokenRParen {
		return nil, ParseError{Pos: p.peek().pos, Expected: "value", Got: ")"}
	}
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if p.match(tokenRParen) {
			break
		}
		if !p.match(tokenComma) {
			tok := p.peek()
			return nil, ParseError{Pos: tok.pos, Expected: "',' or ')'", Got: tok.text}
		}
	}
	_ = pos
	return compareExpr{key: key, op: "in", values: values}, nil
}

func (p *parser) parseValue() (value, error) {
	tok := p.peek()
	switch tok.typ {
	case tokenIdent, tokenString:
		p.pos++
		return value{raw: tok.text, quoted: tok.quoted}, nil
	default:
		return value{}, ParseError{Pos: tok.pos, Expected: "value", Got: tok.text}
	}
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{typ: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) match(typ tokenType) bool {
	if p.peek().typ != typ {
		return false
	}
	p.pos++
	return true
}

func (p *parser) matchKeyword(keyword string) bool {
	tok := p.peek()
	if tok.typ != tokenIdent || tok.text != keyword {
		return false
	}
	p.pos++
	return true
}

func startsOperator(tok token) bool {
	return tok.typ == tokenOp || tok.typ == tokenIdent && tok.text == "in"
}

func validOp(op string) bool {
	switch op {
	case "=", "!=", "~", ">", ">=", "<", "<=":
		return true
	default:
		return false
	}
}

func validKey(key string) bool {
	switch key {
	case "id", "type", "status", "priority", "title", "assignee", "parent", "label", "age":
		return true
	case "ready", "blocked", "archived", "terminal", "working", "parked", "dead_end":
		return true
	default:
		return strings.HasPrefix(key, "field.") && strings.TrimPrefix(key, "field.") != "" ||
			strings.HasPrefix(key, "rel.") && strings.TrimPrefix(key, "rel.") != ""
	}
}

func isBareBoolKey(key string) bool {
	switch key {
	case "ready", "blocked", "archived", "terminal", "working", "parked", "dead_end":
		return true
	default:
		return false
	}
}
