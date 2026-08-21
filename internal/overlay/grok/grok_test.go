package grok

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectJoinsActiveSessionAndCountsRunningSubs(t *testing.T) {
	root := t.TempDir()
	sess := filepath.Join(root, "sessions", "%2Fhome%2Faegis", "01abc")
	if err := os.MkdirAll(filepath.Join(sess, "subagents", "child1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sess, "subagents", "child2"), 0755); err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(root, "active_sessions.json"), `[{"session_id":"01abc","pid":2599958,"cwd":"/home/aegis","opened_at":"2026-08-21T00:00:00Z"}]`)
	must(t, filepath.Join(sess, "summary.json"), `{
		"info":{"id":"01abc","cwd":"/home/aegis/Projects/aitop"},
		"generated_title":"aitop: house-wide agent occupancy TUI",
		"current_model_id":"grok-4.6",
		"agent_name":"grok-build-plan",
		"last_active_at":"2026-08-21T07:52:05Z"
	}`)
	must(t, filepath.Join(sess, "signals.json"), `{"contextTokensUsed":99033,"contextWindowTokens":500000,"contextWindowUsage":19,"primaryModelId":"grok-4.6"}`)
	must(t, filepath.Join(sess, "subagents", "child1", "meta.json"), `{"subagent_id":"child1","parent_session_id":"01abc","status":"running","description":"detector table"}`)
	must(t, filepath.Join(sess, "subagents", "child2", "meta.json"), `{"subagent_id":"child2","parent_session_id":"01abc","status":"completed","description":"done","subagent_type":"explore","started_at":"2026-08-21T07:48:27.350731148Z","completed_at":"2026-08-21T07:48:37.357064547Z"}`)

	ovs, err := Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	var parent, child int
	for _, o := range ovs {
		if o.SessionID == "01abc" && o.PID == 2599958 {
			parent++
			if o.CostUSD != nil {
				t.Fatalf("never-invent-cost violated: CostUSD=%v", o.CostUSD)
			}
			if o.ProvenName != "Grok" {
				t.Fatalf("grok-build-seat-is-Grok violated: ProvenName=%q", o.ProvenName)
			}
			if o.Project != "aitop" {
				t.Fatalf("project-from-overlay-cwd violated: %q", o.Project)
			}
			if o.TokensUsed == nil || *o.TokensUsed != 99033 {
				t.Fatalf("grok-tokens-from-signals violated: %v", o.TokensUsed)
			}
			if o.SubagentLive != 1 || o.SubagentDeclared != 2 {
				t.Fatalf("subagent-live-vs-declared violated: live=%d declared=%d", o.SubagentLive, o.SubagentDeclared)
			}
			if o.Model != "grok-4.6" {
				t.Fatalf("model-from-summary violated: %q", o.Model)
			}
		}
		if o.ParentSession == "01abc" && o.SubagentStatus == "running" {
			child++
			if o.PID != 0 {
				t.Fatalf("in-process-subagent-has-no-pid violated: pid=%d", o.PID)
			}
		}
	}
	if parent != 1 || child != 1 {
		t.Fatalf("grok-parent-and-running-child violated: parent=%d running-child=%d n=%d", parent, child, len(ovs))
	}
	var done int
	for _, o := range ovs {
		if o.ParentSession == "01abc" && o.SubagentStatus == "completed" {
			done++
			if o.SubagentType != "explore" {
				t.Fatalf("finished-subagent-keeps-type violated: %q", o.SubagentType)
			}
			if o.StartedAt.IsZero() || o.CompletedAt.IsZero() || !o.CompletedAt.After(o.StartedAt) {
				t.Fatalf("finished-subagent-has-timestamps violated: start=%v done=%v", o.StartedAt, o.CompletedAt)
			}
		}
	}
	if done != 1 {
		t.Fatalf("finished-subagents-are-emitted-for-history violated: completed=%d", done)
	}
}

func TestGeneralPurposeIsNotProvenGrok(t *testing.T) {
	root := t.TempDir()
	sess := filepath.Join(root, "sessions", "%2Fx", "s2")
	if err := os.MkdirAll(sess, 0755); err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(root, "active_sessions.json"), `[{"session_id":"s2","pid":1,"cwd":"/x"}]`)
	must(t, filepath.Join(sess, "summary.json"), `{"info":{"id":"s2","cwd":"/x"},"agent_name":"general-purpose","session_kind":"subagent"}`)
	ovs, err := Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range ovs {
		if o.ProvenName == "Grok" {
			t.Fatalf("general-purpose-is-not-proven-Grok violated: %+v", o)
		}
	}
}

func must(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
