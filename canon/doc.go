// Package canon provides the bundled standard-library source for triallang.
// triallang programs use legal English: a case is a program, a statute is a
// reusable source file, and an office is a callable routine. Statutes use
// Form S-1; cases use Form K-1.
//
// This package exposes source data for Go tools that inspect, distribute, or
// load the standard library. Importing canon does not compile, enact, or execute
// triallang programs. The compiler and runtime are internal to the triallang
// module; use the [trial CLI] to check or run a program.
//
// # Reading the source from Go
//
// [FS] is a read-only embedded filesystem. Read a file with FS.ReadFile using a
// path relative to its root, such as "statutes-of-arithmetic.trial". No source
// checkout, filesystem setup, or Kafka connection is needed to read these files.
// The embedded contents come from the version of this Go module in your build.
//
// [Files] returns the statute filenames in dependency order. Iterate that list
// when loading the canon: statutes-of-arithmetic.trial precedes the schedule
// statute that depends on it. Each call returns a separate slice that the caller
// may change. The deprecated [Order] variable remains for source compatibility;
// changing it does not change the list returned by Files.
//
// FS also contains Form K-1 demonstration programs named deposing-*.trial.
// Files excludes those programs. Enumerating FS or globbing all *.trial files
// therefore does not produce a list suitable for statute enactment.
//
// # Available statutes
//
// The canon contains five statutes:
//
//   - statutes-of-arithmetic.trial provides absolute-value, maximum, minimum,
//     signum, power, and greatest-common-divisor.
//   - statutes-of-strings.trial provides repetition, reversal, containment, and
//     position-of. String positions count Unicode code points, starting at one.
//   - statutes-of-schedules.trial provides total-of, greatest-of, reversal-of,
//     tally-of, and containment-of. It incorporates statutes-of-arithmetic.
//   - statutes-of-trigonometry.trial provides the-table-of-sines, sine, and
//     cosine. A full turn has 120 integer angle units, each representing three
//     degrees. Sine and cosine values are scaled by 1000; callers create the
//     table once and pass it to each call.
//   - statutes-of-delegation.trial provides applied-to-each, selected-by, and
//     folded-with. These map, filter, and fold schedules using powers of attorney
//     that refer to other offices.
//
// These office names are triallang identifiers, not Go functions. The embedded
// sources contain their signatures and usage notes; the [language specification]
// describes the underlying operations and value types.
//
// # Using a statute in triallang
//
// Incorporate a statute by its declared name, without the .trial extension.
// With the CLI installed, save this case as compare.trial:
//
//	FORM K-1.
//	IN THE MATTER OF: compare.
//	INCORPORATE BY REFERENCE statutes-of-arithmetic.
//	ARTICLE 1.
//	    PROCLAIM THE FINDING OF maximum REGARDING 7 AND 12.
//	    ADJOURN INDEFINITELY.
//
// Run it locally with the bundled statutes:
//
//	trial run --canon compare.trial
//
// The command prints 12 and needs no Kafka broker. To check the same source and
// imports without executing the case, use:
//
//	trial check --canon compare.trial
//
// The repository's separate example statute is named the-statutes-of-arithmetic.
// It is not the bundled statutes-of-arithmetic and is not supplied by --canon.
// See the [triallang quickstart] for installation and more programs.
//
// [trial CLI]: https://github.com/lennrt/trial-lang/blob/main/docs/interfaces.md
// [language specification]: https://github.com/lennrt/trial-lang/blob/main/spec/spec.md
// [triallang quickstart]: https://github.com/lennrt/trial-lang#quickstart
package canon
