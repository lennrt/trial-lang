package canon

import "embed"

// FS contains the bundled triallang source files. Paths are relative to the
// filesystem root, for example "statutes-of-arithmetic.trial".
//
// It includes the Form S-1 statutes returned by [Files] and Form K-1
// demonstration programs named deposing-*.trial. Use Files to select statutes
// in dependency order rather than enumerating every embedded file.
//
//go:embed *.trial
var FS embed.FS

var order = [...]string{
	"statutes-of-arithmetic.trial",
	"statutes-of-strings.trial",
	"statutes-of-schedules.trial",
	"statutes-of-trigonometry.trial",
	"statutes-of-delegation.trial",
}

// Files returns the Form S-1 statute paths in dependency order, with each
// dependency before the statute that incorporates it. Paths are relative to
// [FS] and include the .trial extension. Demonstration programs are excluded.
//
// The returned slice belongs to the caller and may be changed. Each call
// returns a fresh slice; changes to the deprecated [Order] variable do not
// affect it.
func Files() []string {
	return append([]string(nil), order[:]...)
}

// Order lists the canon in dependency order.
//
// Deprecated: use Files. This mutable variable remains for source
// compatibility. Production code does not read it.
var Order = Files()
