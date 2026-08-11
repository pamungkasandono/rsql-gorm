package rsql

import (
	"fmt"
	"strings"
	"unicode"
)

// DoS input bounds enforced by Parse to protect the server from resource
// exhaustion on hostile requests.
const (
	// maxParenDepth caps nested (...) grouping depth.
	maxParenDepth = 100
	// maxFilterLength caps the filter string length in bytes.
	maxFilterLength = 8192
	// maxListValues caps the number of values in =in=/=out= lists.
	maxListValues = 2000
)

type parser struct {
	input []rune
	pos   int
	depth int
}

// Parse parses an RSQL filter string into a Node AST. An empty or
// whitespace-only string returns (nil, nil). Parse rejects input longer than
// maxFilterLength bytes, grouping nested deeper than maxParenDepth, and
// =in=/=out= lists larger than maxListValues.
func Parse(input string) (Node, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > maxFilterLength {
		return nil, fmt.Errorf("filter exceeds maximum length of %d bytes", maxFilterLength)
	}
	p := &parser{input: []rune(trimmed), pos: 0}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipWhitespace()
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected character %q at position %d", string(p.input[p.pos]), p.pos)
	}
	return node, nil
}

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWhitespace()
		if !p.match(',') {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}

		if or, ok := left.(*OrNode); ok {
			or.Children = append(or.Children, right)
		} else {
			left = &OrNode{Children: []Node{left, right}}
		}
	}
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWhitespace()
		if !p.match(';') {
			return left, nil
		}
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}

		if and, ok := left.(*AndNode); ok {
			and.Children = append(and.Children, right)
		} else {
			left = &AndNode{Children: []Node{left, right}}
		}
	}
}

func (p *parser) parsePrimary() (Node, error) {
	p.skipWhitespace()
	if p.match('(') {
		p.depth++
		if p.depth > maxParenDepth {
			return nil, fmt.Errorf("grouping depth %d exceeds maximum %d", p.depth, maxParenDepth)
		}
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if !p.match(')') {
			return nil, fmt.Errorf("expected closing ')' at position %d", p.pos)
		}
		p.depth--
		return node, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (Node, error) {
	selector, err := p.parseSelector()
	if err != nil {
		return nil, err
	}

	op, err := p.parseOperator()
	if err != nil {
		return nil, err
	}

	args, err := p.parseArguments(op)
	if err != nil {
		return nil, err
	}

	return &ComparisonNode{
		Selector:  selector,
		Operator:  op,
		Arguments: args,
	}, nil
}

func (p *parser) parseSelector() (string, error) {
	p.skipWhitespace()
	ident, err := p.readIdentifier()
	if err != nil {
		return "", fmt.Errorf("expected selector at position %d: %w", p.pos, err)
	}

	for p.match('.') {
		next, err := p.readIdentifier()
		if err != nil {
			return "", fmt.Errorf("expected identifier after '.' at position %d: %w", p.pos, err)
		}
		ident += "." + next
	}

	return ident, nil
}

func (p *parser) parseOperator() (string, error) {
	p.skipWhitespace()

	if p.pos >= len(p.input) {
		return "", fmt.Errorf("expected operator at position %d", p.pos)
	}

	remaining := string(p.input[p.pos:])

	for _, candidate := range operatorCandidates {
		if strings.HasPrefix(remaining, candidate) {
			p.pos += len([]rune(candidate))
			return candidate, nil
		}
	}

	return "", fmt.Errorf("unknown operator at position %d: %q", p.pos, string(p.input[p.pos]))
}

var operatorCandidates = []string{
	"=out=",
	"=in=",
	">=",
	"<=",
	"!=",
	"==",
	">",
	"<",
}

func (p *parser) parseArguments(op string) (any, error) {
	if op == "=in=" || op == "=out=" {
		return p.parseListArguments()
	}
	return p.parseSingleArgument()
}

func (p *parser) parseListArguments() (any, error) {
	p.skipWhitespace()
	if !p.match('(') {
		return nil, fmt.Errorf("expected '(' after %s operator at position %d", "=in=/=out=", p.pos)
	}

	var values []string
	for {
		p.skipWhitespace()
		if p.match(')') {
			break
		}
		if len(values) > 0 {
			p.skipWhitespace()
		}

		val := p.readArgumentValue()
		if val == "" && p.peek() != ')' {
			return nil, fmt.Errorf("expected argument value at position %d", p.pos)
		}
		if len(values) == maxListValues {
			return nil, fmt.Errorf("argument list exceeds maximum of %d values", maxListValues)
		}
		values = append(values, val)

		p.skipWhitespace()
		if p.peek() == ')' {
			p.match(')')
			break
		}
		if !p.match(',') {
			return nil, fmt.Errorf("expected ',' or ')' in argument list at position %d", p.pos)
		}
	}

	return values, nil
}

func (p *parser) parseSingleArgument() (any, error) {
	return p.readArgumentValue(), nil
}

func (p *parser) readArgumentValue() string {
	p.skipWhitespace()
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ';' || ch == ',' || ch == '(' || ch == ')' {
			break
		}
		p.pos++
	}
	val := string(inputSlice(p.input, start, p.pos))
	return strings.TrimSpace(val)
}

func (p *parser) readIdentifier() (string, error) {
	p.skipWhitespace()
	start := p.pos
	if p.pos >= len(p.input) {
		return "", fmt.Errorf("unexpected end of input")
	}
	ch := p.input[p.pos]
	if !isIdentStart(ch) {
		return "", fmt.Errorf("invalid identifier start character %q at position %d", ch, p.pos)
	}
	p.pos++
	for p.pos < len(p.input) && isIdentPart(p.input[p.pos]) {
		p.pos++
	}
	return string(p.input[start:p.pos]), nil
}

func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == '*'
}

func isIdentPart(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-' || ch == '*' || ch == '.'
}

func (p *parser) match(expected rune) bool {
	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == expected {
		p.pos++
		p.skipWhitespace()
		return true
	}
	return false
}

func (p *parser) peek() rune {
	pp := p.pos
	for pp < len(p.input) && (p.input[pp] == ' ' || p.input[pp] == '\t') {
		pp++
	}
	if pp < len(p.input) {
		return p.input[pp]
	}
	return 0
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func inputSlice(input []rune, start, end int) []rune {
	return input[start:end]
}
