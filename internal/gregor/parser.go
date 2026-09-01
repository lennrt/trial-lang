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

// peekAt looks n tokens past the current one without consuming anything;
// the parser, like everyone here, is permitted to read ahead in the file
// but not to act on what it finds there.
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
			return reject(t.line, t.col, "expected the word %q, encountered %s %q; the required phrasing is not optional", w, t.kind, t.text)
		}
	}
	return nil
}

func (p *parser) expectPeriod() error {
	t := p.next()
	if t.kind != tokPeriod {
		return reject(t.line, t.col, "a statement must end with a period; encountered %s %q instead. Statements are sentences. You are being sentenced", t.kind, t.text)
	}
	return nil
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
		return nil, reject(formTok.line, formTok.col, "filings are accepted on Form K-1 (a case), Form K-2 (a supplemental filing), or Form S-1 (a statute); %q is not a form, whatever it is", formTok.text)
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

	// INCORPORATE BY REFERENCE statute-name. Statutes are spliced in at
	// filing time; the clerk fetches the enacted text and Gregor
	// compiles it as though you had typed it, which, legally, you did.
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
		// A statute may incorporate statutes (since v1.9, the law
		// accumulates transitively); a supplemental filing still may not.
		if prog.Form == "K-2" {
			return nil, reject(incTok.line, incTok.col, "a supplemental filing may not incorporate a statute; incorporation is performed at the opening of the case, and the case is open")
		}
		prog.Incorporations = append(prog.Incorporations, Incorporation{Name: nameTok.text, Line: incTok.line})
	}

	// Exhibits and defined terms may be established before the
	// articles, definitions being the one kind of paperwork the Court
	// likes to see early.
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

	// Articles. A statute has none: a statute legislates, it does not
	// litigate. A case must have at least one.
	if prog.Form == "S-1" {
		if p.atWord("ARTICLE") {
			t := p.peek()
			return nil, reject(t.line, t.col, "a statute may not contain an ARTICLE; articles are proceedings, and a statute proceeds against no one in particular")
		}
	} else {
		if !p.atWord("ARTICLE") {
			t := p.peek()
			return nil, reject(t.line, t.col, "a case must contain at least one ARTICLE; a filing with no proceedings is a confession")
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
			return nil, reject(t.line, t.col, "a supplemental filing may not establish an office; the building is full")
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
		return nil, reject(t.line, t.col, "a statute that establishes no office regulates nothing, and is void for vagueness")
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
	n, err := strconv.ParseInt(numTok.text, 10, 64)
	if err != nil {
		return 0, 0, reject(numTok.line, numTok.col, "the number %q could not be entered into the ledger", numTok.text)
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

// parseConstDecl reads HEREINAFTER, name SHALL MEAN literal.
// A defined term. It is substituted at compile time wherever the name
// appears; the meaning, once assigned, is not revisited.
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
		n, err := strconv.ParseInt(t.text, 10, 64)
		if err != nil {
			return ConstDecl{}, reject(t.line, t.col, "the number %q exceeds what the ledger can hold", t.text)
		}
		e = IntLit{Val: n}
	case t.kind == tokSum:
		p.next()
		m, ok := law.ParseSum(t.text)
		if !ok {
			return ConstDecl{}, reject(t.line, t.col, "the sum %q could not be entered into the ledger", t.text)
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
		return ConstDecl{}, reject(t.line, t.col, "a defined term must mean an integer, a sum, a string, SUSTAINED, or OVERRULED; it may not mean %s %q, which would require interpretation, which is not this office", t.kind, t.text)
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
				return nil, reject(unit.line, unit.col, "patent terms run in DAYS; %q is not a unit of court time, whatever else it measures", unit.text)
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
		return nil, reject(bad.line, bad.col, "a thing may be RECORDED (in the records) or ENTERED IN an exhibit; %q is neither, and has been done to no one", bad.text)

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
		// AWAIT THE GAZETTE, FILED UNDER x. The court-wide broadcast,
		// read at this case's own cursor, at this case's own pace.
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
		// AWAIT SUMMONS FROM c: the selective receive. The Court
		// listens for one voice among the folk; every record passed
		// over remains where it is, awaiting its own turn.
		var from Expr
		if p.atWord("FROM") {
			p.next()
			f, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			from = f
		}
		// AWAIT SUMMONS FOR AT MOST n DAYS, FILED UNDER x. FAILING
		// WHICH, statement. The receive with a deadline; the artist
		// waits publicly, and there is a limit to it. The FAILING WHICH
		// arm is mandatory: a deadline without a contingency is not a
		// deadline, it is a decoration.
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
				return nil, reject(unit.line, unit.col, "summonses are awaited in DAYS; %q is not a unit of court time, whatever else it measures", unit.text)
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
				return nil, reject(bad.line, bad.col, "a summons awaited FOR AT MOST some term must state what FAILING WHICH; a deadline without a contingency is not a deadline")
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
				return nil, reject(bad.line, bad.col, "FAILING WHICH must be followed by a consequence; encountered %s %q, which consequences nothing", bad.kind, bad.text)
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
			return nil, reject(bad.line, bad.col, "one may refer only to an ARTICLE or a SECTION; %q is outside all jurisdiction", bad.text)
		}
		numTok, err := p.expectKind(tokInt, "identifying the referral")
		if err != nil {
			return nil, err
		}
		n, _ := strconv.ParseInt(numTok.text, 10, 64)
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
			return nil, reject(bad.line, bad.col, "a SHOULD must be followed by a consequence; encountered %s %q, which consequences nothing", bad.kind, bad.text)
		}
		then, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		cnd := Conditional{Cond: cond, Then: then, Line: t.line}
		// FAILING WHICH, statement: the one-armed SHOULD acquires a
		// second arm. It attaches to the nearest SHOULD, as failure does.
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
				return nil, reject(bad.line, bad.col, "FAILING WHICH must be followed by a consequence; encountered %s %q, which consequences nothing", bad.kind, bad.text)
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
		// PETITION UNDER expr [WITH a AND b]. The dynamic petition:
		// the office is whichever one the power of attorney confers.
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
		// ADJOURN FOR expr DAYS. is a continuance: the matter sleeps and
		// resumes of its own accord. ADJOURN INDEFINITELY is the same
		// motion with the resumption left to someone who never comes.
		if p.atWord("FOR") {
			p.next()
			e, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			unit := p.next()
			if unit.kind != tokWord || (unit.text != "DAYS" && unit.text != "DAY") {
				return nil, reject(unit.line, unit.col, "continuances are granted in DAYS; %q is not a unit of court time, whatever else it measures", unit.text)
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
		// ENTER JUDGMENT AGAINST expr, ON THE GROUNDS OF expr. The
		// sentence from the bed: a verdict entered in another case's
		// file, carried out at once, or at any rate at the condemned's
		// next step. Jurisdiction is parental and strict.
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
		// EXPUNGE THE ENTRY UNDER expr IN name. Removes one entry from
		// the register filed under the name; expunging what is not
		// there succeeds vacuously. The Court is no stranger to empty
		// gestures.
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
		// COMMENCE PROCEEDINGS UPON expr, FILED UNDER name. A new case
		// is opened upon the filing (a string bearing Form K-1) and its
		// case number is filed under the name. The new case starts with
		// nothing of its parent's; every accusation begins alone.
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
		// FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE n[, THE
		// GROUNDS FILED UNDER name]. The motion sits on the docket until
		// a verdict would issue, whereupon the Court, exactly once per
		// case, reconsiders instead: the dossier is impounded as the
		// filing fee, the appeals are dismissed with it, and the
		// proceedings resume at the named article.
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
		n, _ := strconv.ParseInt(numTok.text, 10, 64)
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
		// GRANT A LICENSE UNDER name TO expr, FOR A TERM OF expr DAYS.
		// A shared, read-only borrow of the invention: the licensee may
		// practice while the license and the letters both run. A license
		// may not outlive the letters it derives from.
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
			return nil, reject(unit.line, unit.col, "license terms run in DAYS; %q is not a unit of court time, whatever else it measures", unit.text)
		}
		if err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return LicenseGrant{Name: nameTok.text, To: to, Term: term, Line: t.line}, nil

	case "ASSIGN":
		// ASSIGN THE LETTERS FOR name TO expr. The letters move; the
		// previous holder keeps nothing, including the right to practice
		// what he disclosed. Refused while licenses are outstanding.
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
		// PUBLISH expr IN THE GAZETTE. Court-wide broadcast: the
		// transcript is appended to the one gazette everyone reads,
		// within this instruction's own transaction.
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
		// COMMIT expr TO THE ARCHIVE AS expr. The document is entered in
		// the archive, immutably; the catalog is repointed to it. The
		// previous version is not deleted. Nothing is ever deleted.
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
		// HOLD expr IN CONTEMPT. A verdict, on purpose. The expression
		// is evaluated, displayed, and entered as the particulars.
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
		// STRIKE name FROM THE RECORD. The record is struck from the
		// fold and retained in the log, which is both.
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
		return nil, reject(t.line, t.col, "%q is not a comparison recognized in this jurisdiction; the recognized comparisons are EXCEED, FALL SHORT OF, EQUAL, and DIFFER FROM", t.text)
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
		n, err := strconv.ParseInt(t.text, 10, 64)
		if err != nil {
			return nil, reject(t.line, t.col, "the number %q exceeds what the ledger can hold", t.text)
		}
		return IntLit{Val: n}, nil
	case tokSum:
		p.next()
		m, ok := law.ParseSum(t.text)
		if !ok {
			return nil, reject(t.line, t.col, "the sum %q could not be entered into the ledger", t.text)
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
			// A SCHEDULE COMPRISING e AND e AND e, or A REGISTER
			// COMPRISING e UNDER k AND e UNDER k. The ANDs are consumed
			// greedily; inside an enclosing AND-separated list,
			// parenthesize, as one would.
			p.next()
			if p.atWord("POWER") {
				// A POWER OF ATTORNEY OVER THE OFFICE OF f: the office,
				// as a value; the right to petition it, wherever the
				// instrument travels within the case.
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
				return nil, reject(bad.line, bad.col, "the Court keeps empty SCHEDULEs and empty REGISTERs; an empty %q is not an emptiness it recognizes", bad.text)
			}
			if p.wordAt(1) == "EXCERPT" {
				// AN EXCERPT OF factor FROM expr TO expr: a substring,
				// 1-indexed, both ends inclusive. Lawyers count from one.
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
				// AND continues the entries only when what follows reads
				// as one (ident IS …); otherwise the AND belongs to some
				// enclosing list, and the exhibit rests.
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
				// THE CASE AT BAR: the case's own number, a string. Every
				// case knows its own number; it is the one thing it was told.
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
				// THE ROSTER OF r: a schedule of the register's keys, in
				// alphabetical order, because the Court files everything
				// alphabetically eventually.
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
				// THE DATE OF THESE PRESENTS: the current moment, measured
				// in court days since the epoch (1970, when the paperwork
				// began). A court day is one second; see the spec.
				p.next()
				if err := p.expectWords("OF", "THESE", "PRESENTS"); err != nil {
					return nil, err
				}
				return Presents{Line: t.line}, nil
			}
			if p.atWord("DOCUMENT") {
				// THE DOCUMENT expr FROM THE ARCHIVE: the current version
				// of a document the case has committed. The archive keeps
				// every version; the catalog points at the one that counts.
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
				// THE PRACTICE OF name: the disclosed invention, if the
				// case at bar holds the letters or the term has lapsed.
				// Otherwise it is infringement, which explains itself.
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
				// THE DISCRETION OF THE COURT BETWEEN a AND b: an integer
				// the Court selects, inclusive of both bounds, by a process
				// it does not explain and will not repeat.
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
				// THE LENGTH OF factor: of a string, its characters; of
				// an exhibit, its entries. Everything else has only
				// magnitude, or not even that.
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
				// THE RECORD name IN THE MATTER OF factor: discovery.
				// One case reads another's record, through the ledger,
				// so what it did with the answer replays even after the
				// respondent has moved on.
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
				// THE STANDING OF factor: another case's status, as far
				// as this case is permitted to know it. The reading is
				// entered in the ledger, so the answer repeats even when
				// the world does not.
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
				// THE TRANSCRIPT OF factor: any value, rendered as the
				// string PROCLAIM would publish. Transcription is always
				// available. Interpretation is not offered.
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
				// THE SUM CERTAIN OF factor: the integer a string
				// denotes, exactly and entirely, or a verdict.
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
				// THE FINDING UNDER expr REGARDING …: the dynamic
				// consultation; the office is whichever one the power
				// of attorney confers.
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
	return nil, reject(t.line, t.col, "expected a value and encountered %s %q; the Court cannot weigh what has not been entered into evidence", t.kind, t.text)
}
