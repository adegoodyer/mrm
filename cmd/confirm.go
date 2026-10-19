package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/adegoodyer/mrm/internal/term"
)

// errNotConfirmed ends a command the user declined; it exits 1 quietly.
var errNotConfirmed = &exitError{code: 1}

// confirm asks a yes/no question on the terminal. With --yes it says yes;
// without a terminal it refuses, so a script never acts by accident.
func confirm(yes bool, question string) error {
	if yes {
		return nil
	}
	if !term.IsTTY(os.Stdin) {
		return errors.New(question + " - not on a terminal, so add --yes to go ahead")
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	fmt.Fprintln(os.Stderr, "nothing done")
	return errNotConfirmed
}
