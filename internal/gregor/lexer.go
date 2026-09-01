// Package gregor parses, checks, and compiles triallang source.
package gregor

import (
	"fmt"
	"strings"
)

type tokenKind int

const (
	tokEOF    tokenKind = iota
	tokWord             // upper-case keyword word: SHOULD, EXCEED, K-1
	tokIdent            // lower-case identifier: counter, actuarial-services
	tokInt              // integer literal
	tokSum              // sum literal: a fixed-point decimal, stated to the penny
	tokString           // string literal (decoded)
	tokPeriod
	tokComma
	tokColon
	tokLParen
	tokRParen
)

func (k tokenKind) String() string {
	switch k {
	case tokEOF:
		return "the end of the filing"
	case tokWord:
		return "a keyword"
	case tokIdent:
		return "an identifier"
	case tokInt:
		return "an integer"
	case tokSum:
		return "a sum"
	case tokString:
		return "a string"
	case tokPeriod:
		return "'.'"
	case tokComma:
		return "','"
	case tokColon:
		return "':'"
	case tokLParen:
		return "'('"
	case tokRParen:
		return "')'"
	}
	return "an unclassifiable mark"
}

type token struct {
	kind tokenKind
	text string
	line int
	col  int
}

// RejectedFiling is a compile error. The public text cites the Article;
// the particulars are available to counsel.
type RejectedFiling struct {
	Line, Col   int
	Particulars string
}

func (e *RejectedFiling) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Particulars)
}

func reject(line, col int, format string, args ...any) error {
	return &RejectedFiling{Line: line, Col: col, Particulars: fmt.Sprintf(format, args...)}
}

func lex(src string) ([]token, error) {
	var toks []token
	line, col := 1, 1
	i := 0
	n := len(src)

	advance := func(k int) {
		for j := 0; j < k && i < n; j++ {
			if src[i] == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			i++
		}
	}

	isUpper := func(c byte) bool { return c >= 'A' && c <= 'Z' }
	isLower := func(c byte) bool { return c >= 'a' && c <= 'z' }
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }

	for i < n {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			advance(1)

		case isUpper(c):
			startLine, startCol := line, col
			j := i
			for j < n && (isUpper(src[j]) || isDigit(src[j]) || src[j] == '-') {
				j++
			}
			word := src[i:j]
			advance(j - i)
			// OFF THE RECORD: the comment. Detected here, at the only
			// place it can begin, and struck to end of line, though
			// nothing is ever truly off the record; the filing topic
			// retains the original.
			if word == "OFF" && commentFollows(src[i:]) {
				for i < n && src[i] != '\n' {
					advance(1)
				}
				continue
			}
			toks = append(toks, token{tokWord, word, startLine, startCol})

		case isLower(c):
			startLine, startCol := line, col
			j := i
			for j < n && (isLower(src[j]) || isDigit(src[j]) || src[j] == '-') {
				j++
			}
			toks = append(toks, token{tokIdent, src[i:j], startLine, startCol})
			advance(j - i)

		case isDigit(c) || (c == '-' && i+1 < n && isDigit(src[i+1])):
			startLine, startCol := line, col
			j := i + 1
			for j < n && isDigit(src[j]) {
				j++
			}
			// A period followed by a digit is not the end of a sentence;
			// it is a decimal point, and what follows must be exactly two
			// figures. The Court keeps sums to the penny and no further.
			if j+1 < n && src[j] == '.' && isDigit(src[j+1]) {
				k := j + 1
				for k < n && isDigit(src[k]) {
					k++
				}
				if k-j-1 != 2 {
					return nil, reject(startLine, startCol, "the sum %q is not stated to the penny; sums carry exactly two figures after the point, by standing order", src[i:k])
				}
				toks = append(toks, token{tokSum, src[i:k], startLine, startCol})
				advance(k - i)
				continue
			}
			toks = append(toks, token{tokInt, src[i:j], startLine, startCol})
			advance(j - i)

		case c == '"':
			startLine, startCol := line, col
			var sb strings.Builder
			j := i + 1
			for {
				if j >= n || src[j] == '\n' {
					return nil, reject(startLine, startCol, "a quotation was opened and never closed; the record cannot abide an open quotation")
				}
				if src[j] == '"' {
					break
				}
				if src[j] == '\\' {
					if j+1 >= n {
						return nil, reject(startLine, startCol, "the filing ends mid-escape")
					}
					switch src[j+1] {
					case '"':
						sb.WriteByte('"')
					case '\\':
						sb.WriteByte('\\')
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					default:
						return nil, reject(startLine, startCol, "the escape '\\%c' is not recognized by this office", src[j+1])
					}
					j += 2
					continue
				}
				sb.WriteByte(src[j])
				j++
			}
			toks = append(toks, token{tokString, sb.String(), startLine, startCol})
			advance(j + 1 - i)

		case c == '.':
			toks = append(toks, token{tokPeriod, ".", line, col})
			advance(1)
		case c == ',':
			toks = append(toks, token{tokComma, ",", line, col})
			advance(1)
		case c == ':':
			toks = append(toks, token{tokColon, ":", line, col})
			advance(1)
		case c == '(':
			toks = append(toks, token{tokLParen, "(", line, col})
			advance(1)
		case c == ')':
			toks = append(toks, token{tokRParen, ")", line, col})
			advance(1)

		default:
			return nil, reject(line, col, "the character %q has no legal standing", string(c))
		}
	}
	toks = append(toks, token{tokEOF, "", line, col})
	return toks, nil
}

// commentFollows reports whether the text after a lexed "OFF" continues
// "THE RECORD" and a colon, with any amount of horizontal whitespace.
func commentFollows(rest string) bool {
	for _, want := range []string{"THE", "RECORD"} {
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, want) {
			return false
		}
		rest = rest[len(want):]
	}
	return strings.HasPrefix(strings.TrimLeft(rest, " \t"), ":")
}
