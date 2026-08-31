package native

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"aitop/internal/graph"
	"aitop/internal/join"
	"aitop/internal/types"
)

const (
	codexSourceID = graph.SourceID("aitop:native:codex")

	codexSessionsDir   = "sessions"
	codexRolloutPrefix = "rollout-"
	codexRolloutSuffix = ".jsonl"

	codexSessionMetaType = "session_meta"
	codexSubagentSource  = "subagent"

	// codexMaxFirstLine bounds the single line this scanner reads. That line
	// carries base_instructions, which is the whole system prompt: measured over
	// the live corpus on 2026-08-31 it runs 18.8 KB median and 46.6 KB max, so
	// the cap leaves the prompt room to grow by a third before a rollout stops
	// being read. It is a bound, not a buffer target: the poll runs every 2 s and
	// must not be steered into an unbounded allocation by whatever a file holds.
	codexMaxFirstLine = 64 << 10
)

// NewCodex builds the Codex native collector over a ~/.codex home. latest
// supplies the engine's current rows, which own the process binding. Those rows
// carry the ROOT thread id, so a user thread (whose own id is the root) binds to
// its live process while a spawned thread falls back to its invocation
// incarnation; the core's shared helper makes that choice, not this file.
func NewCodex(home string, latest func() []types.Row) *Collector {
	return newCollector(codexSourceID, types.RuntimeCodex, &codexScanner{home: home}, latest)
}

// codexScanner reads ~/.codex. Every read tolerates a missing or unreadable path
// by skipping it, so a half-written rollout costs one thread rather than the
// whole runtime.
type codexScanner struct {
	home string

	// Counters for decisions that produce no sighting. Without them a skipped
	// store and an empty one are the same silence.
	skippedRollouts atomic.Uint64 // unreadable, malformed, oversized, or not a session_meta
	skippedThreads  atomic.Uint64 // a session_meta whose id cannot name a node
	skippedSpawns   atomic.Uint64 // a parent_thread_id that cannot name an endpoint
}

// codexRolloutRecord is one JSONL record. Only the first one matters, and only
// when it is the session_meta.
type codexRolloutRecord struct {
	Type    string           `json:"type"`
	Payload codexSessionMeta `json:"payload"`
}

// codexSessionMeta is the header of a rollout.
//
// THE TRAP: `session_id` is the ROOT thread of the whole run, equal to `id` only
// on a user thread. The thread's own identity is `id`. Keying a node on
// session_id collapses every thread in a run onto one node, and on the live
// corpus that is the majority case, not an edge case: 98 of the 99 rollouts on
// the newest sampled day have id != session_id.
type codexSessionMeta struct {
	ID             string `json:"id"`
	SessionID      string `json:"session_id"`
	ParentThreadID string `json:"parent_thread_id"`
	ThreadSource   string `json:"thread_source"`
	CWD            string `json:"cwd"`
	// Source is a plain string ("cli", "vscode", "exec") on a user thread and an
	// object carrying the spawn record on a spawned one. Typing it as either
	// shape fails the other, so it stays raw and is decoded only where it can be
	// an object.
	Source json.RawMessage `json:"source"`
}

// codexSpawnSource is the object form of `source`.
type codexSpawnSource struct {
	Subagent struct {
		ThreadSpawn struct {
			AgentNickname string `json:"agent_nickname"`
		} `json:"thread_spawn"`
	} `json:"subagent"`
}

// nickname is the display name of a spawned thread, read from inside the spawn
// record. The same value appears again at the top level of the payload and the
// two agree across the whole sampled corpus; this reads the copy that is part of
// the spawn itself. A user thread's `source` is a bare string, which fails to
// decode into the object shape and correctly yields no name.
func (meta codexSessionMeta) nickname() string {
	var source codexSpawnSource
	if err := json.Unmarshal(meta.Source, &source); err != nil {
		return ""
	}
	return source.Subagent.ThreadSpawn.AgentNickname
}

func (meta codexSessionMeta) role() types.Role {
	if meta.ThreadSource == codexSubagentSource {
		return types.RoleSubagent
	}
	return types.RolePrimary
}

// codexRollout is one rollout this scan accepted: its header, the file it came
// from, and the node id that header names.
type codexRollout struct {
	meta codexSessionMeta
	path string
	id   graph.NodeID
}

// scan never returns an error: a box with no Codex on it, an unreadable
// directory, and a corrupt rollout are all ordinary, and returning an error
// would blank the runtime for that tick.
func (s *codexScanner) scan(now time.Time, live map[string]bool) ([]NodeSighting, []SpawnSighting, error) {
	rollouts := s.readRecentRollouts(now, live)
	nodes := make([]NodeSighting, 0, len(rollouts))
	published := make(map[graph.NodeID]bool, len(rollouts))
	observed := make([]codexRollout, 0, len(rollouts))

	for _, rollout := range rollouts {
		// The node is this thread's own id. Everything else in the header names
		// somebody else.
		id, err := graph.CodexThreadID(rollout.meta.ID)
		if err != nil {
			s.skippedThreads.Add(1)
			continue
		}
		if published[id] {
			continue // two rollout files naming one thread; the first wins
		}
		published[id] = true
		rollout.id = id
		observed = append(observed, rollout)
		nodes = append(nodes, NodeSighting{
			ID:        id,
			SessionID: rollout.meta.ID,
			Runtime:   types.RuntimeCodex,
			Role:      rollout.meta.role(),
			Name:      rollout.meta.nickname(),
			Project:   join.ProjectName(rollout.meta.CWD),
			Location:  rollout.path,
			// No state claim and no terminal in v1: a rollout says a thread
			// existed, never that it is still running or that it stopped. A node
			// ages past the horizon and reads Stale rather than being asserted.
		})
	}

	// Edges come from a second pass over the threads this scan will PUBLISH, so
	// a parent is judged against all of them rather than against the ones walked
	// so far, and a thread the first pass skipped can never leave a parent
	// behind it. That matters more here than it does for a runtime with death
	// evidence: no codex sighting carries a terminal, the reconciler's sweep
	// only ever considers nodes that have one, and a parent synthesized for a
	// child nobody published would have no edge, no state and no terminal to
	// end it.
	spawns := make([]SpawnSighting, 0, len(observed))
	for _, rollout := range observed {
		parent := rollout.meta.ParentThreadID
		if parent == "" {
			continue
		}
		if parent == rollout.meta.ID {
			s.skippedSpawns.Add(1)
			continue // a self-edge is not a spawn; the reconciler rejects it too
		}
		parentID, err := graph.CodexThreadID(parent)
		if err != nil {
			s.skippedSpawns.Add(1)
			continue
		}
		if !published[parentID] {
			published[parentID] = true
			// The parent's own rollout is outside the walked window, so the child
			// is all the evidence there is. Minimal on purpose: the child's file
			// names its parent and says nothing else about it, and its cwd is the
			// child's, not the parent's. Anchored on the child's rollout, which
			// is the file the fact came from.
			nodes = append(nodes, NodeSighting{
				ID:        parentID,
				SessionID: parent,
				Runtime:   types.RuntimeCodex,
				Role:      types.RolePrimary,
				Location:  rollout.path,
			})
		}
		spawns = append(spawns, SpawnSighting{
			ParentID:        parentID,
			ParentSessionID: parent,
			ChildID:         rollout.id,
			ChildSessionID:  rollout.meta.ID,
			Relationship:    graph.RelationshipID(rollout.meta.ID),
			Location:        rollout.path,
		})
	}
	return nodes, spawns, nil
}

// readRecentRollouts walks the bounded set of date directories and reads the
// header of every rollout inside the horizon.
func (s *codexScanner) readRecentRollouts(now time.Time, live map[string]bool) []codexRollout {
	var rollouts []codexRollout
	for _, date := range codexDateDirs(now) {
		day := filepath.Join(s.home, codexSessionsDir, date)
		entries, err := os.ReadDir(day)
		if err != nil {
			continue // a date with no sessions under it is ordinary
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(name, codexRolloutPrefix) || !strings.HasSuffix(name, codexRolloutSuffix) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				s.skippedRollouts.Add(1)
				continue
			}
			if now.Sub(info.ModTime()) > nativeHorizon && !codexLiveRollout(name, live) {
				continue
			}
			path := filepath.Join(day, name)
			meta, ok := s.readSessionMeta(path)
			if !ok {
				continue
			}
			rollouts = append(rollouts, codexRollout{meta: meta, path: path})
		}
	}
	return rollouts
}

// codexLiveRollout applies the horizon rule's live-process clause without
// opening the file. A rollout is named rollout-<ts>-<threadId>.jsonl, so the
// thread id is in the name; the alternative is reading the header of every
// stale rollout in the walked days just to learn whether to admit it, which is
// the unbounded read the mtime gate exists to prevent.
//
// The suffix is matched rather than the name parsed. The timestamp in the
// middle carries dashes of its own, so any split would be a claim about that
// stamp's exact width, and the live set is a handful of entries at most.
//
// KNOWN LIMIT: this lifts the mtime bound, never the walk bound. A codex thread
// whose rollout sits outside the walked date directories is still unreachable,
// so a live session running for more than about two days is not observed. That
// is a property of codexDateDirs, and closing it means walking more days for
// every tick rather than for the live set.
func codexLiveRollout(name string, live map[string]bool) bool {
	if len(live) == 0 {
		return false
	}
	base := strings.TrimSuffix(name, codexRolloutSuffix)
	for thread := range live {
		if thread != "" && strings.HasSuffix(base, "-"+thread) {
			return true
		}
	}
	return false
}

// codexDateDirs is the walk bound. ~/.codex keeps every rollout it has ever
// written, so walking the tree would mean opening thousands of files every 2 s.
// codex names each date directory from the LOCAL date (a rollout stamped
// 07:09:33Z sits under 2026/08/30 at UTC-7), so local today and yesterday are
// the pair that matters; the UTC pair covers a box whose codex ran under a
// different zone.
func codexDateDirs(now time.Time) []string {
	return codexDateDirsIn(now, time.Local)
}

// codexDateDirsIn takes the local zone as an argument because the union of the
// two pairs collapses to one pair whenever the local date matches UTC, which is
// most of the day in most zones: a caller that could only ask about the running
// machine's zone could not tell a scanner that walks both from one that walks
// whichever pair happens to be in front of it. Order is fixed rather than
// sorted, so a parent synthesized from the first child that names it is anchored
// on the same file every tick and its event id holds still.
func codexDateDirsIn(now time.Time, loc *time.Location) []string {
	local := now.In(loc)
	days := []time.Time{
		local, local.AddDate(0, 0, -1),
		now.UTC(), now.UTC().AddDate(0, 0, -1),
	}
	dirs := make([]string, 0, len(days))
	seen := make(map[string]bool, len(days))
	for _, day := range days {
		dir := filepath.Join(day.Format("2006"), day.Format("01"), day.Format("02"))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

// readSessionMeta reads the FIRST line of a rollout and nothing else. The rest
// of the file is the conversation and runs to megabytes.
func (s *codexScanner) readSessionMeta(path string) (codexSessionMeta, bool) {
	var meta codexSessionMeta
	file, err := os.Open(path)
	if err != nil {
		s.skippedRollouts.Add(1)
		return meta, false
	}
	defer file.Close()

	// ReadSlice stops at the buffer bound rather than growing to fit, so an
	// oversized first line costs one bounded read instead of the whole line.
	line, err := bufio.NewReaderSize(file, codexMaxFirstLine).ReadSlice('\n')
	if err != nil && (err != io.EOF || len(line) == 0) {
		// bufio.ErrBufferFull means the first line is past the cap. io.EOF with
		// bytes in hand is a file still being written, whose single line either
		// parses or does not.
		s.skippedRollouts.Add(1)
		return meta, false
	}
	var record codexRolloutRecord
	if err := json.Unmarshal(line, &record); err != nil || record.Type != codexSessionMetaType {
		s.skippedRollouts.Add(1)
		return meta, false
	}
	return record.Payload, true
}
