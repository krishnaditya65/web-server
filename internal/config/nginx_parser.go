package config

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

// nginxNode represents a single directive or block in an nginx config.
type nginxNode struct {
	Directive string
	Args      []string
	Block     []*nginxNode // non-nil when this node is a block
}

// ParseNginxFile reads and parses an nginx config file into a flat list of nodes.
func ParseNginxFile(path string) ([]*nginxNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("nginx parser: read %q: %w", path, err)
	}

	l := &lexer{src: string(data)}
	return parseBlock(l)
}

// ---- lexer ----------------------------------------------------------------

type tokenKind int

const (
	tokWord      tokenKind = iota // unquoted or quoted string
	tokSemicolon                  // ;
	tokLBrace                     // {
	tokRBrace                     // }
	tokEOF
)

type token struct {
	kind tokenKind
	val  string
}

type lexer struct {
	src string
	pos int
}

func (l *lexer) next() token {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]

		// Skip whitespace.
		if unicode.IsSpace(rune(ch)) {
			l.pos++
			continue
		}

		// Skip line comments.
		if ch == '#' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}

		switch ch {
		case ';':
			l.pos++
			return token{tokSemicolon, ";"}
		case '{':
			l.pos++
			return token{tokLBrace, "{"}
		case '}':
			l.pos++
			return token{tokRBrace, "}"}
		case '"', '\'':
			return l.readQuoted(ch)
		default:
			return l.readWord()
		}
	}

	return token{tokEOF, ""}
}

func (l *lexer) peek() token {
	saved := l.pos
	t := l.next()
	l.pos = saved
	return t
}

func (l *lexer) readQuoted(quote byte) token {
	l.pos++ // skip opening quote
	var sb strings.Builder

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		l.pos++

		if ch == quote {
			break
		}

		if ch == '\\' && l.pos < len(l.src) {
			sb.WriteByte(l.src[l.pos])
			l.pos++
			continue
		}

		sb.WriteByte(ch)
	}

	return token{tokWord, sb.String()}
}

func (l *lexer) readWord() token {
	start := l.pos

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if unicode.IsSpace(rune(ch)) || ch == ';' || ch == '{' || ch == '}' || ch == '#' {
			break
		}
		l.pos++
	}

	return token{tokWord, l.src[start:l.pos]}
}

// ---- parser ---------------------------------------------------------------

func parseBlock(l *lexer) ([]*nginxNode, error) {
	var nodes []*nginxNode

	for {
		t := l.peek()

		if t.kind == tokEOF || t.kind == tokRBrace {
			return nodes, nil
		}

		node, err := parseNode(l)
		if err != nil {
			return nil, err
		}

		if node != nil {
			nodes = append(nodes, node)
		}
	}
}

func parseNode(l *lexer) (*nginxNode, error) {
	t := l.next()

	if t.kind == tokEOF || t.kind == tokRBrace {
		return nil, nil
	}

	if t.kind != tokWord {
		return nil, fmt.Errorf("nginx parser: expected directive name, got %q", t.val)
	}

	node := &nginxNode{Directive: t.val}

	// Collect arguments until ; or {
	for {
		peek := l.peek()

		if peek.kind == tokSemicolon {
			l.next() // consume ;
			return node, nil
		}

		if peek.kind == tokLBrace {
			l.next() // consume {
			children, err := parseBlock(l)
			if err != nil {
				return nil, err
			}

			// Consume closing }
			closing := l.next()
			if closing.kind != tokRBrace {
				return nil, fmt.Errorf("nginx parser: expected '}', got %q", closing.val)
			}

			node.Block = children
			return node, nil
		}

		if peek.kind == tokEOF {
			return node, nil
		}

		arg := l.next()
		node.Args = append(node.Args, arg.val)
	}
}
