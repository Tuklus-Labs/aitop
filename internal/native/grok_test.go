package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aitop/internal/graph"
	"aitop/internal/types"
)

// Fixture ids and payload shapes are copied from the live ~/.grok survey of
// 2026-08-31 (434 summaries, 228 subagent meta files, 28 cwd buckets). What the
// corpus says, and what these fixtures therefore have to carry:
//
//   - EVERY subagent has its own top-level session directory and summary.json
//     (228 of 228), carrying session_kind "subagent" / "subagent_fork" /
//     "subagent_resume". A dark-main walk that does not read session_kind
//     publishes every subagent twice: once as somebody's child, once as a
//     primary of its own, with two different roles for one node id.
//   - subagent_id == child_session_id in 228 of 228, so no fixture taken from
//     disk can tell the two reads apart. The one fixture where they differ is
//     synthetic, and says so.
//   - meta.json's mtime matches completed_at in 228 of 228 and started_at in 0,
//     so the file is rewritten at completion and a RUNNING child's meta mtime is
//     its spawn time. 1 of 228 subagents ran longer than the horizon.
//   - status is "completed" (215) or "cancelled" (13). Cancelled is a real,
//     common shape that carries NO terminal under the exit rule.
//   - bucket directory mtimes move when a session directory is CREATED inside
//     them, never when a session writes: 0 of 28 buckets were inside the horizon
//     while a session was active two minutes earlier. That is why the walk is
//     unbounded at the directory level and gated on each session's own
//     summary.json instead.
//   - bucket names escape slashes (149) AND spaces (2), and the scanner encodes
//     both. 0 of 434 summaries have a file mtime older than the last_active_at
//     written inside them, which is what makes the mtime gate a safe prefilter
//     for the recorded horizon.
const (
	// Mains, all real session ids off the live box.
	grokLiveSession   = "01a05553-afa1-7ea3-b658-96498482568b" // in the roster
	grokSecondSession = "01a0557d-312b-7170-9f7c-8e94a48c49b6"
	grokDarkSession   = "019fbb2b-40be-7cc2-89a5-ca62441eaf5b" // never in a roster
	grokHostSession   = "01a01634-b76e-76e0-a27d-192cc5da9fd3" // parent whose summary is stale
	grokBuriedSession = "019ffdf3-85af-7171-8216-331c7e11bc19" // parent whose whole store is stale
	// The two halves of the summary-mtime gate: one reached only by the walk,
	// one reached only by the roster.
	grokStaleFileSession = "01a01639-60c4-7fd0-82a9-b1fca7bbd026"
	grokQuietFileSession = "01a01639-60c4-7fd0-82a9-b20e11c2885c"
	// A roster entry whose session directory has no summary.json yet.
	grokNoSummarySession = "019ffdf5-2c64-7ad0-ae70-fac78cb7718c"
	// A rostered session whose cwd carries a character the encoder does not
	// escape, so the two routes to it compute different paths.
	grokColonSession = "019ffdf5-2c64-7ad0-ae70-fade5a965ce7"

	// Children, all real child_session_ids off the live box.
	grokRunningChild    = "019fc18b-9518-7283-900c-e9e6450ba842"
	grokCompletedChild  = "019fda66-8c24-7c80-9a7f-3383839b9e06"
	grokFailedChild     = "019fda70-9b8f-7583-815e-a18bb1a087fb"
	grokCancelledChild  = "019fda72-cc6c-7f53-8dab-813d48f33fcf"
	grokStaleExitChild  = "019fda75-07a3-7720-a046-9f03690ecd52"
	grokUndatedChild    = "019fda77-abf4-7133-8d31-f802bc67d01a"
	grokLongRunChild    = "019fda7f-b335-7f50-a12a-18e0355d75a2"
	grokBuriedChild     = "019fda85-dcff-7b61-9a43-f1f58c4c3461"
	grokDarkChild       = "019fda8d-8a39-7651-82c6-8a44c982180f"
	grokRelabeledChild  = "019fda8e-a041-7253-9024-8dc7843efc7c"
	grokMismatchedChild = "019fda8e-a041-7253-9024-8ddf687717ba"
	grokEmptyIDChild    = "019fda8f-c3d9-7d73-8ac2-fff47a32b274"
	// The label carried by the one meta whose subagent_id and child_session_id
	// differ. Nothing on disk has this shape; see TestGrokScannerSpawnEdges.
	grokRelabelID = "019fda94-8279-7ea0-989d-f076eaba77fc"

	grokProjectCWD = "/home/aegis/Projects/aitop"          // ProjectName -> "aitop"
	grokDaemonCWD  = "/home/aegis/Projects/pensive/daemon" // ProjectName -> "pensive"
	// A child cwd with a space in it, the shape the live box actually has under
	// /home/aegis/Documents/Obsidian Vault.
	grokSpacedChildCWD = "/home/aegis/Documents/Obsidian Vault"

	grokBuildAgent = "grok-build-plan" // agent_name on all 206 mains on disk
	grokModel      = "grok-4.6"
	grokChildModel = "grok-4.5"
)

func grokBase() time.Time { return time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC) }

func grokStamp(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

// grokDiskEncode is what grok ITSELF writes: slashes and spaces both escaped,
// measured across the 28 bucket names on disk. It is spelled out here rather
// than calling grokEncodeCWD on purpose. The fixture tree has to be a statement
// about where grok puts its files, independent of what the scanner computes; a
// fixture built by the code under test agrees with that code by construction and
// can never show it reading the wrong path.
func grokDiskEncode(cwd string) string {
	return strings.ReplaceAll(strings.ReplaceAll(cwd, "/", "%2F"), " ", "%20")
}

// grokTree writes a ~/.grok-shaped fixture into a temp dir. Every file gets an
// explicit mtime, and every bucket directory gets one too via seal: the horizon
// and the dark-main walk bound are both mtime arithmetic, so a fixture that
// inherited the write clock would be testing the machine instead of the rules.
type grokTree struct {
	t    *testing.T
	home string
}

func newGrokTree(t *testing.T) *grokTree {
	t.Helper()
	return &grokTree{t: t, home: t.TempDir()}
}

func (tree *grokTree) write(path, body string, mod time.Time) string {
	tree.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tree.t.Fatalf("grok-fixture-mkdir rule violated: dir=%s err=%v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		tree.t.Fatalf("grok-fixture-write rule violated: path=%s err=%v", path, err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		tree.t.Fatalf("grok-fixture-chtimes rule violated: path=%s mod=%s err=%v", path, mod, err)
	}
	return path
}

func (tree *grokTree) encode(fields map[string]any) string {
	tree.t.Helper()
	body, err := json.Marshal(fields)
	if err != nil {
		tree.t.Fatalf("grok-fixture-encode rule violated: fields=%v err=%v", fields, err)
	}
	return string(body)
}

func (tree *grokTree) bucket(cwd string) string {
	return filepath.Join(tree.home, "sessions", grokDiskEncode(cwd))
}

func (tree *grokTree) sessionDir(cwd, session string) string {
	return filepath.Join(tree.bucket(cwd), session)
}

// seal fixes a bucket directory's mtime. It has to run AFTER every write into
// that bucket, because creating a session directory inside one updates the
// bucket's own mtime: a fixture that sealed first would be measuring the write
// order rather than the walk bound.
func (tree *grokTree) seal(cwd string, mod time.Time) {
	tree.t.Helper()
	tree.sealRaw(grokDiskEncode(cwd), mod)
}

func (tree *grokTree) sealRaw(bucket string, mod time.Time) {
	tree.t.Helper()
	path := filepath.Join(tree.home, "sessions", bucket)
	if err := os.Chtimes(path, mod, mod); err != nil {
		tree.t.Fatalf("grok-fixture-seal rule violated: bucket=%s mod=%s err=%v", path, mod, err)
	}
}

func (tree *grokTree) summary(cwd, session string, fields map[string]any, mod time.Time) string {
	tree.t.Helper()
	return tree.write(filepath.Join(tree.sessionDir(cwd, session), "summary.json"), tree.encode(fields), mod)
}

// rawSummary writes under a literal bucket name, for the decoy paths a wrong
// encoding would reach.
func (tree *grokTree) rawSummary(bucket, session string, fields map[string]any, mod time.Time) string {
	tree.t.Helper()
	return tree.write(filepath.Join(tree.home, "sessions", bucket, session, "summary.json"), tree.encode(fields), mod)
}

// rawMeta writes a child record under a literal bucket name, for a session whose
// on-disk path the scanner's encoder does not compute.
func (tree *grokTree) rawMeta(bucket, parent, child string, fields map[string]any, mod time.Time) string {
	tree.t.Helper()
	path := filepath.Join(tree.home, "sessions", bucket, parent, "subagents", child, "meta.json")
	return tree.write(path, tree.encode(fields), mod)
}

func (tree *grokTree) meta(parentCWD, parent, child string, fields map[string]any, mod time.Time) string {
	tree.t.Helper()
	path := filepath.Join(tree.sessionDir(parentCWD, parent), "subagents", child, "meta.json")
	return tree.write(path, tree.encode(fields), mod)
}

func (tree *grokTree) roster(entries []map[string]any, mod time.Time) string {
	tree.t.Helper()
	body, err := json.Marshal(entries)
	if err != nil {
		tree.t.Fatalf("grok-fixture-encode rule violated: entries=%v err=%v", entries, err)
	}
	return tree.write(filepath.Join(tree.home, "active_sessions.json"), string(body), mod)
}

func (tree *grokTree) rawRoster(body string, mod time.Time) string {
	tree.t.Helper()
	return tree.write(filepath.Join(tree.home, "active_sessions.json"), body, mod)
}

func grokRosterEntryFields(session, cwd string, opened time.Time) map[string]any {
	return map[string]any{
		"session_id": session,
		"pid":        1566150,
		"cwd":        cwd,
		"opened_at":  grokStamp(opened),
	}
}

// grokMainFields is a main session's summary.json: no session_kind, and an
// agent_name of grok-build-plan on all 206 mains on disk.
func grokMainFields(session, cwd, agent, model, title string, created, lastActive time.Time) map[string]any {
	return map[string]any{
		"info":                map[string]any{"id": session, "cwd": cwd},
		"session_summary":     title,
		"created_at":          grokStamp(created),
		"updated_at":          grokStamp(lastActive),
		"num_messages":        7521,
		"num_chat_messages":   213,
		"current_model_id":    model,
		"next_trace_turn":     31,
		"chat_format_version": 1,
		"grok_home":           "/home/aegis/.grok",
		"last_active_at":      grokStamp(lastActive),
		"generated_title":     title,
		"agent_name":          agent,
		"sandbox_profile":     "off",
		"reasoning_effort":    "xhigh",
	}
}

// grokChildOwnFields is what a subagent's OWN session directory holds. All 228
// subagents on disk have one; session_kind is the only thing that says so, and
// it never says whose child this is.
func grokChildOwnFields(session, cwd, kind string, lastActive time.Time) map[string]any {
	fields := grokMainFields(session, cwd, "general-purpose", grokChildModel, "", lastActive, lastActive)
	fields["session_kind"] = kind
	delete(fields, "generated_title")
	return fields
}

// grokMetaFields is subagents/<child>/meta.json, parent-side. subagent_id equals
// child_session_id on all 228 files on disk.
func grokMetaFields(parent, child, kind, model, desc, status, cwd string, started time.Time, completed string) map[string]any {
	return map[string]any{
		"subagent_id":              child,
		"parent_session_id":        parent,
		"child_session_id":         child,
		"subagent_type":            kind,
		"description":              desc,
		"prompt":                   "You are the Goal Plan Writer for the xAI Grok Build harness.",
		"status":                   status,
		"started_at":               grokStamp(started),
		"completed_at":             completed,
		"duration_ms":              286972,
		"tool_calls":               12,
		"turns":                    4,
		"effective_context_source": "fork",
		"child_cwd":                cwd,
		"effective_model_id":       model,
	}
}

func scanGrok(t *testing.T, home string, now time.Time) ([]NodeSighting, []SpawnSighting, *grokScanner) {
	t.Helper()
	scanner := &grokScanner{home: home}
	nodes, spawns, err := scanner.scan(now)
	if err != nil {
		t.Fatalf("grok-scan-tolerates-disk rule violated: home=%s err=%v", home, err)
	}
	return nodes, spawns, scanner
}

func mustGrokID(t *testing.T, session string) graph.NodeID {
	t.Helper()
	id, err := graph.GrokSessionID(session)
	if err != nil {
		t.Fatalf("grok-test-fixture-session-id rule violated: session=%q err=%v", session, err)
	}
	return id
}

func newGrokCollector(home string, now time.Time) *Collector {
	c := newCollector(grokSourceID, types.RuntimeGrok, &grokScanner{home: home}, nil)
	c.now = func() time.Time { return now }
	return c
}

// TestGrokScannerRosterAndDarkMains covers where a primary comes from, where one
// must NOT come from, and the seam between the two routes into a session.
//
// The walk reaches every session directory on the box and gates on each
// session's own summary.json; the roster reaches its sessions by path and is not
// gated that way at all. Each route therefore finds a session the other cannot,
// and this fixture carries one of each.
func TestGrokScannerRosterAndDarkMains(t *testing.T) {
	now := grokBase()
	tree := newGrokTree(t)

	livePath := tree.summary(grokProjectCWD, grokLiveSession,
		grokMainFields(grokLiveSession, grokProjectCWD, grokBuildAgent, grokModel, "native provenance task 4",
			now.Add(-40*time.Minute), now.Add(-2*time.Minute)),
		now.Add(-2*time.Minute))
	// The measured live shape, and the case a bucket-mtime bound used to lose: an
	// active session in a bucket nobody has added a directory to for hours. A
	// bucket's mtime moves on creation and never on a write, so on this box every
	// bucket is stale while sessions inside them are live.
	hostPath := tree.summary(grokProjectCWD, grokHostSession,
		grokMainFields(grokHostSession, grokProjectCWD, grokBuildAgent, grokModel, "stale bucket, live session",
			now.Add(-30*time.Minute), now.Add(-5*time.Minute)),
		now.Add(-5*time.Minute))
	// Reachable ONLY by the roster: its file has not been written in two hours,
	// so the walk's mtime gate refuses it, while the roster's opened_at says a
	// process took this session twenty minutes ago.
	quietPath := tree.summary(grokProjectCWD, grokQuietFileSession,
		grokMainFields(grokQuietFileSession, grokProjectCWD, grokBuildAgent, grokModel, "quiet file, live process",
			now.Add(-3*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-2*time.Hour))
	// The price of gating the walk on a stat: a file whose mtime is OLDER than
	// the last_active_at recorded inside it. That cannot happen on a live box --
	// summary.json is where that timestamp is written, and 0 of 434 summaries
	// break the relation -- so this is what a restored backup or a copied tree
	// looks like, and it is the one shape the prefilter costs us.
	tree.summary(grokProjectCWD, grokStaleFileSession,
		grokMainFields(grokStaleFileSession, grokProjectCWD, grokBuildAgent, grokModel, "restored backup",
			now.Add(-4*time.Hour), now.Add(-2*time.Minute)),
		now.Add(-2*time.Hour))

	darkPath := tree.summary(grokDaemonCWD, grokDarkSession,
		grokMainFields(grokDarkSession, grokDaemonCWD, grokBuildAgent, grokModel, "pensive daemon v3",
			now.Add(-25*time.Minute), now.Add(-10*time.Minute)),
		now.Add(-10*time.Minute))
	// A main on a profile whose agent_name does not start with grok-build. Every
	// agent_name on disk today shares the "grok" prefix, so this value is
	// synthetic on purpose: it is the only shape that can tell the contract's
	// "grok-build" prefix from the shorter prefix every real value would satisfy.
	secondPath := tree.summary(grokDaemonCWD, grokSecondSession,
		grokMainFields(grokSecondSession, grokDaemonCWD, "grok-plan", grokChildModel, "other profile",
			now.Add(-20*time.Minute), now.Add(-3*time.Minute)),
		now.Add(-3*time.Minute))
	// Fresh file, stale recorded activity: the horizon is the runtime's own claim
	// about when it last did anything, not when the file was touched.
	tree.summary(grokDaemonCWD, grokBuriedSession,
		grokMainFields(grokBuriedSession, grokDaemonCWD, grokBuildAgent, grokModel, "yesterday",
			now.Add(-3*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-time.Minute))
	// A subagent's own session directory, which every subagent on disk has.
	tree.summary(grokDaemonCWD, grokRunningChild,
		grokChildOwnFields(grokRunningChild, grokDaemonCWD, "subagent", now.Add(-time.Minute)),
		now.Add(-time.Minute))

	tree.roster([]map[string]any{
		grokRosterEntryFields(grokLiveSession, grokProjectCWD, now.Add(-20*time.Minute)),
		grokRosterEntryFields(grokQuietFileSession, grokProjectCWD, now.Add(-20*time.Minute)),
	}, now.Add(-20*time.Minute))
	// Both buckets are sealed stale on purpose. Nothing in this test may depend
	// on a bucket's own mtime any more, and a fixture that left them fresh could
	// not tell a walk that ignores them from one that does not.
	tree.seal(grokProjectCWD, now.Add(-3*time.Hour))
	tree.seal(grokDaemonCWD, now.Add(-3*time.Hour))

	nodes, spawns, scanner := scanGrok(t, tree.home, now)

	live, ok := nodeByID(nodes, mustGrokID(t, grokLiveSession))
	if !ok {
		t.Fatalf("grok-roster-session-observed rule violated: session=%s absent ids=%v", grokLiveSession, nodeIDs(nodes))
	}
	if live.SessionID != grokLiveSession || live.Runtime != types.RuntimeGrok || live.Role != types.RolePrimary {
		t.Fatalf("grok-main-identity rule violated: sessionID=%q runtime=%q role=%q want=%q/%q/%q",
			live.SessionID, live.Runtime, live.Role, grokLiveSession, types.RuntimeGrok, types.RolePrimary)
	}
	// "Grok", not the agent_name: the display name is the harness, and printing
	// the profile name would put "grok-build-plan" in the roster column.
	if live.Name != "Grok" {
		t.Fatalf("grok-build-agent-names-the-harness rule violated: name=%q agentName=%q want=%q", live.Name, grokBuildAgent, "Grok")
	}
	if live.Model != grokModel || live.TaskName != "native provenance task 4" || live.Project != "aitop" {
		t.Fatalf("grok-main-content-from-summary rule violated: model=%q taskName=%q project=%q want=%q/%q/%q",
			live.Model, live.TaskName, live.Project, grokModel, "native provenance task 4", "aitop")
	}
	if live.Location != livePath {
		t.Fatalf("grok-main-location-is-summary-file rule violated: location=%q want=%q", live.Location, livePath)
	}
	// The roster's opened_at is when the process took the session; created_at is
	// when the session was first written, which can be an older incarnation.
	if live.StartedAt == nil || !live.StartedAt.Equal(now.Add(-20*time.Minute)) {
		t.Fatalf("grok-roster-startedat-is-opened-at rule violated: startedAt=%v want=%s", live.StartedAt, now.Add(-20*time.Minute))
	}
	// Occupancy owns the process truth for a primary; a native claim here would
	// fight it every tick, and v1 claims liveness only for running children.
	if live.State != "" || live.Exit != "" || live.ExitAt != nil {
		t.Fatalf("grok-main-makes-no-state-or-terminal-claim rule violated: state=%q exit=%q exitAt=%v", live.State, live.Exit, live.ExitAt)
	}

	// The reach a bucket-mtime bound did not have. This session is not in the
	// roster and its bucket has not been touched in three hours, so the walk
	// reaching it is the whole amendment.
	host, ok := nodeByID(nodes, mustGrokID(t, grokHostSession))
	if !ok {
		t.Fatalf("grok-dark-walk-reaches-a-stale-bucket rule violated: session=%s absent, bucketAge=3h summaryAge=5m ids=%v", grokHostSession, nodeIDs(nodes))
	}
	if host.Location != hostPath || host.State != "" {
		t.Fatalf("grok-dark-walk-reaches-a-stale-bucket rule violated: location=%q state=%q want=%q/%q", host.Location, host.State, hostPath, "")
	}

	// The other route, which the walk's gate cannot supply: the file is two
	// hours cold and the roster says a process holds the session.
	quiet, ok := nodeByID(nodes, mustGrokID(t, grokQuietFileSession))
	if !ok {
		t.Fatalf("grok-roster-route-is-not-gated-on-file-mtime rule violated: session=%s absent, summaryAge=2h openedAge=20m ids=%v", grokQuietFileSession, nodeIDs(nodes))
	}
	if quiet.Location != quietPath || quiet.StartedAt == nil || !quiet.StartedAt.Equal(now.Add(-20*time.Minute)) {
		t.Fatalf("grok-roster-route-is-not-gated-on-file-mtime rule violated: location=%q startedAt=%v want=%q/%s", quiet.Location, quiet.StartedAt, quietPath, now.Add(-20*time.Minute))
	}

	dark, ok := nodeByID(nodes, mustGrokID(t, grokDarkSession))
	if !ok {
		t.Fatalf("grok-dark-main-observed rule violated: session=%s absent ids=%v", grokDarkSession, nodeIDs(nodes))
	}
	if dark.Name != "Grok" || dark.Model != grokModel || dark.Project != "pensive" || dark.Location != darkPath {
		t.Fatalf("grok-dark-main-content-from-summary rule violated: name=%q model=%q project=%q location=%q want=%q/%q/%q/%q",
			dark.Name, dark.Model, dark.Project, dark.Location, "Grok", grokModel, "pensive", darkPath)
	}
	// No roster entry means no opened_at, and created_at is the only start the
	// session records about itself.
	if dark.StartedAt == nil || !dark.StartedAt.Equal(now.Add(-25*time.Minute)) {
		t.Fatalf("grok-dark-startedat-is-created-at rule violated: startedAt=%v want=%s", dark.StartedAt, now.Add(-25*time.Minute))
	}
	if dark.State != "" || dark.Exit != "" {
		t.Fatalf("grok-main-makes-no-state-or-terminal-claim rule violated: dark state=%q exit=%q", dark.State, dark.Exit)
	}

	second, ok := nodeByID(nodes, mustGrokID(t, grokSecondSession))
	if !ok {
		t.Fatalf("grok-dark-main-observed rule violated: session=%s absent ids=%v", grokSecondSession, nodeIDs(nodes))
	}
	if second.Name != "" || second.Location != secondPath {
		t.Fatalf("grok-non-build-agent-is-unnamed rule violated: name=%q agentName=%q location=%q want=%q/%q", second.Name, "grok-plan", second.Location, "", secondPath)
	}

	for _, absent := range []struct {
		session string
		reason  string
	}{
		{grokBuriedSession, "last_active_at outside the horizon, however fresh the file is"},
		{grokRunningChild, "its own summary says session_kind=subagent, so it is somebody's child"},
		{grokStaleFileSession, "file mtime outside the horizon, so the walk's stat gate never opens it; the fresh last_active_at inside it is the price of that prefilter"},
	} {
		if seen := countNodeID(nodes, mustGrokID(t, absent.session)); seen != 0 {
			t.Fatalf("grok-dark-walk-admits-only-live-mains rule violated: session=%s count=%d reason=%q ids=%v", absent.session, seen, absent.reason, nodeIDs(nodes))
		}
	}
	if len(nodes) != 5 {
		t.Fatalf("grok-dark-walk-admits-only-live-mains rule violated: nodes=%d want=5 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if len(spawns) != 0 {
		t.Fatalf("grok-no-store-no-edges rule violated: spawns=%d want=0 ids=%v", len(spawns), nodeIDs(nodes))
	}
	// A subagent summary that was READ and classified, versus one that was never
	// walked, are the same absence above. This counter is the difference.
	if seen := scanner.subagentSummaries.Load(); seen != 1 {
		t.Fatalf("grok-subagent-summaries-counted rule violated: subagentSummaries=%d want=1 ids=%v", seen, nodeIDs(nodes))
	}
	// A roster session's summary resolved, so nothing was skipped for being
	// unreadable: an unreachable roster path would show up here instead.
	if skipped := scanner.skippedSummaries.Load(); skipped != 0 {
		t.Fatalf("grok-roster-summary-resolves rule violated: skippedSummaries=%d want=0 ids=%v", skipped, nodeIDs(nodes))
	}
}

// TestGrokScannerChildLifecycle covers every status a meta.json carries and both
// halves of the state rule. The exit rule is deliberately narrow: only
// "completed" and "failed" are terminals, so "cancelled" -- 13 of the 228 files
// on disk -- publishes a node with no claim at all rather than a wrong one, and
// a terminal state is sticky per incarnation once it lands.
func TestGrokScannerChildLifecycle(t *testing.T) {
	now := grokBase()
	tree := newGrokTree(t)

	tree.summary(grokProjectCWD, grokLiveSession,
		grokMainFields(grokLiveSession, grokProjectCWD, grokBuildAgent, grokModel, "lifecycle host",
			now.Add(-40*time.Minute), now.Add(-time.Minute)),
		now.Add(-time.Minute))

	runningPath := tree.meta(grokProjectCWD, grokLiveSession, grokRunningChild,
		grokMetaFields(grokLiveSession, grokRunningChild, "general-purpose", grokChildModel, "survey the disk formats",
			"running", grokProjectCWD, now.Add(-8*time.Minute), ""),
		now.Add(-8*time.Minute))
	completedAt := now.Add(-3 * time.Minute)
	completedPath := tree.meta(grokProjectCWD, grokLiveSession, grokCompletedChild,
		grokMetaFields(grokLiveSession, grokCompletedChild, "explore", grokChildModel, "read the reconciler",
			"completed", grokProjectCWD, now.Add(-20*time.Minute), grokStamp(completedAt)),
		completedAt)
	failedAt := now.Add(-2 * time.Minute)
	tree.meta(grokProjectCWD, grokLiveSession, grokFailedChild,
		grokMetaFields(grokLiveSession, grokFailedChild, "explore", grokChildModel, "build the thing",
			"failed", grokProjectCWD, now.Add(-15*time.Minute), grokStamp(failedAt)),
		failedAt)
	// Cancelled: a real and common status that is not a terminal under the rule.
	tree.meta(grokProjectCWD, grokLiveSession, grokCancelledChild,
		grokMetaFields(grokLiveSession, grokCancelledChild, "explore", grokChildModel, "interrupted",
			"cancelled", grokProjectCWD, now.Add(-12*time.Minute), grokStamp(now.Add(-time.Minute))),
		now.Add(-time.Minute))
	// Inside the horizon, outside the exit window: the core must drop the whole
	// sighting rather than publish a node it would evict moments later.
	tree.meta(grokProjectCWD, grokLiveSession, grokStaleExitChild,
		grokMetaFields(grokLiveSession, grokStaleExitChild, "explore", grokChildModel, "finished a while ago",
			"completed", grokProjectCWD, now.Add(-30*time.Minute), grokStamp(now.Add(-6*time.Minute))),
		now.Add(-6*time.Minute))
	// A terminal nobody can date cannot be proven fresh, so it is not observed.
	tree.meta(grokProjectCWD, grokLiveSession, grokUndatedChild,
		grokMetaFields(grokLiveSession, grokUndatedChild, "explore", grokChildModel, "no completed_at",
			"completed", grokProjectCWD, now.Add(-9*time.Minute), ""),
		now.Add(-time.Minute))
	// Outside the horizon: not observed at all, and it must not reach the core's
	// stale-terminal counter either, because it was never a sighting.
	tree.meta(grokProjectCWD, grokLiveSession, grokBuriedChild,
		grokMetaFields(grokLiveSession, grokBuriedChild, "explore", grokChildModel, "yesterday's job",
			"completed", grokProjectCWD, now.Add(-3*time.Hour), grokStamp(now.Add(-2*time.Hour))),
		now.Add(-2*time.Hour))
	// A child still running past the horizon. meta.json is rewritten at
	// completion (228 of 228 on disk), so its mtime is this child's SPAWN time
	// and a meta-only horizon loses it mid-flight; its own session directory is
	// where a running child keeps writing.
	//
	// Its cwd carries a space on purpose. Finding that second signal means
	// computing a path from child_cwd, so this is the one place left where the
	// encoder's completeness still costs something real: get the escape wrong
	// and a live child quietly ages out mid-flight.
	tree.meta(grokProjectCWD, grokLiveSession, grokLongRunChild,
		grokMetaFields(grokLiveSession, grokLongRunChild, "general-purpose", grokChildModel, "the long one",
			"running", grokSpacedChildCWD, now.Add(-90*time.Minute), ""),
		now.Add(-90*time.Minute))
	tree.summary(grokSpacedChildCWD, grokLongRunChild,
		grokChildOwnFields(grokLongRunChild, grokSpacedChildCWD, "subagent", now.Add(-time.Minute)),
		now.Add(-time.Minute))
	tree.seal(grokSpacedChildCWD, now.Add(-3*time.Hour))

	// A parent whose own summary is outside the horizon, in a bucket the walk
	// does reach. Its running child is live evidence; the parent is not.
	tree.summary(grokDaemonCWD, grokHostSession,
		grokMainFields(grokHostSession, grokDaemonCWD, grokBuildAgent, grokModel, "quiet host",
			now.Add(-5*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-2*time.Hour))
	darkChildPath := tree.meta(grokDaemonCWD, grokHostSession, grokDarkChild,
		grokMetaFields(grokHostSession, grokDarkChild, "explore", grokChildModel, "child of a quiet host",
			"running", grokDaemonCWD, now.Add(-6*time.Minute), ""),
		now.Add(-2*time.Minute))
	// That child's OWN session directory, in the walked bucket, with no
	// session_kind at all: a truncated write, or a grok that predates the field.
	// Nothing then marks the file as a child, so the walk sees a perfectly
	// ordinary main and the only thing keeping one node id from being published
	// under two roles is that the child record is read first.
	tree.summary(grokDaemonCWD, grokDarkChild,
		grokMainFields(grokDarkChild, grokDaemonCWD, grokBuildAgent, grokModel, "unmarked child",
			now.Add(-6*time.Minute), now.Add(-time.Minute)),
		now.Add(-time.Minute))

	tree.roster([]map[string]any{
		grokRosterEntryFields(grokLiveSession, grokProjectCWD, now.Add(-45*time.Minute)),
	}, now.Add(-45*time.Minute))
	tree.seal(grokProjectCWD, now.Add(-3*time.Hour))
	tree.seal(grokDaemonCWD, now.Add(-30*time.Minute))

	nodes, spawns, scanner := scanGrok(t, tree.home, now)

	running, ok := nodeByID(nodes, mustGrokID(t, grokRunningChild))
	if !ok {
		t.Fatalf("grok-running-child-observed rule violated: child=%s absent ids=%v", grokRunningChild, nodeIDs(nodes))
	}
	if running.SessionID != grokRunningChild || running.Role != types.RoleSubagent || running.Runtime != types.RuntimeGrok {
		t.Fatalf("grok-child-identity rule violated: sessionID=%q role=%q runtime=%q want=%q/%q/%q",
			running.SessionID, running.Role, running.Runtime, grokRunningChild, types.RoleSubagent, types.RuntimeGrok)
	}
	if running.Name != "general-purpose" || running.Model != grokChildModel || running.TaskName != "survey the disk formats" || running.Project != "aitop" {
		t.Fatalf("grok-child-content-from-meta rule violated: name=%q model=%q taskName=%q project=%q want=%q/%q/%q/%q",
			running.Name, running.Model, running.TaskName, running.Project, "general-purpose", grokChildModel, "survey the disk formats", "aitop")
	}
	if running.Location != runningPath {
		t.Fatalf("grok-child-location-is-meta-file rule violated: location=%q want=%q", running.Location, runningPath)
	}
	if running.StartedAt == nil || !running.StartedAt.Equal(now.Add(-8*time.Minute)) {
		t.Fatalf("grok-child-startedat-is-started-at rule violated: startedAt=%v want=%s", running.StartedAt, now.Add(-8*time.Minute))
	}
	// Half one of the state rule: running status AND a parent the roster knows.
	if running.State != graph.StateActive || running.Exit != "" {
		t.Fatalf("grok-running-child-of-roster-parent-is-active rule violated: state=%q exit=%q want=%q/%q", running.State, running.Exit, graph.StateActive, "")
	}

	completed, ok := nodeByID(nodes, mustGrokID(t, grokCompletedChild))
	if !ok {
		t.Fatalf("grok-completed-child-observed rule violated: child=%s absent ids=%v", grokCompletedChild, nodeIDs(nodes))
	}
	if completed.Exit != graph.OutcomeCompleted || completed.State != "" {
		t.Fatalf("grok-completed-status-is-the-only-completed-terminal rule violated: exit=%q state=%q want=%q/%q", completed.Exit, completed.State, graph.OutcomeCompleted, "")
	}
	if completed.ExitAt == nil || !completed.ExitAt.Equal(completedAt) {
		t.Fatalf("grok-exitat-is-completed-at rule violated: exitAt=%v want=%s location=%q", completed.ExitAt, completedAt, completed.Location)
	}
	if completed.Location != completedPath {
		t.Fatalf("grok-child-location-is-meta-file rule violated: location=%q want=%q", completed.Location, completedPath)
	}

	failed, ok := nodeByID(nodes, mustGrokID(t, grokFailedChild))
	if !ok {
		t.Fatalf("grok-failed-child-observed rule violated: child=%s absent ids=%v", grokFailedChild, nodeIDs(nodes))
	}
	if failed.Exit != graph.OutcomeFailed || failed.ExitAt == nil || !failed.ExitAt.Equal(failedAt) {
		t.Fatalf("grok-failed-status-is-the-only-failed-terminal rule violated: exit=%q exitAt=%v want=%q/%s", failed.Exit, failed.ExitAt, graph.OutcomeFailed, failedAt)
	}

	// Cancelled is not completed and not failed. A terminal is sticky per
	// incarnation, so inventing one here would be unrecoverable for the node.
	cancelled, ok := nodeByID(nodes, mustGrokID(t, grokCancelledChild))
	if !ok {
		t.Fatalf("grok-cancelled-child-observed rule violated: child=%s absent ids=%v", grokCancelledChild, nodeIDs(nodes))
	}
	if cancelled.Exit != "" || cancelled.ExitAt != nil || cancelled.State != "" {
		t.Fatalf("grok-cancelled-status-claims-nothing rule violated: exit=%q exitAt=%v state=%q", cancelled.Exit, cancelled.ExitAt, cancelled.State)
	}

	longRun, ok := nodeByID(nodes, mustGrokID(t, grokLongRunChild))
	if !ok {
		t.Fatalf("grok-long-running-child-stays-visible rule violated: child=%s absent metaAge=90m ids=%v", grokLongRunChild, nodeIDs(nodes))
	}
	if longRun.State != graph.StateActive {
		t.Fatalf("grok-long-running-child-stays-visible rule violated: state=%q want=%q metaAge=90m ownSummaryAge=1m", longRun.State, graph.StateActive)
	}

	// Half two of the state rule: running status, parent the roster does not
	// know. The child is still published; nothing claims it is alive.
	darkChild, ok := nodeByID(nodes, mustGrokID(t, grokDarkChild))
	if !ok {
		t.Fatalf("grok-child-of-roster-absent-parent-observed rule violated: child=%s absent ids=%v", grokDarkChild, nodeIDs(nodes))
	}
	if darkChild.State != "" || darkChild.Exit != "" {
		t.Fatalf("grok-running-child-of-roster-absent-parent-claims-nothing rule violated: state=%q exit=%q parent=%s", darkChild.State, darkChild.Exit, grokHostSession)
	}
	// The same node reachable two ways: as this child, and as the unmarked main
	// its own directory looks like. The child record wins, because it is the only
	// one of the two that knows the role, the parent and the lifecycle.
	if darkChild.Role != types.RoleSubagent || darkChild.Name != "explore" || darkChild.Location != darkChildPath {
		t.Fatalf("grok-child-record-outranks-its-own-summary rule violated: role=%q name=%q location=%q want=%q/%q/%q",
			darkChild.Role, darkChild.Name, darkChild.Location, types.RoleSubagent, "explore", darkChildPath)
	}
	// The parent is out of horizon and never published from its own summary, so
	// the edge needs an endpoint: minimal, anchored on the child's meta file.
	hostParent, ok := nodeByID(nodes, mustGrokID(t, grokHostSession))
	if !ok {
		t.Fatalf("grok-stale-parent-synthesized-for-live-child rule violated: parent=%s absent ids=%v", grokHostSession, nodeIDs(nodes))
	}
	if hostParent.Role != types.RolePrimary || hostParent.Name != "" || hostParent.Location != darkChildPath {
		t.Fatalf("grok-synthesized-parent-is-minimal rule violated: role=%q name=%q location=%q want=%q/%q/%q",
			hostParent.Role, hostParent.Name, hostParent.Location, types.RolePrimary, "", darkChildPath)
	}
	// Roster absence is not death evidence: active_sessions.json is rewritten
	// live, and a torn read would otherwise vanish every parent on the box for a
	// tick, sticking a wrong terminal on each one.
	if hostParent.Exit != "" || hostParent.ExitAt != nil {
		t.Fatalf("grok-roster-absence-is-not-death rule violated: exit=%q exitAt=%v parent=%s", hostParent.Exit, hostParent.ExitAt, grokHostSession)
	}

	if seen := countNodeID(nodes, mustGrokID(t, grokBuriedChild)); seen != 0 {
		t.Fatalf("grok-child-horizon-skips-stale rule violated: child=%s count=%d metaAge=2h horizon=%s ids=%v", grokBuriedChild, seen, nativeHorizon, nodeIDs(nodes))
	}
	if len(nodes) != 10 {
		t.Fatalf("grok-lifecycle-fixture-node-count rule violated: nodes=%d want=10 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if len(spawns) != 8 {
		t.Fatalf("grok-lifecycle-fixture-spawn-count rule violated: spawns=%d want=8", len(spawns))
	}
	if skipped := scanner.skippedChildren.Load(); skipped != 0 {
		t.Fatalf("grok-well-formed-metas-are-not-skipped rule violated: skippedChildren=%d want=0 ids=%v", skipped, nodeIDs(nodes))
	}

	// End to end: the core decides which sightings become events, and the two
	// undatable-or-old terminals must never reach the store at all.
	sink := &recordingSink{}
	collector := newGrokCollector(tree.home, now)
	collector.tick(sink)
	events, invalid := sink.snapshot()
	if len(invalid) != 0 {
		t.Fatalf("grok-emitted-events-validate rule violated: invalid=%v kinds=%v", invalid, kindsOf(events))
	}
	for _, dropped := range []struct {
		session string
		reason  string
	}{
		{grokStaleExitChild, "completed 6m ago, outside the 4m exit window"},
		{grokUndatedChild, "completed with no completed_at, so the terminal cannot be dated"},
		{grokBuriedChild, "meta 2h old, outside the horizon"},
	} {
		id := mustGrokID(t, dropped.session)
		for _, ev := range events {
			if ev.Actor == id || ev.Target == id {
				t.Fatalf("grok-unobservable-terminal-is-not-published rule violated: kind=%s actor=%s target=%s reason=%q", ev.Kind, ev.Actor, ev.Target, dropped.reason)
			}
		}
	}
	if published := countKind(events, graph.EventNodeObserved); published != 8 {
		t.Fatalf("grok-lifecycle-published-node-count rule violated: node_observed=%d want=8 kinds=%v", published, kindsOf(events))
	}
	if states := countKind(events, graph.EventStateObserved); states != 2 {
		t.Fatalf("grok-only-running-children-claim-state rule violated: state_observed=%d want=2 kinds=%v", states, kindsOf(events))
	}
	// Every state claim needs a heartbeat lane or it decays inside 6s and the
	// node reads Stale two ticks later.
	if beats := countKind(events, graph.EventHeartbeatObserved); beats != 2 {
		t.Fatalf("grok-state-claims-get-heartbeat-lanes rule violated: heartbeat_observed=%d state_observed=%d kinds=%v", beats, countKind(events, graph.EventStateObserved), kindsOf(events))
	}
	if exits := countKind(events, graph.EventExitObserved); exits != 2 {
		t.Fatalf("grok-only-completed-and-failed-exit rule violated: exit_observed=%d want=2 kinds=%v", exits, kindsOf(events))
	}
	if edges := countKind(events, graph.EventRelationshipObserved); edges != 6 {
		t.Fatalf("grok-edges-need-both-endpoints-published rule violated: relationship_observed=%d want=6 kinds=%v", edges, kindsOf(events))
	}
	// Two, not three: the buried child was never a sighting, so it cannot be a
	// dropped terminal. Counting it here would mean the horizon had stopped
	// working and the exit window was covering for it.
	if stale := collector.staleTerminals.Load(); stale != 2 {
		t.Fatalf("grok-stale-terminals-counted rule violated: staleTerminals=%d want=2 events=%d", stale, len(events))
	}
	if rejected := collector.Disp().Rejected.Load(); rejected != 0 {
		t.Fatalf("grok-collector-emits-nothing-rejected rule violated: rejected=%d published=%d", rejected, collector.Disp().Published.Load())
	}
}

// TestGrokScannerSpawnEdges covers the parent-child link and the rule that binds
// it: an endpoint the reconciler does not have makes the edge disappear without
// anything appearing in the dispositions, and a node synthesized for a child
// nobody published would sit in the graph forever with no edge, no state and no
// terminal to end it.
func TestGrokScannerSpawnEdges(t *testing.T) {
	now := grokBase()
	tree := newGrokTree(t)

	tree.summary(grokProjectCWD, grokLiveSession,
		grokMainFields(grokLiveSession, grokProjectCWD, grokBuildAgent, grokModel, "spawn host",
			now.Add(-40*time.Minute), now.Add(-time.Minute)),
		now.Add(-time.Minute))
	runningPath := tree.meta(grokProjectCWD, grokLiveSession, grokRunningChild,
		grokMetaFields(grokLiveSession, grokRunningChild, "general-purpose", grokChildModel, "ordinary child",
			"running", grokProjectCWD, now.Add(-5*time.Minute), ""),
		now.Add(-5*time.Minute))

	// The one fixture whose subagent_id and child_session_id differ. They are
	// equal in all 228 files on disk, so no real shape can tell "label the edge
	// with subagent_id" from "label it with the child's session id"; this one is
	// synthetic for exactly that reason, and the node id must still come from
	// child_session_id.
	relabeled := grokMetaFields(grokLiveSession, grokRelabeledChild, "explore", grokChildModel, "relabelled",
		"running", grokProjectCWD, now.Add(-4*time.Minute), "")
	relabeled["subagent_id"] = grokRelabelID
	tree.meta(grokProjectCWD, grokLiveSession, grokRelabeledChild, relabeled, now.Add(-4*time.Minute))

	// A meta whose child names the enclosing session itself. The reconciler
	// rejects self-edges, and publishing it would replace the parent's own node
	// with a subagent sighting carrying the same id.
	tree.meta(grokProjectCWD, grokLiveSession, grokLiveSession,
		grokMetaFields(grokLiveSession, grokLiveSession, "explore", grokChildModel, "myself",
			"running", grokProjectCWD, now.Add(-3*time.Minute), ""),
		now.Add(-3*time.Minute))

	// A meta with no child id: it cannot name a node, so it cannot name an
	// endpoint either.
	empty := grokMetaFields(grokLiveSession, grokEmptyIDChild, "explore", grokChildModel, "no child id",
		"running", grokProjectCWD, now.Add(-3*time.Minute), "")
	empty["child_session_id"] = ""
	tree.meta(grokProjectCWD, grokLiveSession, grokEmptyIDChild, empty, now.Add(-3*time.Minute))

	// A meta that claims a parent other than the directory it lives in. The two
	// agree on all 228 files on disk; when they disagree the record was moved or
	// copied, and neither reading is safe: the directory would attach the edge to
	// a session that never spawned it, the field would name an endpoint nobody
	// published. The child is still a real child, so it publishes without an edge.
	mismatched := grokMetaFields(grokLiveSession, grokMismatchedChild, "explore", grokChildModel, "wrong parent",
		"running", grokProjectCWD, now.Add(-2*time.Minute), "")
	mismatched["parent_session_id"] = grokSecondSession
	tree.meta(grokProjectCWD, grokLiveSession, grokMismatchedChild, mismatched, now.Add(-2*time.Minute))

	// A store whose parent summary is stale and whose only child is live: the
	// parent is synthesized so the edge has an endpoint.
	tree.summary(grokDaemonCWD, grokHostSession,
		grokMainFields(grokHostSession, grokDaemonCWD, grokBuildAgent, grokModel, "quiet host",
			now.Add(-5*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-2*time.Hour))
	darkChildPath := tree.meta(grokDaemonCWD, grokHostSession, grokDarkChild,
		grokMetaFields(grokHostSession, grokDarkChild, "explore", grokChildModel, "live child",
			"running", grokDaemonCWD, now.Add(-6*time.Minute), ""),
		now.Add(-2*time.Minute))

	// A store where nothing is live. Synthesizing a parent here would publish a
	// node for a child this scan refused to publish.
	tree.summary(grokDaemonCWD, grokBuriedSession,
		grokMainFields(grokBuriedSession, grokDaemonCWD, grokBuildAgent, grokModel, "buried host",
			now.Add(-6*time.Hour), now.Add(-3*time.Hour)),
		now.Add(-3*time.Hour))
	tree.meta(grokDaemonCWD, grokBuriedSession, grokBuriedChild,
		grokMetaFields(grokBuriedSession, grokBuriedChild, "explore", grokChildModel, "buried child",
			"completed", grokDaemonCWD, now.Add(-4*time.Hour), grokStamp(now.Add(-3*time.Hour))),
		now.Add(-3*time.Hour))

	// A store whose parent summary is stale and whose only child sits INSIDE the
	// horizon and OUTSIDE the exit window. The child is a sighting the core will
	// drop, so a parent synthesized for it would outlive the only child it exists
	// for, with no edge, no state and no terminal to end it. The two bounds are
	// different rules and this is the only fixture where they disagree.
	tree.summary(grokDaemonCWD, grokSecondSession,
		grokMainFields(grokSecondSession, grokDaemonCWD, grokBuildAgent, grokModel, "ended host",
			now.Add(-5*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-2*time.Hour))
	tree.meta(grokDaemonCWD, grokSecondSession, grokStaleExitChild,
		grokMetaFields(grokSecondSession, grokStaleExitChild, "explore", grokChildModel, "finished a while ago",
			"completed", grokDaemonCWD, now.Add(-30*time.Minute), grokStamp(now.Add(-6*time.Minute))),
		now.Add(-6*time.Minute))

	tree.roster([]map[string]any{
		grokRosterEntryFields(grokLiveSession, grokProjectCWD, now.Add(-45*time.Minute)),
	}, now.Add(-45*time.Minute))
	tree.seal(grokProjectCWD, now.Add(-3*time.Hour))
	tree.seal(grokDaemonCWD, now.Add(-20*time.Minute))

	nodes, spawns, scanner := scanGrok(t, tree.home, now)

	parent := mustGrokID(t, grokLiveSession)
	ordinary, ok := spawnByChild(spawns, mustGrokID(t, grokRunningChild))
	if !ok {
		t.Fatalf("grok-spawn-edge-present rule violated: child=%s absent spawns=%d", grokRunningChild, len(spawns))
	}
	if ordinary.ParentID != parent || ordinary.ParentSessionID != grokLiveSession {
		t.Fatalf("grok-spawn-parent-is-enclosing-session rule violated: parentID=%q parentSessionID=%q want=%q/%q",
			ordinary.ParentID, ordinary.ParentSessionID, parent, grokLiveSession)
	}
	if ordinary.Relationship != graph.RelationshipID(grokRunningChild) || ordinary.Location != runningPath {
		t.Fatalf("grok-relationship-is-subagent-id rule violated: relationship=%q location=%q want=%q/%q",
			ordinary.Relationship, ordinary.Location, grokRunningChild, runningPath)
	}

	relabel, ok := spawnByChild(spawns, mustGrokID(t, grokRelabeledChild))
	if !ok {
		t.Fatalf("grok-spawn-edge-present rule violated: child=%s absent spawns=%d", grokRelabeledChild, len(spawns))
	}
	if relabel.Relationship != graph.RelationshipID(grokRelabelID) {
		t.Fatalf("grok-relationship-is-subagent-id rule violated: relationship=%q want=%q childSessionID=%q",
			relabel.Relationship, grokRelabelID, grokRelabeledChild)
	}
	if relabel.ChildSessionID != grokRelabeledChild {
		t.Fatalf("grok-child-node-is-child-session-id rule violated: childSessionID=%q want=%q relationship=%q",
			relabel.ChildSessionID, grokRelabeledChild, relabel.Relationship)
	}

	dark, ok := spawnByChild(spawns, mustGrokID(t, grokDarkChild))
	if !ok {
		t.Fatalf("grok-spawn-edge-present rule violated: child=%s absent spawns=%d", grokDarkChild, len(spawns))
	}
	if dark.ParentID != mustGrokID(t, grokHostSession) || dark.Location != darkChildPath {
		t.Fatalf("grok-spawn-parent-is-enclosing-session rule violated: parentID=%q location=%q want=%q/%q",
			dark.ParentID, dark.Location, mustGrokID(t, grokHostSession), darkChildPath)
	}

	if _, ok := spawnByChild(spawns, mustGrokID(t, grokMismatchedChild)); ok {
		t.Fatalf("grok-meta-parent-must-match-its-directory rule violated: child=%s has an edge metaParent=%s dirParent=%s",
			grokMismatchedChild, grokSecondSession, grokLiveSession)
	}
	if _, ok := nodeByID(nodes, mustGrokID(t, grokMismatchedChild)); !ok {
		t.Fatalf("grok-mismatched-parent-still-publishes-the-child rule violated: child=%s absent ids=%v", grokMismatchedChild, nodeIDs(nodes))
	}
	// A store whose only child was skipped must leave nothing behind.
	for _, absent := range []struct {
		session string
		reason  string
	}{
		{grokBuriedSession, "every child in its store is outside the horizon"},
		{grokBuriedChild, "outside the horizon"},
		{grokEmptyIDChild, "meta.json has no child_session_id"},
		{grokSecondSession, "its only child is inside the horizon but outside the exit window, so the core drops that child"},
	} {
		if seen := countNodeID(nodes, mustGrokID(t, absent.session)); seen != 0 {
			t.Fatalf("grok-skipped-child-synthesizes-no-parent rule violated: session=%s count=%d reason=%q ids=%v", absent.session, seen, absent.reason, nodeIDs(nodes))
		}
	}
	// Scanned once, published once: a parent synthesized on top of its own
	// summary would replace real content with a nameless stub.
	if seen := countNodeID(nodes, parent); seen != 1 {
		t.Fatalf("grok-published-parent-not-duplicated rule violated: parent=%s count=%d want=1 ids=%v", grokLiveSession, seen, nodeIDs(nodes))
	}
	// Seven: the roster main, its three publishable children, the live dark child
	// and the parent synthesized for it, and the child the core will drop for its
	// stale terminal, which is a sighting here and a node nowhere.
	if len(nodes) != 7 {
		t.Fatalf("grok-spawn-fixture-node-count rule violated: nodes=%d want=7 ids=%v", len(nodes), nodeIDs(nodes))
	}
	if len(spawns) != 4 {
		t.Fatalf("grok-spawn-fixture-edge-count rule violated: spawns=%d want=4 ids=%v", len(spawns), nodeIDs(nodes))
	}
	// Two unusable metas: the self-naming one and the one with no child id. Both
	// are dropped for reasons the node counts above cannot distinguish.
	if skipped := scanner.skippedChildren.Load(); skipped != 2 {
		t.Fatalf("grok-unusable-metas-counted rule violated: skippedChildren=%d want=2 (self-named, empty child id) ids=%v", skipped, nodeIDs(nodes))
	}
	if skipped := scanner.skippedSpawns.Load(); skipped != 1 {
		t.Fatalf("grok-mismatched-parent-counted rule violated: skippedSpawns=%d want=1 ids=%v", skipped, nodeIDs(nodes))
	}

	for _, spawn := range spawns {
		if spawn.ParentID == spawn.ChildID {
			t.Fatalf("grok-spawn-is-not-a-self-edge rule violated: parent=%q child=%q relationship=%q", spawn.ParentID, spawn.ChildID, spawn.Relationship)
		}
		if strings.Contains(string(spawn.Relationship), ":") {
			t.Fatalf("grok-relationship-is-short-id-not-node-id rule violated: relationship=%q child=%q", spawn.Relationship, spawn.ChildID)
		}
		if len(spawn.Relationship) == 0 || len(spawn.Relationship) > maxRelationshipBytes {
			t.Fatalf("grok-relationship-length-bound rule violated: relationship=%q bytes=%d limit=%d", spawn.Relationship, len(spawn.Relationship), maxRelationshipBytes)
		}
	}
}

// TestGrokScannerCWDEncoding pins the path arithmetic that finds a roster
// session at all. grok escapes both slashes and spaces in a bucket name, so the
// scanner does too: encoding only slashes resolves a spaced cwd to a path that
// does not exist and loses the session entirely, which is 1 of the 434 sessions
// on the live box. The walk reads directory names off disk and never computes a
// path, so it is unaffected either way; that is the second fixture here.
func TestGrokScannerCWDEncoding(t *testing.T) {
	now := grokBase()
	tree := newGrokTree(t)

	const plainCWD = "/home/x"
	encodedPath := tree.summary(plainCWD, grokLiveSession,
		grokMainFields(grokLiveSession, plainCWD, grokBuildAgent, grokModel, "percent encoded",
			now.Add(-30*time.Minute), now.Add(-2*time.Minute)),
		now.Add(-2*time.Minute))
	// Decoys, both stale so only a roster lookup could reach them: the Claude
	// project slug, and the encoding with the leading slash dropped. A scanner
	// that reused either would find a session with the right id and the wrong
	// content, which no presence check can see.
	tree.rawSummary("-home-x", grokLiveSession,
		grokMainFields(grokLiveSession, plainCWD, grokBuildAgent, "DECOY-claude-slug", "claude slug decoy",
			now.Add(-30*time.Minute), now.Add(-2*time.Minute)),
		now.Add(-2*time.Minute))
	tree.rawSummary("home%2Fx", grokLiveSession,
		grokMainFields(grokLiveSession, plainCWD, grokBuildAgent, "DECOY-no-leading-slash", "no leading slash decoy",
			now.Add(-30*time.Minute), now.Add(-2*time.Minute)),
		now.Add(-2*time.Minute))

	// A cwd with a space, written where grok writes it: %20 for the space. This
	// is the shape that slash-only encoding loses, and 1 of the 434 sessions on
	// the live box has it.
	//
	// Its file is deliberately two hours cold, so the walk's mtime gate refuses
	// it and the roster is the ONLY route in. With a fresh file the walk supplies
	// the node whatever the encoder computes, and a broken encoder is then
	// visible only in a counter -- which is how the first run of this fixture
	// behaved, and why it now looks like this.
	const spacedCWD = "/home/x/Obsidian Vault"
	spacedPath := tree.summary(spacedCWD, grokSecondSession,
		grokMainFields(grokSecondSession, spacedCWD, grokBuildAgent, grokModel, "vault session",
			now.Add(-3*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-2*time.Hour))

	// The same spaced encoding reached the other way, by a walk that never
	// computes a path at all. Keeping both routes in one fixture is what
	// separates "the encoder is right" from "the walk happened to find it".
	const walkedCWD = "/home/y/Heph Journal"
	walkedPath := tree.summary(walkedCWD, grokDarkSession,
		grokMainFields(grokDarkSession, walkedCWD, grokBuildAgent, grokModel, "journal session",
			now.Add(-25*time.Minute), now.Add(-4*time.Minute)),
		now.Add(-4*time.Minute))

	// The encoder is KNOWN incomplete: it claims the two escapes the corpus
	// proves and no more. This session's cwd carries a colon, so the roster route
	// computes a path that is not where the session lives. Whatever escape grok
	// really uses here does not matter to the test; what matters is that it is
	// not the one the encoder computes, which is the general shape of every
	// character the corpus never showed us.
	//
	// The walk finds it regardless. What must ALSO survive is the roster entry
	// following the session to the directory that exists, because the state rule
	// needs a rostered parent: this child is what would silently stop being
	// claimed if the two routes filed two visits.
	const colonCWD = "/home/x/ws:1"
	const colonBucket = "%2Fhome%2Fx%2Fws%3A1"
	colonPath := tree.rawSummary(colonBucket, grokColonSession,
		grokMainFields(grokColonSession, colonCWD, grokBuildAgent, grokModel, "colon cwd",
			now.Add(-30*time.Minute), now.Add(-2*time.Minute)),
		now.Add(-2*time.Minute))
	colonChildPath := tree.rawMeta(colonBucket, grokColonSession, grokRunningChild,
		grokMetaFields(grokColonSession, grokRunningChild, "general-purpose", grokChildModel, "child of a colon cwd",
			"running", colonCWD, now.Add(-5*time.Minute), ""),
		now.Add(-5*time.Minute))
	tree.sealRaw(colonBucket, now.Add(-3*time.Hour))

	// A roster entry whose session directory holds no summary.json: what the
	// window between opening a session and writing its first summary looks like,
	// and the only thing in this fixture that may move the skip counter.
	tree.roster([]map[string]any{
		grokRosterEntryFields(grokLiveSession, plainCWD, now.Add(-20*time.Minute)),
		grokRosterEntryFields(grokSecondSession, spacedCWD, now.Add(-18*time.Minute)),
		grokRosterEntryFields(grokColonSession, colonCWD, now.Add(-15*time.Minute)),
		grokRosterEntryFields(grokNoSummarySession, plainCWD, now.Add(-1*time.Minute)),
	}, now.Add(-18*time.Minute))
	tree.seal(plainCWD, now.Add(-3*time.Hour))
	tree.sealRaw("-home-x", now.Add(-3*time.Hour))
	tree.sealRaw("home%2Fx", now.Add(-3*time.Hour))
	tree.seal(spacedCWD, now.Add(-3*time.Hour))
	tree.seal(walkedCWD, now.Add(-4*time.Minute))

	nodes, _, scanner := scanGrok(t, tree.home, now)

	live, ok := nodeByID(nodes, mustGrokID(t, grokLiveSession))
	if !ok {
		t.Fatalf("grok-roster-path-is-percent-encoded-cwd rule violated: session=%s absent want dir=%q ids=%v",
			grokLiveSession, strings.ReplaceAll(plainCWD, "/", "%2F"), nodeIDs(nodes))
	}
	if live.Location != encodedPath {
		t.Fatalf("grok-roster-path-is-percent-encoded-cwd rule violated: location=%q want=%q", live.Location, encodedPath)
	}
	if live.Model != grokModel || live.Project != "x" {
		t.Fatalf("grok-roster-path-reads-the-encoded-summary rule violated: model=%q project=%q want=%q/%q (a DECOY model means a wrong bucket was read)",
			live.Model, live.Project, grokModel, "x")
	}

	spaced, ok := nodeByID(nodes, mustGrokID(t, grokSecondSession))
	if !ok {
		t.Fatalf("grok-spaced-cwd-is-reachable-by-roster-path rule violated: session=%s absent cwd=%q want dir=%q ids=%v",
			grokSecondSession, spacedCWD, grokDiskEncode(spacedCWD), nodeIDs(nodes))
	}
	if spaced.Location != spacedPath || spaced.Project != "Obsidian Vault" {
		t.Fatalf("grok-spaced-cwd-is-reachable-by-roster-path rule violated: location=%q project=%q want=%q/%q", spaced.Location, spaced.Project, spacedPath, "Obsidian Vault")
	}
	// The finding this fixture exists for. The session is found either way, so a
	// presence check proves nothing; the loss an incomplete encoder used to cause
	// was the CLAIM, because the roster entry stayed behind on a visit pointing
	// at a directory that does not exist and the child's parent then read as
	// unrostered.
	colon, ok := nodeByID(nodes, mustGrokID(t, grokColonSession))
	if !ok {
		t.Fatalf("grok-unencodable-cwd-is-still-observed rule violated: session=%s absent bucket=%q ids=%v", grokColonSession, colonBucket, nodeIDs(nodes))
	}
	if colon.Location != colonPath {
		t.Fatalf("grok-unencodable-cwd-is-still-observed rule violated: location=%q want=%q", colon.Location, colonPath)
	}
	colonChild, ok := nodeByID(nodes, mustGrokID(t, grokRunningChild))
	if !ok {
		t.Fatalf("grok-unencodable-cwd-keeps-its-children rule violated: child=%s absent parent=%s ids=%v", grokRunningChild, grokColonSession, nodeIDs(nodes))
	}
	if colonChild.State != graph.StateActive || colonChild.Location != colonChildPath {
		t.Fatalf("grok-state-rule-does-not-depend-on-the-encoder rule violated: state=%q location=%q want=%q/%q (the roster entry must follow its session to the directory that exists)",
			colonChild.State, colonChild.Location, graph.StateActive, colonChildPath)
	}

	// One, and exactly one: the entry with no summary.json on disk. This counter
	// is what told us the spaced cwd was being lost in the first place, so it is
	// pinned to a number rather than to zero -- a 2 here means the space is being
	// dropped again, and a 0 means the counter has stopped seeing anything at
	// all. Both readings are wrong and both are visible.
	if skipped := scanner.skippedSummaries.Load(); skipped != 1 {
		t.Fatalf("grok-unreadable-roster-summary-counted rule violated: skippedSummaries=%d want=1 (the entry with no summary.json, and NOT the spaced cwd %q) ids=%v", skipped, spacedCWD, nodeIDs(nodes))
	}
	if seen := countNodeID(nodes, mustGrokID(t, grokNoSummarySession)); seen != 0 {
		t.Fatalf("grok-roster-entry-without-a-summary-publishes-nothing rule violated: session=%s count=%d ids=%v", grokNoSummarySession, seen, nodeIDs(nodes))
	}

	walked, ok := nodeByID(nodes, mustGrokID(t, grokDarkSession))
	if !ok {
		t.Fatalf("grok-bucket-walk-needs-no-encoding rule violated: session=%s absent bucket=%q ids=%v", grokDarkSession, grokDiskEncode(walkedCWD), nodeIDs(nodes))
	}
	if walked.Location != walkedPath || walked.Project != "Heph Journal" {
		t.Fatalf("grok-bucket-walk-needs-no-encoding rule violated: location=%q project=%q want=%q/%q", walked.Location, walked.Project, walkedPath, "Heph Journal")
	}
	// Five, not seven: the two decoy buckets hold the same session id as the real
	// one, so a scanner that read them all would still publish one node, and only
	// the location and content assertions above can tell which file it read.
	if len(nodes) != 5 {
		t.Fatalf("grok-encoding-fixture-node-count rule violated: nodes=%d want=5 ids=%v", len(nodes), nodeIDs(nodes))
	}
}

// TestGrokScannerToleratesRosterShapes covers the roster's two encodings and,
// more importantly, what a missing or unreadable one may cost. active_sessions.
// json is rewritten live: a torn read must not vanish every session on the box,
// and a state claim is the only thing the roster is allowed to gate.
func TestGrokScannerToleratesRosterShapes(t *testing.T) {
	now := grokBase()

	// build lays down one roster session with a running child plus one dark main
	// that no roster mentions. The dark main is the canary: every case below
	// asserts something about what is ABSENT, and a scan that walked nothing at
	// all would satisfy all of it.
	build := func(t *testing.T) *grokTree {
		t.Helper()
		tree := newGrokTree(t)
		tree.summary(grokProjectCWD, grokLiveSession,
			grokMainFields(grokLiveSession, grokProjectCWD, grokBuildAgent, grokModel, "roster host",
				now.Add(-40*time.Minute), now.Add(-time.Minute)),
			now.Add(-time.Minute))
		tree.meta(grokProjectCWD, grokLiveSession, grokRunningChild,
			grokMetaFields(grokLiveSession, grokRunningChild, "general-purpose", grokChildModel, "running child",
				"running", grokProjectCWD, now.Add(-5*time.Minute), ""),
			now.Add(-5*time.Minute))
		tree.summary(grokProjectCWD, grokDarkSession,
			grokMainFields(grokDarkSession, grokProjectCWD, grokBuildAgent, grokModel, "dark canary",
				now.Add(-30*time.Minute), now.Add(-6*time.Minute)),
			now.Add(-6*time.Minute))
		return tree
	}
	// The bucket is fresh in every case here, so the roster is never the only way
	// to reach these sessions and its absence changes claims rather than coverage.
	seal := func(tree *grokTree) { tree.seal(grokProjectCWD, now.Add(-2*time.Minute)) }

	assertCanary := func(t *testing.T, nodes []NodeSighting, label string) {
		t.Helper()
		if _, ok := nodeByID(nodes, mustGrokID(t, grokDarkSession)); !ok {
			t.Fatalf("grok-roster-case-canary rule violated: case=%q dark main=%s absent, so every absence below proves nothing ids=%v", label, grokDarkSession, nodeIDs(nodes))
		}
		if seen := countNodeID(nodes, mustGrokID(t, grokLiveSession)); seen != 1 {
			t.Fatalf("grok-session-published-once rule violated: case=%q session=%s count=%d want=1 ids=%v", label, grokLiveSession, seen, nodeIDs(nodes))
		}
	}

	t.Run("array", func(t *testing.T) {
		tree := build(t)
		tree.roster([]map[string]any{grokRosterEntryFields(grokLiveSession, grokProjectCWD, now.Add(-20*time.Minute))}, now.Add(-20*time.Minute))
		seal(tree)
		nodes, _, scanner := scanGrok(t, tree.home, now)
		assertCanary(t, nodes, "array")
		child, ok := nodeByID(nodes, mustGrokID(t, grokRunningChild))
		if !ok || child.State != graph.StateActive {
			t.Fatalf("grok-array-roster-parses rule violated: child present=%t state=%q want=%q ids=%v", ok, child.State, graph.StateActive, nodeIDs(nodes))
		}
		if skipped := scanner.skippedRoster.Load(); skipped != 0 {
			t.Fatalf("grok-array-roster-parses rule violated: skippedRoster=%d want=0", skipped)
		}
	})

	t.Run("map", func(t *testing.T) {
		tree := build(t)
		// The map form, mirroring the tolerance in internal/overlay/grok.
		tree.rawRoster(tree.encode(map[string]any{
			grokLiveSession: grokRosterEntryFields(grokLiveSession, grokProjectCWD, now.Add(-20*time.Minute)),
		}), now.Add(-20*time.Minute))
		seal(tree)
		nodes, _, scanner := scanGrok(t, tree.home, now)
		assertCanary(t, nodes, "map")
		child, ok := nodeByID(nodes, mustGrokID(t, grokRunningChild))
		if !ok || child.State != graph.StateActive {
			t.Fatalf("grok-map-roster-parses rule violated: child present=%t state=%q want=%q ids=%v", ok, child.State, graph.StateActive, nodeIDs(nodes))
		}
		if skipped := scanner.skippedRoster.Load(); skipped != 0 {
			t.Fatalf("grok-map-roster-parses rule violated: skippedRoster=%d want=0", skipped)
		}
	})

	for _, broken := range []struct {
		name    string
		body    string
		write   bool
		skipped uint64
	}{
		{name: "missing", write: false, skipped: 0},
		{name: "unparseable", body: "{ this is not json", write: true, skipped: 1},
		{name: "truncated-array", body: `[{"session_id":"01a05553-afa1-7e`, write: true, skipped: 1},
	} {
		t.Run(broken.name, func(t *testing.T) {
			tree := build(t)
			if broken.write {
				tree.rawRoster(broken.body, now.Add(-20*time.Minute))
			}
			seal(tree)
			nodes, spawns, scanner := scanGrok(t, tree.home, now)
			assertCanary(t, nodes, broken.name)
			// Coverage survives: what the disk shows is still observed.
			child, ok := nodeByID(nodes, mustGrokID(t, grokRunningChild))
			if !ok {
				t.Fatalf("grok-no-roster-still-observes-the-disk rule violated: case=%q child=%s absent ids=%v", broken.name, grokRunningChild, nodeIDs(nodes))
			}
			// Claims do not: with no roster, no parent is known to be live.
			if child.State != "" {
				t.Fatalf("grok-no-roster-makes-no-state-claim rule violated: case=%q state=%q want=%q", broken.name, child.State, "")
			}
			// And nothing dies. A torn read of a file that is rewritten live must
			// never mark the box's sessions gone; a terminal is sticky per
			// incarnation, so a mass vanish would be unrecoverable.
			for _, node := range nodes {
				if node.Exit != "" {
					t.Fatalf("grok-no-roster-is-not-a-mass-vanish rule violated: case=%q id=%s exit=%q exitAt=%v", broken.name, node.ID, node.Exit, node.ExitAt)
				}
			}
			if len(spawns) != 1 {
				t.Fatalf("grok-no-roster-still-observes-the-disk rule violated: case=%q spawns=%d want=1", broken.name, len(spawns))
			}
			if skipped := scanner.skippedRoster.Load(); skipped != broken.skipped {
				t.Fatalf("grok-unreadable-roster-counted rule violated: case=%q skippedRoster=%d want=%d (a missing file is ordinary, a corrupt one is not)", broken.name, skipped, broken.skipped)
			}
		})
	}

	t.Run("empty-home", func(t *testing.T) {
		nodes, spawns, scanner := scanGrok(t, t.TempDir(), now)
		if len(nodes) != 0 || len(spawns) != 0 || scanner.skippedRoster.Load() != 0 || scanner.skippedSummaries.Load() != 0 {
			t.Fatalf("grok-absent-home-is-not-an-error rule violated: nodes=%d spawns=%d skippedRoster=%d skippedSummaries=%d",
				len(nodes), len(spawns), scanner.skippedRoster.Load(), scanner.skippedSummaries.Load())
		}
	})
}

// TestGrokCollectorLandsSubagentInRealShadow judges the lane by what the
// production reconciler KEPT. Endpoint identity is enforced reconciler-side and
// is invisible in the dispositions, so a scanner that names an endpoint the
// store does not have looks perfectly healthy from inside this package.
func TestGrokCollectorLandsSubagentInRealShadow(t *testing.T) {
	// The reconciler dates freshness from the wall clock, so this fixture lives
	// in real time: a state claim stamped at a synthetic hour arrives already
	// decayed and the run would certify an emission it never judged.
	now := time.Now().UTC()
	tree := newGrokTree(t)
	tree.summary(grokProjectCWD, grokLiveSession,
		grokMainFields(grokLiveSession, grokProjectCWD, grokBuildAgent, grokModel, "shadow host",
			now.Add(-40*time.Minute), now.Add(-time.Minute)),
		now.Add(-time.Minute))
	tree.meta(grokProjectCWD, grokLiveSession, grokRunningChild,
		grokMetaFields(grokLiveSession, grokRunningChild, "general-purpose", grokChildModel, "the running one",
			"running", grokProjectCWD, now.Add(-5*time.Minute), ""),
		now.Add(-5*time.Minute))
	completedAt := now.Add(-time.Minute)
	tree.meta(grokProjectCWD, grokLiveSession, grokCompletedChild,
		grokMetaFields(grokLiveSession, grokCompletedChild, "explore", grokChildModel, "the finished one",
			"completed", grokProjectCWD, now.Add(-20*time.Minute), grokStamp(completedAt)),
		completedAt)
	// A live child under a parent whose own summary is stale, so the synthesized
	// endpoint has to satisfy the reconciler too, not just this package.
	tree.summary(grokDaemonCWD, grokHostSession,
		grokMainFields(grokHostSession, grokDaemonCWD, grokBuildAgent, grokModel, "quiet host",
			now.Add(-5*time.Hour), now.Add(-2*time.Hour)),
		now.Add(-2*time.Hour))
	tree.meta(grokDaemonCWD, grokHostSession, grokDarkChild,
		grokMetaFields(grokHostSession, grokDarkChild, "explore", grokChildModel, "child of a quiet host",
			"running", grokDaemonCWD, now.Add(-6*time.Minute), ""),
		now.Add(-2*time.Minute))
	tree.roster([]map[string]any{
		grokRosterEntryFields(grokLiveSession, grokProjectCWD, now.Add(-45*time.Minute)),
	}, now.Add(-45*time.Minute))
	tree.seal(grokProjectCWD, now.Add(-3*time.Hour))
	tree.seal(grokDaemonCWD, now.Add(-20*time.Minute))

	collector := NewGrok(tree.home, nil)
	// One tick only. A repeating poll heals a broken emission order on the next
	// pass, because the endpoints are already in the store by then.
	collector.interval = time.Hour

	shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), collector)
	if err != nil {
		t.Fatalf("grok-shadow-construction rule violated: err=%v descriptor=%+v", err, collector.Descriptor())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = shadow.Run(ctx) }()

	session := mustGrokID(t, grokLiveSession)
	running := mustGrokID(t, grokRunningChild)
	completed := mustGrokID(t, grokCompletedChild)
	host := mustGrokID(t, grokHostSession)
	darkChild := mustGrokID(t, grokDarkChild)
	// Well under the 6s freshness window, so the state claim asserted below is
	// still the claim this tick made rather than a decayed one.
	deadline := time.Now().Add(3 * time.Second)
	for {
		snapshot := shadow.Snapshot()
		if snapshot != nil && len(snapshot.Edges) >= 3 {
			nodes := map[graph.NodeID]graph.Node{}
			for _, node := range snapshot.Nodes {
				nodes[node.ID] = node
			}
			for _, id := range []graph.NodeID{session, running, completed, host, darkChild} {
				if _, ok := nodes[id]; !ok {
					t.Fatalf("grok-shadow-publishes-every-endpoint rule violated: id=%s absent nodes=%d edges=%d", id, len(snapshot.Nodes), len(snapshot.Edges))
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
				{running, session, graph.RelationshipID(grokRunningChild)},
				{completed, session, graph.RelationshipID(grokCompletedChild)},
				{darkChild, host, graph.RelationshipID(grokDarkChild)},
			} {
				edge, ok := edges[want.child]
				if !ok {
					t.Fatalf("grok-shadow-spawn-edge rule violated: no edge into child=%s edges=%d", want.child, len(snapshot.Edges))
				}
				if edge.Source != want.parent || edge.Type != graph.EdgeSpawn || edge.Provenance != graph.ProvenanceNative {
					t.Fatalf("grok-shadow-spawn-edge rule violated: child=%s source=%s type=%s provenance=%s wantSource=%s",
						want.child, edge.Source, edge.Type, edge.Provenance, want.parent)
				}
				if edge.Relationship != want.relationship {
					t.Fatalf("grok-shadow-relationship-is-subagent-id rule violated: child=%s relationship=%q want=%q", want.child, edge.Relationship, want.relationship)
				}
			}
			if node := nodes[running]; node.Runtime != types.RuntimeGrok || node.Role != types.RoleSubagent || node.ProvenName != "general-purpose" || node.State.Value != graph.StateActive {
				t.Fatalf("grok-shadow-node-content rule violated: runtime=%q role=%q provenName=%q state=%q want=%q/%q/%q/%q",
					node.Runtime, node.Role, node.ProvenName, node.State.Value, types.RuntimeGrok, types.RoleSubagent, "general-purpose", graph.StateActive)
			}
			if node := nodes[session]; node.Role != types.RolePrimary || node.ProvenName != "Grok" || node.Model != grokModel {
				t.Fatalf("grok-shadow-main-content rule violated: role=%q provenName=%q model=%q want=%q/%q/%q",
					node.Role, node.ProvenName, node.Model, types.RolePrimary, "Grok", grokModel)
			}
			if collector.Disp().Rejected.Load() != 0 {
				t.Fatalf("grok-shadow-no-rejections rule violated: rejected=%d published=%d duplicates=%d",
					collector.Disp().Rejected.Load(), collector.Disp().Published.Load(), collector.Disp().Duplicates.Load())
			}
			return
		}
		if time.Now().After(deadline) {
			nodeCount, edgeCount := 0, 0
			if snapshot != nil {
				nodeCount, edgeCount = len(snapshot.Nodes), len(snapshot.Edges)
			}
			t.Fatalf("grok-shadow-spawn-edge rule violated: fewer than 3 edges after 3s nodes=%d edges=%d published=%d rejected=%d",
				nodeCount, edgeCount, collector.Disp().Published.Load(), collector.Disp().Rejected.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
