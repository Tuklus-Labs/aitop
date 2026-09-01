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

// Fixture ids and payload shapes are copied from the live ~/.codex survey of
// 2026-08-31 (1598 rollouts under sessions/2026/08). What the corpus says, and
// what these fixtures therefore have to carry:
//
//   - id != session_id in 98 of 99 rollouts on the newest day. The trap is the
//     common case, not the edge case.
//   - parent_thread_id != session_id in 36 rollouts (depth 2 and 3 spawns).
//     Those are the only ones that can tell a node keyed on the child's own id
//     from a node keyed on the root, so the nested ids below are real ones.
//   - source is an OBJECT on subagent threads and a plain STRING ("vscode",
//     "cli", "exec") on user threads. A decoder that types it as an object
//     fails the whole record and loses every user thread.
//   - first lines run 18.8 KB median, 46.6 KB max, because base_instructions
//     carries the whole system prompt. The fixtures pad to that scale.
const (
	// A depth-2 subagent: three distinct ids in one payload.
	codexNestedThread = "019fbeff-6378-7902-a6b2-57e3d8e09cc2" // its own id
	codexRootThread   = "019faa13-fd80-7db0-a2fa-a71ffaa87690" // session_id: the ROOT, never a node id
	codexMidThread    = "019fb670-f95b-7000-9c92-82d163400945" // parent_thread_id: the real parent

	// A user thread and two of its forks.
	codexUserThread  = "01a0519a-73df-7261-b1ad-32b399e90ed2"
	codexForkChild   = "01a05374-58e3-73e1-b4a7-93e45399d123"
	codexSecondChild = "01a0537d-2b5d-7c60-9b8c-99ff4cecb7ea"

	codexNestedNick = "Hooke"
	codexForkNick   = "Darwin"
	codexSecondNick = "James the 2nd"

	codexFixtureCWD = "/home/aegis/Projects/aitop"
)

func codexBase() time.Time { return time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC) }

// codexTree writes a ~/.codex-shaped fixture into a temp dir. Every file gets
// an explicit mtime and an explicit date directory: the horizon is mtime
// arithmetic and the walk bound is directory arithmetic, so a fixture that
// inherited the write clock or the current date would be testing the machine
// instead of the rules.
type codexTree struct {
	t    *testing.T
	home string
}

func newCodexTree(t *testing.T) *codexTree {
	t.Helper()
	return &codexTree{t: t, home: t.TempDir()}
}

func (tree *codexTree) write(path, body string, mod time.Time) string {
	tree.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tree.t.Fatalf("codex-fixture-mkdir rule violated: dir=%s err=%v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		tree.t.Fatalf("codex-fixture-write rule violated: path=%s err=%v", path, err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		tree.t.Fatalf("codex-fixture-chtimes rule violated: path=%s mod=%s err=%v", path, mod, err)
	}
	return path
}

func (tree *codexTree) encode(fields map[string]any) string {
	tree.t.Helper()
	body, err := json.Marshal(fields)
	if err != nil {
		tree.t.Fatalf("codex-fixture-encode rule violated: fields=%v err=%v", fields, err)
	}
	return string(body)
}

// rolloutPath names the file exactly as codex does: the date directory and the
// filename timestamp are both LOCAL (verified against the corpus, where a
// rollout stamped 2026-08-30T07:09:33Z sits under 2026/08/30 with a 00-09-33
// filename at UTC-7).
func (tree *codexTree) rolloutPath(day time.Time, thread string) string {
	local := day.Local()
	return filepath.Join(tree.home, "sessions",
		local.Format("2006"), local.Format("01"), local.Format("02"),
		fmt.Sprintf("rollout-%s-%s.jsonl", local.Format("2006-01-02T15-04-05"), thread))
}

// rollout writes a session_meta first line plus the following records a real
// rollout carries, so a reader that consumes more than the first line, or one
// that decodes the whole file, cannot pass.
func (tree *codexTree) rollout(day time.Time, thread string, payload map[string]any, mod time.Time) string {
	tree.t.Helper()
	first := tree.encode(map[string]any{
		"timestamp": day.UTC().Format(time.RFC3339Nano),
		"ordinal":   0,
		"type":      "session_meta",
		"payload":   payload,
	})
	return tree.rawRollout(day, thread, first+"\n"+codexTailRecords(), mod)
}

func (tree *codexTree) rawRollout(day time.Time, thread, body string, mod time.Time) string {
	tree.t.Helper()
	return tree.write(tree.rolloutPath(day, thread), body, mod)
}

// codexTailRecords is the rest of a rollout: everything after the first line is
// conversation, and none of it is this package's business.
func codexTailRecords() string {
	return `{"timestamp":"2026-08-31T07:00:01.276Z","ordinal":1,"type":"event_msg","payload":{"type":"task_started","turn_id":"01a05180-88d6-7bd1-9612-6073a370e5ce"}}
{"timestamp":"2026-08-31T07:00:02.971Z","ordinal":2,"type":"response_item","payload":{"type":"message","role":"developer"}}
`
}

// codexBaseInstructions pads a payload to the live scale (18.8 KB median). A
// fixture whose first line is a few hundred bytes cannot see a read that stops
// short of the newline.
func codexBaseInstructions() map[string]any {
	return map[string]any{
		"text":       strings.Repeat("You are Codex, an agent based on GPT-5. Collaborate until the goal is handled. ", 240),
		"provenance": map[string]any{"type": "model", "model": "gpt-5.6-sol"},
	}
}

// codexUserPayload is a user thread: id == session_id, no parent_thread_id, and
// source as a plain STRING.
func codexUserPayload(thread, cwd string) map[string]any {
	return map[string]any{
		"session_id":          thread,
		"id":                  thread,
		"timestamp":           "2026-08-31T06:37:51.839Z",
		"cwd":                 cwd,
		"originator":          "codex_work_desktop",
		"cli_version":         "0.150.1",
		"source":              "vscode",
		"thread_source":       "user",
		"model_provider":      "openai",
		"base_instructions":   codexBaseInstructions(),
		"history_mode":        "paginated",
		"multi_agent_version": "v2",
		"context_window":      map[string]any{"window_id": "01a05180-886b-7123-a27d-491fa83942a0"},
	}
}

// codexSubagentPayload is a spawned thread. session_id is the ROOT of the whole
// run even at depth 1, parent_thread_id is the immediate parent, and the two
// differ on every depth-2 spawn in the corpus. The nickname appears twice on
// disk, nested under source and again at the top level; the corpus has them
// equal in all 98 sampled subagents, and the nested copy is the one inside the
// spawn record.
func codexSubagentPayload(thread, root, parent, nickname, cwd string) map[string]any {
	depth := 1
	if root != parent {
		depth = 2
	}
	return map[string]any{
		"session_id":       root,
		"id":               thread,
		"parent_thread_id": parent,
		"forked_from_id":   parent,
		"timestamp":        "2026-08-31T06:45:28.995Z",
		"cwd":              cwd,
		"originator":       "codex-tui",
		"cli_version":      "0.150.1",
		"source": map[string]any{
			"subagent": map[string]any{
				"thread_spawn": map[string]any{
					"parent_thread_id": parent,
					"depth":            depth,
					"agent_path":       "/root/task9_heartbeat_adversarial",
					"agent_nickname":   nickname,
					"agent_role":       nil,
				},
			},
		},
		"thread_source":       "subagent",
		"agent_nickname":      nickname,
		"agent_path":          "/root/task9_heartbeat_adversarial",
		"model_provider":      "openai",
		"base_instructions":   codexBaseInstructions(),
		"history_mode":        "paginated",
		"multi_agent_version": "v2",
		"context_window":      map[string]any{"window_id": "01a05374-58e3-73e1-b4a7-491fa83942a0"},
	}
}

func scanCodex(t *testing.T, home string, now time.Time) ([]NodeSighting, []SpawnSighting, *codexScanner) {
	t.Helper()
	return scanCodexLive(t, home, now, nil)
}

// scanCodexLive is the same scan with the horizon rule's live-process clause
// supplied. nil and an empty map both mean "no thread has a live process".
func scanCodexLive(t *testing.T, home string, now time.Time, live map[string]bool) ([]NodeSighting, []SpawnSighting, *codexScanner) {
	t.Helper()
	scanner := &codexScanner{home: home}
	nodes, spawns, err := scanner.scan(now, live)
	if err != nil {
		t.Fatalf("codex-scan-tolerates-disk rule violated: home=%s err=%v", home, err)
	}
	return nodes, spawns, scanner
}

func mustThreadID(t *testing.T, thread string) graph.NodeID {
	t.Helper()
	id, err := graph.CodexThreadID(thread)
	if err != nil {
		t.Fatalf("codex-test-fixture-thread-id rule violated: thread=%q err=%v", thread, err)
	}
	return id
}

func newCodexCollector(home string, now time.Time) *Collector {
	c := newCollector(codexSourceID, types.RuntimeCodex, &codexScanner{home: home}, nil)
	c.now = func() time.Time { return now }
	return c
}

// TestCodexScannerNodeUsesOwnThreadID is the named disk trap. session_meta
// carries three ids and only one of them is this thread: `id` is its own,
// `session_id` is the ROOT of the whole tree, and `parent_thread_id` is its
// parent. Keying the node on session_id collapses every thread in a run onto
// one node, and on the live corpus that is the majority case (98 of 99
// rollouts on the newest day have id != session_id).
func TestCodexScannerNodeUsesOwnThreadID(t *testing.T) {
	now := codexBase()
	tree := newCodexTree(t)
	path := tree.rollout(now, codexNestedThread,
		codexSubagentPayload(codexNestedThread, codexRootThread, codexMidThread, codexNestedNick, codexFixtureCWD),
		now.Add(-3*time.Minute))

	nodes, spawns, _ := scanCodex(t, tree.home, now)

	child := mustThreadID(t, codexNestedThread)
	root := mustThreadID(t, codexRootThread)
	mid := mustThreadID(t, codexMidThread)

	node, ok := nodeByID(nodes, child)
	if !ok {
		t.Fatalf("codex-node-is-own-thread-id rule violated: thread=%s absent ids=%v", codexNestedThread, nodeIDs(nodes))
	}
	// The short id feeds the invocation incarnation, so it has to be the thread's
	// own id too: a node with the right NodeID and the root's short id would bind
	// this thread's incarnation to the root's.
	if node.SessionID != codexNestedThread {
		t.Fatalf("codex-node-short-id-is-own-thread-id rule violated: sessionID=%q want=%q id=%s", node.SessionID, codexNestedThread, node.ID)
	}
	if node.Runtime != types.RuntimeCodex || node.Role != types.RoleSubagent {
		t.Fatalf("codex-node-identity rule violated: runtime=%q role=%q want=%q/%q", node.Runtime, node.Role, types.RuntimeCodex, types.RoleSubagent)
	}
	if node.Name != codexNestedNick {
		t.Fatalf("codex-name-from-spawn-nickname rule violated: name=%q want=%q", node.Name, codexNestedNick)
	}
	if node.Project != "aitop" {
		t.Fatalf("codex-project-from-cwd rule violated: project=%q cwd=%q want=%q", node.Project, codexFixtureCWD, "aitop")
	}
	if node.Location != path {
		t.Fatalf("codex-location-is-source-file rule violated: location=%q want=%q", node.Location, path)
	}
	// v1 has no death evidence for a codex thread and makes no state claim: a
	// node ages past the horizon and reads Stale rather than being asserted.
	if node.State != "" || node.Exit != "" || node.ExitAt != nil {
		t.Fatalf("codex-makes-no-state-or-terminal-claim rule violated: state=%q exit=%q exitAt=%v", node.State, node.Exit, node.ExitAt)
	}

	// The trap itself. session_id names a thread that is neither this one nor its
	// parent, and nothing in the walked set is that thread, so any node for it is
	// this bug and nothing else.
	if seen := countNodeID(nodes, root); seen != 0 {
		t.Fatalf("codex-session-id-is-never-a-node rule violated: nodes for session_id=%s count=%d ids=%v", codexRootThread, seen, nodeIDs(nodes))
	}
	// Two nodes: this thread and the minimal parent its spawn edge needs. A third
	// would be the root sneaking in under some other name.
	if len(nodes) != 2 {
		t.Fatalf("codex-node-count rule violated: nodes=%d want=2 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if _, ok := nodeByID(nodes, mid); !ok {
		t.Fatalf("codex-parent-is-parent-thread-id rule violated: parent=%s absent ids=%v", codexMidThread, nodeIDs(nodes))
	}

	spawn, ok := spawnByChild(spawns, child)
	if !ok {
		t.Fatalf("codex-spawn-edge-present rule violated: child=%s absent spawns=%d", child, len(spawns))
	}
	// parent_thread_id, not session_id: reading the parent off session_id hangs
	// every depth-2 thread directly on the root and flattens the tree.
	if spawn.ParentID != mid || spawn.ParentSessionID != codexMidThread {
		t.Fatalf("codex-parent-is-parent-thread-id rule violated: parentID=%q parentSessionID=%q want=%q/%q",
			spawn.ParentID, spawn.ParentSessionID, mid, codexMidThread)
	}
	if spawn.Relationship != graph.RelationshipID(codexNestedThread) {
		t.Fatalf("codex-relationship-is-own-thread-id rule violated: relationship=%q want=%q", spawn.Relationship, codexNestedThread)
	}
}

// TestCodexScannerSpawnFromParentThreadID covers the edge and both shapes of
// parent endpoint: one whose own rollout was scanned (and must not be
// duplicated by a synthesized minimal one) and one that was not (and must be
// synthesized, or the reconciler drops the edge for an unknown endpoint).
func TestCodexScannerSpawnFromParentThreadID(t *testing.T) {
	now := codexBase()
	tree := newCodexTree(t)
	userPath := tree.rollout(now, codexUserThread, codexUserPayload(codexUserThread, codexFixtureCWD), now.Add(-2*time.Minute))
	forkPath := tree.rollout(now, codexForkChild,
		codexSubagentPayload(codexForkChild, codexUserThread, codexUserThread, codexForkNick, codexFixtureCWD),
		now.Add(-time.Minute))
	// A child of a thread whose own rollout is nowhere in the walked window.
	secondPath := tree.rollout(now, codexSecondChild,
		codexSubagentPayload(codexSecondChild, codexRootThread, codexRootThread, codexSecondNick, codexFixtureCWD),
		now.Add(-90*time.Second))

	nodes, spawns, _ := scanCodex(t, tree.home, now)

	user := mustThreadID(t, codexUserThread)
	fork := mustThreadID(t, codexForkChild)
	second := mustThreadID(t, codexSecondChild)
	root := mustThreadID(t, codexRootThread)

	// The user thread's `source` is the plain string "vscode". Typing that field
	// as an object fails the whole record, and every user thread on the box
	// disappears while the subagents keep working.
	userNode, ok := nodeByID(nodes, user)
	if !ok {
		t.Fatalf("codex-string-source-still-decodes rule violated: user thread=%s absent ids=%v", codexUserThread, nodeIDs(nodes))
	}
	if userNode.Role != types.RolePrimary || userNode.Name != "" || userNode.Project != "aitop" || userNode.Location != userPath {
		t.Fatalf("codex-user-thread-identity rule violated: role=%q name=%q project=%q location=%q want=%q/%q/%q/%q",
			userNode.Role, userNode.Name, userNode.Project, userNode.Location, types.RolePrimary, "", "aitop", userPath)
	}
	// Scanned once, published once. A parent synthesized on top of its own
	// rollout would replace this content with a nameless, projectless stub.
	if seen := countNodeID(nodes, user); seen != 1 {
		t.Fatalf("codex-scanned-parent-not-duplicated rule violated: nodes for parent=%s count=%d want=1 ids=%v", codexUserThread, seen, nodeIDs(nodes))
	}

	forkNode, ok := nodeByID(nodes, fork)
	if !ok {
		t.Fatalf("codex-fork-child-observed rule violated: child=%s absent ids=%v", codexForkChild, nodeIDs(nodes))
	}
	if forkNode.Role != types.RoleSubagent || forkNode.Name != codexForkNick || forkNode.Location != forkPath {
		t.Fatalf("codex-fork-child-identity rule violated: role=%q name=%q location=%q want=%q/%q/%q",
			forkNode.Role, forkNode.Name, forkNode.Location, types.RoleSubagent, codexForkNick, forkPath)
	}

	// The synthesized parent: minimal on purpose. The child's file says who its
	// parent is and nothing else about it, so a name or a project here would be
	// invented. Its location is the child's rollout, which is the file the fact
	// came from.
	rootNode, ok := nodeByID(nodes, root)
	if !ok {
		t.Fatalf("codex-unscanned-parent-synthesized rule violated: parent=%s absent ids=%v", codexRootThread, nodeIDs(nodes))
	}
	if rootNode.Role != types.RolePrimary || rootNode.Name != "" || rootNode.Project != "" {
		t.Fatalf("codex-synthesized-parent-is-minimal rule violated: role=%q name=%q project=%q want=%q/%q/%q",
			rootNode.Role, rootNode.Name, rootNode.Project, types.RolePrimary, "", "")
	}
	if rootNode.SessionID != codexRootThread || rootNode.Location != secondPath {
		t.Fatalf("codex-synthesized-parent-anchored-on-child-file rule violated: sessionID=%q location=%q want=%q/%q",
			rootNode.SessionID, rootNode.Location, codexRootThread, secondPath)
	}
	if rootNode.State != "" || rootNode.Exit != "" {
		t.Fatalf("codex-makes-no-state-or-terminal-claim rule violated: synthesized parent state=%q exit=%q", rootNode.State, rootNode.Exit)
	}

	if len(nodes) != 4 {
		t.Fatalf("codex-spawn-fixture-node-count rule violated: nodes=%d want=4 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if len(spawns) != 2 {
		t.Fatalf("codex-spawn-edge-count rule violated: spawns=%d want=2", len(spawns))
	}

	forkSpawn, ok := spawnByChild(spawns, fork)
	if !ok {
		t.Fatalf("codex-spawn-edge-present rule violated: child=%s absent spawns=%d", fork, len(spawns))
	}
	if forkSpawn.ParentID != user || forkSpawn.ParentSessionID != codexUserThread || forkSpawn.Location != forkPath {
		t.Fatalf("codex-spawn-parent-endpoint rule violated: parentID=%q parentSessionID=%q location=%q want=%q/%q/%q",
			forkSpawn.ParentID, forkSpawn.ParentSessionID, forkSpawn.Location, user, codexUserThread, forkPath)
	}
	// This child's session_id equals its parent id, so an implementation that
	// labels the edge with session_id looks right here and is wrong: the label
	// has to be the CHILD's own id.
	if forkSpawn.Relationship != graph.RelationshipID(codexForkChild) || forkSpawn.ChildSessionID != codexForkChild {
		t.Fatalf("codex-relationship-is-own-thread-id rule violated: relationship=%q childSessionID=%q want=%q",
			forkSpawn.Relationship, forkSpawn.ChildSessionID, codexForkChild)
	}

	secondSpawn, ok := spawnByChild(spawns, second)
	if !ok {
		t.Fatalf("codex-spawn-edge-present rule violated: child=%s absent spawns=%d", second, len(spawns))
	}
	if secondSpawn.ParentID != root || secondSpawn.Relationship != graph.RelationshipID(codexSecondChild) {
		t.Fatalf("codex-spawn-parent-endpoint rule violated: parentID=%q relationship=%q want=%q/%q",
			secondSpawn.ParentID, secondSpawn.Relationship, root, codexSecondChild)
	}

	for _, spawn := range spawns {
		if spawn.ParentID == spawn.ChildID {
			t.Fatalf("codex-spawn-is-not-a-self-edge rule violated: parent=%q child=%q relationship=%q", spawn.ParentID, spawn.ChildID, spawn.Relationship)
		}
		if strings.Contains(string(spawn.Relationship), ":") {
			t.Fatalf("codex-relationship-is-short-id-not-node-id rule violated: relationship=%q child=%q", spawn.Relationship, spawn.ChildID)
		}
		if len(spawn.Relationship) == 0 || len(spawn.Relationship) > maxRelationshipBytes {
			t.Fatalf("codex-relationship-length-bound rule violated: relationship=%q bytes=%d limit=%d", spawn.Relationship, len(spawn.Relationship), maxRelationshipBytes)
		}
	}
}

// TestCodexScannerSkipsMalformedAndStale covers every reason to read nothing
// out of a rollout, and the rule that binds them: a child this scan skipped
// must not leave a synthesized parent behind. With no death evidence for a
// codex thread, such a parent has no state, no terminal and no edge, and the
// reconciler's sweep only ever looks at nodes carrying a GhostExpiresAt, which
// is set from a terminal. It would sit in the graph forever.
func TestCodexScannerSkipsMalformedAndStale(t *testing.T) {
	now := codexBase()
	tree := newCodexTree(t)

	// Canary: one good thread, no parent of its own. Every other fixture here is
	// meant to yield nothing, and a zero-sighting result is byte-identical to a
	// scanner that never walked anything.
	goodPath := tree.rollout(now, codexUserThread, codexUserPayload(codexUserThread, codexFixtureCWD), now.Add(-time.Minute))

	// Past the horizon: fresh content, old file.
	tree.rollout(now, codexForkChild,
		codexSubagentPayload(codexForkChild, codexRootThread, codexRootThread, codexForkNick, codexFixtureCWD),
		now.Add(-2*time.Hour))

	// Malformed first line.
	tree.rawRollout(now, "019fbf6c-73f4-7c31-b617-1fa074e2bd7d", "{ this is not json\n"+codexTailRecords(), now.Add(-time.Minute))

	// Valid JSON, a session_meta type, and a field of the wrong type. This is the
	// only shape that separates the decode gate from the type gate: a syntax
	// error never populates anything (encoding/json validates the whole input
	// before decoding), so both gates catch it, while a type error leaves `type`
	// set and only the decode gate sees it.
	tree.rawRollout(now, "019fbd5d-6c9c-7c90-be06-757d9b3c42a3",
		`{"timestamp":"2026-08-31T07:00:00.000Z","ordinal":0,"type":"session_meta","payload":{"id":42,"session_id":"019faa13-fd80-7db0-a2fa-a71ffaa87690","thread_source":"user","cwd":"/home/aegis"}}`+"\n"+codexTailRecords(),
		now.Add(-time.Minute))

	// A first record that is not session_meta: what a rotated or truncated file
	// looks like. Its payload carries no thread ids at all.
	tree.rawRollout(now, "019fbf7b-41b3-7780-a7a0-5fda51ee8d69",
		`{"timestamp":"2026-08-31T07:00:01.276Z","ordinal":1,"type":"event_msg","payload":{"type":"task_started"}}`+"\n"+codexTailRecords(),
		now.Add(-time.Minute))

	// A first line past the read cap. Valid session_meta in every other respect,
	// so the size is the only reason to skip it. The padding is an ABSOLUTE
	// 96 KiB, not codexMaxFirstLine plus a margin: a fixture sized from the
	// constant under test grows with it, and a cap raised to any value would
	// still exceed it, so nothing here could ever see the cap move. 96 KiB sits
	// above the 64 KiB cap and well above the 46.6 KB largest first line in the
	// live corpus, so raising the cap past it is the deliberate act this
	// assertion is here to notice.
	oversize := codexSubagentPayload(codexSecondChild, codexRootThread, codexRootThread, codexSecondNick, codexFixtureCWD)
	oversize["base_instructions"] = map[string]any{"text": strings.Repeat("x", 96<<10)}
	tree.rollout(now, codexSecondChild, oversize, now.Add(-time.Minute))

	// session_meta with no id: the record cannot name a thread, so it cannot
	// name a child either, and its parent_thread_id must not be believed.
	empty := codexSubagentPayload(codexNestedThread, codexRootThread, codexMidThread, codexNestedNick, codexFixtureCWD)
	empty["id"] = ""
	tree.rollout(now, codexNestedThread, empty, now.Add(-time.Minute))

	nodes, spawns, scanner := scanCodex(t, tree.home, now)

	good, ok := nodeByID(nodes, mustThreadID(t, codexUserThread))
	if !ok {
		t.Fatalf("codex-canary-thread-observed rule violated: thread=%s absent, so the zero results below prove nothing ids=%v", codexUserThread, nodeIDs(nodes))
	}
	if good.Location != goodPath {
		t.Fatalf("codex-location-is-source-file rule violated: location=%q want=%q", good.Location, goodPath)
	}
	// Exactly one node. Every extra would be a parent synthesized for a child
	// this scan refused to publish.
	if len(nodes) != 1 {
		t.Fatalf("codex-skipped-child-synthesizes-no-parent rule violated: nodes=%d want=1 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if len(spawns) != 0 {
		t.Fatalf("codex-skipped-child-emits-no-edge rule violated: spawns=%d want=0 ids=%v", len(spawns), nodeIDs(nodes))
	}
	for _, absent := range []struct {
		id     graph.NodeID
		reason string
	}{
		{mustThreadID(t, codexForkChild), "past the horizon"},
		{mustThreadID(t, codexSecondChild), "first line past the read cap"},
		{mustThreadID(t, codexNestedThread), "session_meta with an empty id"},
		{mustThreadID(t, codexRootThread), "parent of a child that was skipped"},
		{mustThreadID(t, codexMidThread), "parent of a child that was skipped"},
	} {
		if seen := countNodeID(nodes, absent.id); seen != 0 {
			t.Fatalf("codex-skipped-child-synthesizes-no-parent rule violated: id=%s count=%d reason=%q ids=%v", absent.id, seen, absent.reason, nodeIDs(nodes))
		}
	}

	// A skipped file and an absent one are the same silence without these.
	if skipped := scanner.skippedRollouts.Load(); skipped != 4 {
		t.Fatalf("codex-unreadable-rollouts-counted rule violated: skippedRollouts=%d want=4 (malformed, wrong-typed id, non-session_meta, oversized) ids=%v", skipped, nodeIDs(nodes))
	}
	if skipped := scanner.skippedThreads.Load(); skipped != 1 {
		t.Fatalf("codex-unusable-thread-ids-counted rule violated: skippedThreads=%d want=1 (empty id) ids=%v", skipped, nodeIDs(nodes))
	}

	// An empty ~/.codex is ordinary, not an error: returning one would blank the
	// runtime for the tick.
	emptyNodes, emptySpawns, emptyScanner := scanCodex(t, t.TempDir(), now)
	if len(emptyNodes) != 0 || len(emptySpawns) != 0 || emptyScanner.skippedRollouts.Load() != 0 {
		t.Fatalf("codex-absent-home-is-not-an-error rule violated: nodes=%d spawns=%d skipped=%d",
			len(emptyNodes), len(emptySpawns), emptyScanner.skippedRollouts.Load())
	}
}

// TestCodexScannerScansOnlyRecentDateDirs pins the walk bound. ~/.codex holds
// 1598 rollouts for August alone and grows forever; a scanner that walked the
// tree would open every one of them every 2 s. The fixtures outside the window
// carry FRESH mtimes, so the horizon cannot be what excludes them: only the
// directory arithmetic can.
func TestCodexScannerScansOnlyRecentDateDirs(t *testing.T) {
	now := codexBase()
	tree := newCodexTree(t)

	// UTC today and UTC yesterday are in the walked set under every timezone,
	// because the set is the union of the UTC and local pairs.
	todayPath := tree.rollout(now, codexUserThread, codexUserPayload(codexUserThread, codexFixtureCWD), now.Add(-time.Minute))
	yesterdayPath := tree.rollout(now.AddDate(0, 0, -1), codexForkChild, codexUserPayload(codexForkChild, codexFixtureCWD), now.Add(-2*time.Minute))

	// Three days back is outside the set under every timezone: the local date can
	// lead or lag UTC by one day, so the widest possible union is UTC-today+1
	// through UTC-today-2.
	tree.rollout(now.AddDate(0, 0, -3), codexSecondChild, codexUserPayload(codexSecondChild, codexFixtureCWD), now.Add(-time.Minute))
	// The brief's case: an old date directory holding a file touched seconds ago.
	tree.rollout(time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC), codexNestedThread,
		codexUserPayload(codexNestedThread, codexFixtureCWD), now.Add(-30*time.Second))

	nodes, _, scanner := scanCodex(t, tree.home, now)

	for _, want := range []struct {
		thread string
		path   string
	}{
		{codexUserThread, todayPath},
		{codexForkChild, yesterdayPath},
	} {
		node, ok := nodeByID(nodes, mustThreadID(t, want.thread))
		if !ok {
			t.Fatalf("codex-walk-covers-today-and-yesterday rule violated: thread=%s absent ids=%v", want.thread, nodeIDs(nodes))
		}
		if node.Location != want.path {
			t.Fatalf("codex-location-is-source-file rule violated: location=%q want=%q", node.Location, want.path)
		}
	}
	for _, absent := range []struct {
		thread string
		reason string
	}{
		{codexSecondChild, "date dir three days back"},
		{codexNestedThread, "date dir 2026/02/01 with a fresh mtime"},
	} {
		if seen := countNodeID(nodes, mustThreadID(t, absent.thread)); seen != 0 {
			t.Fatalf("codex-walk-skips-old-date-dirs rule violated: thread=%s count=%d reason=%q ids=%v", absent.thread, seen, absent.reason, nodeIDs(nodes))
		}
	}
	if len(nodes) != 2 {
		t.Fatalf("codex-walk-skips-old-date-dirs rule violated: nodes=%d want=2 ids=%v", len(nodes), nodeIDs(nodes))
	}
	// The distinction this test exists for: those two files were never OPENED,
	// not opened and rejected. A whole-tree walk would read them, find them
	// fresh and well formed, and publish them; a walk that read and then
	// discarded them would move a skip counter.
	if skipped := scanner.skippedRollouts.Load(); skipped != 0 {
		t.Fatalf("codex-walk-does-not-open-old-date-dirs rule violated: skippedRollouts=%d want=0 ids=%v", skipped, nodeIDs(nodes))
	}

	// The other half of the rule, which the fixtures above cannot see: the walked
	// set is the UTC pair AND the local pair, deduped. Those two pairs are the
	// same pair for most of the day in most zones, so the zones here are pinned
	// rather than inherited -- under the machine's own zone a scanner that walks
	// only one pair is indistinguishable from one that walks both.
	for _, probe := range []struct {
		name   string
		now    time.Time
		offset int // seconds east of UTC
	}{
		{"local-behind-utc", now, -12 * 3600},
		{"local-ahead-of-utc", time.Date(2026, 8, 31, 23, 0, 0, 0, time.UTC), 14 * 3600},
	} {
		loc := time.FixedZone(probe.name, probe.offset)
		local := probe.now.In(loc)
		if local.Format("2006-01-02") == probe.now.UTC().Format("2006-01-02") {
			t.Fatalf("codex-date-dirs-probe-is-discriminating rule violated: probe=%q local=%s utc=%s share a date, so this probe cannot tell the two pairs apart",
				probe.name, local.Format("2006-01-02"), probe.now.UTC().Format("2006-01-02"))
		}
		dirs := codexDateDirsIn(probe.now, loc)
		if len(dirs) < 2 || len(dirs) > 4 {
			t.Fatalf("codex-date-dirs-bounded rule violated: probe=%q dirs=%v count=%d want 2..4", probe.name, dirs, len(dirs))
		}
		seen := map[string]bool{}
		for _, dir := range dirs {
			if seen[dir] {
				t.Fatalf("codex-date-dirs-deduped rule violated: probe=%q dir=%q repeated dirs=%v", probe.name, dir, dirs)
			}
			seen[dir] = true
		}
		for _, want := range []time.Time{
			probe.now.UTC(), probe.now.UTC().AddDate(0, 0, -1),
			local, local.AddDate(0, 0, -1),
		} {
			dir := filepath.Join(want.Format("2006"), want.Format("01"), want.Format("02"))
			if !seen[dir] {
				t.Fatalf("codex-date-dirs-cover-utc-and-local rule violated: probe=%q dir=%q absent dirs=%v", probe.name, dir, dirs)
			}
		}
	}
}

// TestCodexCollectorMakesNoStateOrTerminalClaims judges the whole lane by what
// reaches the sink. Codex v1 knows who spawned whom and nothing about state or
// terminal outcomes. Poll heartbeats are visibility leases for the identity
// sightings; they do not invent a state claim.
func TestCodexCollectorMakesNoStateOrTerminalClaims(t *testing.T) {
	now := codexBase()
	tree := newCodexTree(t)
	tree.rollout(now, codexUserThread, codexUserPayload(codexUserThread, codexFixtureCWD), now.Add(-2*time.Minute))
	tree.rollout(now, codexForkChild,
		codexSubagentPayload(codexForkChild, codexUserThread, codexUserThread, codexForkNick, codexFixtureCWD),
		now.Add(-time.Minute))

	sink := &recordingSink{}
	collector := newCodexCollector(tree.home, now)
	collector.tick(sink)

	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("codex-emitted-events-validate rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
	}
	if published := countKind(events, graph.EventNodeObserved); published != 2 {
		t.Fatalf("codex-collector-publishes-both-endpoints rule violated: node_observed=%d want=2 kinds=%v", published, kindsOf(events))
	}
	if edges := countKind(events, graph.EventRelationshipObserved); edges != 1 {
		t.Fatalf("codex-collector-publishes-spawn-edge rule violated: relationship_observed=%d want=1 kinds=%v", edges, kindsOf(events))
	}
	for _, banned := range []graph.EventKind{graph.EventStateObserved, graph.EventExitObserved} {
		if seen := countKind(events, banned); seen != 0 {
			t.Fatalf("codex-makes-no-state-or-terminal-claim rule violated: kind=%s count=%d kinds=%v", banned, seen, kindsOf(events))
		}
	}
	if beats := countKind(events, graph.EventHeartbeatObserved); beats != 2 {
		t.Fatalf("codex native identity visibility lease rule violated: heartbeat_observed=%d want=2 kinds=%v", beats, kindsOf(events))
	}
	if rejected := collector.Disp().Rejected.Load(); rejected != 0 {
		t.Fatalf("codex-collector-emits-nothing-rejected rule violated: rejected=%d published=%d", rejected, collector.Disp().Published.Load())
	}
	if dropped := collector.malformedRelationships.Load(); dropped != 0 {
		t.Fatalf("codex-relationship-passes-seam-guard rule violated: malformedRelationships=%d want=0 edges=%d", dropped, countKind(events, graph.EventRelationshipObserved))
	}

	// Single tick, so the ordering is the one the reconciler actually sees on a
	// cold store: a repeating poll would heal a broken order on the next pass.
	user := mustThreadID(t, codexUserThread)
	fork := mustThreadID(t, codexForkChild)
	nodeIndex := map[graph.NodeID]int{}
	edgeIndex := -1
	for i, ev := range events {
		switch ev.Kind {
		case graph.EventNodeObserved:
			if _, seen := nodeIndex[ev.Actor]; !seen {
				nodeIndex[ev.Actor] = i
			}
		case graph.EventRelationshipObserved:
			edgeIndex = i
		}
	}
	parentIndex, parentSeen := nodeIndex[user]
	childIndex, childSeen := nodeIndex[fork]
	if !parentSeen || !childSeen || parentIndex >= edgeIndex || childIndex >= edgeIndex {
		t.Fatalf("codex-emit-order-nodes-before-edges rule violated: parentIdx=%d parentSeen=%t childIdx=%d childSeen=%t edgeIdx=%d kinds=%v",
			parentIndex, parentSeen, childIndex, childSeen, edgeIndex, kindsOf(events))
	}
}

// TestCodexCollectorLandsForkInRealShadow judges the lane by what the
// production reconciler KEPT. Endpoint identity is enforced reconciler-side and
// is invisible in the dispositions, so a scanner that names an endpoint the
// store does not have looks perfectly healthy from inside this package.
func TestCodexCollectorLandsForkInRealShadow(t *testing.T) {
	// The walk bound is date arithmetic against the real clock, so this fixture
	// has to live on today's real date; a synthetic one lands in a directory the
	// scanner will not walk and the run would certify nothing.
	now := time.Now().UTC()
	tree := newCodexTree(t)
	tree.rollout(now, codexUserThread, codexUserPayload(codexUserThread, codexFixtureCWD), now.Add(-2*time.Minute))
	tree.rollout(now, codexForkChild,
		codexSubagentPayload(codexForkChild, codexUserThread, codexUserThread, codexForkNick, codexFixtureCWD),
		now.Add(-time.Minute))
	// A child whose parent's rollout is outside the window: the synthesized
	// endpoint has to satisfy the reconciler too, not just this package.
	tree.rollout(now, codexSecondChild,
		codexSubagentPayload(codexSecondChild, codexRootThread, codexRootThread, codexSecondNick, codexFixtureCWD),
		now.Add(-90*time.Second))

	collector := NewCodex(tree.home, nil)
	// One tick only. A repeating poll heals a broken emission order on the next
	// pass, because the endpoints are already in the store by then.
	collector.interval = time.Hour

	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), collector)
	if err != nil {
		t.Fatalf("codex-shadow-construction rule violated: err=%v descriptor=%+v", err, collector.Descriptor())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = shadow.Run(ctx) }()

	user := mustThreadID(t, codexUserThread)
	fork := mustThreadID(t, codexForkChild)
	second := mustThreadID(t, codexSecondChild)
	root := mustThreadID(t, codexRootThread)
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot := shadow.Snapshot()
		if snapshot != nil && len(snapshot.Edges) >= 2 {
			nodes := map[graph.NodeID]graph.Node{}
			for _, node := range snapshot.Nodes {
				nodes[node.ID] = node
			}
			for _, id := range []graph.NodeID{user, fork, second, root} {
				if _, ok := nodes[id]; !ok {
					t.Fatalf("codex-shadow-publishes-every-endpoint rule violated: id=%s absent nodes=%d edges=%d", id, len(snapshot.Nodes), len(snapshot.Edges))
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
				{fork, user, graph.RelationshipID(codexForkChild)},
				{second, root, graph.RelationshipID(codexSecondChild)},
			} {
				edge, ok := edges[want.child]
				if !ok {
					t.Fatalf("codex-shadow-spawn-edge rule violated: no edge into child=%s edges=%d", want.child, len(snapshot.Edges))
				}
				if edge.Source != want.parent || edge.Type != graph.EdgeSpawn || edge.Provenance != graph.ProvenanceNative {
					t.Fatalf("codex-shadow-spawn-edge rule violated: child=%s source=%s type=%s provenance=%s wantSource=%s",
						want.child, edge.Source, edge.Type, edge.Provenance, want.parent)
				}
				if edge.Relationship != want.relationship {
					t.Fatalf("codex-shadow-relationship-is-thread-id rule violated: child=%s relationship=%q want=%q", want.child, edge.Relationship, want.relationship)
				}
			}
			if node := nodes[fork]; node.Runtime != types.RuntimeCodex || node.Role != types.RoleSubagent || node.ProvenName != codexForkNick {
				t.Fatalf("codex-shadow-node-content rule violated: runtime=%q role=%q provenName=%q want=%q/%q/%q",
					node.Runtime, node.Role, node.ProvenName, types.RuntimeCodex, types.RoleSubagent, codexForkNick)
			}
			if collector.Disp().Rejected.Load() != 0 {
				t.Fatalf("codex-shadow-no-rejections rule violated: rejected=%d published=%d duplicates=%d",
					collector.Disp().Rejected.Load(), collector.Disp().Published.Load(), collector.Disp().Duplicates.Load())
			}
			return
		}
		if time.Now().After(deadline) {
			nodeCount, edgeCount := 0, 0
			if snapshot != nil {
				nodeCount, edgeCount = len(snapshot.Nodes), len(snapshot.Edges)
			}
			t.Fatalf("codex-shadow-spawn-edge rule violated: fewer than 2 edges after 5s nodes=%d edges=%d published=%d rejected=%d",
				nodeCount, edgeCount, collector.Disp().Published.Load(), collector.Disp().Rejected.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCodexScannerLiveThreadIsInsideHorizon covers the second clause of the
// horizon rule for codex: a thread is observed when its rollout is fresh OR
// when a live process holds it. The engine's rows carry the ROOT thread id for
// a live codex process, which is a user thread's own id, so the id set is
// matched against the rollout FILE NAME -- the name carries the thread id, and
// reading a stale rollout's header just to learn whether to admit it would
// reopen the walk bound this scanner exists to respect.
//
// Two cold rollouts, one of them live. A scanner that simply stopped applying
// the horizon would admit both, so selectivity is the assertion that separates
// the lift from a removed bound.
func TestCodexScannerLiveThreadIsInsideHorizon(t *testing.T) {
	now := codexBase()
	tree := newCodexTree(t)
	cold := now.Add(-3 * time.Hour) // well past nativeHorizon
	livePath := tree.rollout(now, codexUserThread, codexUserPayload(codexUserThread, codexFixtureCWD), cold)
	tree.rollout(now, codexSecondChild, codexUserPayload(codexSecondChild, codexFixtureCWD), cold)
	liveID := mustThreadID(t, codexUserThread)
	deadID := mustThreadID(t, codexSecondChild)

	nodes, _, _ := scanCodex(t, tree.home, now)
	if _, ok := nodeByID(nodes, liveID); ok {
		t.Fatalf("codex-cold-thread-with-no-process-is-not-observed rule violated: id=%s rolloutAge=%s horizon=%s nodes=%d", liveID, now.Sub(cold), nativeHorizon, len(nodes))
	}

	nodes, _, scanner := scanCodexLive(t, tree.home, now, map[string]bool{codexUserThread: true})
	node, ok := nodeByID(nodes, liveID)
	if !ok {
		t.Fatalf("codex-live-thread-is-inside-horizon rule violated: id=%s absent rolloutAge=%s liveSet=[%s] nodes=%d", liveID, now.Sub(cold), codexUserThread, len(nodes))
	}
	if _, ok := nodeByID(nodes, deadID); ok {
		t.Fatalf("codex-live-lift-admits-only-the-live-thread rule violated: id=%s admitted while absent from liveSet=[%s]", deadID, codexUserThread)
	}
	// The lift admits the rollout; it must not change what the rollout says.
	if node.Location != livePath || node.Project != "aitop" || node.Role != types.RolePrimary {
		t.Fatalf("codex-live-thread-carries-its-rollout-content rule violated: project=%q role=%q location=%q want location=%s", node.Project, node.Role, node.Location, livePath)
	}
	if node.State != "" || node.Exit != "" {
		t.Fatalf("codex-live-thread-makes-no-state-or-terminal-claim rule violated: state=%q exit=%q", node.State, node.Exit)
	}
	if n := scanner.skippedRollouts.Load(); n != 0 {
		t.Fatalf("codex-live-thread-is-not-a-skip rule violated: skippedRollouts=%d", n)
	}
}
