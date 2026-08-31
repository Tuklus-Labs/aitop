package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aitop/internal/graph"
	"aitop/internal/types"
)

// Fixture ids are real shapes off the 2026-08-31 survey: a claude session uuid,
// an agent id in the `a<name>-<hex16>` form, and a codex thread uuid7.
const (
	attachSession   = "84a06b9a-0873-4655-9eb9-d7cf99554b05"
	attachAgent     = "alane1r-architect-b8b06ac8c091add8"
	attachPID       = 4242
	attachStartTick = 7241979

	// nativeFirstTickWindow is how long an occupancy-only shadow is watched for
	// a native node that must never arrive. A native collector ticks once as
	// soon as it runs, so this only has to outlast scheduling, not the 2s poll
	// interval; it is generous rather than tight because the assertion it
	// guards is an absence and a short window would make it vacuous.
	nativeFirstTickWindow = 750 * time.Millisecond
)

// attachHomes writes a ~/.claude-shaped home holding one live session and, when
// withAgent is set, one subagent under it. Every file is stamped now so the
// horizon is not what this test is measuring.
func attachClaudeHome(t *testing.T, now time.Time, withAgent bool) string {
	t.Helper()
	home := t.TempDir()
	write := func(path string, fields map[string]any) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("attach-fixture-mkdir rule violated: dir=%s err=%v", filepath.Dir(path), err)
		}
		body, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("attach-fixture-encode rule violated: err=%v", err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatalf("attach-fixture-write rule violated: path=%s err=%v", path, err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatalf("attach-fixture-chtimes rule violated: path=%s err=%v", path, err)
		}
	}
	write(filepath.Join(home, "sessions", fmt.Sprintf("%d.json", attachPID)), map[string]any{
		"pid":        attachPID,
		"sessionId":  attachSession,
		"cwd":        "/home/aegis/Projects/aitop",
		"startedAt":  int64(1788136922001),
		"procStart":  fmt.Sprint(attachStartTick),
		"name":       "aegis-48",
		"status":     "idle",
		"kind":       "interactive",
		"entrypoint": "cli",
	})
	if !withAgent {
		return home
	}
	store := filepath.Join(home, "projects", "-home-aegis", attachSession, "subagents")
	write(filepath.Join(store, "agent-"+attachAgent+".meta.json"), map[string]any{
		"agentType":   "lane1r-architect",
		"description": "wire the collectors",
		"name":        "Lane 1R",
		"model":       "opus",
		"spawnDepth":  1,
	})
	// The sibling transcript is the child's activity signal; its sessionId is
	// the PARENT's, exactly as on disk.
	line, err := json.Marshal(map[string]any{"type": "user", "sessionId": attachSession, "cwd": "/home/aegis/Projects/aitop"})
	if err != nil {
		t.Fatalf("attach-fixture-encode rule violated: err=%v", err)
	}
	path := filepath.Join(store, "agent-"+attachAgent+".jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatalf("attach-fixture-write rule violated: path=%s err=%v", path, err)
	}
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("attach-fixture-chtimes rule violated: path=%s err=%v", path, err)
	}
	return home
}

// attachProcRoot writes a /proc-shaped tree holding one claude process. Only
// stat is required: proc.readOne reads it first and everything else is
// optional, and the comm inside it is what classify keys on.
func attachProcRoot(t *testing.T, pid int, start uint64) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, fmt.Sprint(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("attach-proc-mkdir rule violated: dir=%s err=%v", dir, err)
	}
	// After the comm: state ppid pgrp session tty tpgid flags minflt cminflt
	// majflt cmajflt utime stime cutime cstime prio nice threads itrealvalue
	// starttime vsize rss. starttime is field 20 of that list and rss is 22.
	stat := fmt.Sprintf("%d (claude) S 1 %d %d 0 -1 4194304 100 0 0 0 5 3 0 0 20 0 12 0 %d 1000000 5000\n",
		pid, pid, pid, start)
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatalf("attach-proc-write rule violated: err=%v", err)
	}
	return root
}

// waitForNodes polls the shadow until pred is satisfied or the deadline passes.
// It returns the last snapshot either way, so a failure can name what WAS there.
func waitForNodes(shadow *graph.Shadow, timeout time.Duration, pred func(*graph.Snapshot) bool) *graph.Snapshot {
	deadline := time.Now().Add(timeout)
	var last *graph.Snapshot
	for time.Now().Before(deadline) {
		last = shadow.Snapshot()
		if last != nil && pred(last) {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	return last
}

func nodeIDs(snap *graph.Snapshot) []graph.NodeID {
	if snap == nil {
		return nil
	}
	out := make([]graph.NodeID, 0, len(snap.Nodes))
	for _, node := range snap.Nodes {
		out = append(out, node.ID)
	}
	return out
}

func hasPrefix(snap *graph.Snapshot, prefix string) bool {
	for _, id := range nodeIDs(snap) {
		if strings.HasPrefix(string(id), prefix) {
			return true
		}
	}
	return false
}

// runShadow starts a shadow and stops it when the test ends.
func runShadow(t *testing.T, shadow *graph.Shadow) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = shadow.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("shadow-run-stops-on-cancel rule violated: still running 2s after cancel")
		}
	})
}

// TestAttachGraphRegistersNativeCollectors is the wiring assertion: a home
// handed to AttachGraph becomes a source in the shadow. Occupancy alone cannot
// produce a claude session node here, because the engine has no rows at all.
func TestAttachGraphRegistersNativeCollectors(t *testing.T) {
	now := time.Now()
	homes := GraphHomes{
		Claude: attachClaudeHome(t, now, false),
		Codex:  t.TempDir(),
		Grok:   t.TempDir(),
	}
	eng := &Engine{ProcRoot: t.TempDir()}
	shadow, occ, err := AttachGraph(eng, homes)
	if err != nil {
		t.Fatalf("attach-graph-constructs rule violated: err=%v", err)
	}
	if occ == nil || shadow == nil {
		t.Fatalf("attach-graph-returns-both rule violated: shadow=%v occ=%v", shadow, occ)
	}
	if eng.GraphSnapshot == nil {
		t.Fatalf("attach-graph-points-the-engine-at-the-shadow rule violated: GraphSnapshot=nil")
	}
	runShadow(t, shadow)

	snap := waitForNodes(shadow, 2*time.Second, func(s *graph.Snapshot) bool {
		return hasPrefix(s, "claude:session:")
	})
	if !hasPrefix(snap, "claude:session:") {
		t.Fatalf("attach-graph-registers-native-collectors rule violated: no claude:session: node within 2s, nodes=%v", nodeIDs(snap))
	}
}

// TestAttachGraphEmptyHomesIsOccupancyOnly pins the wrapper's contract: with no
// homes, nothing native is registered and the shadow is what it was before this
// task. The claude fixture is present on disk and unreachable, so the test
// separates "no home was passed" from "no fixture existed".
func TestAttachGraphEmptyHomesIsOccupancyOnly(t *testing.T) {
	now := time.Now()
	attachClaudeHome(t, now, false) // written, never handed over
	eng := &Engine{ProcRoot: t.TempDir()}
	shadow, occ, err := AttachOccupancyGraph(eng)
	if err != nil {
		t.Fatalf("attach-occupancy-graph-constructs rule violated: err=%v", err)
	}
	if occ == nil || eng.GraphSnapshot == nil {
		t.Fatalf("attach-occupancy-graph-returns-a-collector rule violated: occ=%v graphSnapshot=%v", occ, eng.GraphSnapshot == nil)
	}
	runShadow(t, shadow)

	// Give the native poll interval a full tick's worth of room to be wrong in.
	deadline := time.Now().Add(nativeFirstTickWindow)
	for time.Now().Before(deadline) {
		if snap := shadow.Snapshot(); hasPrefix(snap, "claude:session:") {
			t.Fatalf("attach-graph-empty-homes-is-occupancy-only rule violated: nodes=%v", nodeIDs(snap))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAttachGraphBindsNativeNodesToLiveProcesses is the end-to-end proof that
// the rows closure reaches the native collectors. Every native scanner test
// runs with latest=nil, so nothing before this asserts that production hands
// the collectors any rows at all.
//
// The spawn edge is the instrument, not the node. Edge admission requires both
// endpoints to have been published with EXACTLY the incarnations the edge
// names, and occupancy publishes the parent session at its PROCESS incarnation.
// A native collector reading no rows would name that parent by its INVOCATION
// incarnation instead, so the edge is the one assertion that cannot be
// satisfied by occupancy alone and cannot be satisfied by a native collector
// that never saw the process binding.
func TestAttachGraphBindsNativeNodesToLiveProcesses(t *testing.T) {
	now := time.Now()
	homes := GraphHomes{Claude: attachClaudeHome(t, now, true)}
	eng := &Engine{
		ProcRoot: attachProcRoot(t, attachPID, attachStartTick),
		Overlay: func() ([]types.Overlay, error) {
			return []types.Overlay{{
				SessionID: attachSession,
				PID:       attachPID,
				StartTime: attachStartTick,
				Runtime:   types.RuntimeClaude,
			}}, nil
		},
	}
	eng.RefreshOverlay()

	rows := eng.Rows()
	if len(rows) != 1 || rows[0].Overlay.SessionID != attachSession || rows[0].Process.StartTime != attachStartTick {
		t.Fatalf("attach-fixture-produces-a-bound-row rule violated: rows=%+v", rows)
	}

	shadow, _, err := AttachGraph(eng, homes)
	if err != nil {
		t.Fatalf("attach-graph-constructs rule violated: err=%v", err)
	}
	runShadow(t, shadow)

	sessionID, err := graph.ClaudeSessionID(attachSession)
	if err != nil {
		t.Fatalf("attach-fixture-session-id rule violated: err=%v", err)
	}
	agentID, err := graph.ClaudeAgentID(attachSession, attachAgent)
	if err != nil {
		t.Fatalf("attach-fixture-agent-id rule violated: err=%v", err)
	}
	wantIncarnation, err := graph.ProcessIncarnation(types.RuntimeClaude, graph.ProcessIdentity{PID: attachPID, StartTicks: attachStartTick})
	if err != nil {
		t.Fatalf("attach-fixture-incarnation rule violated: err=%v", err)
	}

	snap := waitForNodes(shadow, 3*time.Second, func(s *graph.Snapshot) bool {
		return spawnEdge(s, sessionID, agentID) != nil
	})

	// The agent node is native-only evidence: occupancy names sessions and
	// processes and can never produce a claude:agent: id.
	agent, ok := nodeByGraphID(snap, agentID)
	if !ok {
		t.Fatalf("attach-graph-publishes-native-subagents rule violated: %s absent, nodes=%v", agentID, nodeIDs(snap))
	}
	if agent.Role != types.RoleSubagent || agent.TaskName != "wire the collectors" {
		t.Fatalf("attach-graph-native-subagent-content rule violated: role=%q task=%q", agent.Role, agent.TaskName)
	}

	session, ok := nodeByGraphID(snap, sessionID)
	if !ok {
		t.Fatalf("attach-graph-publishes-the-session rule violated: %s absent, nodes=%v", sessionID, nodeIDs(snap))
	}
	if session.Incarnation != wantIncarnation {
		t.Fatalf("attach-graph-binds-sessions-to-live-processes rule violated: incarnation=%q want=%q", session.Incarnation, wantIncarnation)
	}
	if session.Process == nil || session.Process.PID != attachPID || session.Process.StartTicks != attachStartTick {
		t.Fatalf("attach-graph-session-carries-its-process rule violated: process=%+v", session.Process)
	}

	edge := spawnEdge(snap, sessionID, agentID)
	if edge == nil {
		t.Fatalf("attach-graph-native-spawn-edge-survives-admission rule violated: no spawn edge %s -> %s, edges=%d nodes=%v", sessionID, agentID, len(snap.Edges), nodeIDs(snap))
	}
	if edge.Provenance != graph.ProvenanceNative {
		t.Fatalf("attach-graph-spawn-edge-is-native rule violated: provenance=%q", edge.Provenance)
	}
	if string(edge.Relationship) != attachAgent {
		t.Fatalf("attach-graph-spawn-relationship-is-the-child-short-id rule violated: relationship=%q want=%q", edge.Relationship, attachAgent)
	}
}

func nodeByGraphID(snap *graph.Snapshot, id graph.NodeID) (graph.Node, bool) {
	if snap == nil {
		return graph.Node{}, false
	}
	for _, node := range snap.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return graph.Node{}, false
}

func spawnEdge(snap *graph.Snapshot, parent, child graph.NodeID) *graph.Edge {
	if snap == nil {
		return nil
	}
	for i := range snap.Edges {
		edge := &snap.Edges[i]
		if edge.Type == graph.EdgeSpawn && edge.Source == parent && edge.Target == child {
			return edge
		}
	}
	return nil
}

// TestSchema2WritesUnclaimedStateAsUnknown covers a node the schema-2 writer
// could not previously meet. The reconciler sets no default state, so a node
// whose only evidence is a NodeObserved event carries the ZERO NodeState, and
// an empty string is not one of the thirteen values the schema admits.
//
// That was unreachable while occupancy was the only source, because occupancy
// claims a state for every node it publishes. The native collectors publish
// primaries with no state claim on purpose -- occupancy owns the process truth
// and a native claim would fight it every tick -- so attaching them makes an
// unclaimed node the ordinary case, and `aitop --json` failed outright on it.
//
// "unknown" is the graph's own word for a state nobody has determined, and it
// is what the passive normalizer already returns in the same situation, so the
// unclaimed node is spelled in the vocabulary the schema already has rather
// than the schema growing an empty member.
func TestSchema2WritesUnclaimedStateAsUnknown(t *testing.T) {
	unclaimed := graph.Node{
		ID:          "claude:session:" + attachSession,
		Incarnation: "claude:invocation:" + attachSession,
		Runtime:     types.RuntimeClaude,
		Role:        types.RolePrimary,
		TelemetryAt: time.Now().UTC().Truncate(time.Millisecond),
		Transitions: []graph.Transition{},
	}
	if unclaimed.State.Value != "" {
		t.Fatalf("unclaimed-node-fixture-has-no-state rule violated: state=%q", unclaimed.State.Value)
	}
	snap := &Snapshot{
		At: time.Now().UTC().Truncate(time.Millisecond),
		Graph: &graph.Snapshot{
			At:    time.Now().UTC().Truncate(time.Millisecond),
			Nodes: []graph.Node{unclaimed},
			Edges: []graph.Edge{},
			Gaps:  []graph.Gap{},
		},
	}

	var out strings.Builder
	if err := WriteJSON(snap, &out, snap.At); err != nil {
		t.Fatalf("schema-2-writes-an-unclaimed-state rule violated: err=%v", err)
	}

	var decoded struct {
		Graph struct {
			Nodes []struct {
				State struct {
					Value  string          `json:"value"`
					Source json.RawMessage `json:"source"`
					Since  string          `json:"since"`
				} `json:"state"`
			} `json:"nodes"`
		} `json:"graph"`
	}
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("schema-2-is-json rule violated: err=%v body=%s", err, out.String())
	}
	if len(decoded.Graph.Nodes) != 1 {
		t.Fatalf("schema-2-writes-the-node rule violated: nodes=%d body=%s", len(decoded.Graph.Nodes), out.String())
	}
	state := decoded.Graph.Nodes[0].State
	if state.Value != string(graph.StateUnknown) {
		t.Fatalf("schema-2-unclaimed-state-is-unknown rule violated: value=%q want=%q", state.Value, graph.StateUnknown)
	}
	// The value is filled in; the EVIDENCE is not invented. A source and a since
	// are a matched pair in this schema, and claiming either would assert that
	// somebody determined this state when nobody did.
	if len(state.Source) != 0 && string(state.Source) != "null" {
		t.Fatalf("schema-2-unclaimed-state-invents-no-source rule violated: source=%s", state.Source)
	}
	if state.Since != "" {
		t.Fatalf("schema-2-unclaimed-state-invents-no-since rule violated: since=%q", state.Since)
	}
}
