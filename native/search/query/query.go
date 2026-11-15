// Package query parses the stable, backend-neutral search query language.
package query

import (
	"fmt"
	"strings"
)

type Filter struct {
	Field, Operator, Value string
	Negated                bool
}

type AST struct {
	Text    string
	Filters []Filter
}

// Parse accepts free-text terms and typed filters such as tag:work,
// type=reference, created>=2026-01-01, and quoted values containing spaces.
// Unknown fields are retained in the AST and rejected during plan compilation,
// allowing field contributions to be registered independently of the parser.
func Parse(input string) (AST, error) {
	tokens, err := lex(input)
	if err != nil {
		return AST{}, err
	}
	var text []string
	var filters []Filter
	for _, token := range tokens {
		negated := strings.HasPrefix(token, "-") && len(token) > 1
		if negated {
			token = token[1:]
		}
		field, op, value, ok := splitFilter(token)
		if !ok {
			if negated {
				return AST{}, fmt.Errorf("search query: negation requires a field filter: %q", token)
			}
			text = append(text, token)
			continue
		}
		if field == "" || value == "" {
			return AST{}, fmt.Errorf("search query: filter requires field and value: %q", token)
		}
		filters = append(filters, Filter{Field: strings.ToLower(field), Operator: op, Value: value, Negated: negated})
	}
	return AST{Text: strings.Join(text, " "), Filters: filters}, nil
}

func splitFilter(token string) (field, op, value string, ok bool) {
	for _, candidate := range []string{">=", "<=", "!=", ">", "<", "=", ":"} {
		if at := strings.Index(token, candidate); at > 0 {
			op = candidate
			if op == ":" || op == "=" {
				op = "eq"
			} else if op == "!=" {
				op = "ne"
			}
			return token[:at], op, token[at+len(candidate):], true
		}
	}
	return "", "", "", false
}

func lex(input string) ([]string, error) {
	var tokens []string
	var token strings.Builder
	var quote rune
	escaped := false
	started := false
	for _, r := range input {
		if escaped {
			token.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				token.WriteRune(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if started {
				tokens = append(tokens, token.String())
				token.Reset()
				started = false
			}
			continue
		}
		token.WriteRune(r)
		started = true
	}
	if escaped {
		token.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("search query: unterminated quoted value")
	}
	if started {
		tokens = append(tokens, token.String())
	}
	return tokens, nil
}
