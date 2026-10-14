//go:build !darwin

package gui

import (
	"errors"
	"strings"
)

func OpenAddInTerminal(string) error {
	return errors.New("run `hat add` in a terminal to add an account")
}

func OpenHatInTerminal(args ...string) error {
	return errors.New("run `hat " + strings.Join(args, " ") + "` in a terminal")
}
