// Package canon embeds the standard Form S-1 statutes.
package canon

import "embed"

//go:embed *.trial
var FS embed.FS

var order = [...]string{
	"statutes-of-arithmetic.trial",
	"statutes-of-strings.trial",
	"statutes-of-schedules.trial",
	"statutes-of-trigonometry.trial",
	"statutes-of-delegation.trial",
}

// Files lists the canon in dependency order. The returned slice belongs to
// the caller and may be changed.
func Files() []string {
	return append([]string(nil), order[:]...)
}

// Order lists the canon in dependency order.
//
// Deprecated: use Files. This mutable variable remains for source
// compatibility. Production code does not read it.
var Order = Files()
