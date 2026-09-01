package gregor

// The examiner rejects visible use after assignment in one straight-line block.
// A conditional assignment does not mark the value as assigned because the arm
// may not execute. Registry-dependent ownership checks remain runtime checks.

// examine applies the static rules to a parsed filing.
func examine(prog *Program) error {
	for _, art := range prog.Articles {
		if err := examineBlock(art.Stmts); err != nil {
			return err
		}
	}
	for _, off := range prog.Offices {
		if err := examineBlock(off.Stmts); err != nil {
			return err
		}
		for _, sec := range off.Sections {
			if err := examineBlock(sec.Stmts); err != nil {
				return err
			}
		}
	}
	return nil
}

// examineBlock walks one straight-line statement sequence.
func examineBlock(stmts []Stmt) error {
	assignedAt := map[string]int{} // invention -> line it was assigned away
	for _, s := range stmts {
		if err := checkStmt(s, assignedAt); err != nil {
			return err
		}
		// Only an unconditional, top-level assignment marks: an
		// assignment inside a SHOULD arm may not happen.
		if a, ok := s.(AssignLetters); ok {
			assignedAt[a.Name] = a.Line
		}
	}
	return nil
}

// checkStmt reports the first use of assigned-away letters within the
// statement, conditional arms included.
func checkStmt(s Stmt, assignedAt map[string]int) error {
	use := func(name string, line int, what string) error {
		at, gone := assignedAt[name]
		if !gone {
			return nil
		}
		return reject(line, 1, "the letters for %q were assigned at line %d; %s at line %d is use after assignment", name, at, what, line)
	}
	var checkExpr func(Expr) error
	checkExprs := func(exprs []Expr) error {
		for _, expr := range exprs {
			if err := checkExpr(expr); err != nil {
				return err
			}
		}
		return nil
	}
	checkExpr = func(e Expr) error {
		switch ex := e.(type) {
		case Practice:
			return use(ex.Name, ex.Line, "the practice")
		case Binary:
			if err := checkExpr(ex.L); err != nil {
				return err
			}
			return checkExpr(ex.R)
		case Call:
			return checkExprs(ex.Args)
		case CallUnder:
			if err := checkExpr(ex.Power); err != nil {
				return err
			}
			return checkExprs(ex.Args)
		case ExhibitLit:
			for _, field := range ex.Fields {
				if err := checkExpr(field.Expr); err != nil {
					return err
				}
			}
		case RegisterLit:
			for _, entry := range ex.Entries {
				if err := checkExpr(entry.Value); err != nil {
					return err
				}
				if err := checkExpr(entry.Key); err != nil {
					return err
				}
			}
		case Inspect:
			return checkExpr(ex.Of)
		case Measure:
			return checkExpr(ex.Of)
		case Excerpt:
			if err := checkExpr(ex.Of); err != nil {
				return err
			}
			if err := checkExpr(ex.From); err != nil {
				return err
			}
			return checkExpr(ex.To)
		case Transcript:
			return checkExpr(ex.Of)
		case SumCertain:
			return checkExpr(ex.Of)
		case ScheduleLit:
			return checkExprs(ex.Items)
		case ItemAt:
			if err := checkExpr(ex.Index); err != nil {
				return err
			}
			return checkExpr(ex.Of)
		case EntryAt:
			if err := checkExpr(ex.Key); err != nil {
				return err
			}
			return checkExpr(ex.Of)
		case RosterOf:
			return checkExpr(ex.Of)
		case Discretion:
			if err := checkExpr(ex.Lo); err != nil {
				return err
			}
			return checkExpr(ex.Hi)
		case DocumentFrom:
			return checkExpr(ex.Name)
		case Standing:
			return checkExpr(ex.Of)
		case Discovery:
			return checkExpr(ex.Of)
		}
		return nil
	}
	var checkCond func(Cond) error
	checkCond = func(c Cond) error {
		switch cn := c.(type) {
		case Clause:
			if err := checkExpr(cn.Left); err != nil {
				return err
			}
			return checkExpr(cn.Right)
		case CondBinary:
			if err := checkCond(cn.L); err != nil {
				return err
			}
			return checkCond(cn.R)
		}
		return nil
	}

	switch st := s.(type) {
	case Recording:
		return checkExpr(st.Expr)
	case Entering:
		return checkExpr(st.Expr)
	case Proclaim:
		return checkExpr(st.Expr)
	case Summons:
		if st.From != nil {
			return checkExpr(st.From)
		}
	case Conditional:
		if err := checkCond(st.Cond); err != nil {
			return err
		}
		if err := checkStmt(st.Then, assignedAt); err != nil {
			return err
		}
		if st.Else != nil {
			return checkStmt(st.Else, assignedAt)
		}
	case TimedSummons:
		if st.From != nil {
			if err := checkExpr(st.From); err != nil {
				return err
			}
		}
		if err := checkExpr(st.Days); err != nil {
			return err
		}
		return checkStmt(st.Else, assignedAt)
	case Petition:
		return checkExprs(st.Args)
	case PetitionUnder:
		if err := checkExpr(st.Power); err != nil {
			return err
		}
		return checkExprs(st.Args)
	case Remand:
		if st.Value != nil {
			return checkExpr(st.Value)
		}
	case Contempt:
		return checkExpr(st.Expr)
	case Serve:
		if err := checkExpr(st.Value); err != nil {
			return err
		}
		return checkExpr(st.Target)
	case Judgment:
		if err := checkExpr(st.Grounds); err != nil {
			return err
		}
		return checkExpr(st.Target)
	case Annex:
		return checkExpr(st.Expr)
	case Substitute:
		if err := checkExpr(st.Expr); err != nil {
			return err
		}
		return checkExpr(st.Index)
	case Inscribe:
		if err := checkExpr(st.Value); err != nil {
			return err
		}
		return checkExpr(st.Key)
	case Expunge:
		return checkExpr(st.Key)
	case AdjournFor:
		return checkExpr(st.Days)
	case ArchiveCommit:
		if err := checkExpr(st.Value); err != nil {
			return err
		}
		return checkExpr(st.Name)
	case PatentGrant:
		if err := use(st.Name, st.Line, "the re-issuance"); err != nil {
			return err
		}
		if err := checkExpr(st.Disclosure); err != nil {
			return err
		}
		return checkExpr(st.Term)
	case LicenseGrant:
		if err := use(st.Name, st.Line, "the license"); err != nil {
			return err
		}
		if err := checkExpr(st.To); err != nil {
			return err
		}
		return checkExpr(st.Term)
	case AssignLetters:
		if err := use(st.Name, st.Line, "the second assignment"); err != nil {
			return err
		}
		return checkExpr(st.To)
	case Commence:
		return checkExpr(st.Source)
	case Publish:
		return checkExpr(st.Expr)
	}
	return nil
}
