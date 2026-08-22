package classify

import (
	"path"
	"strings"

	"aitop/internal/proc"
	"aitop/internal/types"
)

type Result struct {
	Role           types.Role
	Runtime        types.Runtime
	CollapseKey    string
	AgentRoot      bool
	ProvenNameHint string
	ModelHint      string // local backends: the model file they serve
	Tag            string // "rc" for a session the Remote Control daemon spawned
}

func Classify(p types.Process) Result {
	return ClassifyCgroupParent(p, types.Process{}, "")
}

func ClassifyWithParent(p, parent types.Process) Result {
	return ClassifyCgroupParent(p, parent, "")
}

func ClassifyCgroup(p types.Process, cgroup string) Result {
	return ClassifyCgroupParent(p, types.Process{}, cgroup)
}

func ClassifyCgroupParent(p, parent types.Process, cgroup string) Result {
	argv := p.Cmdline
	joined := strings.Join(argv, " ")
	exe := p.Exe
	comm := p.Comm

	if comm == "chrome_crashpad" || comm == "browser_crashpa" || strings.Contains(joined, "crashpad_handler") {
		return Result{Role: types.RoleIgnore}
	}
	if comm == "ChatGPT" && hasChatGPTType(argv) {
		return Result{Role: types.RoleIgnore}
	}
	if strings.Contains(exe, "/usr/lib/obsidian/") || (comm == "electron" && strings.Contains(joined, "obsidian")) {
		return Result{Role: types.RoleIgnore}
	}
	if proc.ArgvContains(argv, "@playwright/mcp/cli.js") {
		return Result{Role: types.RoleIgnore}
	}
	if comm == "zsh" && (proc.ArgvContains(argv, "GROK_AGENT") || proc.ArgvContains(argv, ".claude/shell-snapshots/snapshot-zsh-")) {
		return Result{Role: types.RoleIgnore}
	}
	if comm == "systemd-inhibit" && proc.ArgvContains(argv, "--who=grok") {
		return Result{Role: types.RoleIgnore}
	}
	// Local inference and Iris's own sidecars: not agents, but the house
	// wants them on the board, so they get a row in the locals group.
	if comm == "ollama" && proc.ArgvContains(argv, "serve") {
		return local("ollama", "", cgroup)
	}
	if proc.ArgvContains(argv, "hermes-agent/venv") && proc.ArgvContains(argv, "/Projects/talaria/") {
		return local("talaria", "", cgroup)
	}
	if proc.ArgvContains(argv, "aegis-model-proxy.py") {
		return local("model-proxy", "", cgroup)
	}
	if comm == "llama-server" || comm == "local-brain" || strings.HasPrefix(comm, "vllm") || isVLLM(argv) {
		name := comm
		if isVLLM(argv) {
			name = "vllm"
		}
		return local(name, modelArg(argv), cgroup)
	}
	if comm == "forgejo" || comm == "forgejo-runner" || strings.Contains(exe, "/usr/bin/forgejo") {
		return Result{Role: types.RoleIgnore}
	}
	if comm == "parlor_relayd" || strings.HasSuffix(exe, "parlor_relayd") {
		return Result{Role: types.RoleIgnore}
	}

	if comm == "ChatGPT" && !hasChatGPTType(argv) && (strings.HasSuffix(exe, "/ChatGPT") || exe == "" || strings.Contains(exe, "chatgpt")) {
		return Result{
			Role:        types.RoleDesktop,
			Runtime:     types.RuntimeCodex,
			CollapseKey: "chatgpt:" + itoa(p.PID),
			AgentRoot:   true,
		}
	}
	if strings.Contains(exe, "/usr/lib/chatgpt/resources/codex") && proc.ArgvContains(argv, "app-server") {
		return Result{Role: types.RoleSidecar, Runtime: types.RuntimeCodex, CollapseKey: "chatgpt:parent"}
	}

	if proc.ArgvContains(argv, "parlor/dist/sidecar/parlor-sidecar-worker.mjs") || strings.Contains(cgroup, "parlor-sidecar@") {
		inst := parlorInstance(cgroup)
		key := "parlor-sidecar:" + inst
		return Result{Role: types.RoleSidecar, Runtime: types.RuntimeParlor, CollapseKey: key, AgentRoot: true, ProvenNameHint: inst}
	}
	if comm == "parlor-doorman" || (len(argv) > 0 && path.Base(argv[0]) == "parlor-doorman") {
		return Result{Role: types.RoleSidecar, Runtime: types.RuntimeParlor, CollapseKey: "parlor-doorman", AgentRoot: true}
	}
	if comm == "parlor-impulse" || (len(argv) > 0 && path.Base(argv[0]) == "parlor-impulse") {
		return Result{Role: types.RoleSidecar, Runtime: types.RuntimeParlor, CollapseKey: "parlor-impulse", AgentRoot: true}
	}
	if proc.ArgvContains(argv, "parlor-presence.sh") {
		return Result{Role: types.RoleMonitor, Runtime: types.RuntimeParlor, CollapseKey: "parlor-presence"}
	}
	if proc.ArgvContains(argv, "forge/native/sidecar/forge-sidecar.mjs") && !proc.ArgvContains(argv, "bugforge") {
		return Result{Role: types.RoleSidecar, Runtime: types.RuntimeForge, AgentRoot: true}
	}
	if comm == "charon" {
		return Result{Role: types.RoleMonitor, CollapseKey: "charon"}
	}

	if isHermesAgent(argv, exe, comm) {
		return Result{Role: types.RolePrimary, Runtime: types.RuntimeHermes, AgentRoot: true, ProvenNameHint: "Iris"}
	}

	if isClaudeFamily(comm, exe) {
		// The Remote Control daemon is plumbing, not a session.
		if isClaudeRC(argv) {
			return Result{Role: types.RoleMonitor, Runtime: types.RuntimeClaude, CollapseKey: "claude-rc", ProvenNameHint: "claude rc"}
		}
		// Every other claude process is its own session with its own sidecar.
		// Claude's subagents are in-process; a child claude PID (spawned by
		// the rc daemon, or `claude -p` from a session's shell) is a root row.
		r := Result{Role: types.RolePrimary, Runtime: types.RuntimeClaude, AgentRoot: true, ProvenNameHint: "claude"}
		if isClaudeFamily(parent.Comm, parent.Exe) && isClaudeRC(parent.Cmdline) {
			r.Tag = "rc"
		}
		return r
	}

	if comm == "grok" || strings.Contains(exe, "/.grok/downloads/grok-") || argv0IsGrok(argv) {
		if parent.Comm == "grok" {
			return Result{Role: types.RoleSubagent, Runtime: types.RuntimeGrok, CollapseKey: "grok-sub:" + itoa(parent.PID)}
		}
		return Result{Role: types.RolePrimary, Runtime: types.RuntimeGrok, AgentRoot: true, ProvenNameHint: "Grok"}
	}

	if comm == "codex" && proc.ArgvContains(argv, "exec") {
		return Result{Role: types.RolePrimary, Runtime: types.RuntimeCodex, CollapseKey: "codex-cli:" + itoa(p.PID), AgentRoot: true}
	}
	if comm == "codex" {
		return Result{Role: types.RolePrimary, Runtime: types.RuntimeCodex, CollapseKey: "codex-cli:" + itoa(p.PID), AgentRoot: true}
	}
	if comm == "codex-code-mode" || strings.HasSuffix(exe, "codex-code-mode-host") {
		return Result{Role: types.RoleSidecar, Runtime: types.RuntimeCodex}
	}
	if strings.HasPrefix(comm, "node") && proc.ArgvContains(argv, "/.local/bin/codex") {
		return Result{Role: types.RoleIgnore}
	}

	return Result{Role: types.RoleDrop}
}

// local builds the row for a local inference backend. The name is the
// systemd unit instance when there is one (hermes-qwen38.service -> qwen38),
// else the comm; the model is whatever the argv says it loaded.
func local(name, model, cgroup string) Result {
	if u := unitName(cgroup); u != "" {
		name = strings.TrimPrefix(u, "hermes-")
	}
	return Result{
		Role:           types.RoleSidecar,
		Runtime:        types.RuntimeLocal,
		CollapseKey:    "local:" + name,
		AgentRoot:      true,
		ProvenNameHint: name,
		ModelHint:      model,
	}
}

// isVLLM: `vllm serve ...` via the console script, or `python -m vllm.entrypoints...`.
func isVLLM(argv []string) bool {
	for i, a := range argv {
		if path.Base(a) == "vllm" && i+1 < len(argv) && argv[i+1] == "serve" {
			return true
		}
		if strings.Contains(a, "vllm.entrypoints") {
			return true
		}
	}
	return false
}

// unitName returns "foo" when the cgroup's final path component is
// foo.service. The user manager (user@1000.service) and session scopes sit
// higher up the path and are not a process's own unit.
func unitName(cgroup string) string {
	for _, line := range strings.Split(cgroup, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.LastIndexByte(line, '/'); i >= 0 {
			line = line[i+1:]
		}
		if !strings.HasSuffix(line, ".service") {
			continue
		}
		u := strings.TrimSuffix(line, ".service")
		if strings.HasPrefix(u, "user@") || u == "" {
			continue
		}
		return u
	}
	return ""
}

// modelArg returns the basename (no extension) of the model file named by
// -m / --model / --model-path, or "" when argv does not say.
func modelArg(argv []string) string {
	for i, a := range argv {
		if a == "serve" && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") && i > 0 && path.Base(argv[i-1]) == "vllm" {
			return modelBase(argv[i+1])
		}
		switch {
		case a == "-m" && i+1 < len(argv) && !strings.HasPrefix(path.Base(argv[0]), "python"):
			return modelBase(argv[i+1]) // llama-server -m; python -m is a module, not a model
		case (a == "--model" || a == "--model-path") && i+1 < len(argv):
			return modelBase(argv[i+1])
		case strings.HasPrefix(a, "--model="):
			return modelBase(strings.TrimPrefix(a, "--model="))
		}
	}
	return ""
}

func modelBase(p string) string {
	b := path.Base(p)
	for _, ext := range []string{".gguf", ".safetensors", ".bin"} {
		b = strings.TrimSuffix(b, ext)
	}
	return b
}

func hasChatGPTType(argv []string) bool {
	for _, k := range []string{"renderer", "gpu-process", "zygote", "utility"} {
		if proc.HasTypeFlag(argv, k) {
			return true
		}
	}
	return false
}

func parlorInstance(cgroup string) string {
	const needle = "parlor-sidecar@"
	i := strings.Index(cgroup, needle)
	if i < 0 {
		return "unknown"
	}
	rest := cgroup[i+len(needle):]
	rest = strings.TrimSuffix(rest, ".service")
	if n := strings.IndexAny(rest, " \n/"); n >= 0 {
		rest = rest[:n]
	}
	return rest
}

func isHermesAgent(argv []string, exe, comm string) bool {
	if proc.ArgvContains(argv, "/Projects/talaria/") {
		return false
	}
	if strings.Contains(exe, "hermes-agent/venv/bin/hermes") {
		return true
	}
	if comm == "hermes" {
		return true
	}
	if proc.ArgvContains(argv, "hermes_cli") || proc.ArgvContains(argv, "hermes chat") || proc.ArgvContains(argv, "hermes --tui") {
		return true
	}
	return false
}

// isClaudeFamily: comm "claude", or any exe under the versioned install
// (the rc daemon execs the version binary directly, comm "2.1.239").
func isClaudeFamily(comm, exe string) bool {
	return comm == "claude" || strings.Contains(exe, "/.local/share/claude/versions/")
}

func isClaudeRC(argv []string) bool {
	return len(argv) >= 2 && argv[1] == "rc"
}

func hasPrintFlag(argv []string) bool {
	for _, a := range argv {
		if a == "-p" || a == "--print" || strings.HasPrefix(a, "--output-format") {
			return true
		}
	}
	return false
}

func argv0IsGrok(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	return strings.HasSuffix(argv[0], "/grok") || argv[0] == "grok"
}

func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
