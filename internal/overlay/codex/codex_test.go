package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRolloutHeadAndTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-08-20T20-04-31-01abc.jsonl")
	nl := "\n"
	var body string
	body += `{"type":"session_meta","payload":{"session_id":"01abc","cwd":"/home/aegis/Projects/mira","originator":"codex-tui","model_provider":"openai"}}` + nl
	for i := 0; i < 50; i++ {
		body += `{"type":"event_msg","payload":{"type":"noise"}}` + nl
	}
	body += `{"type":"turn_context","payload":{"model":"gpt-5.6-sol","cwd":"/home/aegis/Projects/mira"}}` + nl
	body += `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":76970},"model_context_window":828400}}}` + nl
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ov, err := ParseRollout(path)
	if err != nil {
		t.Fatal(err)
	}
	if ov.SessionID != "01abc" {
		t.Fatalf("codex-session-id-from-meta violated: %q", ov.SessionID)
	}
	if ov.Project != "mira" {
		t.Fatalf("codex-project-from-cwd violated: %q", ov.Project)
	}
	if ov.Model != "gpt-5.6-sol" || ov.ProvenName != "Sol" {
		t.Fatalf("codex-sol-from-exact-model violated: model=%q name=%q", ov.Model, ov.ProvenName)
	}
	if ov.TokensUsed == nil || *ov.TokensUsed != 76970 {
		t.Fatalf("codex-tokens-from-tail violated: %v", ov.TokensUsed)
	}
	if ov.ContextWindow == nil || *ov.ContextWindow != 828400 {
		t.Fatalf("codex-window-from-event violated: %v", ov.ContextWindow)
	}
	if ov.CostUSD != nil {
		t.Fatalf("never-invent-cost violated")
	}
}

func TestDaybreakIsNotSol(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"session_meta","payload":{"session_id":"x","cwd":"/home/aegis"}}` + "\n"
	body += `{"type":"turn_context","payload":{"model":"gpt-daybreak-blue-latest"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ov, err := ParseRollout(path)
	if err != nil {
		t.Fatal(err)
	}
	if ov.ProvenName == "Sol" {
		t.Fatalf("daybreak-is-not-sol violated: ProvenName=%q", ov.ProvenName)
	}
}

func TestCollectJoinsFDToRollout(t *testing.T) {
	home := t.TempDir()
	sess := filepath.Join(home, "sessions", "2026", "08", "20")
	if err := os.MkdirAll(sess, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sess, "rollout-01dead.jsonl")
	body := `{"type":"session_meta","payload":{"session_id":"01dead","cwd":"/home/aegis/Projects/aitop"}}` + "\n"
	body += `{"type":"turn_context","payload":{"model":"gpt-5.6-luna"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	fds := map[int32][]string{
		4128056: {path, filepath.Join(home, "thread-writer-locks", "01dead.lock")},
	}
	ovs, err := Collect(home, fds)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 || ovs[0].PID != 4128056 {
		t.Fatalf("codex-fd-join-to-pid violated: %+v", ovs)
	}
	if ovs[0].ProvenName != "Luna" || ovs[0].Project != "aitop" {
		t.Fatalf("codex-luna-project violated: name=%q proj=%q", ovs[0].ProvenName, ovs[0].Project)
	}
}

func TestParseLsofFDsKeepsOnlyCodexSessionFiles(t *testing.T) {
	raw := []byte("p101\nf3\nn/Users/kai/.codex/sessions/2026/08/rollout-abc.jsonl\nf4\nn/Users/kai/.codex/thread-writer-locks/abc.lock\nf5\nn/Users/kai/.codex/config.toml\np102\nn/Users/kai/.codex/sessions/rollout-def.jsonl (deleted)\nn/Users/kai/.codex/sessions/rollout-def.jsonl (deleted)\n")
	got := parseLsofFDs(raw)
	if len(got) != 2 || len(got[101]) != 2 || len(got[102]) != 1 {
		t.Fatalf("lsof-session-fd-filtering violated: %#v", got)
	}
	if got[102][0] != "/Users/kai/.codex/sessions/rollout-def.jsonl" {
		t.Fatalf("lsof-deleted-suffix-normalization violated: %#v", got[102])
	}
}
