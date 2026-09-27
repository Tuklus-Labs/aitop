package classify

import (
	"testing"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestMacDesktopApplications(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime types.Runtime
	}{
		{"Claude", types.RuntimeClaude},
		{"Hermes", types.RuntimeHermes},
		{"Codex", types.RuntimeCodex},
		{"ChatGPT", types.RuntimeCodex},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exe := "/Applications/" + tc.name + ".app/Contents/MacOS/" + tc.name
			p := types.Process{PID: 42, Comm: tc.name, Exe: exe, Cmdline: []string{exe}}
			got := Classify(p)
			if got.Role != types.RoleDesktop || got.Runtime != tc.runtime || !got.AgentRoot {
				t.Fatalf("desktop %s was not detected: %+v", tc.name, got)
			}
			p.Cmdline = append(p.Cmdline, "--type=renderer")
			if got := Classify(p); got.Role != types.RoleIgnore {
				t.Fatalf("desktop renderer counted as agent: %+v", got)
			}
		})
	}
}

func TestMacHermesPythonBackend(t *testing.T) {
	p := types.Process{
		Comm:    "Python",
		Exe:     "/opt/homebrew/Frameworks/Python.framework/Versions/3.13/Python",
		Cmdline: []string{"/Users/kai/.hermes/hermes-agent/venv/bin/python", "-m", "hermes_cli.main", "serve"},
	}
	if got := Classify(p); got.Runtime != types.RuntimeHermes || got.Role != types.RolePrimary {
		t.Fatalf("Hermes backend was not detected: %+v", got)
	}
}

func TestMacClaudeEmbeddedCLIStaysIndependent(t *testing.T) {
	p := types.Process{
		Comm: "claude",
		Exe:  "/Users/kai/Library/Application Support/Claude/claude-code/2.1.281/claude.app/Contents/MacOS/claude",
	}
	if got := Classify(p); got.Runtime != types.RuntimeClaude || got.Role != types.RolePrimary || !got.AgentRoot {
		t.Fatalf("embedded Claude CLI must remain a session row: %+v", got)
	}
}

func TestMacDesktopNamesNeedBundleEvidence(t *testing.T) {
	for _, name := range []string{"Claude", "Hermes", "Codex"} {
		if got := Classify(types.Process{Comm: name, Exe: "/tmp/" + name}); got.Role == types.RoleDesktop {
			t.Fatalf("desktop guessed from process name %s: %+v", name, got)
		}
	}
}
