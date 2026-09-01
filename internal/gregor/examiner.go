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
		return reject(line, 1, "the letters for %q were assigned away at line %d; %s at line %d is use after assignment, and the examiner sees it without convening anyone", name, at, what, line)
	}
	var checkExpr func(e Expr, line int) error
	checkExpr = func(e Expr, line int) error {
		switch ex := e.(type) {
		case Practice:
			return use(ex.Name, ex.Line, "the practice")
		case Binary:
			if err := checkExpr(ex.L, line); err != nil {
				return err
			}
			return checkExpr(ex.R, line)
		case Call:
			for _, a := range ex.Args {
				if err := checkExpr(a, line); err != nil {
					return err
				}
			}
		case ExhibitLit:
			for _, f := range ex.Fields {
				if err := checkExpr(f.Expr, line); err != nil {
					return err
				}
			}
		case Inspect:
			return checkExpr(ex.Of, line)
		case Measure:
			return checkExpr(ex.Of, line)
		case Excerpt:
			if err := checkExpr(ex.Of, line); err != nil {
				return err
			}
			if err := checkExpr(ex.From, line); err != nil {
				return err
			}
			return checkExpr(ex.To, line)
		case Transcript:
			return checkExpr(ex.Of, line)
		case SumCertain:
			return checkExpr(ex.Of, line)
		case ScheduleLit:
			for _, item := range ex.Items {
				if err := checkExpr(item, line); err != nil {
					return err
				}
			}
		case ItemAt:
			if err := checkExpr(ex.Index, line); err != nil {
				return err
			}
			return checkExpr(ex.Of, line)
		case Discretion:
			if err := checkExpr(ex.Lo, line); err != nil {
				return err
			}
			return checkExpr(ex.Hi, line)
		case DocumentFrom:
			return checkExpr(ex.Name, line)
		case Standing:
			return checkExpr(ex.Of, line)
		case Discovery:
			return checkExpr(ex.Of, line)
		}
		return nil
	}
	var checkCond func(Cond) error
	checkCond = func(c Cond) error {
		switch cn := c.(type) {
		case Clause:
			if err := checkExpr(cn.Left, 0); err != nil {
				return err
			}
			return checkExpr(cn.Right, 0)
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
		return checkExpr(st.Expr, st.Line)
	case Entering:
		return checkExpr(st.Expr, st.Line)
	case Proclaim:
		return checkExpr(st.Expr, st.Line)
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
		if err := checkExpr(st.Days, st.Line); err != nil {
			return err
		}
		return checkStmt(st.Else, assignedAt)
	case Petition:
		for _, a := range st.Args {
			if err := checkExpr(a, st.Line); err != nil {
				return err
			}
		}
	case Remand:
		if st.Value != nil {
			return checkExpr(st.Value, st.Line)
		}
	case Contempt:
		return checkExpr(st.Expr, st.Line)
	case Serve:
		if err := checkExpr(st.Value, st.Line); err != nil {
			return err
		}
		return checkExpr(st.Target, st.Line)
	case Judgment:
		if err := checkExpr(st.Grounds, st.Line); err != nil {
			return err
		}
		return checkExpr(st.Target, st.Line)
	case Annex:
		return checkExpr(st.Expr, st.Line)
	case Substitute:
		if err := checkExpr(st.Expr, st.Line); err != nil {
			return err
		}
		return checkExpr(st.Index, st.Line)
	case AdjournFor:
		return checkExpr(st.Days, st.Line)
	case ArchiveCommit:
		if err := checkExpr(st.Value, st.Line); err != nil {
			return err
		}
		return checkExpr(st.Name, st.Line)
	case PatentGrant:
		if err := use(st.Name, st.Line, "the re-issuance"); err != nil {
			return err
		}
		if err := checkExpr(st.Disclosure, st.Line); err != nil {
			return err
		}
		return checkExpr(st.Term, st.Line)
	case LicenseGrant:
		if err := use(st.Name, st.Line, "the license"); err != nil {
			return err
		}
		if err := checkExpr(st.To, st.Line); err != nil {
			return err
		}
		return checkExpr(st.Term, st.Line)
	case AssignLetters:
		if err := use(st.Name, st.Line, "the second assignment"); err != nil {
			return err
		}
		return checkExpr(st.To, st.Line)
	case Commence:
		return checkExpr(st.Source, st.Line)
	case Publish:
		return checkExpr(st.Expr, st.Line)
	}
	return nil
}
