package gregor

// This file defines the parser's abstract syntax tree.

type Program struct {
	Form           string // "K-1" (a case), "K-2" (a supplemental filing), or "S-1" (a statute)
	Name           string
	Incorporations []Incorporation
	Constants      []ConstDecl
	Exhibits       []ExhibitDecl
	Articles       []Article
	Offices        []Office
}

type Incorporation struct { // INCORPORATE BY REFERENCE name.
	Name string
	Line int
}

type ConstDecl struct { // HEREINAFTER, name SHALL MEAN literal.
	Name string
	Expr Expr // an IntLit, StrLit, or FindingLit; nothing else may be meant
	Line int
}

type ExhibitDecl struct { // THE EXHIBIT OF name, COMPRISING f AND g.
	Name   string
	Fields []string
	Line   int
}

type Article struct {
	Number int64
	Stmts  []Stmt
	Line   int
}

type Office struct {
	Name     string
	Params   []string
	Stmts    []Stmt
	Sections []Section
	Line     int
}

type Section struct {
	Number int64
	Stmts  []Stmt
	Line   int
}

type Stmt interface{ stmt() }

type Recording struct { // LET IT BE RECORDED THAT name IS expr.
	Name string
	Expr Expr
	Line int
}
type Proclaim struct { // PROCLAIM expr.
	Expr Expr
	Line int
}
type Summons struct { // AWAIT SUMMONS [FROM expr], FILED UNDER name.
	From Expr // nil: the next record, whoever served it; else the case whose voice is awaited
	Name string
	Line int
}
type TimedSummons struct { // AWAIT SUMMONS [FROM expr] FOR AT MOST expr DAYS, FILED UNDER name. FAILING WHICH, stmt
	From Expr // nil: the next record, whoever served it; else the case whose voice is awaited
	Days Expr
	Name string
	Else Stmt // mandatory: a deadline without a contingency is not a deadline
	Line int
}
type Referral struct { // REFER TO ARTICLE n. / REFER TO SECTION n.
	ToSection bool
	Number    int64
	Line      int
}
type Conditional struct { // SHOULD cond, stmt [FAILING WHICH, stmt]
	Cond Cond
	Then Stmt
	Else Stmt // nil when nothing follows from failure, which is rare
	Line int
}
type Entering struct { // LET IT BE ENTERED IN name THAT field IS expr.
	Name  string
	Field string
	Expr  Expr
	Line  int
}
type Petition struct { // PETITION THE OFFICE OF name WITH args.
	Office string
	Args   []Expr
	Line   int
}
type Remand struct { // REMAND. / REMAND WITH expr.
	Value Expr // nil when the office simply stops corresponding
	Line  int
}
type Adjourn struct { // ADJOURN INDEFINITELY.
	Line int
}
type Contempt struct { // HOLD expr IN CONTEMPT.
	Expr Expr
	Line int
}
type Strike struct { // STRIKE name FROM THE RECORD.
	Name string
	Line int
}
type Serve struct { // SERVE NOTICE OF expr UPON expr.
	Value  Expr
	Target Expr
	Line   int
}
type Judgment struct { // ENTER JUDGMENT AGAINST expr, ON THE GROUNDS OF expr.
	Target  Expr
	Grounds Expr
	Line    int
}
type Annex struct { // ANNEX expr TO name.
	Expr Expr
	Name string
	Line int
}
type Substitute struct { // SUBSTITUTE expr FOR ITEM expr OF name.
	Expr  Expr
	Index Expr
	Name  string
	Line  int
}
type Inscribe struct { // INSCRIBE expr UNDER expr IN name.
	Value Expr
	Key   Expr
	Name  string
	Line  int
}
type Expunge struct { // EXPUNGE THE ENTRY UNDER expr IN name.
	Key  Expr
	Name string
	Line int
}
type PetitionUnder struct { // PETITION UNDER expr [WITH a AND b].
	Power Expr
	Args  []Expr
	Line  int
}
type AdjournFor struct { // ADJOURN FOR expr DAYS.
	Days Expr
	Line int
}
type ArchiveCommit struct { // COMMIT expr TO THE ARCHIVE AS expr.
	Value Expr
	Name  Expr
	Line  int
}
type PatentGrant struct { // LET LETTERS PATENT ISSUE FOR name, DISCLOSING expr, FOR A TERM OF expr DAYS.
	Name       string
	Disclosure Expr
	Term       Expr
	Line       int
}
type Commence struct { // COMMENCE PROCEEDINGS UPON expr, FILED UNDER name.
	Source Expr
	Name   string
	Line   int
}
type Motion struct { // FILE A MOTION TO RECONSIDER, REFERRING TO ARTICLE n[, THE GROUNDS FILED UNDER name].
	Article int64
	Grounds string // "" when the movant does not care why
	Line    int
}
type Publish struct { // PUBLISH expr IN THE GAZETTE.
	Expr Expr
	Line int
}
type LicenseGrant struct { // GRANT A LICENSE UNDER name TO expr, FOR A TERM OF expr DAYS.
	Name string
	To   Expr
	Term Expr
	Line int
}
type AssignLetters struct { // ASSIGN THE LETTERS FOR name TO expr.
	Name string
	To   Expr
	Line int
}
type GazetteAwait struct { // AWAIT THE GAZETTE, FILED UNDER name.
	Name string
	Line int
}

func (Recording) stmt()     {}
func (Judgment) stmt()      {}
func (Entering) stmt()      {}
func (Proclaim) stmt()      {}
func (Summons) stmt()       {}
func (TimedSummons) stmt()  {}
func (Referral) stmt()      {}
func (Conditional) stmt()   {}
func (Petition) stmt()      {}
func (Remand) stmt()        {}
func (Adjourn) stmt()       {}
func (Contempt) stmt()      {}
func (Strike) stmt()        {}
func (Serve) stmt()         {}
func (AdjournFor) stmt()    {}
func (Annex) stmt()         {}
func (Substitute) stmt()    {}
func (ArchiveCommit) stmt() {}
func (PatentGrant) stmt()   {}
func (Commence) stmt()      {}
func (Motion) stmt()        {}
func (Publish) stmt()       {}
func (GazetteAwait) stmt()  {}
func (LicenseGrant) stmt()  {}
func (AssignLetters) stmt() {}
func (Inscribe) stmt()      {}
func (Expunge) stmt()       {}
func (PetitionUnder) stmt() {}

type Comparator string

const (
	CmpExceed    Comparator = "EXCEED"
	CmpFallShort Comparator = "FALL SHORT OF"
	CmpEqual     Comparator = "EQUAL"
	CmpDiffer    Comparator = "DIFFER FROM"
)

// Cond is a condition: a single comparison clause, or clauses joined by
// the connectives. AND ALSO binds tighter than OR IN THE ALTERNATIVE;
// both are left-associative. Every clause is heard; the Court reads
// everything, whether or not the outcome is already decided.
type Cond interface{ cond() }

type Clause struct { // expr [FAIL TO] comparator expr
	Left    Expr
	Right   Expr
	Cmp     Comparator
	Negated bool // FAIL TO
}

type CondBinary struct { // cond AND ALSO cond / cond OR IN THE ALTERNATIVE cond
	Op   string // "AND ALSO" or "OR IN THE ALTERNATIVE"
	L, R Cond
	Line int
}

func (Clause) cond()     {}
func (CondBinary) cond() {}

type Expr interface{ expr() }

type IntLit struct{ Val int64 }
type SumLit struct{ Mantissa int64 } // 12.50, held in pennies; no float was consulted
type StrLit struct{ Val string }
type FindingLit struct{ Val bool } // SUSTAINED / OVERRULED
type Var struct {
	Name string
	Line int
}
type Binary struct {
	Op   string // PLUS, LESS, TIMES, APPORTIONED AMONG, NOTWITHSTANDING
	L, R Expr
	Line int
}
type Call struct { // THE FINDING OF office REGARDING args
	Office string
	Args   []Expr
	Line   int
}
type ExhibitLit struct { // AN EXHIBIT OF name WHEREIN f IS expr AND g IS expr
	Of     string
	Fields []FieldInit
	Line   int
}
type FieldInit struct {
	Name string
	Expr Expr
}
type Inspect struct { // THE field ENTERED IN factor
	Field string
	Of    Expr
	Line  int
}
type Measure struct { // THE LENGTH OF factor
	Of   Expr
	Line int
}
type Excerpt struct { // AN EXCERPT OF factor FROM expr TO expr
	Of       Expr
	From, To Expr
	Line     int
}
type Transcript struct { // THE TRANSCRIPT OF factor
	Of   Expr
	Line int
}
type SumCertain struct { // THE SUM CERTAIN OF factor
	Of   Expr
	Line int
}
type CaseAtBar struct { // THE CASE AT BAR: this case's own number
	Line int
}
type Presents struct { // THE DATE OF THESE PRESENTS: now, in court days since the epoch
	Line int
}
type ScheduleLit struct { // A SCHEDULE COMPRISING e AND e / AN EMPTY SCHEDULE
	Items []Expr // empty for AN EMPTY SCHEDULE
	Line  int
}
type ItemAt struct { // THE ITEM AT expr IN factor
	Index Expr
	Of    Expr
	Line  int
}
type RegisterLit struct { // A REGISTER COMPRISING e UNDER k AND … / AN EMPTY REGISTER
	Entries []RegisterInit // empty for AN EMPTY REGISTER
	Line    int
}
type RegisterInit struct { // one inscription of a register literal
	Value Expr
	Key   Expr
}
type EntryAt struct { // THE ENTRY UNDER expr IN factor
	Key  Expr
	Of   Expr
	Line int
}
type RosterOf struct { // THE ROSTER OF factor: the keys, alphabetically
	Of   Expr
	Line int
}
type PowerOf struct { // A POWER OF ATTORNEY OVER THE OFFICE OF name
	Office string
	Line   int
}
type CallUnder struct { // THE FINDING UNDER expr REGARDING a AND b
	Power Expr
	Args  []Expr
	Line  int
}
type Discretion struct { // THE DISCRETION OF THE COURT BETWEEN expr AND expr
	Lo, Hi Expr
	Line   int
}
type DocumentFrom struct { // THE DOCUMENT expr FROM THE ARCHIVE
	Name Expr
	Line int
}
type Practice struct { // THE PRACTICE OF name: the disclosure, if you may
	Name string
	Line int
}
type Standing struct { // THE STANDING OF factor: another case's status
	Of   Expr
	Line int
}
type Discovery struct { // THE RECORD name IN THE MATTER OF factor: another case's record
	Name string
	Of   Expr
	Line int
}

func (IntLit) expr()       {}
func (StrLit) expr()       {}
func (FindingLit) expr()   {}
func (Var) expr()          {}
func (Binary) expr()       {}
func (Call) expr()         {}
func (ExhibitLit) expr()   {}
func (Inspect) expr()      {}
func (Measure) expr()      {}
func (Excerpt) expr()      {}
func (Transcript) expr()   {}
func (SumCertain) expr()   {}
func (CaseAtBar) expr()    {}
func (Discretion) expr()   {}
func (Presents) expr()     {}
func (ScheduleLit) expr()  {}
func (ItemAt) expr()       {}
func (SumLit) expr()       {}
func (DocumentFrom) expr() {}
func (Practice) expr()     {}
func (Standing) expr()     {}
func (Discovery) expr()    {}
func (RegisterLit) expr()  {}
func (EntryAt) expr()      {}
func (RosterOf) expr()     {}
func (PowerOf) expr()      {}
func (CallUnder) expr()    {}
