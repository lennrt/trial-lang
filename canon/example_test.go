package canon_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/lennrt/trial-lang/canon"
)

func Example() {
	source, err := canon.FS.ReadFile("statutes-of-arithmetic.trial")
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.SplitN(string(source), "\n", 3)
	fmt.Println(lines[0])
	fmt.Println(lines[1])
	// Output:
	// FORM S-1.
	// IN THE MATTER OF: statutes-of-arithmetic.
}

func ExampleFiles() {
	// Load dependencies before statutes that incorporate them. FS also holds
	// demonstration cases, so its directory listing is not an enactment list.
	for _, name := range canon.Files() {
		source, err := canon.FS.ReadFile(name)
		if err != nil {
			log.Fatal(err)
		}
		form, _, _ := strings.Cut(string(source), "\n")
		fmt.Printf("%s: %s\n", name, form)
	}
	// Output:
	// statutes-of-arithmetic.trial: FORM S-1.
	// statutes-of-strings.trial: FORM S-1.
	// statutes-of-schedules.trial: FORM S-1.
	// statutes-of-trigonometry.trial: FORM S-1.
	// statutes-of-delegation.trial: FORM S-1.
}
