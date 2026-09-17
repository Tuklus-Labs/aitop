package native

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureHome is the home directory the captured session trees in this package
// were recorded under. Several assertions check a project label derived from a
// fixture cwd, and that derivation is home-relative, so the suite is pinned to
// the home the captures assume.
//
// Without this pin the package only passed on the machine that produced the
// fixtures: every project label silently degraded to a bare basename elsewhere,
// which is a green suite reporting on a tool that no longer works.
const fixtureHome = "/home/aegis"

func TestMain(m *testing.M) {
	os.Setenv("HOME", fixtureHome)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(fixtureHome, ".config"))
	os.Exit(m.Run())
}
