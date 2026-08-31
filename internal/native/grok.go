package native

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"aitop/internal/graph"
	"aitop/internal/join"
	"aitop/internal/types"
)

const (
	grokSourceID = graph.SourceID("aitop:native:grok")

	grokRosterFile   = "active_sessions.json"
	grokSessionsDir  = "sessions"
	grokSubagentsDir = "subagents"
	grokSummaryFile  = "summary.json"
	grokMetaFile     = "meta.json"

	// grokBuildAgentPrefix marks a session opened by the grok-build harness.
	// Every agent_name on the live corpus starts with "grok", so the longer
	// prefix is the load-bearing part.
	grokBuildAgentPrefix = "grok-build"
	grokHarnessName      = "Grok"

	// grokSubagentKindPrefix covers "subagent", "subagent_fork" and
	// "subagent_resume", the three values session_kind takes on a child. It says
	// THAT a session is somebody's subagent and never whose, which is why the
	// child's role, parent and lifecycle all come from the parent's meta.json
	// instead.
	grokSubagentKindPrefix = "subagent"

	grokStatusRunning   = "running"
	grokStatusCompleted = "completed"
	grokStatusFailed    = "failed"
)

// NewGrok builds the Grok native collector over a ~/.grok home. latest supplies
// the engine's current rows, which own the process binding: active_sessions.json
// carries a pid but no process start ticks, so nothing in the grok files is
// allowed to decide an incarnation.
func NewGrok(home string, latest func() []types.Row) *Collector {
	return newCollector(grokSourceID, types.RuntimeGrok, &grokScanner{home: home}, latest)
}

// grokScanner reads ~/.grok. Every read tolerates a missing or unreadable path
// by skipping it, so a half-written summary costs one session rather than the
// whole runtime.
type grokScanner struct {
	home string

	// Counters for decisions that produce no sighting. Without them a skipped
	// store and an empty one are the same silence.
	skippedRoster     atomic.Uint64 // roster present but unusable, or an entry with no session id
	skippedSummaries  atomic.Uint64 // summary unreadable, malformed, undatable, or unable to name a node
	subagentSummaries atomic.Uint64 // summaries classified as somebody's child rather than a main
	skippedChildren   atomic.Uint64 // meta unreadable, malformed, self-naming, or a child already published
	skippedSpawns     atomic.Uint64 // an endpoint that cannot be named or does not match its own directory
}

// grokRosterEntry is one entry of active_sessions.json. The file is rewritten
// live, so it is evidence of life and never evidence of death.
type grokRosterEntry struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	OpenedAt  string `json:"opened_at"`
}

// grokSummary is sessions/<enc(cwd)>/<sessionId>/summary.json.
type grokSummary struct {
	Info struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"info"`
	CurrentModelID string `json:"current_model_id"`
	AgentName      string `json:"agent_name"`
	GeneratedTitle string `json:"generated_title"`
	CreatedAt      string `json:"created_at"`
	LastActiveAt   string `json:"last_active_at"`
	SessionKind    string `json:"session_kind"`
}

// grokChildMeta is <sessionDir>/subagents/<childSessionId>/meta.json. It is
// parent-side: nothing in the child's own directory names its parent, which is
// why the whole child record is read from here.
type grokChildMeta struct {
	SubagentID       string `json:"subagent_id"`
	ParentSessionID  string `json:"parent_session_id"`
	ChildSessionID   string `json:"child_session_id"`
	SubagentType     string `json:"subagent_type"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at"`
	CompletedAt      string `json:"completed_at"`
	EffectiveModelID string `json:"effective_model_id"`
	ChildCWD         string `json:"child_cwd"`
}

// grokVisit is one session directory this scan will look at, and the roster
// entry naming it if there is one.
type grokVisit struct {
	session string
	dir     string
	roster  *grokRosterEntry
}

// scan never returns an error: a box with no Grok on it, an unreadable
// directory, and a corrupt file are all ordinary, and returning an error would
// blank the runtime for that tick.
func (s *grokScanner) scan(now time.Time) ([]NodeSighting, []SpawnSighting, error) {
	visits := s.visits(now)

	nodes := make([]NodeSighting, 0, len(visits))
	emitted := make(map[graph.NodeID]bool, len(visits))
	var spawns []SpawnSighting
	// anchors names the stores that produced an edge and the child file to date
	// the missing parent from.
	anchors := make(map[int]string, len(visits))

	// Children first. Every subagent also has a top-level session directory of
	// its own, so one session id can be reached two ways; a node somebody
	// declares as their subagent IS a subagent, and only the parent's meta.json
	// carries the role, the parent and the lifecycle. Publishing the summary
	// first would file the same node as a primary with no spawn edge under it.
	for i, visit := range visits {
		children, storeSpawns, anchor := s.scanStore(now, visit)
		for _, child := range children {
			if emitted[child.ID] {
				// Two stores claiming one child. The first store wins so the
				// choice is stable across ticks rather than directory-order noise.
				s.skippedChildren.Add(1)
				continue
			}
			emitted[child.ID] = true
			nodes = append(nodes, child)
		}
		spawns = append(spawns, storeSpawns...)
		if anchor != "" {
			anchors[i] = anchor
		}
	}

	for _, visit := range visits {
		sighting, ok := s.readMain(now, visit)
		if !ok || emitted[sighting.ID] {
			continue
		}
		emitted[sighting.ID] = true
		nodes = append(nodes, sighting)
	}

	// Synthesized parents last, and only for a store whose child produced an
	// edge this tick. The reconciler creates no placeholder for an endpoint
	// nobody published, so without this the edge is dropped; with it applied any
	// wider, a node with no edge, no state and no terminal would sit in the graph
	// forever, because the sweep only ever considers nodes carrying a terminal.
	for i, visit := range visits {
		anchor, ok := anchors[i]
		if !ok {
			continue
		}
		id, err := graph.GrokSessionID(visit.session)
		if err != nil {
			s.skippedSpawns.Add(1)
			continue
		}
		if emitted[id] {
			continue
		}
		emitted[id] = true
		// Minimal on purpose: the child's file names its parent and says nothing
		// else about it, and the child's cwd is the child's. No terminal either --
		// a summary too stale to publish is not death evidence, and a terminal is
		// sticky for the whole incarnation.
		nodes = append(nodes, NodeSighting{
			ID:        id,
			SessionID: visit.session,
			Runtime:   types.RuntimeGrok,
			Role:      types.RolePrimary,
			Location:  anchor,
		})
	}
	return nodes, spawns, nil
}

// visits is the walk. It is the roster's own sessions, reached by path, plus
// every session directory under every cwd bucket.
//
// The walk is deliberately unbounded at the directory level. A bucket
// directory's mtime moves when a session directory is CREATED or removed inside
// it and never when a session writes, so bounding the walk on it admits
// sessions BORN inside the horizon rather than sessions ACTIVE inside it:
// measured on the live box, 0 of 28 buckets were inside the horizon while a
// session had been active two minutes earlier. The freshness gate therefore
// lives on each session's own summary.json (see readMain), which is the file
// that actually moves when a session does something.
//
// Enumerating a session directory is not the same as reading it. Every visit
// costs one ReadDir of its subagents store, which is how a live child under a
// parent whose own summary has gone quiet still reaches the graph; only visits
// that pass the summary gate cost a read.
func (s *grokScanner) visits(now time.Time) []grokVisit {
	sessions := filepath.Join(s.home, grokSessionsDir)
	roster := s.readRoster()
	visits := make([]grokVisit, 0, len(roster))
	seen := make(map[string]bool, len(roster))
	for i := range roster {
		entry := &roster[i]
		dir := filepath.Join(sessions, grokEncodeCWD(entry.CWD), entry.SessionID)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		visits = append(visits, grokVisit{session: entry.SessionID, dir: dir, roster: entry})
	}

	buckets, err := os.ReadDir(sessions)
	if err != nil {
		return visits // no sessions directory at all is ordinary
	}
	for _, bucket := range buckets {
		if !bucket.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(sessions, bucket.Name()))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(sessions, bucket.Name(), entry.Name())
			if seen[dir] {
				continue
			}
			seen[dir] = true
			visits = append(visits, grokVisit{session: entry.Name(), dir: dir})
		}
	}
	return visits
}

// readRoster reads active_sessions.json in both shapes it takes. A file that is
// absent, half-written, or unparseable yields no roster and no error: the disk
// is still readable, so the scan goes on with what it can see and simply makes
// no liveness claim. Marking sessions gone on a torn read of a file that is
// rewritten live would stick a terminal on every session on the box, and a
// terminal is unrecoverable for that incarnation.
func (s *grokScanner) readRoster() []grokRosterEntry {
	body, err := os.ReadFile(filepath.Join(s.home, grokRosterFile))
	if err != nil {
		if !os.IsNotExist(err) {
			s.skippedRoster.Add(1)
		}
		return nil
	}
	var list []grokRosterEntry
	if err := json.Unmarshal(body, &list); err != nil {
		// The map form, mirroring the tolerance in internal/overlay/grok. Keys
		// are sorted because Go randomizes map order, and an unstable roster
		// order would move which file a synthesized parent is anchored on.
		var byKey map[string]grokRosterEntry
		if err := json.Unmarshal(body, &byKey); err != nil {
			s.skippedRoster.Add(1)
			return nil
		}
		keys := make([]string, 0, len(byKey))
		for key := range byKey {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		list = make([]grokRosterEntry, 0, len(byKey))
		for _, key := range keys {
			list = append(list, byKey[key])
		}
	}
	kept := list[:0]
	for _, entry := range list {
		if entry.SessionID == "" {
			s.skippedRoster.Add(1)
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// readMain turns one session's summary.json into a primary sighting. It makes no
// state claim in either direction: occupancy holds the process truth for a
// primary and a native claim would fight it every tick, and a session that stops
// being observed ages to Stale rather than being asserted dead.
func (s *grokScanner) readMain(now time.Time, visit grokVisit) (NodeSighting, bool) {
	var sighting NodeSighting
	path := filepath.Join(visit.dir, grokSummaryFile)
	if visit.roster == nil {
		// The walk enumerates every session directory on the box, so the file's
		// own mtime is the cheap gate that keeps a 434-session store from being
		// read in full every 2 s: one stat, and a read only for the fresh ones.
		//
		// It is a prefilter, not the rule. The horizon below is the runtime's own
		// recorded claim about its activity, and this gate is only safe because
		// it is a superset of it: summary.json is the file last_active_at is
		// written into, and 0 of the 434 summaries on the live box have an mtime
		// older than the timestamp inside them. A session that broke that
		// relation (a restored backup, a copied tree) would be missed here.
		//
		// The roster route is deliberately NOT gated this way. A roster entry is
		// a live process's claim on a session, dated by opened_at, and it must
		// not depend on when the session last wrote a file.
		info, err := os.Stat(path)
		if err != nil || now.Sub(info.ModTime()) > nativeHorizon {
			return sighting, false
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if visit.roster != nil {
			// A roster entry whose summary cannot be opened. The path is built
			// from the roster's cwd, so this is also where an encoding the disk
			// does not use shows up.
			s.skippedSummaries.Add(1)
		}
		return sighting, false
	}
	var summary grokSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		s.skippedSummaries.Add(1)
		return sighting, false
	}
	if strings.HasPrefix(summary.SessionKind, grokSubagentKindPrefix) {
		// Somebody's subagent. Its node comes from the parent's meta.json, with
		// a subagent role and a lifecycle; publishing this file as a primary
		// would file the same node id twice with two different roles.
		s.subagentSummaries.Add(1)
		return sighting, false
	}
	id, err := graph.GrokSessionID(visit.session)
	if err != nil {
		s.skippedSummaries.Add(1)
		return sighting, false
	}

	started := grokTime(summary.CreatedAt)
	activity := grokTime(summary.LastActiveAt)
	if visit.roster != nil {
		if opened := grokTime(visit.roster.OpenedAt); opened != nil {
			// When the process took the session, which is what a live incarnation
			// started at; created_at can belong to an older one.
			started = opened
			if activity == nil || opened.After(*activity) {
				activity = opened
			}
		}
	}
	if activity == nil {
		// Undatable, so it cannot be shown to be inside the horizon.
		s.skippedSummaries.Add(1)
		return sighting, false
	}
	if now.Sub(*activity) > nativeHorizon {
		return sighting, false
	}

	cwd := summary.Info.CWD
	if cwd == "" && visit.roster != nil {
		cwd = visit.roster.CWD
	}
	name := ""
	if strings.HasPrefix(summary.AgentName, grokBuildAgentPrefix) {
		// The harness, not the profile: agent_name reads "grok-build-plan", and
		// printing it would put a profile name in the roster column.
		name = grokHarnessName
	}
	return NodeSighting{
		ID:        id,
		SessionID: visit.session,
		Runtime:   types.RuntimeGrok,
		Role:      types.RolePrimary,
		Name:      name,
		Model:     summary.CurrentModelID,
		Project:   join.ProjectName(cwd),
		TaskName:  summary.GeneratedTitle,
		StartedAt: started,
		Location:  path,
	}, true
}

// scanStore reads one session's subagents directory. The returned anchor names
// the first child file that produced both a publishable sighting and an edge;
// it stays empty when the store has nothing to root, which is what keeps a
// parent from being synthesized for children this scan refused to publish.
func (s *grokScanner) scanStore(now time.Time, visit grokVisit) ([]NodeSighting, []SpawnSighting, string) {
	store := filepath.Join(visit.dir, grokSubagentsDir)
	entries, err := os.ReadDir(store)
	if err != nil {
		return nil, nil, "" // a session with no subagents is the common case
	}
	parentID, parentErr := graph.GrokSessionID(visit.session)
	var children []NodeSighting
	var spawns []SpawnSighting
	anchor := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(store, entry.Name(), grokMetaFile)
		info, err := os.Stat(metaPath)
		if err != nil {
			s.skippedChildren.Add(1)
			continue
		}
		body, err := os.ReadFile(metaPath)
		if err != nil {
			s.skippedChildren.Add(1)
			continue
		}
		var meta grokChildMeta
		if err := json.Unmarshal(body, &meta); err != nil {
			s.skippedChildren.Add(1)
			continue
		}
		child := meta.ChildSessionID
		if child == "" || child == visit.session {
			// A record that names no child, or names the enclosing session as its
			// own child. The second would publish the parent's node id under a
			// subagent role and hand the reconciler a self-edge, which it rejects.
			s.skippedChildren.Add(1)
			continue
		}
		childID, err := graph.GrokSessionID(child)
		if err != nil {
			s.skippedChildren.Add(1)
			continue
		}
		if now.Sub(s.childActivity(meta, info.ModTime(), now)) > nativeHorizon {
			continue
		}

		sighting := NodeSighting{
			ID:        childID,
			SessionID: child,
			Runtime:   types.RuntimeGrok,
			Role:      types.RoleSubagent,
			Name:      meta.SubagentType,
			Model:     meta.EffectiveModelID,
			Project:   join.ProjectName(meta.ChildCWD),
			TaskName:  meta.Description,
			StartedAt: grokTime(meta.StartedAt),
			Location:  metaPath,
		}
		switch meta.Status {
		case grokStatusRunning:
			// Both halves are required. A running status is the child's own last
			// word and can outlive the process that wrote it; the roster is what
			// says the session hosting it is still up. Neither alone is liveness.
			if visit.roster != nil {
				sighting.State = graph.StateActive
			}
		case grokStatusCompleted:
			sighting.Exit = graph.OutcomeCompleted
			sighting.ExitAt = grokTime(meta.CompletedAt)
		case grokStatusFailed:
			sighting.Exit = graph.OutcomeFailed
			sighting.ExitAt = grokTime(meta.CompletedAt)
			// Everything else, "cancelled" included, claims nothing. A terminal is
			// sticky for the incarnation, so guessing one is unrecoverable, and the
			// node ages to Stale on its own.
		}
		children = append(children, sighting)

		if parentErr != nil {
			s.skippedSpawns.Add(1)
			continue
		}
		if meta.ParentSessionID != "" && meta.ParentSessionID != visit.session {
			// The record disagrees with the directory it lives in. Believing the
			// directory would attach the edge to a session that never spawned this
			// child; believing the field would name an endpoint nobody published.
			// The child is still a real child, so it keeps its node.
			s.skippedSpawns.Add(1)
			continue
		}
		// The label is subagent_id verbatim. It equals child_session_id on every
		// file in the corpus, and the core's seam guard is the one owner of what
		// a usable relationship looks like.
		spawns = append(spawns, SpawnSighting{
			ParentID:        parentID,
			ParentSessionID: visit.session,
			ChildID:         childID,
			ChildSessionID:  child,
			Relationship:    graph.RelationshipID(meta.SubagentID),
			Location:        metaPath,
		})
		if anchor == "" && !terminalOutsideWindow(sighting, now) {
			anchor = metaPath
		}
	}
	return children, spawns, anchor
}

// childActivity dates a child. meta.json is rewritten when the child finishes
// (its mtime matches completed_at on all 228 files in the corpus and started_at
// on none), so for a RUNNING child that mtime is its spawn time and a long job
// would age out of the horizon mid-flight. The child's own session directory is
// where it keeps writing, so it is consulted as a second signal -- and only when
// the meta is already too old to admit, which keeps the extra stat off the
// common path.
func (s *grokScanner) childActivity(meta grokChildMeta, metaModTime time.Time, now time.Time) time.Time {
	if now.Sub(metaModTime) <= nativeHorizon || meta.ChildCWD == "" {
		return metaModTime
	}
	own := filepath.Join(s.home, grokSessionsDir, grokEncodeCWD(meta.ChildCWD), meta.ChildSessionID, grokSummaryFile)
	info, err := os.Stat(own)
	if err != nil || !info.ModTime().After(metaModTime) {
		return metaModTime
	}
	return info.ModTime()
}

// grokEncodeCWD is grok's own bucket naming, measured off the live box on
// 2026-08-31 rather than taken from the contract: the 28 bucket names there
// carry 149 %2F and 2 %20, so the scheme escapes spaces as well as slashes.
//
// Slash-only encoding, which is what internal/overlay/grok still does, resolves
// a spaced cwd to a path that does not exist and loses the session entirely on
// the roster route: 1 of the 434 sessions on that box, under
// /home/aegis/Documents/Obsidian Vault. That gap is still open in the overlay
// package and is ledgered for review; nothing in this package depends on it.
//
// Only these two escapes are claimed. No other character has been observed
// escaped in a bucket name, and guessing at a wider set from two samples would
// be inventing a scheme rather than matching one.
func grokEncodeCWD(cwd string) string {
	if cwd == "" {
		return ""
	}
	return strings.ReplaceAll(strings.ReplaceAll(cwd, "/", "%2F"), " ", "%20")
}

func grokTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	at = at.UTC()
	return &at
}
