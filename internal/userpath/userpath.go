// Package userpath resolves the per-user directories aitop reads from.
//
// Every path aitop touches belongs to whoever is running it. Resolving that
// at runtime rather than baking a literal home directory into the binary is
// what lets one build serve every account on the machine.
package userpath

import (
	"os"
	"path/filepath"
)

// Home is the running user's home directory, or "" when neither the
// environment nor the user database can name one. Callers treat "" as
// "no home-relative path is knowable" rather than falling back to "/".
//
// HOME wins over the user database so that a session which deliberately
// re-points it (a test harness, a sandbox, su without -l) is believed.
func Home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// ConfigHome is $XDG_CONFIG_HOME, or ~/.config when it is unset, per the XDG
// base directory spec. It returns "" when there is no home to fall back to.
func ConfigHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	if h := Home(); h != "" {
		return filepath.Join(h, ".config")
	}
	return ""
}
