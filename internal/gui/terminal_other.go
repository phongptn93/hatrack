//go:build !darwin

package gui

import "errors"

func OpenAddInTerminal(string) error {
	return errors.New("run `hat add` in a terminal to add an account")
}
