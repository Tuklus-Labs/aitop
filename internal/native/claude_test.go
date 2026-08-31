package native

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

// Fixture ids and payload shapes are copied from the live ~/.claude survey of
// 2026-08-31: a sidecar carries procStart as a JSON string and a pile of keys
// this package ignores, and an agent meta file may omit name, model, or
// description entirely (269 of 600 sampled metas carried name; 132 carried
// neither name nor model).
const (
	claudeFixtureSession   = "84a06b9a-0873-4655-9eb9-d7cf99554b05"
	claudeFixtureSecond    = "8ad6c366-70de-429c-98bf-a1c350d6ac31"
	claudeFixtureThird     = "9646f67b-0120-4d1e-836e-2e21fc16d80c"
	claudeFixtureOrphan    = "06e524ab-4afa-4a36-a578-1a629d4853c3"
	claudeFixtureOrphanOld = "0b5eee95-4523-4249-8c82-463efd174f6a"
	claudeFixtureStaleHost = "0bb9b35b-251d-4c4d-bdb7-44c68f2860a2"
	claudeFixtureSlug      = "-home-aegis"
	claudeNamedAgent       = "alane1r-architect-b8b06ac8c091add8"
	claudeNestedAgent      = "a2045f6a2f6cc8f6a"
	claudeScoutAgent       = "aapi-scout-d8c49c78d4a1fab2"
	claudeFixtureStartedAt = int64(1788136922001)
)

func claudeBase() time.Time { return time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC) }

// claudeTree writes a ~/.claude-shaped fixture into a temp dir. Every file gets
// an explicit mtime: the horizon, the activity window, and the exit window are
// all mtime arithmetic, so a fixture that inherited the write clock would be
// testing the machine's timing instead of the rules.
type claudeTree struct {
	t    *testing.T
	home string
}

func newClaudeTree(t *testing.T) *claudeTree {
	t.Helper()
	return &claudeTree{t: t, home: t.TempDir()}
}

func (tree *claudeTree) write(path, body string, mod time.Time) string {
	tree.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tree.t.Fatalf("claude-fixture-mkdir rule violated: dir=%s err=%v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		tree.t.Fatalf("claude-fixture-write rule violated: path=%s err=%v", path, err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		tree.t.Fatalf("claude-fixture-chtimes rule violated: path=%s mod=%s err=%v", path, mod, err)
	}
	return path
}

func (tree *claudeTree) encode(fields map[string]any) string {
	tree.t.Helper()
	body, err := json.Marshal(fields)
	if err != nil {
		tree.t.Fatalf("claude-fixture-encode rule violated: fields=%v err=%v", fields, err)
	}
	return string(body)
}

// sidecar writes sessions/<pid>.json with the live shape, including the keys
// this package never reads, so an over-strict decoder cannot pass.
func (tree *claudeTree) sidecar(pid int, session, cwd, name string, mod time.Time) string {
	tree.t.Helper()
	return tree.sidecarFields(pid, map[string]any{
		"pid":          pid,
		"sessionId":    session,
		"cwd":          cwd,
		"startedAt":    claudeFixtureStartedAt,
		"procStart":    "7241979",
		"version":      "2.1.251",
		"peerProtocol": 1,
		"peerFeatures": []string{"notify_idle", "reply_across_default_dirs"},
		"kind":         "interactive",
		"entrypoint":   "cli",
		"name":         name,
		"nameSource":   "derived",
		"status":       "idle",
		"updatedAt":    claudeFixtureStartedAt + 1000,
	}, mod)
}

func (tree *claudeTree) sidecarFields(pid int, fields map[string]any, mod time.Time) string {
	tree.t.Helper()
	return tree.rawSidecar(pid, tree.encode(fields), mod)
}

func (tree *claudeTree) rawSidecar(pid int, body string, mod time.Time) string {
	tree.t.Helper()
	return tree.write(filepath.Join(tree.home, "sessions", fmt.Sprintf("%d.json", pid)), body, mod)
}

func (tree *claudeTree) agentMeta(session, agent string, fields map[string]any, mod time.Time) string {
	tree.t.Helper()
	path := filepath.Join(tree.home, "projects", claudeFixtureSlug, session, "subagents", "agent-"+agent+".meta.json")
	return tree.write(path, tree.encode(fields), mod)
}

// agentTranscript writes the child transcript. Its sessionId is the PARENT's,
// exactly as on disk: that is the trap this fixture exists to keep armed.
func (tree *claudeTree) agentTranscript(session, agent string, mod time.Time) string {
	tree.t.Helper()
	line := tree.encode(map[string]any{
		"type":      "user",
		"sessionId": session,
		"uuid":      "b0f1c2d3-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
		"cwd":       "/home/aegis/Projects/aitop",
	})
	path := filepath.Join(tree.home, "projects", claudeFixtureSlug, session, "subagents", "agent-"+agent+".jsonl")
	return tree.write(path, line+"\n", mod)
}

func scanClaude(t *testing.T, home string, now time.Time) ([]NodeSighting, []SpawnSighting, *claudeScanner) {
	t.Helper()
	scanner := &claudeScanner{home: home}
	nodes, spawns, err := scanner.scan(now)
	if err != nil {
		t.Fatalf("claude-scan-tolerates-disk rule violated: home=%s err=%v", home, err)
	}
	return nodes, spawns, scanner
}

func nodeByID(nodes []NodeSighting, id graph.NodeID) (NodeSighting, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return NodeSighting{}, false
}

func countNodeID(nodes []NodeSighting, id graph.NodeID) int {
	n := 0
	for _, node := range nodes {
		if node.ID == id {
			n++
		}
	}
	return n
}

func spawnByChild(spawns []SpawnSighting, child graph.NodeID) (SpawnSighting, bool) {
	for _, spawn := range spawns {
		if spawn.ChildID == child {
			return spawn, true
		}
	}
	return SpawnSighting{}, false
}

func nodeIDs(nodes []NodeSighting) []graph.NodeID {
	ids := make([]graph.NodeID, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func TestClaudeScannerFindsSidecarPrimaries(t *testing.T) {
	now := claudeBase()
	tree := newClaudeTree(t)
	firstPath := tree.sidecar(1450932, claudeFixtureSession, "/home/aegis/Projects/aitop", "aegis-bd", now.Add(-5*time.Minute))
	secondPath := tree.sidecar(27273, claudeFixtureSecond, "/home/aegis/Projects/aitop/.worktrees/native-provenance", "aegis-48", now.Add(-time.Minute))
	// Two unusable roster entries. Skipping them must be counted: a scanner
	// that died on the corrupt file and one that found nothing look identical.
	tree.rawSidecar(99001, "{ this is not json", now.Add(-time.Minute))
	tree.sidecarFields(99002, map[string]any{"pid": 99002, "sessionId": "", "cwd": "/home/aegis", "procStart": "12"}, now.Add(-time.Minute))

	nodes, spawns, scanner := scanClaude(t, tree.home, now)

	if len(spawns) != 0 {
		t.Fatalf("claude-sidecar-emits-no-spawns rule violated: spawns=%d nodes=%v", len(spawns), nodeIDs(nodes))
	}
	if len(nodes) != 2 {
		t.Fatalf("claude-sidecar-primary-count rule violated: nodes=%d want=2 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if skipped := scanner.skippedSidecars.Load(); skipped != 2 {
		t.Fatalf("claude-sidecar-skips-counted rule violated: skippedSidecars=%d want=2 ids=%v", skipped, nodeIDs(nodes))
	}

	first, ok := nodeByID(nodes, mustSessionID(t, claudeFixtureSession))
	if !ok {
		t.Fatalf("claude-sidecar-primary-identity rule violated: session=%s absent ids=%v", claudeFixtureSession, nodeIDs(nodes))
	}
	if first.SessionID != claudeFixtureSession || first.Runtime != types.RuntimeClaude || first.Role != types.RolePrimary {
		t.Fatalf("claude-sidecar-primary-identity rule violated: sessionID=%q runtime=%q role=%q want=%q/%q/%q",
			first.SessionID, first.Runtime, first.Role, claudeFixtureSession, types.RuntimeClaude, types.RolePrimary)
	}
	if first.Name != "aegis-bd" || first.Project != "aitop" {
		t.Fatalf("claude-sidecar-primary-display rule violated: name=%q project=%q want=%q/%q", first.Name, first.Project, "aegis-bd", "aitop")
	}
	if first.Location != firstPath {
		t.Fatalf("claude-sidecar-location-is-source-file rule violated: location=%q want=%q", first.Location, firstPath)
	}
	if first.StartedAt == nil || first.StartedAt.UnixMilli() != claudeFixtureStartedAt {
		t.Fatalf("claude-sidecar-startedat-from-epoch-ms rule violated: startedAt=%v want=%d", first.StartedAt, claudeFixtureStartedAt)
	}
	// v1 leaves primary state to the occupancy collector, which holds the
	// process truth; a native claim here would fight it every tick.
	if first.State != "" || first.Exit != "" {
		t.Fatalf("claude-sidecar-primary-makes-no-state-claim rule violated: state=%q exit=%q", first.State, first.Exit)
	}

	second, ok := nodeByID(nodes, mustSessionID(t, claudeFixtureSecond))
	if !ok {
		t.Fatalf("claude-sidecar-primary-identity rule violated: session=%s absent ids=%v", claudeFixtureSecond, nodeIDs(nodes))
	}
	if second.Project != "aitop/native-provenance" || second.Location != secondPath {
		t.Fatalf("claude-sidecar-project-from-cwd rule violated: project=%q location=%q want=%q/%q",
			second.Project, second.Location, "aitop/native-provenance", secondPath)
	}
}

func TestClaudeScannerEmitsAgentSpawnChain(t *testing.T) {
	now := claudeBase()
	tree := newClaudeTree(t)
	tree.sidecar(1450932, claudeFixtureSecond, "/home/aegis/Projects/aitop", "aegis-48", now.Add(-2*time.Minute))
	namedMeta := tree.agentMeta(claudeFixtureSecond, claudeNamedAgent, map[string]any{
		"agentType":   "Explore",
		"description": "Survey the on-disk formats",
		"name":        "lane1r-architect",
		"model":       "claude-opus-5",
		"spawnDepth":  0,
		"taskKind":    "in_process_teammate",
		"teamName":    "session-83fc0182",
	}, now.Add(-10*time.Minute))
	tree.agentTranscript(claudeFixtureSecond, claudeNamedAgent, now.Add(-9*time.Minute))
	nestedMeta := tree.agentMeta(claudeFixtureSecond, claudeNestedAgent, map[string]any{
		"agentType":     "Explore",
		"description":   "Survey channels + classification backfill",
		"toolUseId":     "toolu_011eWeA1qRFoa13Kno5RgTjD",
		"parentAgentId": claudeNamedAgent,
		"spawnDepth":    1,
	}, now.Add(-8*time.Minute))
	tree.agentTranscript(claudeFixtureSecond, claudeNestedAgent, now.Add(-7*time.Minute))

	nodes, spawns, _ := scanClaude(t, tree.home, now)

	session := mustSessionID(t, claudeFixtureSecond)
	named := mustAgentID(t, claudeFixtureSecond, claudeNamedAgent)
	nested := mustAgentID(t, claudeFixtureSecond, claudeNestedAgent)
	if len(nodes) != 3 || len(spawns) != 2 {
		t.Fatalf("claude-spawn-chain-shape rule violated: nodes=%d spawns=%d want=3/2 ids=%v", len(nodes), len(spawns), nodeIDs(nodes))
	}

	namedNode, ok := nodeByID(nodes, named)
	if !ok {
		t.Fatalf("claude-agent-node-identity rule violated: agent=%s absent ids=%v", named, nodeIDs(nodes))
	}
	if namedNode.Role != types.RoleSubagent || namedNode.SessionID != claudeNamedAgent {
		t.Fatalf("claude-agent-node-identity rule violated: role=%q sessionID=%q want=%q/%q",
			namedNode.Role, namedNode.SessionID, types.RoleSubagent, claudeNamedAgent)
	}
	if namedNode.Name != "lane1r-architect" || namedNode.Model != "claude-opus-5" || namedNode.TaskName != "Survey the on-disk formats" {
		t.Fatalf("claude-agent-node-display rule violated: name=%q model=%q taskName=%q want=%q/%q/%q",
			namedNode.Name, namedNode.Model, namedNode.TaskName, "lane1r-architect", "claude-opus-5", "Survey the on-disk formats")
	}
	if namedNode.Project != "aitop" {
		t.Fatalf("claude-agent-project-from-live-parent rule violated: project=%q want=%q", namedNode.Project, "aitop")
	}
	if namedNode.Location != namedMeta {
		t.Fatalf("claude-agent-location-is-source-file rule violated: location=%q want=%q", namedNode.Location, namedMeta)
	}

	nestedNode, ok := nodeByID(nodes, nested)
	if !ok {
		t.Fatalf("claude-agent-node-identity rule violated: agent=%s absent ids=%v", nested, nodeIDs(nodes))
	}
	// Most metas on disk carry no name; falling back to agentType is what keeps
	// those children from rendering nameless.
	if nestedNode.Name != "Explore" || nestedNode.Model != "" {
		t.Fatalf("claude-agent-name-falls-back-to-type rule violated: name=%q model=%q want=%q/%q", nestedNode.Name, nestedNode.Model, "Explore", "")
	}

	namedSpawn, ok := spawnByChild(spawns, named)
	if !ok {
		t.Fatalf("claude-spawn-edge-present rule violated: child=%s absent spawns=%d", named, len(spawns))
	}
	if namedSpawn.ParentID != session || namedSpawn.ParentSessionID != claudeFixtureSecond {
		t.Fatalf("claude-spawn-parent-is-enclosing-session rule violated: parentID=%q parentSessionID=%q want=%q/%q",
			namedSpawn.ParentID, namedSpawn.ParentSessionID, session, claudeFixtureSecond)
	}
	if namedSpawn.Relationship != graph.RelationshipID(claudeNamedAgent) || namedSpawn.ChildSessionID != claudeNamedAgent {
		t.Fatalf("claude-spawn-relationship-is-agent-id rule violated: relationship=%q childSessionID=%q want=%q",
			namedSpawn.Relationship, namedSpawn.ChildSessionID, claudeNamedAgent)
	}
	if namedSpawn.Location != namedMeta {
		t.Fatalf("claude-spawn-location-is-source-file rule violated: location=%q want=%q", namedSpawn.Location, namedMeta)
	}

	nestedSpawn, ok := spawnByChild(spawns, nested)
	if !ok {
		t.Fatalf("claude-spawn-edge-present rule violated: child=%s absent spawns=%d", nested, len(spawns))
	}
	// The nested child's parent is the sibling AGENT, not the session: reading
	// spawnDepth or the directory name instead of parentAgentId flattens the
	// chain into two children of one session.
	if nestedSpawn.ParentID != named || nestedSpawn.ParentSessionID != claudeNamedAgent {
		t.Fatalf("claude-spawn-parent-from-parentagentid rule violated: parentID=%q parentSessionID=%q want=%q/%q",
			nestedSpawn.ParentID, nestedSpawn.ParentSessionID, named, claudeNamedAgent)
	}
	if nestedSpawn.Relationship != graph.RelationshipID(claudeNestedAgent) || nestedSpawn.Location != nestedMeta {
		t.Fatalf("claude-spawn-relationship-is-agent-id rule violated: relationship=%q location=%q want=%q/%q",
			nestedSpawn.Relationship, nestedSpawn.Location, claudeNestedAgent, nestedMeta)
	}

	for _, spawn := range spawns {
		if strings.Contains(string(spawn.Relationship), ":") {
			t.Fatalf("claude-relationship-is-short-id-not-node-id rule violated: relationship=%q child=%q", spawn.Relationship, spawn.ChildID)
		}
		if len(spawn.Relationship) == 0 || len(spawn.Relationship) > 64 {
			t.Fatalf("claude-relationship-length-bound rule violated: relationship=%q bytes=%d limit=64", spawn.Relationship, len(spawn.Relationship))
		}
	}
}

func TestClaudeScannerHorizonSkipsStale(t *testing.T) {
	now := claudeBase()
	stale := now.Add(-2 * time.Hour)
	tree := newClaudeTree(t)
	tree.sidecar(1450932, claudeFixtureSession, "/home/aegis/Projects/aitop", "live", now.Add(-time.Minute))
	tree.sidecar(27273, claudeFixtureSecond, "/home/aegis/Projects/aitop", "stale", stale)

	// Both files stale: gone.
	tree.agentMeta(claudeFixtureSession, "astale00000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, stale)
	tree.agentTranscript(claudeFixtureSession, "astale00000000000", stale)
	// Stale meta, fresh transcript: a long-running child whose spawn record was
	// written hours ago. max(meta, transcript) is what keeps it visible.
	tree.agentMeta(claudeFixtureSession, "amixed00000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, stale)
	tree.agentTranscript(claudeFixtureSession, "amixed00000000000", now.Add(-3*time.Minute))
	// Fresh meta, no transcript yet: a child that has just been spawned.
	tree.agentMeta(claudeFixtureSession, "anojsonl000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-5*time.Minute))

	nodes, spawns, _ := scanClaude(t, tree.home, now)

	present := []graph.NodeID{
		mustSessionID(t, claudeFixtureSession),
		mustAgentID(t, claudeFixtureSession, "amixed00000000000"),
		mustAgentID(t, claudeFixtureSession, "anojsonl000000000"),
	}
	for _, id := range present {
		if _, ok := nodeByID(nodes, id); !ok {
			t.Fatalf("claude-horizon-admits-recent-activity rule violated: id=%s absent ids=%v", id, nodeIDs(nodes))
		}
	}
	absent := []graph.NodeID{
		mustSessionID(t, claudeFixtureSecond),
		mustAgentID(t, claudeFixtureSession, "astale00000000000"),
	}
	for _, id := range absent {
		if _, ok := nodeByID(nodes, id); ok {
			t.Fatalf("claude-horizon-skips-stale rule violated: id=%s present horizon=%s ids=%v", id, nativeHorizon, nodeIDs(nodes))
		}
	}
	if len(nodes) != len(present) {
		t.Fatalf("claude-horizon-skips-stale rule violated: nodes=%d want=%d ids=%v", len(nodes), len(present), nodeIDs(nodes))
	}
	staleAgent := mustAgentID(t, claudeFixtureSession, "astale00000000000")
	if _, ok := spawnByChild(spawns, staleAgent); ok {
		t.Fatalf("claude-horizon-skips-stale-edges rule violated: child=%s has a spawn spawns=%d", staleAgent, len(spawns))
	}
	if len(spawns) != 2 {
		t.Fatalf("claude-horizon-keeps-fresh-edges rule violated: spawns=%d want=2", len(spawns))
	}
}

func TestClaudeScannerVanishesOrphanedChildren(t *testing.T) {
	now := claudeBase()
	tree := newClaudeTree(t)

	// Live parent: a child touched inside the activity window is running, one
	// touched outside it makes no claim at all.
	tree.sidecar(1450932, claudeFixtureSession, "/home/aegis/Projects/aitop", "live", now.Add(-time.Minute))
	tree.agentMeta(claudeFixtureSession, "aactive0000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-4*time.Minute))
	tree.agentTranscript(claudeFixtureSession, "aactive0000000000", now.Add(-10*time.Second))
	tree.agentMeta(claudeFixtureSession, "aquiet00000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-4*time.Minute))
	tree.agentTranscript(claudeFixtureSession, "aquiet00000000000", now.Add(-2*time.Minute))

	// No sidecar for these two sessions: in-process children die with the
	// parent, so a fresh one vanished and an old one is not ours to observe.
	orphanAt := now.Add(-time.Minute)
	tree.agentMeta(claudeFixtureOrphan, "aorphan0000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-6*time.Minute))
	tree.agentTranscript(claudeFixtureOrphan, "aorphan0000000000", orphanAt)
	tree.agentMeta(claudeFixtureOrphanOld, "aburied0000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-40*time.Minute))
	tree.agentTranscript(claudeFixtureOrphanOld, "aburied0000000000", now.Add(-30*time.Minute))

	// A roster entry too old to publish a primary is still a roster entry. Its
	// children are not orphans, and the parent synthesized to root them must
	// not invent a death nothing witnessed.
	tree.sidecar(31337, claudeFixtureStaleHost, "/home/aegis/Projects/aitop", "quiet-host", now.Add(-2*time.Hour))
	tree.agentMeta(claudeFixtureStaleHost, "ahosted0000000000", map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-5*time.Minute))
	tree.agentTranscript(claudeFixtureStaleHost, "ahosted0000000000", now.Add(-2*time.Minute))

	nodes, _, _ := scanClaude(t, tree.home, now)

	active, ok := nodeByID(nodes, mustAgentID(t, claudeFixtureSession, "aactive0000000000"))
	if !ok {
		t.Fatalf("claude-live-child-observed rule violated: agent=aactive0000000000 absent ids=%v", nodeIDs(nodes))
	}
	if active.State != graph.StateActive || active.Exit != "" {
		t.Fatalf("claude-live-child-claims-active rule violated: state=%q exit=%q want=%q/%q", active.State, active.Exit, graph.StateActive, "")
	}
	quiet, ok := nodeByID(nodes, mustAgentID(t, claudeFixtureSession, "aquiet00000000000"))
	if !ok {
		t.Fatalf("claude-live-child-observed rule violated: agent=aquiet00000000000 absent ids=%v", nodeIDs(nodes))
	}
	if quiet.State != "" || quiet.Exit != "" {
		t.Fatalf("claude-quiet-child-makes-no-claim rule violated: state=%q exit=%q window=%s", quiet.State, quiet.Exit, claudeChildActiveWindow)
	}

	orphan, ok := nodeByID(nodes, mustAgentID(t, claudeFixtureOrphan, "aorphan0000000000"))
	if !ok {
		t.Fatalf("claude-orphan-child-observed rule violated: agent=aorphan0000000000 absent ids=%v", nodeIDs(nodes))
	}
	if orphan.Exit != graph.OutcomeVanished || orphan.State != "" {
		t.Fatalf("claude-orphan-child-vanishes rule violated: exit=%q state=%q want=%q/%q", orphan.Exit, orphan.State, graph.OutcomeVanished, "")
	}
	if orphan.ExitAt == nil || !orphan.ExitAt.Equal(orphanAt) {
		t.Fatalf("claude-orphan-exitat-is-last-activity rule violated: exitAt=%v want=%s", orphan.ExitAt, orphanAt)
	}
	// The synthesized parent keeps the spawn edge admissible: the reconciler
	// creates no placeholder for an endpoint nobody published.
	orphanParent, ok := nodeByID(nodes, mustSessionID(t, claudeFixtureOrphan))
	if !ok {
		t.Fatalf("claude-orphan-parent-synthesized rule violated: session=%s absent ids=%v", claudeFixtureOrphan, nodeIDs(nodes))
	}
	if orphanParent.Role != types.RolePrimary || orphanParent.Name != "" {
		t.Fatalf("claude-orphan-parent-is-minimal rule violated: role=%q name=%q want=%q/%q", orphanParent.Role, orphanParent.Name, types.RolePrimary, "")
	}
	// The missing roster entry that proves the children died proves the session
	// died. A synthesized parent without a terminal is immortal: the reconciler
	// sets GhostExpiresAt only from a terminal state and its sweep skips a node
	// that has none, while MaxNodes gates admission rather than eviction.
	if orphanParent.Exit != graph.OutcomeVanished || orphanParent.ExitAt == nil || !orphanParent.ExitAt.Equal(orphanAt) {
		t.Fatalf("claude-orphan-parent-vanishes-with-its-children rule violated: exit=%q exitAt=%v want=%q/%s", orphanParent.Exit, orphanParent.ExitAt, graph.OutcomeVanished, orphanAt)
	}

	buried, ok := nodeByID(nodes, mustAgentID(t, claudeFixtureOrphanOld, "aburied0000000000"))
	if !ok {
		t.Fatalf("claude-old-orphan-still-carries-terminal rule violated: agent=aburied0000000000 absent ids=%v", nodeIDs(nodes))
	}
	if buried.Exit != graph.OutcomeVanished || buried.ExitAt == nil || now.Sub(*buried.ExitAt) <= nativeExitWindow {
		t.Fatalf("claude-old-orphan-terminal-is-dated rule violated: exit=%q exitAt=%v window=%s", buried.Exit, buried.ExitAt, nativeExitWindow)
	}
	// Every child of that store is past the exit window and will be dropped by
	// the core, so a parent synthesized for them would outlive every child it
	// exists for, with no edge, no state and no terminal to end it.
	buriedParent := mustSessionID(t, claudeFixtureOrphanOld)
	if _, ok := nodeByID(nodes, buriedParent); ok {
		t.Fatalf("claude-buried-store-synthesizes-no-parent rule violated: session=%s present with every child past window=%s ids=%v", claudeFixtureOrphanOld, nativeExitWindow, nodeIDs(nodes))
	}

	// The other half of the same rule: presence, however stale, is not death
	// evidence. Nothing here witnessed an exit, so nothing may claim one.
	hostParent, ok := nodeByID(nodes, mustSessionID(t, claudeFixtureStaleHost))
	if !ok {
		t.Fatalf("claude-stale-host-parent-synthesized rule violated: session=%s absent ids=%v", claudeFixtureStaleHost, nodeIDs(nodes))
	}
	if hostParent.Exit != "" || hostParent.ExitAt != nil {
		t.Fatalf("claude-present-sidecar-invents-no-terminal rule violated: exit=%q exitAt=%v sidecarAge=2h horizon=%s", hostParent.Exit, hostParent.ExitAt, nativeHorizon)
	}
	hosted, ok := nodeByID(nodes, mustAgentID(t, claudeFixtureStaleHost, "ahosted0000000000"))
	if !ok {
		t.Fatalf("claude-stale-host-child-observed rule violated: agent=ahosted0000000000 absent ids=%v", nodeIDs(nodes))
	}
	if hosted.Exit != "" {
		t.Fatalf("claude-child-of-present-sidecar-does-not-vanish rule violated: exit=%q exitAt=%v", hosted.Exit, hosted.ExitAt)
	}

	if len(nodes) != 8 {
		t.Fatalf("claude-orphan-fixture-node-count rule violated: nodes=%d want=8 ids=%v", len(nodes), nodeIDs(nodes))
	}

	// End-to-end: the core must drop that old terminal entirely rather than
	// publish a node it would evict moments later.
	sink := &recordingSink{}
	collector := newTestCollector(&claudeScanner{home: tree.home}, nil, now)
	collector.tick(sink)
	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("claude-emitted-events-validate rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
	}
	buriedID := mustAgentID(t, claudeFixtureOrphanOld, "aburied0000000000")
	for _, ev := range events {
		if ev.Actor == buriedID || ev.Target == buriedID || ev.Actor == buriedParent || ev.Target == buriedParent {
			t.Fatalf("claude-old-orphan-not-observed rule violated: kind=%s actor=%s target=%s exitAt=%v window=%s", ev.Kind, ev.Actor, ev.Target, buried.ExitAt, nativeExitWindow)
		}
	}
	// Seven published nodes: the live session and its two children, the fresh
	// orphan and its synthesized parent, and the stale host with its child. An
	// eighth would be the immortal one this test exists to keep out.
	if published := countKind(events, graph.EventNodeObserved); published != 7 {
		t.Fatalf("claude-orphan-published-node-count rule violated: node_observed=%d want=7 kinds=%v", published, kindsOf(events))
	}
	// One, not two: the buried child is dropped here, while its parent was
	// never synthesized in the first place.
	if skipped := collector.staleTerminals.Load(); skipped != 1 {
		t.Fatalf("claude-old-orphan-drop-counted rule violated: staleTerminals=%d want=1 events=%d", skipped, len(events))
	}
	orphanID := mustAgentID(t, claudeFixtureOrphan, "aorphan0000000000")
	if _, ok := firstOfKind(events, graph.EventExitObserved, orphanID); !ok {
		t.Fatalf("claude-fresh-orphan-exit-emitted rule violated: no exit_observed for actor=%s kinds=%v", orphanID, kindsOf(events))
	}
	orphanParentID := mustSessionID(t, claudeFixtureOrphan)
	if _, ok := firstOfKind(events, graph.EventExitObserved, orphanParentID); !ok {
		t.Fatalf("claude-orphan-parent-exit-emitted rule violated: no exit_observed for actor=%s kinds=%v", orphanParentID, kindsOf(events))
	}
}

func TestClaudeScannerAgentIDFromFilename(t *testing.T) {
	now := claudeBase()
	tree := newClaudeTree(t)
	tree.sidecar(1450932, claudeFixtureThird, "/home/aegis/Projects/aitop", "aegis-bd", now.Add(-time.Minute))
	// Both agentId shapes seen on disk: a<name>-<hex16> and a<hex17>.
	tree.agentMeta(claudeFixtureThird, claudeScoutAgent, map[string]any{
		"agentType":   "Explore",
		"description": "Survey the API surface",
		"spawnDepth":  0,
	}, now.Add(-6*time.Minute))
	tree.agentTranscript(claudeFixtureThird, claudeScoutAgent, now.Add(-5*time.Minute))
	tree.agentMeta(claudeFixtureThird, claudeNestedAgent, map[string]any{"agentType": "Explore", "spawnDepth": 0}, now.Add(-6*time.Minute))
	tree.agentTranscript(claudeFixtureThird, claudeNestedAgent, now.Add(-5*time.Minute))

	nodes, spawns, _ := scanClaude(t, tree.home, now)

	session := mustSessionID(t, claudeFixtureThird)
	for _, agent := range []string{claudeScoutAgent, claudeNestedAgent} {
		want := mustAgentID(t, claudeFixtureThird, agent)
		node, ok := nodeByID(nodes, want)
		if !ok {
			t.Fatalf("claude-agent-id-from-filename rule violated: agent=%s absent ids=%v", agent, nodeIDs(nodes))
		}
		if node.SessionID != agent {
			t.Fatalf("claude-agent-id-from-filename rule violated: sessionID=%q want=%q id=%s", node.SessionID, agent, node.ID)
		}
		// The child transcript's own sessionId field holds the PARENT's session.
		// Identity taken from inside the transcript collapses the child onto its
		// parent; identity taken from the filename cannot.
		if node.ID == session || node.SessionID == claudeFixtureThird {
			t.Fatalf("claude-child-identity-never-from-transcript rule violated: id=%q sessionID=%q parent=%q", node.ID, node.SessionID, claudeFixtureThird)
		}
	}
	if seen := countNodeID(nodes, session); seen != 1 {
		t.Fatalf("claude-child-identity-never-from-transcript rule violated: nodes with parent id=%d want=1 ids=%v", seen, nodeIDs(nodes))
	}
	if len(spawns) != 2 {
		t.Fatalf("claude-agent-id-from-filename rule violated: spawns=%d want=2", len(spawns))
	}
	for _, spawn := range spawns {
		if spawn.ParentID == spawn.ChildID {
			t.Fatalf("claude-spawn-is-not-a-self-edge rule violated: parent=%q child=%q relationship=%q", spawn.ParentID, spawn.ChildID, spawn.Relationship)
		}
		if string(spawn.Relationship) == claudeFixtureThird {
			t.Fatalf("claude-relationship-never-parent-session rule violated: relationship=%q child=%q", spawn.Relationship, spawn.ChildID)
		}
	}
}

// TestClaudeCollectorLandsChainInRealShadow judges the whole lane by what the
// production reconciler kept, not by what the collector published: endpoint
// identity is enforced reconciler-side and is invisible in the dispositions.
func TestClaudeCollectorLandsChainInRealShadow(t *testing.T) {
	// The reconciler dates freshness from the wall clock, so this fixture lives
	// in real time: a state claim stamped at a synthetic hour arrives already
	// decayed and the run would certify an emission it never judged.
	now := time.Now().UTC()
	tree := newClaudeTree(t)
	tree.sidecar(1450932, claudeFixtureSecond, "/home/aegis/Projects/aitop", "aegis-48", now.Add(-time.Minute))
	tree.agentMeta(claudeFixtureSecond, claudeNamedAgent, map[string]any{
		"agentType": "Explore", "name": "lane1r-architect", "model": "claude-opus-5",
		"description": "Survey the on-disk formats", "spawnDepth": 0,
	}, now.Add(-4*time.Minute))
	tree.agentTranscript(claudeFixtureSecond, claudeNamedAgent, now.Add(-10*time.Second))
	tree.agentMeta(claudeFixtureSecond, claudeNestedAgent, map[string]any{
		"agentType": "Explore", "parentAgentId": claudeNamedAgent, "spawnDepth": 1,
	}, now.Add(-3*time.Minute))
	tree.agentTranscript(claudeFixtureSecond, claudeNestedAgent, now.Add(-2*time.Minute))

	collector := NewClaude(tree.home, nil)
	collector.now = func() time.Time { return now }
	// One tick only. A repeating poll heals a broken emission order on the next
	// pass, because the endpoints are already in the store by then.
	collector.interval = time.Hour

	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), collector)
	if err != nil {
		t.Fatalf("claude-shadow-construction rule violated: err=%v descriptor=%+v", err, collector.Descriptor())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = shadow.Run(ctx) }()

	session := mustSessionID(t, claudeFixtureSecond)
	named := mustAgentID(t, claudeFixtureSecond, claudeNamedAgent)
	nested := mustAgentID(t, claudeFixtureSecond, claudeNestedAgent)
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot := shadow.Snapshot()
		if snapshot != nil && len(snapshot.Edges) >= 2 {
			nodes := map[graph.NodeID]graph.Node{}
			for _, node := range snapshot.Nodes {
				nodes[node.ID] = node
			}
			for _, id := range []graph.NodeID{session, named, nested} {
				if _, ok := nodes[id]; !ok {
					t.Fatalf("claude-shadow-publishes-every-endpoint rule violated: id=%s absent nodes=%d edges=%d", id, len(snapshot.Nodes), len(snapshot.Edges))
				}
			}
			edges := map[graph.NodeID]graph.Edge{}
			for _, edge := range snapshot.Edges {
				edges[edge.Target] = edge
			}
			for _, want := range []struct {
				child        graph.NodeID
				parent       graph.NodeID
				relationship graph.RelationshipID
			}{
				{named, session, graph.RelationshipID(claudeNamedAgent)},
				{nested, named, graph.RelationshipID(claudeNestedAgent)},
			} {
				edge, ok := edges[want.child]
				if !ok {
					t.Fatalf("claude-shadow-spawn-chain rule violated: no edge into child=%s edges=%d", want.child, len(snapshot.Edges))
				}
				if edge.Source != want.parent || edge.Type != graph.EdgeSpawn || edge.Provenance != graph.ProvenanceNative {
					t.Fatalf("claude-shadow-spawn-chain rule violated: child=%s source=%s type=%s provenance=%s wantSource=%s",
						want.child, edge.Source, edge.Type, edge.Provenance, want.parent)
				}
				if edge.Relationship != want.relationship {
					t.Fatalf("claude-shadow-relationship-is-agent-id rule violated: child=%s relationship=%q want=%q", want.child, edge.Relationship, want.relationship)
				}
			}
			if node := nodes[named]; node.Role != types.RoleSubagent || node.ProvenName != "lane1r-architect" || node.State.Value != graph.StateActive {
				t.Fatalf("claude-shadow-node-content rule violated: role=%q provenName=%q state=%q want=%q/%q/%q",
					node.Role, node.ProvenName, node.State.Value, types.RoleSubagent, "lane1r-architect", graph.StateActive)
			}
			return
		}
		if time.Now().After(deadline) {
			nodes, edges := 0, 0
			if snapshot != nil {
				nodes, edges = len(snapshot.Nodes), len(snapshot.Edges)
			}
			t.Fatalf("claude-shadow-spawn-chain rule violated: fewer than 2 edges after 5s nodes=%d edges=%d published=%d rejected=%d",
				nodes, edges, collector.Disp().Published.Load(), collector.Disp().Rejected.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
