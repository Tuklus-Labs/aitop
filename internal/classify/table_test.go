package classify

import (
	"testing"

	"aitop/internal/proc"
	"aitop/internal/types"
)

func TestPlaywrightMCPIsNotPrimary(t *testing.T) {
	p := types.Process{
		PID:     35355,
		PPID:    35037,
		Comm:    "node-MainThread",
		Exe:     "/usr/bin/node",
		Cmdline: []string{"node", "/home/aegis/.local/lib/playwright-mcp/node_modules/@playwright/mcp/cli.js", "--executable-path", "/opt/brave-bin/brave"},
	}
	got := Classify(p)
	if got.Role == types.RolePrimary {
		t.Fatalf("playwright-mcp-is-not-a-primary violated: pid=%d comm=%s role=%s cmdline=%v", p.PID, p.Comm, got.Role, p.Cmdline)
	}
	if got.Role != types.RoleIgnore {
		t.Fatalf("playwright-mcp-is-not-a-primary violated: want ignore got %s", got.Role)
	}
}

func TestChatGPTRendererIsNotPrimary(t *testing.T) {
	blob := "/usr/lib/chatgpt/ChatGPT --type=renderer --crashpad-handler-pid=1"
	p := types.Process{
		PID:     1022955,
		PPID:    1021735,
		Comm:    "ChatGPT",
		Exe:     "/usr/lib/chatgpt/ChatGPT",
		Cmdline: proc.ParseCmdline([]byte(blob)),
	}
	got := Classify(p)
	if got.Role == types.RolePrimary || got.Role == types.RoleDesktop {
		t.Fatalf("chatgpt-renderer-is-not-primary violated: pid=%d comm=%s role=%s argv=%v", p.PID, p.Comm, got.Role, p.Cmdline)
	}
}

func TestChatGPTMainIsPrimaryDesktop(t *testing.T) {
	p := types.Process{
		PID:     1021406,
		PPID:    1,
		Comm:    "ChatGPT",
		Exe:     "/usr/lib/chatgpt/ChatGPT",
		Cmdline: proc.ParseCmdline([]byte("/usr/lib/chatgpt/ChatGPT")),
	}
	got := Classify(p)
	if got.Role != types.RoleDesktop {
		t.Fatalf("chatgpt-main-is-primary violated: pid=%d comm=%s role=%s (main has no --type=)", p.PID, p.Comm, got.Role)
	}
	if got.CollapseKey != "chatgpt:1021406" {
		t.Fatalf("chatgpt-collapse-key invariant violated: key=%q", got.CollapseKey)
	}
}

func TestGrokToolShellIsNotAgent(t *testing.T) {
	p := types.Process{
		PID:     2014640,
		PPID:    1853851,
		Comm:    "zsh",
		Cmdline: []string{"/usr/bin/zsh", "-c", "builtin export GROK_AGENT=1; something"},
	}
	got := Classify(p)
	if got.Role != types.RoleIgnore {
		t.Fatalf("GROK_AGENT-is-tool-shell-not-agent violated: role=%s", got.Role)
	}
}

func TestClaudeInteractiveIsPrimary(t *testing.T) {
	p := types.Process{
		PID:     35037,
		PPID:    34826,
		Comm:    "claude",
		Exe:     "/home/aegis/.local/share/claude/versions/2.1.233",
		Cmdline: []string{"claude", "--dangerously-skip-permissions"},
		CWD:     "/home/aegis",
	}
	parent := types.Process{PID: 34826, Comm: "zsh"}
	got := ClassifyWithParent(p, parent)
	if got.Role != types.RolePrimary || got.Runtime != types.RuntimeClaude {
		t.Fatalf("claude-interactive-is-primary violated: role=%s runtime=%s", got.Role, got.Runtime)
	}
	if got.ProvenNameHint == "Heph" {
		t.Fatalf("claude-without-proof-is-not-heph violated: classifier stamped Heph")
	}
}

func TestParlorSidecarCgroup(t *testing.T) {
	p := types.Process{
		PID:     464995,
		Comm:    "node-MainThread",
		Exe:     "/usr/bin/node",
		Cmdline: []string{"/usr/bin/node", "/home/aegis/Projects/parlor/dist/sidecar/parlor-sidecar-worker.mjs"},
	}
	got := ClassifyCgroup(p, "0::/user.slice/user-1000.slice/app.slice/parlor-sidecar@machine-gary-grok.service")
	if got.Role != types.RoleSidecar || got.Runtime != types.RuntimeParlor {
		t.Fatalf("parlor-sidecar-from-cgroup violated: role=%s runtime=%s", got.Role, got.Runtime)
	}
	if got.CollapseKey != "parlor-sidecar:machine-gary-grok" {
		t.Fatalf("parlor-instance-name invariant violated: key=%q", got.CollapseKey)
	}
}

func TestTalariaIsNotIris(t *testing.T) {
	p := types.Process{
		Comm:    "python",
		Cmdline: []string{"/home/aegis/.hermes/hermes-agent/venv/bin/python", "/home/aegis/Projects/talaria/server.py", "serve"},
	}
	got := Classify(p)
	if got.Role == types.RolePrimary && got.Runtime == types.RuntimeHermes {
		t.Fatalf("talaria-is-not-iris violated: classified as hermes primary")
	}
}

func TestForgeSidecarIsForge(t *testing.T) {
	p := types.Process{
		Comm:    "node-MainThread",
		Cmdline: []string{"node", "/home/aegis/Projects/forge/native/sidecar/forge-sidecar.mjs"},
	}
	got := Classify(p)
	if got.Role != types.RoleSidecar || got.Runtime != types.RuntimeForge {
		t.Fatalf("forge-sidecar-is-forge violated: role=%s runtime=%s", got.Role, got.Runtime)
	}
}

func TestBugforgeIsNotForgeSidecar(t *testing.T) {
	p := types.Process{
		Comm:    "node",
		Cmdline: []string{"node", "/home/aegis/Projects/bugforge/server.mjs"},
	}
	got := Classify(p)
	if got.Runtime == types.RuntimeForge {
		t.Fatalf("bugforge-is-not-forge-sidecar violated: %+v", got)
	}
}

func TestHermesTUIIsIris(t *testing.T) {
	p := types.Process{
		Comm: "hermes",
		Exe:  "/home/aegis/.hermes/hermes-agent/venv/bin/hermes",
	}
	got := Classify(p)
	if got.Role != types.RolePrimary || got.Runtime != types.RuntimeHermes || got.ProvenNameHint != "Iris" {
		t.Fatalf("hermes-tui-is-iris violated: %+v", got)
	}
}

func TestCodexNodeWrapperIsNotSecondAgent(t *testing.T) {
	p := types.Process{
		Comm:    "node-MainThread",
		Cmdline: []string{"node", "/home/aegis/.local/bin/codex", "--yolo"},
	}
	got := Classify(p)
	if got.Role == types.RolePrimary {
		t.Fatalf("codex-node-wrapper-is-not-a-second-agent violated: role=%s", got.Role)
	}
}

// Every comm the table matches on must be a proc.Candidate, or the two-pass
// walk hands the classifier a process with no cmdline/exe and the rule
// silently never fires.
func TestEveryTableCommIsACandidate(t *testing.T) {
	for _, comm := range []string{
		"chrome_crashpad", "browser_crashpa", "ChatGPT", "electron", "node-MainThread", "node",
		"zsh", "bash", "systemd-inhibit", "ollama", "python", "python3", "forgejo", "forgejo-runner",
		"parlor_relayd", "llama-server", "local-brain", "parlor-doorman", "parlor-impulse", "charon",
		"hermes", "claude", "grok", "codex", "codex-code-mode",
	} {
		if !proc.Candidate(comm) {
			t.Fatalf("classifier-comm-is-walk-candidate violated: %q would reach the table with no cmdline", comm)
		}
	}
	if proc.Candidate("kworker/0:1") || proc.Candidate("firefox") {
		t.Fatal("non-agent-comm-is-not-candidate violated: walk would read cmdline for every process")
	}
}

func TestLocalBackendsGetARow(t *testing.T) {
	llama := types.Process{PID: 2322227, PPID: 1389, Comm: "llama-server",
		Cmdline: []string{"/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server", "-m", "/home/aegis/Models/Qwen3.8-27B/Qwen3.8-27B-Q4_K_M.gguf", "-ngl", "99"}}
	got := ClassifyCgroup(llama, "0::/user.slice/user-1000.slice/user@1000.service/app.slice/hermes-qwen38.service")
	if got.Runtime != types.RuntimeLocal || !got.AgentRoot || got.Role != types.RoleSidecar {
		t.Fatalf("local-backend-is-a-row violated: %+v", got)
	}
	if got.ProvenNameHint != "qwen38" || got.ModelHint != "Qwen3.8-27B-Q4_K_M" {
		t.Fatalf("local-backend-named-from-unit-and-argv violated: name=%q model=%q", got.ProvenNameHint, got.ModelHint)
	}
	ollama := types.Process{PID: 8009, PPID: 1, Comm: "ollama", Cmdline: []string{"/usr/local/bin/ollama", "serve"}}
	if got := ClassifyCgroup(ollama, "0::/system.slice/ollama.service"); got.Runtime != types.RuntimeLocal || got.ProvenNameHint != "ollama" {
		t.Fatalf("ollama-is-a-local-row violated: %+v", got)
	}
	talaria := types.Process{PID: 1, Comm: "python", Cmdline: []string{"/home/aegis/.hermes/hermes-agent/venv/bin/python", "/home/aegis/Projects/talaria/server.py", "serve"}}
	if got := ClassifyCgroup(talaria, "0::/user.slice/.../talaria.service"); got.Runtime != types.RuntimeLocal || got.ProvenNameHint == "Iris" || got.Role == types.RolePrimary {
		t.Fatalf("talaria-is-local-not-iris violated: %+v", got)
	}
	if !proc.Candidate("vllm") || !proc.Candidate("llama-server") {
		t.Fatal("local-comms-are-full-candidates violated")
	}
}

// Remote Control sessions: the daemon (`claude rc`) execs the versioned
// binary directly, so comm is "2.1.239" and the parent is claude. Those are
// primaries, tagged rc; the daemon is a monitor; nothing is a subagent.
func TestRemoteControlSessionIsAPrimaryNotASubagent(t *testing.T) {
	daemon := types.Process{PID: 1367570, PPID: 1367012, Comm: "claude", Exe: "/home/aegis/.local/share/claude/versions/2.1.239", Cmdline: []string{"claude", "rc"}}
	if got := Classify(daemon); got.Role != types.RoleMonitor || got.AgentRoot {
		t.Fatalf("claude-rc-daemon-is-a-monitor violated: %+v", got)
	}
	sess := types.Process{PID: 1368294, PPID: 1367570, Comm: "2.1.239", Exe: "/home/aegis/.local/share/claude/versions/2.1.239",
		Cmdline: []string{"/home/aegis/.local/share/claude/versions/2.1.239", "--print", "--sdk-url", "https://api.anthropic.com/v1/code/sessions/cse_x", "--session-id", "cse_x", "--input-format", "stream-json"}}
	got := ClassifyWithParent(sess, daemon)
	if got.Role != types.RolePrimary || !got.AgentRoot || got.Runtime != types.RuntimeClaude {
		t.Fatalf("remote-control-session-is-a-primary violated: %+v", got)
	}
	if got.Tag != "rc" {
		t.Fatalf("remote-control-session-tagged-rc violated: tag=%q", got.Tag)
	}
	if got.ProvenNameHint != "claude" {
		t.Fatalf("version-comm-session-still-reads-claude violated: hint=%q (comm is 2.1.239)", got.ProvenNameHint)
	}
	if u := unitName("0::/user.slice/user-1000.slice/user@1000.service/session.slice/session-2.scope"); u != "" {
		t.Fatalf("unit-name-is-final-component-never-user-manager violated: %q", u)
	}
	if u := unitName("0::/user.slice/user-1000.slice/user@1000.service/app.slice/hermes-qwen38.service"); u != "hermes-qwen38" {
		t.Fatalf("unit-name-final-component violated: %q", u)
	}
	if !proc.Candidate("2.1.239") {
		t.Fatal("version-string-comm-is-a-walk-candidate violated: the walk would never read this process's exe")
	}
	// A headless claude spawned from another session's shell is its own session too.
	child := types.Process{PID: 2, PPID: 35037, Comm: "claude", Cmdline: []string{"claude", "-p", "hello"}}
	if got := ClassifyWithParent(child, types.Process{PID: 35037, Comm: "claude", Cmdline: []string{"claude"}}); got.Role != types.RolePrimary || !got.AgentRoot || got.Tag != "" {
		t.Fatalf("claude-child-process-is-its-own-session violated: %+v", got)
	}
}
