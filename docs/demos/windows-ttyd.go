//go:build ignore

// This optional adapter passes an explicit working directory to ttyd 1.7.7.
// Build it as ttyd.exe in a separate directory and set VHS_TTYD_REAL to the
// original executable. See README.md in this directory for Windows setup.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	real := os.Getenv("VHS_TTYD_REAL")
	self, err := os.Executable()
	if err != nil || !filepath.IsAbs(real) || strings.EqualFold(filepath.Clean(real), filepath.Clean(self)) {
		fmt.Fprintln(os.Stderr, "VHS_TTYD_REAL must name the absolute path to the original ttyd executable")
		os.Exit(2)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmd := exec.Command(real, append([]string{"--cwd", cwd}, os.Args[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
