package counsel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/lennrt/trial-lang/internal/gregor"
)

type symbolRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}
type documentSymbol struct {
	Name           string           `json:"name"`
	Kind           int              `json:"kind"`
	Range          symbolRange      `json:"range"`
	SelectionRange symbolRange      `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

// documentSymbols reads only the current buffer. Invalid syntax already has a
// diagnostic; it must not create a misleading partial outline.
func documentSymbols(ctx context.Context, source string, byteBudget int) ([]documentSymbol, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	symbols := make([]documentSymbol, 0)
	program, err := gregor.Parse(source)
	if err != nil {
		return symbols, nil //nolint:nilerr // Invalid syntax has separate diagnostics and intentionally returns an empty outline.
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Reject results that cannot fit before allocating their nodes or position
	// index. Names only add bytes to the minimum empty-name symbol encoding;
	// positions and nested children can make the actual result larger still.
	if !outlineCanFit(program, byteBudget) {
		return nil, errOutlineResponseLimit
	}
	positions := indexSourcePositions(source)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	end := positions.end(source)
	declaration := func(name string, kind, line int, span gregor.DeclarationSpan) documentSymbol {
		start := positions.position(line, span.Column)
		selectionEnd := positions.position(span.EndLine, span.EndColumn)
		return documentSymbol{Name: name, Kind: kind, Range: symbolRange{Start: start, End: end}, SelectionRange: symbolRange{Start: start, End: selectionEnd}}
	}
	for _, constant := range program.Constants {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		symbols = append(symbols, declaration(constant.Name, 14, constant.Line, constant.Span))
	}
	for _, exhibit := range program.Exhibits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		symbols = append(symbols, declaration(exhibit.Name, 23, exhibit.Line, exhibit.Span))
	}
	for _, article := range program.Articles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		symbols = append(symbols, declaration(fmt.Sprintf("ARTICLE %d", article.Number), 3, article.Line, article.Span))
	}
	for _, office := range program.Offices {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		symbol := declaration(office.Name, 12, office.Line, office.Span)
		for _, section := range office.Sections {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			symbol.Children = append(symbol.Children, declaration(fmt.Sprintf("SECTION %d", section.Number), 3, section.Line, section.Span))
		}
		symbols = append(symbols, symbol)
	}
	finalizeSymbolRanges(symbols)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return symbols, nil
}

var errOutlineResponseLimit = errors.New("document symbols exceed response limit")

// An empty-name symbol with zero-valued positions and kind encodes to 177
// bytes. A flat array is a lower bound even when some symbols are children.
const minimumDocumentSymbolBytes = 177

func outlineCanFit(program *gregor.Program, budget int) bool {
	minimumBytes := 1
	add := func(name string) bool {
		minimumBytes += minimumDocumentSymbolBytes + len(name) + 1
		return minimumBytes <= budget
	}
	for _, constant := range program.Constants {
		if !add(constant.Name) {
			return false
		}
	}
	for _, exhibit := range program.Exhibits {
		if !add(exhibit.Name) {
			return false
		}
	}
	for _, article := range program.Articles {
		if !add(fmt.Sprintf("ARTICLE %d", article.Number)) {
			return false
		}
	}
	for _, office := range program.Offices {
		if !add(office.Name) {
			return false
		}
		for _, section := range office.Sections {
			if !add(fmt.Sprintf("SECTION %d", section.Number)) {
				return false
			}
		}
	}
	return true
}

func (s *Server) respondDocumentSymbols(req *rpcRequest, source string) error {
	if len(req.ID) == 0 {
		return nil
	}
	// Reserve the exact encoded ID and JSON-RPC wrapper, including any escaping.
	empty, err := marshalResponse(rpcResponse{ID: req.ID, Result: json.RawMessage("[]")})
	if err != nil {
		return err
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	symbols, err := documentSymbols(ctx, source, maxMessageBytes-len(empty)+2)
	if err == nil {
		err = s.respond(req, rpcResponse{Result: symbols})
	}
	if _, oversized := errors.AsType[*responseLimitError](err); errors.Is(err, errOutlineResponseLimit) || oversized {
		return s.respond(req, rpcResponse{Error: &rpcError{Code: -32603, Message: errOutlineResponseLimit.Error()}})
	}
	return err
}

type sourcePositionIndex struct {
	lines       []sourceAdjustment
	adjustments []sourceAdjustment
}

type sourceAdjustment struct {
	offset     int
	adjustment int
}

// Scan source once. Only non-ASCII rune boundaries need correction checkpoints;
// ASCII columns are direct lookups, and Unicode columns need one binary search.
func indexSourcePositions(source string) sourcePositionIndex {
	index := sourcePositionIndex{lines: []sourceAdjustment{{}}}
	adjustment := 0
	for offset := 0; offset < len(source); {
		r, width := utf8.DecodeRuneInString(source[offset:])
		offset += width
		if r == '\n' {
			index.lines = append(index.lines, sourceAdjustment{offset: offset, adjustment: adjustment})
		}
		if difference := width - utf16.RuneLen(r); difference != 0 {
			adjustment += difference
			index.adjustments = append(index.adjustments, sourceAdjustment{offset: offset, adjustment: adjustment})
		}
	}
	return index
}

func (index sourcePositionIndex) position(line, column int) position {
	start := index.lines[line-1]
	column--
	offset := start.offset + column
	adjustment := 0
	i := sort.Search(len(index.adjustments), func(i int) bool { return index.adjustments[i].offset > offset })
	if i > 0 {
		adjustment = index.adjustments[i-1].adjustment
	}
	return position{Line: line - 1, Character: column - adjustment + start.adjustment}
}

func (index sourcePositionIndex) end(source string) position {
	line := len(index.lines)
	column := len(source) - index.lines[line-1].offset
	if strings.HasSuffix(source, "\r") {
		column--
	}
	return index.position(line, column+1)
}

// finalizeSymbolRanges orders declarations and bounds siblings and nested sections.
func finalizeSymbolRanges(symbols []documentSymbol) {
	sort.SliceStable(symbols, func(i, j int) bool { return positionBefore(symbols[i].Range.Start, symbols[j].Range.Start) })
	for i := range symbols {
		if i+1 < len(symbols) {
			symbols[i].Range.End = symbols[i+1].Range.Start
		}
		for j := range symbols[i].Children {
			child := &symbols[i].Children[j]
			child.Range.End = symbols[i].Range.End
			if j+1 < len(symbols[i].Children) {
				child.Range.End = symbols[i].Children[j+1].Range.Start
			}
		}
	}
}

func positionBefore(a, b position) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Character < b.Character
}
