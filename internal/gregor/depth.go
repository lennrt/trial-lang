package gregor

import "slices"

// maxDepth bounds both active recursive syntax entries and paths through
// statement, condition, and expression nodes. Containers do not add depth.
const maxDepth = 256

func depthRejection(line, col int) error {
	return reject(line, col, "syntax or expression depth exceeds the limit of %d", maxDepth)
}

func (p *parser) enterDepth() error {
	if p.depth >= maxDepth {
		t := p.peek()
		return depthRejection(t.line, t.col)
	}
	p.depth++
	return nil
}

type depthRoot struct {
	node   any
	line   int
	source string
}

// validateDepth runs before recursive examination or generation, including for
// ASTs constructed without the parser.
func validateDepth(prog *Program) error {
	for _, root := range depthRoots(prog) {
		if err := checkDepth(root); err != nil {
			return err
		}
	}
	return nil
}

// depthRoots keeps declaration provenance attached to each independent tree.
func depthRoots(prog *Program) []depthRoot {
	var roots []depthRoot
	for _, cd := range prog.Constants {
		roots = append(roots, depthRoot{cd.Expr, cd.Line, cd.source})
	}
	for _, art := range prog.Articles {
		for _, st := range art.Stmts {
			roots = append(roots, depthRoot{st, art.Line, art.source})
		}
	}
	for _, off := range prog.Offices {
		for _, st := range off.Stmts {
			roots = append(roots, depthRoot{st, off.Line, off.source})
		}
		for _, sec := range off.Sections {
			for _, st := range sec.Stmts {
				roots = append(roots, depthRoot{st, sec.Line, off.source})
			}
		}
	}
	return roots
}

// checkDepth uses an explicit work list so checking the bound cannot itself
// overflow the call stack. Children retain source order for the first rejection.
func checkDepth(root depthRoot) error {
	type entry struct {
		node        any
		depth, line int
	}
	pending := []entry{{root.node, 1, root.line}}
	for len(pending) > 0 {
		at := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if at.node == nil {
			continue
		}
		if at.depth > maxDepth {
			return withSource(depthRejection(at.line, 1), root.source)
		}
		var children []any
		line := depthChildren(at.node, at.line, func(child any) {
			if child != nil {
				children = append(children, child)
			}
		})
		if line <= 0 {
			line = at.line
		}
		for _, child := range slices.Backward(children) {
			pending = append(pending, entry{child, at.depth + 1, line})
		}
	}
	return nil
}

// depthChildren enumerates only semantic AST nodes. Entry/field list wrappers
// do not add depth; absent optional operands do not consume it.
func depthChildren(node any, line int, add func(any)) int {
	switch n := node.(type) {
	case Recording:
		add(n.Expr)
		return n.Line
	case Proclaim:
		add(n.Expr)
		return n.Line
	case Summons:
		add(n.From)
		return n.Line
	case TimedSummons:
		add(n.From)
		add(n.Days)
		add(n.Else)
		return n.Line
	case Conditional:
		add(n.Cond)
		add(n.Then)
		add(n.Else)
		return n.Line
	case Entering:
		add(n.Expr)
		return n.Line
	case Petition:
		for _, child := range n.Args {
			add(child)
		}
		return n.Line
	case Remand:
		add(n.Value)
		return n.Line
	case Contempt:
		add(n.Expr)
		return n.Line
	case Serve:
		add(n.Value)
		add(n.Target)
		return n.Line
	case Judgment:
		add(n.Target)
		add(n.Grounds)
		return n.Line
	case Annex:
		add(n.Expr)
		return n.Line
	case Substitute:
		add(n.Expr)
		add(n.Index)
		return n.Line
	case Inscribe:
		add(n.Value)
		add(n.Key)
		return n.Line
	case Expunge:
		add(n.Key)
		return n.Line
	case PetitionUnder:
		add(n.Power)
		for _, child := range n.Args {
			add(child)
		}
		return n.Line
	case AdjournFor:
		add(n.Days)
		return n.Line
	case ArchiveCommit:
		add(n.Value)
		add(n.Name)
		return n.Line
	case PatentGrant:
		add(n.Disclosure)
		add(n.Term)
		return n.Line
	case Commence:
		add(n.Source)
		return n.Line
	case Publish:
		add(n.Expr)
		return n.Line
	case LicenseGrant:
		add(n.To)
		add(n.Term)
		return n.Line
	case AssignLetters:
		add(n.To)
		return n.Line
	case Clause:
		add(n.Left)
		add(n.Right)
		return line
	case CondBinary:
		add(n.L)
		add(n.R)
		return n.Line
	case Binary:
		add(n.L)
		add(n.R)
		return n.Line
	case Call:
		for _, child := range n.Args {
			add(child)
		}
		return n.Line
	case Inspect:
		add(n.Of)
		return n.Line
	case Measure:
		add(n.Of)
		return n.Line
	case Excerpt:
		add(n.Of)
		add(n.From)
		add(n.To)
		return n.Line
	case Transcript:
		add(n.Of)
		return n.Line
	case SumCertain:
		add(n.Of)
		return n.Line
	case ScheduleLit:
		for _, child := range n.Items {
			add(child)
		}
		return n.Line
	case ItemAt:
		add(n.Index)
		add(n.Of)
		return n.Line
	case EntryAt:
		add(n.Key)
		add(n.Of)
		return n.Line
	case RosterOf:
		add(n.Of)
		return n.Line
	case CallUnder:
		add(n.Power)
		for _, child := range n.Args {
			add(child)
		}
		return n.Line
	case Discretion:
		add(n.Lo)
		add(n.Hi)
		return n.Line
	case DocumentFrom:
		add(n.Name)
		return n.Line
	case Standing:
		add(n.Of)
		return n.Line
	case Discovery:
		add(n.Of)
		return n.Line
	case ExhibitLit:
		for _, field := range n.Fields {
			add(field.Expr)
		}
		return n.Line
	case RegisterLit:
		for _, entry := range n.Entries {
			add(entry.Value)
			add(entry.Key)
		}
		return n.Line
	}
	return line
}
