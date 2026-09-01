package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aitop/internal/graph"
	"aitop/internal/native"
	"aitop/internal/types"
)

// Fixture ids are real shapes off the 2026-08-31 survey: a claude session uuid,
// an agent id in the `a<name>-<hex16>` form, and a codex thread uuid7.
const (
	attachSession   = "84a06b9a-0873-4655-9eb9-d7cf99554b05"
	attachAgent     = "alane1r-architect-b8b06ac8c091add8"
	attachCodexOnce = "019f6b2a-0000-7000-8000-000000000021"
	attachGrokOnce  = "94a06b9a-0873-4655-9eb9-d7cf99554b06"
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
// withAgent is set, one subagent under it. Callers stamp the files WELL INSIDE
// the horizon but PAST the core's bind window, which is what keeps these tests
// about the wiring: a sighting that is both unbound and newborn is deliberately
// held back one poll so the engine can bind a process to it first, and a
// fixture stamped `now` would make every assertion here a race against the 2s
// poll rather than a statement about the attach.
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

func attachCodexOnceHome(t *testing.T, now time.Time) string {
	t.Helper()
	home := t.TempDir()
	day := now.In(time.Local)
	dir := filepath.Join(home, "sessions", day.Format("2006"), day.Format("01"), day.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("attach-codex-once-mkdir rule violated: dir=%s error=%v", dir, err)
	}
	record := map[string]any{
		"type": "session_meta",
		"payload": map[string]any{
			"id": attachCodexOnce, "session_id": attachCodexOnce, "thread_source": "user",
			"cwd": "/home/aegis/Projects/aitop", "source": "cli",
		},
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("attach-codex-once-encode rule violated: error=%v", err)
	}
	path := filepath.Join(dir, "rollout-2026-08-31T00-00-00-"+attachCodexOnce+".jsonl")
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("attach-codex-once-write rule violated: path=%s error=%v", path, err)
	}
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("attach-codex-once-chtimes rule violated: path=%s error=%v", path, err)
	}
	return home
}

func attachGrokOnceHome(t *testing.T, now time.Time) string {
	t.Helper()
	home := t.TempDir()
	cwd := "/home/aegis/Projects/aitop"
	bucket := strings.ReplaceAll(cwd, "/", "%2F")
	dir := filepath.Join(home, "sessions", bucket, attachGrokOnce)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("attach-grok-once-mkdir rule violated: dir=%s error=%v", dir, err)
	}
	record := map[string]any{
		"info":             map[string]any{"id": attachGrokOnce, "cwd": cwd},
		"current_model_id": "grok-4", "agent_name": "Grok", "generated_title": "attach once",
		"created_at":     now.Add(-time.Minute).Format(time.RFC3339Nano),
		"last_active_at": now.Format(time.RFC3339Nano), "session_kind": "user",
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("attach-grok-once-encode rule violated: error=%v", err)
	}
	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("attach-grok-once-write rule violated: path=%s error=%v", path, err)
	}
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("attach-grok-once-chtimes rule violated: path=%s error=%v", path, err)
	}
	return home
}

// attachFixtureClock is two minutes ago: comfortably inside the one hour
// horizon, and comfortably past the six second window in which the core holds a
// newborn back for its process binding.
func attachFixtureClock() time.Time { return time.Now().Add(-2 * time.Minute) }

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
	now := attachFixtureClock()
	homes := GraphHomes{
		Claude: attachClaudeHome(t, now, false),
		Codex:  t.TempDir(),
		Grok:   t.TempDir(),
	}
	eng := &Engine{ProcRoot: t.TempDir()}
	shadow, occ, _, err := AttachGraph(eng, homes)
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

func TestAttachGraphOncePublishesFreshUnboundNodeForEveryRuntime(t *testing.T) {
	now := time.Now()
	claudeID, err := graph.ClaudeSessionID(attachSession)
	if err != nil {
		t.Fatalf("attach-once-claude-id fixture rule violated: error=%v", err)
	}
	codexID, err := graph.CodexThreadID(attachCodexOnce)
	if err != nil {
		t.Fatalf("attach-once-codex-id fixture rule violated: error=%v", err)
	}
	grokID, err := graph.GrokSessionID(attachGrokOnce)
	if err != nil {
		t.Fatalf("attach-once-grok-id fixture rule violated: error=%v", err)
	}
	cases := []struct {
		name  string
		homes GraphHomes
		want  graph.NodeID
	}{
		{name: "claude", homes: GraphHomes{Claude: attachClaudeHome(t, now, false)}, want: claudeID},
		{name: "codex", homes: GraphHomes{Codex: attachCodexOnceHome(t, now)}, want: codexID},
		{name: "grok", homes: GraphHomes{Grok: attachGrokOnceHome(t, now)}, want: grokID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng := &Engine{ProcRoot: t.TempDir()}
			shadow, _, lanes, err := AttachGraphOnce(eng, tc.homes)
			if err != nil || shadow == nil || len(lanes) != 1 {
				t.Fatalf("attach-once runtime construction rule violated: runtime=%s error=%v shadowNil=%t lanes=%d", tc.name, err, shadow == nil, len(lanes))
			}
			runShadow(t, shadow)
			ready := lanes.WaitNativeEvidence(shadow, time.Second)
			snapshot := shadow.Snapshot()
			node, visible := nodeByGraphID(snapshot, tc.want)
			if ready != 1 || !visible || node.Incarnation == "" || node.Process != nil {
				t.Fatalf("attach-once fresh unbound runtime node rule violated: runtime=%s ready=%d wantReady=1 visible=%t node=%+v want=%s snapshot=%+v", tc.name, ready, visible, node, tc.want, snapshot)
			}
		})
	}
}

// TestAttachGraphEmptyHomesIsOccupancyOnly pins the wrapper's contract: with no
// homes, nothing native is registered and the shadow is what it was before this
// task. The claude fixture is present on disk and unreachable, so the test
// separates "no home was passed" from "no fixture existed".
func TestAttachGraphEmptyHomesIsOccupancyOnly(t *testing.T) {
	now := attachFixtureClock()
	// The engine knows where the claude home is, and the wrapper still must not
	// pass it. That is the plausible refactor this test exists to catch: the
	// engine already carries the homes, so delegating with them looks like
	// tidying up and silently gives every existing AttachOccupancyGraph caller
	// three collectors it never asked for.
	eng := &Engine{ProcRoot: t.TempDir(), ClaudeHome: attachClaudeHome(t, now, false)}
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
	now := attachFixtureClock()
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

	shadow, _, _, err := AttachGraph(eng, homes)
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

// TestAttachGraphResolvesRelativeHomes pins the rule Task 3 left to the caller.
// A native sighting's Location is the absolute path of the file the fact came
// from, and that Location joins the record digest in the event id: a relative
// home would make a node's identity a function of the process's working
// directory, so the same session observed from two directories would publish
// the same fact under two ids and neither would replay against the other.
//
// The empty case is the disable switch and must survive untouched, because
// filepath.Abs("") returns the working directory rather than the empty string,
// which would register a collector on the repo the operator happens to be in.
func TestAttachGraphResolvesRelativeHomes(t *testing.T) {
	got, err := absHome(filepath.Join(".claude", "sessions"))
	if err != nil {
		t.Fatalf("attach-home-resolves rule violated: err=%v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("attach-home-is-absolute rule violated: home=%q", got)
	}
	if !strings.HasSuffix(got, filepath.Join(".claude", "sessions")) {
		t.Fatalf("attach-home-keeps-what-it-was-given rule violated: home=%q", got)
	}

	already := t.TempDir()
	if got, err := absHome(already); err != nil || got != already {
		t.Fatalf("attach-home-leaves-an-absolute-home-alone rule violated: home=%q want=%q err=%v", got, already, err)
	}

	if got, err := absHome(""); err != nil || got != "" {
		t.Fatalf("attach-empty-home-stays-empty rule violated: home=%q err=%v", got, err)
	}
}

// slowLane is a native lane whose first tick takes a controlled time, which is
// what a real one is: a disk walk over a runtime's whole session store.
type slowLane struct {
	ready chan struct{}
	nodes []graph.NodeID
	// asked records that the wait reached this lane at all. It is the one
	// witness of the budget rule that does not depend on how the machine was
	// scheduled: a wait that has already spent its budget must never get here.
	asked atomic.Bool
}

func newSlowLane(delay time.Duration) *slowLane {
	lane := &slowLane{ready: make(chan struct{})}
	go func() {
		time.Sleep(delay)
		close(lane.ready)
	}()
	return lane
}

func (l *slowLane) FirstTick() <-chan struct{} {
	l.asked.Store(true)
	return l.ready
}

func (l *slowLane) consulted() bool { return l.asked.Load() }

func (l *slowLane) FirstTickNodes() []graph.NodeID { return l.nodes }

// TestWaitNativeEvidenceReturnsWhenEveryLaneIsIn covers the waiting half. The count
// is the point: a caller that only learned "the wait returned" cannot tell a
// quiet box from a lane still walking, which is the whole defect this closes.
func TestWaitNativeEvidenceReturnsWhenEveryLaneIsIn(t *testing.T) {
	lanes := NativeLanes{newSlowLane(10 * time.Millisecond), newSlowLane(30 * time.Millisecond), newSlowLane(0)}
	started := time.Now()
	if ready := lanes.WaitNativeEvidence(nil, 2*time.Second); ready != 3 {
		t.Fatalf("wait-native-evidence-counts-every-lane-that-came-in rule violated: ready=%d want=3", ready)
	}
	// Returned on the lanes, not on the budget: a wait that simply slept its
	// budget would also report three.
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("wait-native-evidence-returns-on-the-lanes-not-the-budget rule violated: elapsed=%s budget=2s", elapsed)
	}
}

func TestNativeFirstTickWitnessRejectsNodeOnlySnapshot(t *testing.T) {
	const parent = graph.NodeID("native-witness-parent")
	const child = graph.NodeID("native-witness-child")
	const parentIncarnation = graph.IncarnationID("native-witness-parent-incarnation")
	const childIncarnation = graph.IncarnationID("native-witness-child-incarnation")
	source := graph.SourceRef{ID: "native-witness-source", Runtime: types.RuntimeCodex, Incarnation: 1, Authority: graph.AuthorityNative}
	edgeKey := graph.RelationshipEdgeKey(graph.EdgeSpawn, parent, child, "native-witness-child")
	witness := native.FirstTickWitness{
		Nodes: []graph.NodeID{parent, child}, Edges: []graph.EdgeKey{edgeKey}, States: map[graph.NodeID]graph.State{child: graph.StateActive},
		Incarnations: map[graph.NodeID]graph.IncarnationID{parent: parentIncarnation, child: childIncarnation},
		StateSources: map[graph.NodeID]graph.SourceRef{child: source}, EdgeProvenance: map[graph.EdgeKey]graph.Provenance{edgeKey: graph.ProvenanceNative},
	}
	nodesOnly := &graph.Snapshot{Nodes: []graph.Node{{ID: parent, Incarnation: parentIncarnation}, {ID: child, Incarnation: childIncarnation}}, Edges: []graph.Edge{}, Gaps: []graph.Gap{}}
	if nativeFirstTickWitnessVisible(nodesOnly, witness) {
		t.Fatalf("native first-tick node-prefix is not whole-public-tick rule violated: snapshot=%+v witness=%+v", nodesOnly, witness)
	}
	stateOnly := graph.CloneSnapshot(nodesOnly)
	stateOnly.Nodes[1].State = graph.NodeState{Value: graph.StateActive}
	if nativeFirstTickWitnessVisible(stateOnly, witness) {
		t.Fatalf("native first-tick wrong-source state witness rule violated: snapshot=%+v witness=%+v", stateOnly, witness)
	}
	stateOnly.Nodes[1].State.Source = source
	if nativeFirstTickWitnessVisible(stateOnly, witness) {
		t.Fatalf("native first-tick missing-edge witness rule violated: snapshot=%+v witness=%+v", stateOnly, witness)
	}
	wrongEdge := graph.CloneSnapshot(stateOnly)
	wrongEdge.Edges = []graph.Edge{{Key: edgeKey, Source: parent, Target: child, Type: graph.EdgeSpawn, Provenance: graph.ProvenanceAITopSidecar, Lifecycle: graph.LifecycleActive}}
	if nativeFirstTickWitnessVisible(wrongEdge, witness) {
		t.Fatalf("native first-tick wrong-provenance edge witness rule violated: snapshot=%+v witness=%+v", wrongEdge, witness)
	}
	complete := graph.CloneSnapshot(wrongEdge)
	complete.Edges[0].Provenance = graph.ProvenanceNative
	wrongIncarnation := graph.CloneSnapshot(complete)
	wrongIncarnation.Nodes[1].Incarnation = "native-witness-wrong-incarnation"
	if nativeFirstTickWitnessVisible(wrongIncarnation, witness) {
		t.Fatalf("native first-tick wrong-incarnation node witness rule violated: snapshot=%+v witness=%+v", wrongIncarnation, witness)
	}
	if !nativeFirstTickWitnessVisible(complete, witness) {
		t.Fatalf("native first-tick whole-public-tick visibility rule violated: snapshot=%+v witness=%+v", complete, witness)
	}
}

type immediateNativeLane struct {
	ready chan struct{}
	nodes []graph.NodeID
}

func (l *immediateNativeLane) FirstTick() <-chan struct{} { return l.ready }
func (l *immediateNativeLane) FirstTickNodes() []graph.NodeID {
	return append([]graph.NodeID{}, l.nodes...)
}

type flushWitnessGraph struct {
	snapshot *graph.Snapshot
	flushes  atomic.Int32
	flushErr error
	onFlush  func()
}

func (g *flushWitnessGraph) Snapshot() *graph.Snapshot { return g.snapshot }
func (g *flushWitnessGraph) Flush(context.Context) error {
	g.flushes.Add(1)
	if g.onFlush != nil {
		g.onFlush()
	}
	return g.flushErr
}

func TestWaitNativeEvidenceFlushesReadyPrefixAfterLaterLaneTimeout(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	stalled := make(chan struct{})
	const id = graph.NodeID("ready-before-stalled-native-node")
	first := &immediateNativeLane{ready: ready, nodes: []graph.NodeID{id}}
	second := &immediateNativeLane{ready: stalled}
	target := &flushWitnessGraph{snapshot: &graph.Snapshot{Edges: []graph.Edge{}, Gaps: []graph.Gap{}}}
	target.onFlush = func() {
		target.snapshot = &graph.Snapshot{Nodes: []graph.Node{{ID: id}}, Edges: []graph.Edge{}, Gaps: []graph.Gap{}}
	}
	readyCount := (NativeLanes{first, second}).waitNativeEvidence(target, 25*time.Millisecond)
	if readyCount != 1 || target.flushes.Load() != 1 || target.snapshot == nil || len(target.snapshot.Nodes) != 1 || target.snapshot.Nodes[0].ID != id {
		t.Fatalf("native evidence later-lane timeout preserves ready prefix rule violated: ready=%d want=1 flushes=%d want=1 snapshot=%+v", readyCount, target.flushes.Load(), target.snapshot)
	}
}

func TestWaitNativeEvidenceDoesNotTrustAggregateAfterFlushError(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	const id = graph.NodeID("aggregate-with-rejected-native-prefix")
	lane := &immediateNativeLane{ready: ready, nodes: []graph.NodeID{id}}
	target := &flushWitnessGraph{
		snapshot: &graph.Snapshot{Nodes: []graph.Node{{ID: id}}, Edges: []graph.Edge{}, Gaps: []graph.Gap{}},
		flushErr: &graph.AdmissionError{Kind: graph.AdmissionCountLimit},
	}
	started := time.Now()
	readyCount := (NativeLanes{lane}).waitNativeEvidence(target, 40*time.Millisecond)
	elapsed := time.Since(started)
	if readyCount != 1 || target.flushes.Load() != 1 || elapsed < 25*time.Millisecond {
		t.Fatalf("native evidence aggregate cannot satisfy rejected prefix rule violated: ready=%d want=1 flushes=%d want=1 elapsed=%s floor=25ms snapshot=%+v", readyCount, target.flushes.Load(), elapsed, target.snapshot)
	}
}

func TestWaitNativeEvidenceFlushesBeforeAggregateMatch(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	const id = graph.NodeID("occupancy-equal-native-node")
	lane := &immediateNativeLane{ready: ready, nodes: []graph.NodeID{id}}
	target := &flushWitnessGraph{snapshot: &graph.Snapshot{Nodes: []graph.Node{{ID: id}}, Edges: []graph.Edge{}, Gaps: []graph.Gap{}}}
	readyCount := (NativeLanes{lane}).waitNativeEvidence(target, time.Second)
	if readyCount != 1 || target.flushes.Load() != 1 {
		t.Fatalf("native evidence flush precedes aggregate witness match rule violated: ready=%d want=1 flushes=%d want=1 snapshot=%+v", readyCount, target.flushes.Load(), target.snapshot)
	}
}

// TestWaitNativeEvidenceIsBoundedByItsBudget is the other direction. A pathological
// store must cost a late graph, never a hung command.
func TestWaitNativeEvidenceIsBoundedByItsBudget(t *testing.T) {
	lanes := NativeLanes{newSlowLane(0), newSlowLane(time.Hour)}
	started := time.Now()
	ready := lanes.WaitNativeEvidence(nil, 150*time.Millisecond)
	elapsed := time.Since(started)
	if ready != 1 {
		t.Fatalf("wait-native-evidence-reports-only-the-lanes-that-came-in rule violated: ready=%d want=1", ready)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("wait-native-evidence-is-bounded rule violated: elapsed=%s budget=150ms", elapsed)
	}
}

// TestWaitNativeEvidenceSharesOneBudgetAcrossLanes pins the budget to the CALL
// and not to each lane. A budget per lane would let three slow homes cost a
// one-shot three times the bound its caller asked for, which is the shape of a
// "bounded" wait that is not bounded by anything the caller can name.
//
// The lanes have to come in one after another for this to mean anything. An
// earlier version of this test used three lanes that all hung, and it could not
// discriminate the rule it was named for: the wait gives up on the FIRST lane
// whose bound expires, so a shared budget and a per-lane budget both returned
// at one budget with nothing to tell them apart. That version passed against a
// deliberately per-lane implementation. Here the first lane spends most of the
// budget and the second cannot fit in what is left.
//
// Timings, and the slack in each direction, because a reader will want to check
// them rather than trust them: the budget is 400ms, the first lane comes in at
// 250ms (150ms of room) and the second at 500ms, which is 100ms PAST the shared
// budget. Every margin is one-directional -- a loaded machine only makes a lane
// later, never earlier -- so load can only push this toward reporting fewer
// lanes, never toward reporting the per-lane behaviour it exists to catch.
func TestWaitNativeEvidenceSharesOneBudgetAcrossLanes(t *testing.T) {
	const budget = 400 * time.Millisecond
	first := newSlowLane(250 * time.Millisecond)
	second := newSlowLane(500 * time.Millisecond)
	third := newSlowLane(time.Hour)
	lanes := NativeLanes{first, second, third}

	started := time.Now()
	ready := lanes.WaitNativeEvidence(nil, budget)
	elapsed := time.Since(started)

	// Asserted first because it is the only witness here that is a fact rather
	// than a measurement: reaching the third lane at all means the budget was
	// not shared, however the machine was scheduled.
	if third.consulted() {
		t.Fatalf("wait-native-evidence-shares-one-budget-across-lanes rule violated: the third lane was consulted after %s on a %s budget", elapsed, budget)
	}
	if ready > 1 {
		t.Fatalf("wait-native-evidence-shares-one-budget-across-lanes rule violated: ready=%d on a %s budget whose first two lanes need 250ms and 500ms", ready, budget)
	}
	if elapsed > 2*budget {
		t.Fatalf("wait-native-evidence-is-bounded rule violated: elapsed=%s budget=%s", elapsed, budget)
	}
	// The first lane must actually have been reached, or the two assertions
	// above are satisfied by a wait that did nothing at all.
	if !first.consulted() {
		t.Fatalf("wait-native-evidence-consults-its-first-lane rule violated: elapsed=%s ready=%d", elapsed, ready)
	}
}

// TestWaitNativeEvidenceWithNoLanesIsFree keeps the occupancy-only path free of a
// wait it has no reason to take.
func TestWaitNativeEvidenceWithNoLanesIsFree(t *testing.T) {
	started := time.Now()
	if ready := (NativeLanes(nil)).WaitNativeEvidence(nil, time.Hour); ready != 0 {
		t.Fatalf("wait-native-evidence-with-no-lanes-is-free rule violated: ready=%d", ready)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("wait-native-evidence-with-no-lanes-is-free rule violated: elapsed=%s on an empty lane set", elapsed)
	}
}

// TestAttachGraphReturnsOneLanePerHome pins what the one-shot ends up waiting
// on. A home that registers a collector but no lane would be waited on by
// nobody, which reads exactly like a fast lane.
func TestAttachGraphReturnsOneLanePerHome(t *testing.T) {
	now := attachFixtureClock()
	eng := &Engine{ProcRoot: t.TempDir()}
	for _, tc := range []struct {
		name  string
		homes GraphHomes
		want  int
	}{
		{"none", GraphHomes{}, 0},
		{"claude", GraphHomes{Claude: attachClaudeHome(t, now, false)}, 1},
		{"claude+codex", GraphHomes{Claude: attachClaudeHome(t, now, false), Codex: t.TempDir()}, 2},
		{"all three", GraphHomes{Claude: attachClaudeHome(t, now, false), Codex: t.TempDir(), Grok: t.TempDir()}, 3},
	} {
		shadow, _, lanes, err := AttachGraph(eng, tc.homes)
		if err != nil {
			t.Fatalf("attach-graph-constructs rule violated: case=%s err=%v", tc.name, err)
		}
		if len(lanes) != tc.want {
			t.Fatalf("attach-graph-returns-one-lane-per-home rule violated: case=%s lanes=%d want=%d", tc.name, len(lanes), tc.want)
		}
		for i, lane := range lanes {
			if lane == nil {
				t.Fatalf("attach-graph-lanes-are-usable rule violated: case=%s lane %d is nil", tc.name, i)
			}
			select {
			case <-lane.FirstTick():
				t.Fatalf("attach-graph-lanes-start-unready rule violated: case=%s lane %d signalled before the shadow ran", tc.name, i)
			default:
			}
		}
		_ = shadow
	}
}
