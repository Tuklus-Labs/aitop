package claude

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureHome is the home directory the verbatim sidecar and transcript
// captures in this package were recorded under. Assertions that derive a
// project label from a fixture cwd are home-relative, so the suite pins HOME to
// the home those captures assume and holds on any machine.
const fixtureHome = "/home/aegis"

// liveHome is the real home of whoever is running the tests, captured before
// the fixture pin. TestLiveClaudeHomeCanary runs against an actual ~/.claude
// tree, so it needs the caller's home rather than the fixture one.
var liveHome string

func TestMain(m *testing.M) {
	liveHome = os.Getenv("HOME")
	if liveHome == "" {
		if h, err := os.UserHomeDir(); err == nil {
			liveHome = h
		}
	}
	os.Setenv("HOME", fixtureHome)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(fixtureHome, ".config"))
	os.Exit(m.Run())
}
