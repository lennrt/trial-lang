package gregor

import (
	"strconv"

	"github.com/lennrt/trial-lang/internal/law"
)

type parser struct {
	toks []token
	pos  int
}

// Parse reads a complete case file. Errors are *RejectedFiling.
func Parse(src string) (*Program, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	return p.parseCaseFile()
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }
func (p *parser) atWord(w string) bool {
	t := p.peek()
	return t.kind == tokWord && t.text == w
}

// peekAt returns the token n positions ahead without consuming it.
func (p *parser) peekAt(n int) token {
	if p.pos+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.pos+n]
}

func (p *parser) wordAt(n int) string {
	t := p.peekAt(n)
	if t.kind == tokWord {
		return t.text
	}
	return ""
}

func (p *parser) expectKind(k tokenKind, context string) (token, error) {
	t := p.next()
	if t.kind != k {
		return t, reject(t.line, t.col, "expected %s %s, encountered %s %q", k, context, t.kind, t.text)
	}
	return t, nil
}

// expectWords consumes an exact sequence of upper-case keyword words.
func (p *parser) expectWords(words ...string) error {
	for _, w := range words {
		t := p.next()
		if t.kind != tokWord || t.text != w {
			return reject(t.line, t.col, "expected the word %q, encountered %s %q", w, t.kind, t.text)
		}
	}
	return nil
}

func (p *parser) expectPeriod() error {
	t := p.next()
	if t.kind != tokPeriod {
		return reject(t.line, t.col, "expected a period, encountered %s %q", t.kind, t.text)
	}
	return nil
}

func parseInteger(t token) (int64, error) {
	n, err := strconv.ParseInt(t.text, 10, 64)
	if err != nil {
		return 0, reject(t.line, t.col, "integer %q is outside the 64-bit range", t.text)
	}
	return n, nil
}

func (p *parser) parseCaseFile() (*Program, error) {
	prog := &Program{}

	// FORM K-1. (a case), FORM K-2. (a supplemental filing), or
	// FORM S-1. (a statute, offered for enactment).
	if err := p.expectWords("FORM"); err != nil {
		return nil, err
	}
	formTok := p.next()
	switch {
	case formTok.kind == tokWord && formTok.text == "K-1":
		prog.Form = "K-1"
	case formTok.kind == tokWord && formTok.text == "K-2":
		prog.Form = "K-2"
	case formTok.kind == tokWord && formTok.text == "S-1":
		prog.Form = "S-1"
	default:
		return nil, reject(formTok.line, formTok.col, "expected Form K-1, K-2, or S-1; encountered %q", formTok.text)
	}
	if err := p.expectPeriod(); err != nil {
		return nil, err
	}

	// IN THE MATTER OF: name.
	if err := p.expectWords("IN", "THE", "MATTER", "OF"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(tokColon, "after the caption"); err != nil {
		return nil, err
	}
	nameTok, err := p.expectKind(tokIdent, "naming the matter")
	if err != nil {
		return nil, err
	}
	prog.Name = nameTok.text
	if err := p.expectPeriod(); err != nil {
		return nil, err
	}

	// Optional filing clauses: FILED BY: free text.
	for p.atWord("FILED") {
		p.next()
		if err := p.expectWords("BY"); err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokColon, "after FILED BY"); err != nil {
			return nil, err
		}
		for p.peek().kind != tokPeriod && p.peek().kind != tokEOF {
			p.next() // recorded verbatim in the filing topic; never read
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
	}

	// INCORPORATE BY REFERENCE statute-name. Filing resolves and compiles the
	// enacted source with the case.
	for p.atWord("INCORPORATE") {
		incTok := p.next()
		if err := p.expectWords("BY", "REFERENCE"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the statute incorporated")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		// Statutes may incorporate other statutes; supplemental filings may not.
		if prog.Form == "K-2" {
			return nil, reject(incTok.line, incTok.col, "a supplemental filing may not incorporate a statute")
		}
		prog.Incorporations = append(prog.Incorporations, Incorporation{Name: nameTok.text, Line: incTok.line})
	}

	// Exhibits and defined terms may appear before the articles.
	for {
		if p.atWord("THE") && p.wordAt(1) == "EXHIBIT" {
			ex, err := p.parseExhibitDecl()
			if err != nil {
				return nil, err
			}
			prog.Exhibits = append(prog.Exhibits, ex)
			continue
		}
		if p.atWord("HEREINAFTER") {
			cd, err := p.parseConstDecl()
			if err != nil {
				return nil, err
			}
			prog.Constants = append(prog.Constants, cd)
			continue
		}
		break
	}

	// Statutes have no articles. Cases require at least one.
	if prog.Form == "S-1" {
		if p.atWord("ARTICLE") {
			t := p.peek()
			return nil, reject(t.line, t.col, "a statute may not contain an ARTICLE")
		}
	} else {
		if !p.atWord("ARTICLE") {
			t := p.peek()
			return nil, reject(t.line, t.col, "a case must contain at least one ARTICLE")
		}
		for p.atWord("ARTICLE") {
			art, err := p.parseArticle()
			if err != nil {
				return nil, err
			}
			prog.Articles = append(prog.Articles, art)
		}
	}

	// Offices, further exhibits, and further defined terms, in whatever
	// order they arrive.
	for p.atWord("THE") || p.atWord("HEREINAFTER") {
		if p.atWord("HEREINAFTER") {
			cd, err := p.parseConstDecl()
			if err != nil {
				return nil, err
			}
			prog.Constants = append(prog.Constants, cd)
			continue
		}
		if p.wordAt(1) == "EXHIBIT" {
			ex, err := p.parseExhibitDecl()
			if err != nil {
				return nil, err
			}
			prog.Exhibits = append(prog.Exhibits, ex)
			continue
		}
		if prog.Form == "K-2" {
			t := p.peek()
			return nil, reject(t.line, t.col, "a supplemental filing may not establish an office")
		}
		off, err := p.parseOffice()
		if err != nil {
			return nil, err
		}
		prog.Offices = append(prog.Offices, off)
	}

	if t := p.peek(); t.kind != tokEOF {
		return nil, reject(t.line, t.col, "the filing continues past its own conclusion, beginning with %s %q", t.kind, t.text)
	}
	if prog.Form == "S-1" && len(prog.Offices) == 0 {
		t := p.peek()
		return nil, reject(t.line, t.col, "a statute must establish at least one office")
	}
	return prog, nil
}

func (p *parser) parseNumberedHeading(word string) (int64, int, error) {
	line := p.peek().line
	if err := p.expectWords(word); err != nil {
		return 0, 0, err
	}
	numTok, err := p.expectKind(tokInt, "numbering the "+word)
	if err != nil {
		return 0, 0, err
	}
	n, err := parseInteger(numTok)
	if err != nil {
		return 0, 0, err
	}
	if err := p.expectPeriod(); err != nil {
		return 0, 0, err
	}
	return n, line, nil
}

func (p *parser) parseArticle() (Article, error) {
	n, line, err := p.parseNumberedHeading("ARTICLE")
	if err != nil {
		return Article{}, err
	}
	art := Article{Number: n, Line: line}
	for p.startsStatement() {
		s, err := p.parseStatement()
		if err != nil {
			return Article{}, err
		}
		art.Stmts = append(art.Stmts, s)
	}
	return art, nil
}

// parseExhibitDecl reads an exhibit declaration, which establishes a shape and
// emits no instructions.
func (p *parser) parseExhibitDecl() (ExhibitDecl, error) {
	line := p.peek().line
	if err := p.expectWords("THE", "EXHIBIT", "OF"); err != nil {
		return ExhibitDecl{}, err
	}
	nameTok, err := p.expectKind(tokIdent, "naming the exhibit")
	if err != nil {
		return ExhibitDecl{}, err
	}
	ex := ExhibitDecl{Name: nameTok.text, Line: line}
	if _, err := p.expectKind(tokComma, "before COMPRISING"); err != nil {
		return ExhibitDecl{}, err
	}
	if err := p.expectWords("COMPRISING"); err != nil {
		return ExhibitDecl{}, err
	}
	for {
		f, err := p.expectKind(tokIdent, "as an entry of the exhibit")
		if err != nil {
			return ExhibitDecl{}, err
		}
		ex.Fields = append(ex.Fields, f.text)
		if p.atWord("AND") {
			p.next()
			continue
		}
		break
	}
	if err := p.expectPeriod(); err != nil {
		return ExhibitDecl{}, err
	}
	return ex, nil
}

// parseConstDecl reads HEREINAFTER, name SHALL MEAN literal. The compiler
// substitutes the literal wherever the name appears.
func (p *parser) parseConstDecl() (ConstDecl, error) {
	line := p.peek().line
	if err := p.expectWords("HEREINAFTER"); err != nil {
		return ConstDecl{}, err
	}
	if _, err := p.expectKind(tokComma, "after HEREINAFTER"); err != nil {
		return ConstDecl{}, err
	}
	nameTok, err := p.expectKind(tokIdent, "as the term defined")
	if err != nil {
		return ConstDecl{}, err
	}
	if err := p.expectWords("SHALL", "MEAN"); err != nil {
		return ConstDecl{}, err
	}
	t := p.peek()
	var e Expr
	switch {
	case t.kind == tokInt:
		p.next()
		n, err := parseInteger(t)
		if err != nil {
			return ConstDecl{}, err
		}
		e = IntLit{Val: n}
	case t.kind == tokSum:
		p.next()
		m, ok := law.ParseSum(t.text)
		if !ok {
			return ConstDecl{}, reject(t.line, t.col, "sum %q is outside the 64-bit range", t.text)
		}
		e = SumLit{Mantissa: m}
	case t.kind == tokString:
		p.next()
		e = StrLit{Val: t.text}
	case t.kind == tokWord && t.text == "SUSTAINED":
		p.next()
		e = FindingLit{Val: true}
	case t.kind == tokWord && t.text == "OVERRULED":
		p.next()
		e = FindingLit{Val: false}
	default:
		return ConstDecl{}, reject(t.line, t.col, "a defined term must be an integer, sum, string, SUSTAINED, or OVERRULED; encountered %s %q", t.kind, t.text)
	}
	if err := p.expectPeriod(); err != nil {
		return ConstDecl{}, err
	}
	return ConstDecl{Name: nameTok.text, Expr: e, Line: line}, nil
}

func (p *parser) parseOffice() (Office, error) {
	line := p.peek().line
	if err := p.expectWords("THE", "OFFICE", "OF"); err != nil {
		return Office{}, err
	}
	nameTok, err := p.expectKind(tokIdent, "naming the office")
	if err != nil {
		return Office{}, err
	}
	off := Office{Name: nameTok.text, Line: line}
	if p.peek().kind == tokComma {
		p.next()
		if err := p.expectWords("CONCERNING"); err != nil {
			return Office{}, err
		}
		for {
			param, err := p.expectKind(tokIdent, "as a matter of concern")
			if err != nil {
				return Office{}, err
			}
			off.Params = append(off.Params, param.text)
			if p.atWord("AND") {
				p.next()
				continue
			}
			break
		}
	}
	if err := p.expectPeriod(); err != nil {
		return Office{}, err
	}
	for p.startsStatement() {
		s, err := p.parseStatement()
		if err != nil {
			return Office{}, err
		}
		off.Stmts = append(off.Stmts, s)
	}
	for p.atWord("SECTION") {
		n, sline, err := p.parseNumberedHeading("SECTION")
		if err != nil {
			return Office{}, err
		}
		sec := Section{Number: n, Line: sline}
		for p.startsStatement() {
			s, err := p.parseStatement()
			if err != nil {
				return Office{}, err
			}
			sec.Stmts = append(sec.Stmts, s)
		}
		off.Sections = append(off.Sections, sec)
	}
	return off, nil
}

func (p *parser) startsStatement() bool {
	t := p.peek()
	if t.kind != tokWord {
		return false
	}
	switch t.text {
	case "LET", "PROCLAIM", "AWAIT", "REFER", "SHOULD", "PETITION", "REMAND", "ADJOURN", "HOLD", "STRIKE", "SERVE", "ANNEX", "SUBSTITUTE", "COMMIT", "COMMENCE", "FILE", "PUBLISH", "GRANT", "ASSIGN", "INSCRIBE", "EXPUNGE", "ENTER":
		return true
	}
	return false
}

func (p *parser) parseStatement() (Stmt, error) {
	t := p.peek()
	switch t.text {
	case "LET":
		p.next()
		// LET LETTERS PATENT ISSUE FOR name, DISCLOSING expr, FOR A TERM
		// OF expr DAYS. The disclosure is mandatory and public.
		if p.atWord("LETTERS") {
			if err := p.expectWords("LETTERS", "PATENT", "ISSUE", "FOR"); err != nil {
				return nil, err
			}
			nameTok, err := p.expectKind(tokIdent, "naming the invention")
			if err != nil {
				return nil, err
			}
			if _, err := p.expectKind(tokComma, "before DISCLOSING"); err != nil {
				return nil, err
			}
			if err := p.expectWords("DISCLOSING"); err != nil {
				return nil, err
			}
			disclosure, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if _, err := p.expectKind(tokComma, "before the term"); err != nil {
				return nil, err
			}
			if err := p.expectWords("FOR", "A", "TERM", "OF"); err != nil {
				return nil, err
			}
			term, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			unit := p.next()
			if unit.kind != tokWord || (unit.text != "DAYS" && unit.text != "DAY") {
				return nil, reject(unit.line, unit.col, "a patent term must use DAY or DAYS; encountered %s %q", unit.kind, unit.text)
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			return PatentGrant{Name: nameTok.text, Disclosure: disclosure, Term: term, Line: t.line}, nil
		}
		if err := p.expectWords("IT", "BE"); err != nil {
			return nil, err
		}
		switch {
		case p.atWord("RECORDED"):
			p.next()
			if err := p.expectWords("THAT"); err != nil {
				return nil, err
			}
			nameTok, err := p.expectKind(tokIdent, "to be recorded")
			if err != nil {
				return nil, err
			}
			if err := p.expectWords("IS"); err != nil {
				return nil, err
			}
			e, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			return Recording{Name: nameTok.text, Expr: e, Line: t.line}, nil

		case p.atWord("ENTERED"):
			// LET IT BE ENTERED IN k THAT field IS expr.
			p.next()
			if err := p.expectWords("IN"); err != nil {
				return nil, err
			}
			nameTok, err := p.expectKind(tokIdent, "naming the exhibit amended")
			if err != nil {
				return nil, err
			}
			if err := p.expectWords("THAT"); err != nil {
				return nil, err
			}
			fieldTok, err := p.expectKind(tokIdent, "naming the entry amended")
			if err != nil {
				return nil, err
			}
			if err := p.expectWords("IS"); err != nil {
				return nil, err
			}
			e, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			return Entering{Name: nameTok.text, Field: fieldTok.text, Expr: e, Line: t.line}, nil
		}
		bad := p.peek()
		return nil, reject(bad.line, bad.col, "expected RECORDED or ENTERED; encountered %q", bad.text)

	case "PROCLAIM":
		p.next()
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Proclaim{Expr: e, Line: t.line}, nil

	case "AWAIT":
		p.next()
		// AWAIT THE GAZETTE, FILED UNDER x. Each case has its own cursor.
		if p.atWord("THE") {
			if err := p.expectWords("THE", "GAZETTE"); err != nil {
				return nil, err
			}
			if _, err := p.expectKind(tokComma, "after AWAIT THE GAZETTE"); err != nil {
				return nil, err
			}
			if err := p.expectWords("FILED", "UNDER"); err != nil {
				return nil, err
			}
			nameTok, err := p.expectKind(tokIdent, "to file the edition under")
			if err != nil {
				return nil, err
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			return GazetteAwait{Name: nameTok.text, Line: t.line}, nil
		}
		if err := p.expectWords("SUMMONS"); err != nil {
			return nil, err
		}
		// AWAIT SUMMONS FROM c selects a sender without consuming records from
		// other senders.
		var from Expr
		if p.atWord("FROM") {
			p.next()
			f, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			from = f
		}
		// A timed summons requires a FAILING WHICH arm.
		if p.atWord("FOR") {
			p.next()
			if err := p.expectWords("AT", "MOST"); err != nil {
				return nil, err
			}
			days, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			unit := p.next()
			if unit.kind != tokWord || (unit.text != "DAYS" && unit.text != "DAY") {
				return nil, reject(unit.line, unit.col, "a summons timeout must use DAY or DAYS; encountered %s %q", unit.kind, unit.text)
			}
			if _, err := p.expectKind(tokComma, "before FILED UNDER"); err != nil {
				return nil, err
			}
			if err := p.expectWords("FILED", "UNDER"); err != nil {
				return nil, err
			}
			nameTok, err := p.expectKind(tokIdent, "to file the response under")
			if err != nil {
				return nil, err
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			if !p.atWord("FAILING") {
				bad := p.peek()
				return nil, reject(bad.line, bad.col, "a timed summons must include FAILING WHICH")
			}
			p.next()
			if err := p.expectWords("WHICH"); err != nil {
				return nil, err
			}
			if _, err := p.expectKind(tokComma, "after FAILING WHICH"); err != nil {
				return nil, err
			}
			if !p.startsStatement() {
				bad := p.peek()
				return nil, reject(bad.line, bad.col, "FAILING WHICH must be followed by a statement; encountered %s %q", bad.kind, bad.text)
			}
			els, err := p.parseStatement()
			if err != nil {
				return nil, err
			}
			return TimedSummons{From: from, Days: days, Name: nameTok.text, Else: els, Line: t.line}, nil
		}
		if _, err := p.expectKind(tokComma, "after AWAIT SUMMONS"); err != nil {
			return nil, err
		}
		if err := p.expectWords("FILED", "UNDER"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "to file the response under")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Summons{From: from, Name: nameTok.text, Line: t.line}, nil

	case "REFER":
		p.next()
		if err := p.expectWords("TO"); err != nil {
			return nil, err
		}
		var toSection bool
		switch {
		case p.atWord("ARTICLE"):
			p.next()
		case p.atWord("SECTION"):
			p.next()
			toSection = true
		default:
			bad := p.peek()
			return nil, reject(bad.line, bad.col, "expected ARTICLE or SECTION; encountered %q", bad.text)
		}
		numTok, err := p.expectKind(tokInt, "identifying the referral")
		if err != nil {
			return nil, err
		}
		n, err := parseInteger(numTok)
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Referral{ToSection: toSection, Number: n, Line: t.line}, nil

	case "SHOULD":
		p.next()
		cond, err := p.parseCondition()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokComma, "between a condition and its consequence"); err != nil {
			return nil, err
		}
		if !p.startsStatement() {
			bad := p.peek()
			return nil, reject(bad.line, bad.col, "SHOULD must be followed by a statement; encountered %s %q", bad.kind, bad.text)
		}
		then, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		cnd := Conditional{Cond: cond, Then: then, Line: t.line}
		// FAILING WHICH attaches to the nearest SHOULD.
		if p.atWord("FAILING") {
			p.next()
			if err := p.expectWords("WHICH"); err != nil {
				return nil, err
			}
			if _, err := p.expectKind(tokComma, "after FAILING WHICH"); err != nil {
				return nil, err
			}
			if !p.startsStatement() {
				bad := p.peek()
				return nil, reject(bad.line, bad.col, "FAILING WHICH must be followed by a statement; encountered %s %q", bad.kind, bad.text)
			}
			els, err := p.parseStatement()
			if err != nil {
				return nil, err
			}
			cnd.Else = els
		}
		return cnd, nil

	case "PETITION":
		p.next()
		// PETITION UNDER uses the office carried by a power of attorney.
		if p.atWord("UNDER") {
			p.next()
			power, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			var args []Expr
			if p.atWord("WITH") {
				p.next()
				args, err = p.parseArgumentList()
				if err != nil {
					return nil, err
				}
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			return PetitionUnder{Power: power, Args: args, Line: t.line}, nil
		}
		if err := p.expectWords("THE", "OFFICE", "OF"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the office petitioned")
		if err != nil {
			return nil, err
		}
		var args []Expr
		if p.atWord("WITH") {
			p.next()
			args, err = p.parseArgumentList()
			if err != nil {
				return nil, err
			}
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Petition{Office: nameTok.text, Args: args, Line: t.line}, nil

	case "REMAND":
		p.next()
		var val Expr
		if p.atWord("WITH") {
			p.next()
			e, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			val = e
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Remand{Value: val, Line: t.line}, nil

	case "ADJOURN":
		p.next()
		// ADJOURN FOR sets a continuance; ADJOURN INDEFINITELY suspends.
		if p.atWord("FOR") {
			p.next()
			e, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			unit := p.next()
			if unit.kind != tokWord || (unit.text != "DAYS" && unit.text != "DAY") {
				return nil, reject(unit.line, unit.col, "a continuance must use DAY or DAYS; encountered %s %q", unit.kind, unit.text)
			}
			if err := p.expectPeriod(); err != nil {
				return nil, err
			}
			return AdjournFor{Days: e, Line: t.line}, nil
		}
		if err := p.expectWords("INDEFINITELY"); err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Adjourn{Line: t.line}, nil

	case "SERVE":
		// SERVE NOTICE OF expr UPON expr. Cross-case correspondence: the
		// notice is appended to the respondent's summons topic, in the
		// same transaction as everything else this instruction does.
		p.next()
		if err := p.expectWords("NOTICE", "OF"); err != nil {
			return nil, err
		}
		val, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("UPON"); err != nil {
			return nil, err
		}
		target, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Serve{Value: val, Target: target, Line: t.line}, nil

	case "ENTER":
		// ENTER JUDGMENT records a verdict in a case commenced by this case.
		p.next()
		if err := p.expectWords("JUDGMENT", "AGAINST"); err != nil {
			return nil, err
		}
		target, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokComma, "before the grounds"); err != nil {
			return nil, err
		}
		if err := p.expectWords("ON", "THE", "GROUNDS", "OF"); err != nil {
			return nil, err
		}
		grounds, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Judgment{Target: target, Grounds: grounds, Line: t.line}, nil

	case "ANNEX":
		// ANNEX expr TO name. Appends one item to the schedule filed
		// under the name (a copy is retrieved, extended, and refiled;
		// schedules are documents too).
		p.next()
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("TO"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the schedule annexed to")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Annex{Expr: e, Name: nameTok.text, Line: t.line}, nil

	case "SUBSTITUTE":
		// SUBSTITUTE expr FOR ITEM expr OF name. Replaces one item of
		// the schedule filed under the name, 1-indexed.
		p.next()
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("FOR", "ITEM"); err != nil {
			return nil, err
		}
		idx, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("OF"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the schedule amended")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Substitute{Expr: e, Index: idx, Name: nameTok.text, Line: t.line}, nil

	case "INSCRIBE":
		// INSCRIBE expr UNDER expr IN name. Enters one entry in the
		// register filed under the name (a copy is retrieved, amended,
		// and refiled; registers are documents too).
		p.next()
		val, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("UNDER"); err != nil {
			return nil, err
		}
		key, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("IN"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the register inscribed in")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Inscribe{Value: val, Key: key, Name: nameTok.text, Line: t.line}, nil

	case "EXPUNGE":
		// EXPUNGE removes a register entry. An absent entry is a no-op.
		p.next()
		if err := p.expectWords("THE", "ENTRY", "UNDER"); err != nil {
			return nil, err
		}
		key, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("IN"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the register expunged from")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Expunge{Key: key, Name: nameTok.text, Line: t.line}, nil

	case "COMMENCE":
		// COMMENCE files a Form K-1 string as a new case and stores its case
		// number under name. The new case does not inherit parent state.
		p.next()
		if err := p.expectWords("PROCEEDINGS", "UPON"); err != nil {
			return nil, err
		}
		src, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokComma, "before FILED UNDER"); err != nil {
			return nil, err
		}
		if err := p.expectWords("FILED", "UNDER"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "to file the new case number under")
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Commence{Source: src, Name: nameTok.text, Line: t.line}, nil

	case "FILE":
		// A motion intercepts one verdict, clears both stacks, and resumes at
		// the named article.
		p.next()
		if err := p.expectWords("A", "MOTION", "TO", "RECONSIDER"); err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokComma, "before REFERRING TO ARTICLE"); err != nil {
			return nil, err
		}
		if err := p.expectWords("REFERRING", "TO", "ARTICLE"); err != nil {
			return nil, err
		}
		numTok, err := p.expectKind(tokInt, "identifying where reconsideration resumes")
		if err != nil {
			return nil, err
		}
		n, err := parseInteger(numTok)
		if err != nil {
			return nil, err
		}
		m := Motion{Article: n, Line: t.line}
		if p.peek().kind == tokComma {
			p.next()
			if err := p.expectWords("THE", "GROUNDS", "FILED", "UNDER"); err != nil {
				return nil, err
			}
			nameTok, err := p.expectKind(tokIdent, "to file the grounds under")
			if err != nil {
				return nil, err
			}
			m.Grounds = nameTok.text
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return m, nil

	case "GRANT":
		// A license permits practice until it or the patent expires.
		p.next()
		if err := p.expectWords("A", "LICENSE", "UNDER"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the licensed invention")
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("TO"); err != nil {
			return nil, err
		}
		to, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokComma, "before the term"); err != nil {
			return nil, err
		}
		if err := p.expectWords("FOR", "A", "TERM", "OF"); err != nil {
			return nil, err
		}
		term, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		unit := p.next()
		if unit.kind != tokWord || (unit.text != "DAYS" && unit.text != "DAY") {
			return nil, reject(unit.line, unit.col, "a license term must use DAY or DAYS; encountered %s %q", unit.kind, unit.text)
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return LicenseGrant{Name: nameTok.text, To: to, Term: term, Line: t.line}, nil

	case "ASSIGN":
		// Assignment transfers ownership and is refused while licenses remain.
		p.next()
		if err := p.expectWords("THE", "LETTERS", "FOR"); err != nil {
			return nil, err
		}
		nameTok, err := p.expectKind(tokIdent, "naming the assigned invention")
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("TO"); err != nil {
			return nil, err
		}
		to, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return AssignLetters{Name: nameTok.text, To: to, Line: t.line}, nil

	case "PUBLISH":
		// PUBLISH appends to the court-wide gazette in this transaction.
		p.next()
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("IN", "THE", "GAZETTE"); err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Publish{Expr: e, Line: t.line}, nil

	case "COMMIT":
		// COMMIT appends an immutable archive version and updates its catalog entry.
		p.next()
		val, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("TO", "THE", "ARCHIVE", "AS"); err != nil {
			return nil, err
		}
		name, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return ArchiveCommit{Value: val, Name: name, Line: t.line}, nil

	case "HOLD":
		// HOLD evaluates the expression and records its display as verdict details.
		p.next()
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("IN", "CONTEMPT"); err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Contempt{Expr: e, Line: t.line}, nil

	case "STRIKE":
		// STRIKE removes a record from folded state but not from the log.
		p.next()
		nameTok, err := p.expectKind(tokIdent, "naming the record struck")
		if err != nil {
			return nil, err
		}
		if err := p.expectWords("FROM", "THE", "RECORD"); err != nil {
			return nil, err
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return Strike{Name: nameTok.text, Line: t.line}, nil
	}
	return nil, reject(t.line, t.col, "the word %q begins no statement known to this office", t.text)
}

// parseCondition reads a full condition: clauses joined by AND ALSO and
// OR IN THE ALTERNATIVE. AND ALSO binds tighter, as conjunction does;
// both operators associate to the left.
func (p *parser) parseCondition() (Cond, error) {
	left, err := p.parseConjunction()
	if err != nil {
		return nil, err
	}
	for p.atWord("OR") {
		line := p.peek().line
		p.next()
		if err := p.expectWords("IN", "THE", "ALTERNATIVE"); err != nil {
			return nil, err
		}
		right, err := p.parseConjunction()
		if err != nil {
			return nil, err
		}
		left = CondBinary{Op: "OR IN THE ALTERNATIVE", L: left, R: right, Line: line}
	}
	return left, nil
}

func (p *parser) parseConjunction() (Cond, error) {
	left, err := p.parseClause()
	if err != nil {
		return nil, err
	}
	// AND continues the condition only as AND ALSO; a bare AND belongs
	// to whatever list encloses it, if any does.
	for p.atWord("AND") && p.wordAt(1) == "ALSO" {
		line := p.peek().line
		p.next()
		p.next()
		right, err := p.parseClause()
		if err != nil {
			return nil, err
		}
		left = CondBinary{Op: "AND ALSO", L: left, R: right, Line: line}
	}
	return left, nil
}

func (p *parser) parseClause() (Cond, error) {
	left, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	cond := Clause{Left: left}
	if p.atWord("FAIL") {
		p.next()
		if err := p.expectWords("TO"); err != nil {
			return nil, err
		}
		cond.Negated = true
	}
	t := p.next()
	if t.kind != tokWord {
		return nil, reject(t.line, t.col, "expected a comparator, encountered %s %q", t.kind, t.text)
	}
	switch t.text {
	case "EXCEED":
		cond.Cmp = CmpExceed
	case "EQUAL":
		cond.Cmp = CmpEqual
	case "FALL":
		if err := p.expectWords("SHORT", "OF"); err != nil {
			return nil, err
		}
		cond.Cmp = CmpFallShort
	case "DIFFER":
		if err := p.expectWords("FROM"); err != nil {
			return nil, err
		}
		cond.Cmp = CmpDiffer
	default:
		return nil, reject(t.line, t.col, "expected EXCEED, FALL SHORT OF, EQUAL, or DIFFER FROM; encountered %q", t.text)
	}
	right, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	cond.Right = right
	return cond, nil
}

func (p *parser) parseArgumentList() ([]Expr, error) {
	var args []Expr
	for {
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		args = append(args, e)
		if p.atWord("AND") {
			p.next()
			continue
		}
		return args, nil
	}
}

func (p *parser) parseExpression() (Expr, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tokWord {
			return left, nil
		}
		var op string
		switch t.text {
		case "PLUS":
			op = "PLUS"
		case "LESS":
			op = "LESS"
		default:
			return left, nil
		}
		p.next()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = Binary{Op: op, L: left, R: right, Line: t.line}
	}
}

func (p *parser) parseTerm() (Expr, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tokWord {
			return left, nil
		}
		var op string
		switch t.text {
		case "TIMES":
			p.next()
			op = "TIMES"
		case "APPORTIONED":
			p.next()
			if err := p.expectWords("AMONG"); err != nil {
				return nil, err
			}
			op = "APPORTIONED AMONG"
		case "NOTWITHSTANDING":
			p.next()
			op = "NOTWITHSTANDING"
		default:
			return left, nil
		}
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = Binary{Op: op, L: left, R: right, Line: t.line}
	}
}

func (p *parser) parseFactor() (Expr, error) {
	t := p.peek()
	switch t.kind {
	case tokInt:
		p.next()
		n, err := parseInteger(t)
		if err != nil {
			return nil, err
		}
		return IntLit{Val: n}, nil
	case tokSum:
		p.next()
		m, ok := law.ParseSum(t.text)
		if !ok {
			return nil, reject(t.line, t.col, "sum %q is outside the 64-bit range", t.text)
		}
		return SumLit{Mantissa: m}, nil
	case tokString:
		p.next()
		return StrLit{Val: t.text}, nil
	case tokIdent:
		p.next()
		return Var{Name: t.text, Line: t.line}, nil
	case tokLParen:
		p.next()
		e, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectKind(tokRParen, "closing the parenthetical"); err != nil {
			return nil, err
		}
		return e, nil
	case tokWord:
		switch t.text {
		case "SUSTAINED":
			p.next()
			return FindingLit{Val: true}, nil
		case "OVERRULED":
			p.next()
			return FindingLit{Val: false}, nil
		case "A":
			// Schedule and register literals consume AND-separated entries
			// greedily. Parentheses disambiguate an enclosing list.
			p.next()
			if p.atWord("POWER") {
				// A power of attorney carries an office as a value.
				p.next()
				if err := p.expectWords("OF", "ATTORNEY", "OVER", "THE", "OFFICE", "OF"); err != nil {
					return nil, err
				}
				nameTok, err := p.expectKind(tokIdent, "naming the office conferred")
				if err != nil {
					return nil, err
				}
				return PowerOf{Office: nameTok.text, Line: t.line}, nil
			}
			if p.atWord("REGISTER") {
				p.next()
				if err := p.expectWords("COMPRISING"); err != nil {
					return nil, err
				}
				lit := RegisterLit{Line: t.line}
				for {
					v, err := p.parseExpression()
					if err != nil {
						return nil, err
					}
					if err := p.expectWords("UNDER"); err != nil {
						return nil, err
					}
					k, err := p.parseExpression()
					if err != nil {
						return nil, err
					}
					lit.Entries = append(lit.Entries, RegisterInit{Value: v, Key: k})
					if p.atWord("AND") {
						p.next()
						continue
					}
					break
				}
				return lit, nil
			}
			if err := p.expectWords("SCHEDULE", "COMPRISING"); err != nil {
				return nil, err
			}
			lit := ScheduleLit{Line: t.line}
			for {
				e, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				lit.Items = append(lit.Items, e)
				if p.atWord("AND") {
					p.next()
					continue
				}
				break
			}
			return lit, nil
		case "AN":
			// AN EXHIBIT OF …, AN EXCERPT OF …, AN EMPTY SCHEDULE, or
			// AN EMPTY REGISTER
			if p.wordAt(1) == "EMPTY" {
				p.next()
				p.next()
				switch {
				case p.atWord("SCHEDULE"):
					p.next()
					return ScheduleLit{Line: t.line}, nil
				case p.atWord("REGISTER"):
					p.next()
					return RegisterLit{Line: t.line}, nil
				}
				bad := p.peek()
				return nil, reject(bad.line, bad.col, "expected SCHEDULE or REGISTER after AN EMPTY; encountered %q", bad.text)
			}
			if p.wordAt(1) == "EXCERPT" {
				// Excerpt bounds are 1-indexed and inclusive.
				p.next()
				if err := p.expectWords("EXCERPT", "OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("FROM"); err != nil {
					return nil, err
				}
				from, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("TO"); err != nil {
					return nil, err
				}
				to, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				return Excerpt{Of: of, From: from, To: to, Line: t.line}, nil
			}
			// AN EXHIBIT OF name WHEREIN f IS expr AND g IS expr
			p.next()
			if err := p.expectWords("EXHIBIT", "OF"); err != nil {
				return nil, err
			}
			ofTok, err := p.expectKind(tokIdent, "naming the exhibit offered")
			if err != nil {
				return nil, err
			}
			if err := p.expectWords("WHEREIN"); err != nil {
				return nil, err
			}
			lit := ExhibitLit{Of: ofTok.text, Line: t.line}
			for {
				fTok, err := p.expectKind(tokIdent, "naming an entry of the exhibit")
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("IS"); err != nil {
					return nil, err
				}
				e, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				lit.Fields = append(lit.Fields, FieldInit{Name: fTok.text, Expr: e})
				// AND continues the exhibit only when followed by ident IS.
				if p.atWord("AND") && p.peekAt(1).kind == tokIdent && p.wordAt(2) == "IS" {
					p.next()
					continue
				}
				break
			}
			return lit, nil
		case "THE":
			p.next()
			if p.atWord("CASE") {
				// THE CASE AT BAR returns the current case number as a string.
				p.next()
				if err := p.expectWords("AT", "BAR"); err != nil {
					return nil, err
				}
				return CaseAtBar{Line: t.line}, nil
			}
			if item := p.atWord("ITEM"); item || p.atWord("ENTRY") {
				// THE ITEM AT i IN s and THE ENTRY UNDER k IN r share the same
				// expression and collection parsing.
				p.next()
				preposition := "UNDER"
				if item {
					preposition = "AT"
				}
				if err := p.expectWords(preposition); err != nil {
					return nil, err
				}
				selector, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("IN"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				if item {
					return ItemAt{Index: selector, Of: of, Line: t.line}, nil
				}
				return EntryAt{Key: selector, Of: of, Line: t.line}, nil
			}
			if p.atWord("ROSTER") {
				// THE ROSTER OF returns register keys in alphabetical order.
				p.next()
				if err := p.expectWords("OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				return RosterOf{Of: of, Line: t.line}, nil
			}
			if p.atWord("DATE") {
				// THE DATE OF THESE PRESENTS returns seconds since the Unix epoch.
				p.next()
				if err := p.expectWords("OF", "THESE", "PRESENTS"); err != nil {
					return nil, err
				}
				return Presents{Line: t.line}, nil
			}
			if p.atWord("DOCUMENT") {
				// THE DOCUMENT reads the version selected by the archive catalog.
				p.next()
				name, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("FROM", "THE", "ARCHIVE"); err != nil {
					return nil, err
				}
				return DocumentFrom{Name: name, Line: t.line}, nil
			}
			if p.atWord("PRACTICE") {
				// THE PRACTICE OF reads a disclosure when access rules allow it.
				p.next()
				if err := p.expectWords("OF"); err != nil {
					return nil, err
				}
				nameTok, err := p.expectKind(tokIdent, "naming the invention practiced")
				if err != nil {
					return nil, err
				}
				return Practice{Name: nameTok.text, Line: t.line}, nil
			}
			if p.atWord("DISCRETION") {
				// THE DISCRETION OF THE COURT selects an integer in the inclusive range.
				p.next()
				if err := p.expectWords("OF", "THE", "COURT", "BETWEEN"); err != nil {
					return nil, err
				}
				lo, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("AND"); err != nil {
					return nil, err
				}
				hi, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				return Discretion{Lo: lo, Hi: hi, Line: t.line}, nil
			}
			if p.atWord("LENGTH") {
				// THE LENGTH OF counts string runes or collection entries.
				p.next()
				if err := p.expectWords("OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				return Measure{Of: of, Line: t.line}, nil
			}
			if p.atWord("RECORD") {
				// Discovery reads another case's record through the replay ledger.
				p.next()
				nameTok, err := p.expectKind(tokIdent, "naming the record discovered")
				if err != nil {
					return nil, err
				}
				if err := p.expectWords("IN", "THE", "MATTER", "OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				return Discovery{Name: nameTok.text, Of: of, Line: t.line}, nil
			}
			if p.atWord("STANDING") {
				// THE STANDING OF reads another case's status through the replay ledger.
				p.next()
				if err := p.expectWords("OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				return Standing{Of: of, Line: t.line}, nil
			}
			if p.atWord("TRANSCRIPT") {
				// THE TRANSCRIPT OF renders a value as PROCLAIM would.
				p.next()
				if err := p.expectWords("OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				return Transcript{Of: of, Line: t.line}, nil
			}
			if p.atWord("SUM") {
				// THE SUM CERTAIN OF parses a complete numeric string.
				p.next()
				if err := p.expectWords("CERTAIN", "OF"); err != nil {
					return nil, err
				}
				of, err := p.parseFactor()
				if err != nil {
					return nil, err
				}
				return SumCertain{Of: of, Line: t.line}, nil
			}
			if p.atWord("FINDING") {
				p.next()
				// THE FINDING UNDER calls the office carried by a power of attorney.
				if p.atWord("UNDER") {
					p.next()
					power, err := p.parseFactor()
					if err != nil {
						return nil, err
					}
					var args []Expr
					if p.atWord("REGARDING") {
						p.next()
						args, err = p.parseArgumentList()
						if err != nil {
							return nil, err
						}
					}
					return CallUnder{Power: power, Args: args, Line: t.line}, nil
				}
				if err := p.expectWords("OF"); err != nil {
					return nil, err
				}
				nameTok, err := p.expectKind(tokIdent, "naming the office consulted")
				if err != nil {
					return nil, err
				}
				var args []Expr
				if p.atWord("REGARDING") {
					p.next()
					args, err = p.parseArgumentList()
					if err != nil {
						return nil, err
					}
				}
				return Call{Office: nameTok.text, Args: args, Line: t.line}, nil
			}
			// THE field ENTERED IN factor
			fieldTok, err := p.expectKind(tokIdent, "naming the entry inspected")
			if err != nil {
				return nil, err
			}
			if err := p.expectWords("ENTERED", "IN"); err != nil {
				return nil, err
			}
			of, err := p.parseFactor()
			if err != nil {
				return nil, err
			}
			return Inspect{Field: fieldTok.text, Of: of, Line: t.line}, nil
		}
	case tokEOF, tokPeriod, tokComma, tokColon, tokRParen:
	}
	return nil, reject(t.line, t.col, "expected a value, encountered %s %q", t.kind, t.text)
}
