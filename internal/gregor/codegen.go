package gregor

import (
	"fmt"
	"slices"

	"github.com/lennrt/trial-lang/internal/law"
)

// CompileAt compiles a filing for placement at base, shifting its referral and
// petition targets accordingly. A supplemental filing may refer only to its
// own articles.
func CompileAt(prog *Program, base int64) ([]law.Instr, error) {
	instrs, err := Compile(prog)
	if err != nil {
		return nil, err
	}
	for i := range instrs {
		switch instrs[i].Op {
		case law.OpRefer, law.OpReferOverruled, law.OpPetition, law.OpMotion, law.OpAwaitFor, law.OpAwaitFromFor, law.OpPower:
			instrs[i].Target += base
		}
	}
	return instrs, nil
}

// Compile transforms a parsed case file into a flat list of proceedings. Each
// instruction's index becomes its offset; article and section labels do not
// survive compilation. The layout is the case body, an implicit ADJOURN when
// offices follow, then each office ending in an implicit REMAND.
func Compile(prog *Program) ([]law.Instr, error) {
	// The examiner reads the filing before anything is laid down: use
	// after assignment, where the filing itself makes it visible, is
	// refused at the counter rather than punished at runtime.
	if err := examine(prog); err != nil {
		return nil, err
	}
	c := &codegen{
		officeEntry:   make(map[string]int64),
		officeParams:  make(map[string][]string),
		exhibitFields: make(map[string][]string),
		constants:     make(map[string]law.Value),
	}

	// Pass 0: office existence and arity, so calls can be checked and
	// emitted before the office bodies are laid down. Exhibit shapes
	// likewise; an exhibit declaration is pure paperwork and emits
	// nothing, which is why it is processed first. Defined terms are
	// paperwork of the purest kind: they compile to their meanings and
	// leave no other trace.
	for _, cd := range prog.Constants {
		if _, dup := c.constants[cd.Name]; dup {
			return nil, reject(cd.Line, 1, "the term %q has been defined twice; a term that means two things means nothing, and the filing follows it", cd.Name)
		}
		switch lit := cd.Expr.(type) {
		case IntLit:
			c.constants[cd.Name] = law.Int(lit.Val)
		case SumLit:
			c.constants[cd.Name] = law.Sum(lit.Mantissa)
		case StrLit:
			c.constants[cd.Name] = law.Str(lit.Val)
		case FindingLit:
			c.constants[cd.Name] = law.Finding(lit.Val)
		default:
			return nil, reject(cd.Line, 1, "a defined term must mean a literal; this one has been given something else to mean, and declines")
		}
	}
	for _, off := range prog.Offices {
		if _, dup := c.officeParams[off.Name]; dup {
			return nil, reject(off.Line, 1, "the office of %s has been established twice; one of them is an impostor, and the Court declines to determine which", off.Name)
		}
		for _, param := range off.Params {
			if _, isConst := c.constants[param]; isConst {
				return nil, reject(off.Line, 1, "the office of %s concerns itself with %q, which is a defined term; a defined term cannot be a matter of concern, having already been settled", off.Name, param)
			}
		}
		c.officeParams[off.Name] = off.Params
	}
	for _, ex := range prog.Exhibits {
		if _, dup := c.exhibitFields[ex.Name]; dup {
			return nil, reject(ex.Line, 1, "the exhibit of %s has been established twice; the Court cannot determine which is the forgery, and rejects both", ex.Name)
		}
		seen := map[string]bool{}
		for _, f := range ex.Fields {
			if seen[f] {
				return nil, reject(ex.Line, 1, "the exhibit of %s lists the entry %q twice; one blank per entry", ex.Name, f)
			}
			seen[f] = true
		}
		c.exhibitFields[ex.Name] = ex.Fields
	}

	// Pass 1: the case in chief.
	articleAt := make(map[int64]int64)
	for _, art := range prog.Articles {
		if _, dup := articleAt[art.Number]; dup {
			return nil, reject(art.Line, 1, "ARTICLE %d appears twice in the same case; the duplicate has not been struck, and the case is dismissed instead", art.Number)
		}
		articleAt[art.Number] = int64(len(c.instrs))
		for _, s := range art.Stmts {
			if err := c.genStmt(s, scope{articles: articleAt, inOffice: false}); err != nil {
				return nil, err
			}
		}
	}
	if len(prog.Offices) > 0 {
		c.emit(law.Instr{Op: law.OpAdjourn})
	}

	// Pass 2: the offices.
	for _, off := range prog.Offices {
		c.officeEntry[off.Name] = int64(len(c.instrs))
		sectionAt := make(map[int64]int64)
		// Section addresses must be known before the office body is
		// generated (a body statement may refer forward to a section),
		// so section jumps go through the patch list like everything else.
		sc := scope{sections: sectionAt, inOffice: true, params: off.Params}
		for _, s := range off.Stmts {
			if err := c.genStmt(s, sc); err != nil {
				return nil, err
			}
		}
		for _, sec := range off.Sections {
			if _, dup := sectionAt[sec.Number]; dup {
				return nil, reject(sec.Line, 1, "SECTION %d appears twice within the office of %s", sec.Number, off.Name)
			}
			sectionAt[sec.Number] = int64(len(c.instrs))
			for _, s := range sec.Stmts {
				if err := c.genStmt(s, sc); err != nil {
					return nil, err
				}
			}
		}
		c.emit(law.Instr{Op: law.OpRemand}) // the office stops corresponding
		// Patch section referrals within this office.
		for _, pt := range c.sectionPatches {
			target, ok := sectionAt[pt.number]
			if !ok {
				return nil, reject(pt.line, 1, "the office of %s contains no SECTION %d; the referral is returned unopened", off.Name, pt.number)
			}
			c.instrs[pt.at].Target = target
		}
		c.sectionPatches = nil
	}

	// Pass 3: patch article referrals and office petitions.
	for _, pt := range c.articlePatches {
		target, ok := articleAt[pt.number]
		if !ok {
			return nil, reject(pt.line, 1, "the case contains no ARTICLE %d; the referral is returned unopened", pt.number)
		}
		c.instrs[pt.at].Target = target
	}
	for _, pt := range c.petitionPatches {
		target, ok := c.officeEntry[pt.name]
		if !ok {
			return nil, reject(pt.line, 1, "there is no office of %s in this building, though several people have claimed to have visited it", pt.name)
		}
		c.instrs[pt.at].Target = target
	}
	return c.instrs, nil
}

type scope struct {
	articles map[int64]int64
	sections map[int64]int64
	params   []string
	inOffice bool
}

type patch struct {
	at     int64
	number int64
	name   string
	line   int
}

type codegen struct {
	instrs          []law.Instr
	officeEntry     map[string]int64
	officeParams    map[string][]string
	exhibitFields   map[string][]string
	constants       map[string]law.Value
	articlePatches  []patch
	sectionPatches  []patch
	petitionPatches []patch
}

func (c *codegen) emit(i law.Instr) int64 {
	c.instrs = append(c.instrs, i)
	return int64(len(c.instrs) - 1)
}

// genStoredUpdate retrieves a stored collection, evaluates the update
// operands in order, applies op, and files the result under the same name.
func (c *codegen) genStoredUpdate(name string, line int, op string, sc scope, operands ...Expr) error {
	c.emit(law.Instr{Op: law.OpRetrieve, Name: name, Pos: pos(line)})
	for _, operand := range operands {
		if err := c.genExpr(operand, sc); err != nil {
			return err
		}
	}
	c.emit(law.Instr{Op: op, Pos: pos(line)})
	c.emit(law.Instr{Op: law.OpFile, Name: name, Pos: pos(line)})
	return nil
}

func (c *codegen) genStmt(s Stmt, sc scope) error {
	switch st := s.(type) {
	case Recording:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER and means what it means; it cannot be recorded over. Definitions are not suggestions", st.Name)
		}
		if err := c.genExpr(st.Expr, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpFile, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case Entering:
		// Exhibits are documents, not places: the amendment retrieves a
		// copy, corrects it, and files the corrected copy over the old
		// one. The old one remains in the records topic regardless.
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER; nothing may be entered in a definition", st.Name)
		}
		c.emit(law.Instr{Op: law.OpRetrieve, Name: st.Name, Pos: pos(st.Line)})
		if err := c.genExpr(st.Expr, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpEnter, Name: st.Field, Pos: pos(st.Line)})
		c.emit(law.Instr{Op: law.OpFile, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case Proclaim:
		if err := c.genExpr(st.Expr, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpProclaim, Pos: pos(st.Line)})
		return nil

	case Summons:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "a summons cannot be filed under %q, which is a defined term; the definition was here first", st.Name)
		}
		if st.From != nil {
			// The selective receive: the case whose voice is awaited
			// goes to the dossier first; AWAIT-FROM pops it and listens.
			if err := c.genExpr(st.From, sc); err != nil {
				return err
			}
			c.emit(law.Instr{Op: law.OpAwaitFrom, Pos: pos(st.Line)})
		} else {
			c.emit(law.Instr{Op: law.OpAwait, Pos: pos(st.Line)})
		}
		c.emit(law.Instr{Op: law.OpFile, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case TimedSummons:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "a summons cannot be filed under %q, which is a defined term; the definition was here first", st.Name)
		}
		// Layout: [the voice, if one is named,] the term, the timed
		// await, then the service path (file the response, step over
		// the arm), then the expiry arm. The await's Target is where
		// nobody-came lands.
		if st.From != nil {
			if err := c.genExpr(st.From, sc); err != nil {
				return err
			}
		}
		if err := c.genExpr(st.Days, sc); err != nil {
			return err
		}
		op := law.OpAwaitFor
		if st.From != nil {
			op = law.OpAwaitFromFor
		}
		at := c.emit(law.Instr{Op: op, Pos: pos(st.Line)})
		c.emit(law.Instr{Op: law.OpFile, Name: st.Name, Pos: pos(st.Line)})
		over := c.emit(law.Instr{Op: law.OpRefer, Pos: pos(st.Line)})
		c.instrs[at].Target = int64(len(c.instrs))
		if err := c.genStmt(st.Else, sc); err != nil {
			return err
		}
		c.instrs[over].Target = int64(len(c.instrs))
		return nil

	case Referral:
		if st.ToSection && !sc.inOffice {
			return reject(st.Line, 1, "SECTION referrals are lawful only within an office; the case in chief has articles, and must be content with them")
		}
		if !st.ToSection && sc.inOffice {
			return reject(st.Line, 1, "an office may not refer to an ARTICLE; that would exceed its jurisdiction, and jurisdiction is jurisdiction")
		}
		at := c.emit(law.Instr{Op: law.OpRefer, Pos: pos(st.Line)})
		if st.ToSection {
			c.sectionPatches = append(c.sectionPatches, patch{at: at, number: st.Number, line: st.Line})
		} else {
			c.articlePatches = append(c.articlePatches, patch{at: at, number: st.Number, line: st.Line})
		}
		return nil

	case Motion:
		// The motion refers to an ARTICLE, so it is a creature of the
		// case in chief; an office does not move the Court, it is moved.
		if sc.inOffice {
			return reject(st.Line, 1, "a motion to reconsider is filed in the case in chief; an office does not move the Court, being itself merely a place the Court goes")
		}
		if _, isConst := c.constants[st.Grounds]; st.Grounds != "" && isConst {
			return reject(st.Line, 1, "the grounds cannot be filed under %q, which is a defined term; the definition was here first", st.Grounds)
		}
		at := c.emit(law.Instr{Op: law.OpMotion, Name: st.Grounds, Pos: pos(st.Line)})
		c.articlePatches = append(c.articlePatches, patch{at: at, number: st.Article, line: st.Line})
		return nil

	case Conditional:
		if err := c.genCond(st.Cond, sc, st.Line); err != nil {
			return err
		}
		skip := c.emit(law.Instr{Op: law.OpReferOverruled, Pos: pos(st.Line)})
		if err := c.genStmt(st.Then, sc); err != nil {
			return err
		}
		if st.Else != nil {
			over := c.emit(law.Instr{Op: law.OpRefer, Pos: pos(st.Line)})
			c.instrs[skip].Target = int64(len(c.instrs))
			if err := c.genStmt(st.Else, sc); err != nil {
				return err
			}
			c.instrs[over].Target = int64(len(c.instrs))
		} else {
			c.instrs[skip].Target = int64(len(c.instrs))
		}
		return nil

	case Petition:
		return c.genCall(st.Office, st.Args, false, st.Line, sc)

	case PetitionUnder:
		return c.genCallUnder(st.Power, st.Args, false, st.Line, sc)

	case Remand:
		if !sc.inOffice {
			return reject(st.Line, 1, "REMAND in the case in chief: there is no higher court to remand to; there is no higher court at all")
		}
		if st.Value != nil {
			if err := c.genExpr(st.Value, sc); err != nil {
				return err
			}
			c.emit(law.Instr{Op: law.OpRemand, With: true, Pos: pos(st.Line)})
		} else {
			c.emit(law.Instr{Op: law.OpRemand, Pos: pos(st.Line)})
		}
		return nil

	case Adjourn:
		c.emit(law.Instr{Op: law.OpAdjourn, Pos: pos(st.Line)})
		return nil

	case AdjournFor:
		if err := c.genExpr(st.Days, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpContinuance, Pos: pos(st.Line)})
		return nil

	case Serve:
		// Evaluate the notice before the respondent; SERVE consumes them in
		// stack order.
		if err := c.genExpr(st.Value, sc); err != nil {
			return err
		}
		if err := c.genExpr(st.Target, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpServe, Pos: pos(st.Line)})
		return nil

	case Judgment:
		// Evaluate the grounds before the target; JUDGMENT consumes them in
		// stack order.
		if err := c.genExpr(st.Grounds, sc); err != nil {
			return err
		}
		if err := c.genExpr(st.Target, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpJudgment, Pos: pos(st.Line)})
		return nil

	case Annex:
		// Retrieve a copy of the schedule, extend it, file it back.
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER; nothing may be annexed to a definition", st.Name)
		}
		return c.genStoredUpdate(st.Name, st.Line, law.OpAnnex, sc, st.Expr)

	case Inscribe:
		// Retrieve a copy of the register, inscribe it, file it back.
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER; nothing may be inscribed in a definition", st.Name)
		}
		return c.genStoredUpdate(st.Name, st.Line, law.OpInscribe, sc, st.Key, st.Value)

	case Expunge:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER; nothing may be expunged from a definition", st.Name)
		}
		return c.genStoredUpdate(st.Name, st.Line, law.OpExpunge, sc, st.Key)

	case Substitute:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER; nothing in a definition may be substituted", st.Name)
		}
		return c.genStoredUpdate(st.Name, st.Line, law.OpSubstitute, sc, st.Index, st.Expr)

	case Contempt:
		if err := c.genExpr(st.Expr, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpContempt, Pos: pos(st.Line)})
		return nil

	case ArchiveCommit:
		// Evaluate the document before its name; ARCHIVE consumes them in
		// stack order.
		if err := c.genExpr(st.Value, sc); err != nil {
			return err
		}
		if err := c.genExpr(st.Name, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpArchive, Pos: pos(st.Line)})
		return nil

	case PatentGrant:
		// The disclosure, then the term. The name rides the instruction:
		// inventions are named at filing time, like everything here.
		if err := c.genExpr(st.Disclosure, sc); err != nil {
			return err
		}
		if err := c.genExpr(st.Term, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpPatent, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case LicenseGrant:
		// The licensee, then the term; the invention rides the instruction.
		if err := c.genExpr(st.To, sc); err != nil {
			return err
		}
		if err := c.genExpr(st.Term, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpLicense, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case AssignLetters:
		if err := c.genExpr(st.To, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpAssign, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case Commence:
		// The filing first; the case number it earns is filed under the
		// name, exactly as a summons response would be.
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "a case number cannot be filed under %q, which is a defined term; the definition was here first", st.Name)
		}
		if err := c.genExpr(st.Source, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpCommence, Pos: pos(st.Line)})
		c.emit(law.Instr{Op: law.OpFile, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case Publish:
		if err := c.genExpr(st.Expr, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpPublish, Pos: pos(st.Line)})
		return nil

	case GazetteAwait:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "an edition of the gazette cannot be filed under %q, which is a defined term; the definition was here first", st.Name)
		}
		c.emit(law.Instr{Op: law.OpAwaitGazette, Pos: pos(st.Line)})
		c.emit(law.Instr{Op: law.OpFile, Name: st.Name, Pos: pos(st.Line)})
		return nil

	case Strike:
		if _, isConst := c.constants[st.Name]; isConst {
			return reject(st.Line, 1, "the term %q was defined HEREINAFTER and cannot be struck; definitions outlive their authors", st.Name)
		}
		if slices.Contains(sc.params, st.Name) {
			return reject(st.Line, 1, "an office may not strike %q from the record; it is one of the office's own concerns, and the concerns were assigned", st.Name)
		}
		c.emit(law.Instr{Op: law.OpStrike, Name: st.Name, Pos: pos(st.Line)})
		return nil
	}
	return fmt.Errorf("a statement of unknown species reached the codegen; this is Gregor's fault, not yours")
}

// genCond lays down the instructions that leave exactly one finding on
// the dossier. Connective conditions evaluate every clause: the Court
// reads everything, whether or not the outcome is already decided.
func (c *codegen) genCond(cond Cond, sc scope, line int) error {
	switch cn := cond.(type) {
	case Clause:
		if err := c.genExpr(cn.Left, sc); err != nil {
			return err
		}
		if err := c.genExpr(cn.Right, sc); err != nil {
			return err
		}
		switch cn.Cmp {
		case CmpExceed:
			c.emit(law.Instr{Op: law.OpExceeds, Pos: pos(line)})
		case CmpFallShort:
			c.emit(law.Instr{Op: law.OpFallsShort, Pos: pos(line)})
		case CmpEqual:
			c.emit(law.Instr{Op: law.OpEquals, Pos: pos(line)})
		case CmpDiffer:
			c.emit(law.Instr{Op: law.OpDiffers, Pos: pos(line)})
		}
		if cn.Negated {
			c.emit(law.Instr{Op: law.OpOverturn, Pos: pos(line)})
		}
		return nil
	case CondBinary:
		if err := c.genCond(cn.L, sc, line); err != nil {
			return err
		}
		if err := c.genCond(cn.R, sc, line); err != nil {
			return err
		}
		if cn.Op == "AND ALSO" {
			c.emit(law.Instr{Op: law.OpConsolidate, Pos: pos(cn.Line)})
		} else {
			c.emit(law.Instr{Op: law.OpAlternative, Pos: pos(cn.Line)})
		}
		return nil
	}
	return fmt.Errorf("a condition of unknown species reached the codegen; this is Gregor's fault, not yours")
}

func (c *codegen) genExpr(e Expr, sc scope) error {
	switch ex := e.(type) {
	case IntLit:
		v := law.Int(ex.Val)
		c.emit(law.Instr{Op: law.OpSubmit, Value: &v})
	case SumLit:
		v := law.Sum(ex.Mantissa)
		c.emit(law.Instr{Op: law.OpSubmit, Value: &v})
	case StrLit:
		v := law.Str(ex.Val)
		c.emit(law.Instr{Op: law.OpSubmit, Value: &v})
	case FindingLit:
		v := law.Finding(ex.Val)
		c.emit(law.Instr{Op: law.OpSubmit, Value: &v})
	case Var:
		// A defined term compiles to its meaning. In an office whose
		// concerns include the name, the concern would be shadowed by
		// the definition, which is why such offices are rejected at the
		// door (see Compile, pass 0).
		if v, isConst := c.constants[ex.Name]; isConst {
			v := v
			c.emit(law.Instr{Op: law.OpSubmit, Value: &v, Pos: pos(ex.Line)})
			return nil
		}
		c.emit(law.Instr{Op: law.OpRetrieve, Name: ex.Name, Pos: pos(ex.Line)})
	case Binary:
		if err := c.genExpr(ex.L, sc); err != nil {
			return err
		}
		if err := c.genExpr(ex.R, sc); err != nil {
			return err
		}
		var op string
		switch ex.Op {
		case "PLUS":
			op = law.OpCombine
		case "LESS":
			op = law.OpDeduct
		case "TIMES":
			op = law.OpCompound
		case "APPORTIONED AMONG":
			op = law.OpApportion
		case "NOTWITHSTANDING":
			op = law.OpNotwithstanding
		}
		c.emit(law.Instr{Op: op, Pos: pos(ex.Line)})
	case Call:
		return c.genCall(ex.Office, ex.Args, true, ex.Line, sc)
	case CallUnder:
		return c.genCallUnder(ex.Power, ex.Args, true, ex.Line, sc)
	case PowerOf:
		params, known := c.officeParams[ex.Office]
		if !known {
			return reject(ex.Line, 1, "there is no office of %s in this building to confer; the instrument would be paper over nothing", ex.Office)
		}
		at := c.emit(law.Instr{Op: law.OpPower, Name: ex.Office, Params: params, Pos: pos(ex.Line)})
		c.petitionPatches = append(c.petitionPatches, patch{at: at, name: ex.Office, line: ex.Line})
	case ExhibitLit:
		declared, known := c.exhibitFields[ex.Of]
		if !known {
			return reject(ex.Line, 1, "no exhibit of %s has been established with this court; exhibits must be established (THE EXHIBIT OF %s, COMPRISING ...) before they may be offered", ex.Of, ex.Of)
		}
		isDeclared := make(map[string]bool, len(declared))
		for _, f := range declared {
			isDeclared[f] = true
		}
		filled := make(map[string]bool, len(ex.Fields))
		names := make([]string, 0, len(ex.Fields))
		for _, f := range ex.Fields {
			if !isDeclared[f.Name] {
				return reject(ex.Line, 1, "the exhibit of %s comprises no entry %q; the entry is stricken, and so is the filing", ex.Of, f.Name)
			}
			if filled[f.Name] {
				return reject(ex.Line, 1, "the entry %q has been made twice in the same exhibit; the Court declines to determine which is the correction", f.Name)
			}
			filled[f.Name] = true
			names = append(names, f.Name)
			if err := c.genExpr(f.Expr, sc); err != nil {
				return err
			}
		}
		for _, f := range declared {
			if !filled[f] {
				return reject(ex.Line, 1, "the exhibit of %s is incomplete: the entry %q was left blank. Incomplete forms are rejected in their entirety", ex.Of, f)
			}
		}
		c.emit(law.Instr{Op: law.OpExhibit, Name: ex.Of, Params: names, Pos: pos(ex.Line)})
	case Inspect:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpInspect, Name: ex.Field, Pos: pos(ex.Line)})
	case Measure:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpMeasure, Pos: pos(ex.Line)})
	case Excerpt:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		if err := c.genExpr(ex.From, sc); err != nil {
			return err
		}
		if err := c.genExpr(ex.To, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpExcerpt, Pos: pos(ex.Line)})
	case Transcript:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpTranscribe, Pos: pos(ex.Line)})
	case SumCertain:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpSumCertain, Pos: pos(ex.Line)})
	case CaseAtBar:
		c.emit(law.Instr{Op: law.OpCaseAtBar, Pos: pos(ex.Line)})
	case Presents:
		c.emit(law.Instr{Op: law.OpPresents, Pos: pos(ex.Line)})
	case ScheduleLit:
		for _, item := range ex.Items {
			if err := c.genExpr(item, sc); err != nil {
				return err
			}
		}
		c.emit(law.Instr{Op: law.OpSchedule, Count: len(ex.Items), Pos: pos(ex.Line)})
	case ItemAt:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		if err := c.genExpr(ex.Index, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpItem, Pos: pos(ex.Line)})
	case RegisterLit:
		// An empty register is submitted whole; a comprising literal
		// inscribes its entries one by one, in filing order, so a later
		// inscription under the same key is the correction of an
		// earlier one, as it would be anywhere else in this court.
		empty := law.Register(map[string]law.Value{})
		c.emit(law.Instr{Op: law.OpSubmit, Value: &empty, Pos: pos(ex.Line)})
		for _, entry := range ex.Entries {
			if err := c.genExpr(entry.Key, sc); err != nil {
				return err
			}
			if err := c.genExpr(entry.Value, sc); err != nil {
				return err
			}
			c.emit(law.Instr{Op: law.OpInscribe, Pos: pos(ex.Line)})
		}
	case EntryAt:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		if err := c.genExpr(ex.Key, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpEntry, Pos: pos(ex.Line)})
	case RosterOf:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpRoster, Pos: pos(ex.Line)})
	case Discretion:
		if err := c.genExpr(ex.Lo, sc); err != nil {
			return err
		}
		if err := c.genExpr(ex.Hi, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpDiscretion, Pos: pos(ex.Line)})
	case DocumentFrom:
		if err := c.genExpr(ex.Name, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpDocument, Pos: pos(ex.Line)})
	case Practice:
		c.emit(law.Instr{Op: law.OpPractice, Name: ex.Name, Pos: pos(ex.Line)})
	case Standing:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpStanding, Pos: pos(ex.Line)})
	case Discovery:
		if err := c.genExpr(ex.Of, sc); err != nil {
			return err
		}
		c.emit(law.Instr{Op: law.OpDiscovery, Name: ex.Name, Pos: pos(ex.Line)})
	}
	return nil
}

func (c *codegen) genCall(office string, args []Expr, wants bool, line int, sc scope) error {
	params, known := c.officeParams[office]
	if !known {
		return reject(line, 1, "there is no office of %s in this building, though several people have claimed to have visited it", office)
	}
	if len(args) != len(params) {
		return reject(line, 1, "the office of %s concerns itself with %d matter(s); the petition presents %d. The petition is malformed and the office is offended", office, len(params), len(args))
	}
	for _, a := range args {
		if err := c.genExpr(a, sc); err != nil {
			return err
		}
	}
	at := c.emit(law.Instr{Op: law.OpPetition, Name: office, Params: params, Wants: wants, Pos: pos(line)})
	c.petitionPatches = append(c.petitionPatches, patch{at: at, name: office, line: line})
	return nil
}

// genCallUnder compiles the dynamic petition: the power of attorney
// first, then the arguments, then PETITION-UNDER with the argument
// count. Which office answers, and whether the arity fits, is settled
// at exercise time; the instrument is examined when it is used, which
// is the only time this court examines anything.
func (c *codegen) genCallUnder(power Expr, args []Expr, wants bool, line int, sc scope) error {
	if err := c.genExpr(power, sc); err != nil {
		return err
	}
	for _, a := range args {
		if err := c.genExpr(a, sc); err != nil {
			return err
		}
	}
	c.emit(law.Instr{Op: law.OpPetitionUnder, Count: len(args), Wants: wants, Pos: pos(line)})
	return nil
}

func pos(line int) string { return fmt.Sprintf("filing line %d", line) }
