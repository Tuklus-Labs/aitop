package native

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/graph"
	"github.com/Tuklus-Labs/aitop/internal/join"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

const (
	claudeSourceID = graph.SourceID("aitop:native:claude")

	// claudeChildActiveWindow bounds how recently a child transcript must have
	// been written for the child to count as running. It is far under the
	// reconciler's decay window, so a child that stops writing stops being
	// claimed rather than being asserted stale.
	claudeChildActiveWindow = 30 * time.Second

	claudeSessionsDir  = "sessions"
	claudeProjectsDir  = "projects"
	claudeSubagentsDir = "subagents"
	claudeAgentPrefix  = "agent-"
	claudeMetaSuffix   = ".meta.json"
	claudeLogSuffix    = ".jsonl"
)

// NewClaude builds the Claude native collector over a ~/.claude home. latest
// supplies the engine's current rows, which own the process binding; nothing in
// the Claude files is allowed to decide an incarnation.
func NewClaude(home string, latest func() []types.Row) *Collector {
	return newCollector(claudeSourceID, types.RuntimeClaude, &claudeScanner{home: home}, latest)
}

// NewClaudeOnce builds a collector for a one-shot over already-frozen Engine
// rows. Fresh unbound sightings publish on the first and only poll.
func NewClaudeOnce(home string, latest func() []types.Row) *Collector {
	return newCaptureCollector(claudeSourceID, types.RuntimeClaude, &claudeScanner{home: home}, latest)
}

// claudeScanner reads ~/.claude. Every read tolerates a missing or unreadable
// path by skipping it, so a half-written file costs one session rather than the
// whole runtime.
type claudeScanner struct {
	home string

	// Counters for decisions that produce no sighting. Without them a skipped
	// store and an empty one are the same silence.
	skippedSidecars atomic.Uint64
	skippedAgents   atomic.Uint64
	skippedSpawns   atomic.Uint64
}

// claudeSidecarFile is one entry of the live-session roster, sessions/<pid>.json.
type claudeSidecarFile struct {
	PID       int32  `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Name      string `json:"name"`
	StartedAt int64  `json:"startedAt"` // epoch ms
	// ProcStart is written as a JSON STRING of proc start ticks. Decoding it
	// into a number fails the whole record and empties the roster, which is why
	// it is typed here at all: the process binding itself comes from the engine
	// rows, never from this file.
	ProcStart string `json:"procStart"`
}

// claudeSidecar is a roster entry plus where it was read from.
type claudeSidecar struct {
	claudeSidecarFile
	path    string
	modTime time.Time
}

// claudeAgentMeta is projects/<slug>/<session>/subagents/agent-<id>.meta.json.
// name, model, and description are all absent on much of the corpus.
type claudeAgentMeta struct {
	AgentType     string `json:"agentType"`
	Description   string `json:"description"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	ParentAgentID string `json:"parentAgentId"`
}

// scan never returns an error: a box with no Claude on it, an unreadable
// directory, and a corrupt file are all ordinary, and returning an error would
// blank the runtime for that tick.
// liveProcs is the horizon rule's live-process clause: a session in it is
// observed whatever its files' mtimes say. It is named apart from the roster's
// own `live` map, which answers a different question (which sidecar FILE owns a
// session id) and cannot stand in for a process binding.
func (s *claudeScanner) scan(now time.Time, liveProcs map[string]bool) ([]NodeSighting, []SpawnSighting, error) {
	roster, live := s.readSidecars()
	nodes := make([]NodeSighting, 0, len(roster))
	emitted := make(map[graph.NodeID]bool, len(roster))

	for _, sidecar := range roster {
		if live[sidecar.SessionID] != sidecar {
			continue // an older roster file for a session a newer one owns
		}
		// A live process holds the session open whatever its sidecar's mtime
		// says, and the sidecar is only rewritten when the session's status
		// changes: a session left thinking for an hour goes cold on disk while
		// being the most active thing on the box.
		if now.Sub(sidecar.modTime) > nativeHorizon && !liveProcs[sidecar.SessionID] {
			continue
		}
		id, err := graph.ClaudeSessionID(sidecar.SessionID)
		if err != nil {
			s.skippedSidecars.Add(1)
			continue
		}
		if emitted[id] {
			continue
		}
		emitted[id] = true
		nodes = append(nodes, NodeSighting{
			ID:        id,
			SessionID: sidecar.SessionID,
			Runtime:   types.RuntimeClaude,
			Role:      types.RolePrimary,
			Name:      sidecar.Name,
			Project:   join.ProjectName(sidecar.CWD),
			StartedAt: epochMillis(sidecar.StartedAt),
			// The same timestamp the horizon above judged, so the core's
			// newborn rule and the horizon can never disagree about how
			// fresh this session is.
			ActivityAt: &sidecar.modTime,
			Location:   sidecar.path,
			// No state claim: occupancy holds the process truth for a primary,
			// and a native claim would fight it every tick.
		})
	}

	children, spawns := s.scanSubagentStores(now, live, emitted, &nodes)
	nodes = append(nodes, children...)
	return nodes, spawns, nil
}

// readSidecars returns the roster in directory order plus the winning file per
// session id. Two files can name one session after a resume; the newer one
// wins, and directory order breaks the tie so the choice is stable.
func (s *claudeScanner) readSidecars() ([]*claudeSidecar, map[string]*claudeSidecar) {
	dir := filepath.Join(s.home, claudeSessionsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, map[string]*claudeSidecar{}
	}
	roster := make([]*claudeSidecar, 0, len(entries))
	live := make(map[string]*claudeSidecar, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue // sessions/ also holds per-session .key files
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			s.skippedSidecars.Add(1)
			continue
		}
		var decoded claudeSidecarFile
		if err := json.Unmarshal(body, &decoded); err != nil || decoded.SessionID == "" {
			s.skippedSidecars.Add(1)
			continue
		}
		info, err := entry.Info()
		if err != nil {
			s.skippedSidecars.Add(1)
			continue
		}
		sidecar := &claudeSidecar{claudeSidecarFile: decoded, path: path, modTime: info.ModTime()}
		roster = append(roster, sidecar)
		if current, ok := live[decoded.SessionID]; !ok || sidecar.modTime.After(current.modTime) {
			live[decoded.SessionID] = sidecar
		}
	}
	return roster, live
}

// scanSubagentStores walks projects/<slug>/<session>/subagents. It appends any
// synthesized parent session directly to nodes, because a spawn edge whose
// parent nobody published is dropped: the reconciler creates no placeholders.
func (s *claudeScanner) scanSubagentStores(now time.Time, live map[string]*claudeSidecar, emitted map[graph.NodeID]bool, nodes *[]NodeSighting) ([]NodeSighting, []SpawnSighting) {
	projects := filepath.Join(s.home, claudeProjectsDir)
	slugs, err := os.ReadDir(projects)
	if err != nil {
		return nil, nil
	}
	var children []NodeSighting
	var spawns []SpawnSighting
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		sessions, err := os.ReadDir(filepath.Join(projects, slug.Name()))
		if err != nil {
			continue
		}
		for _, session := range sessions {
			if !session.IsDir() {
				continue // <sessionId>.jsonl main transcripts are not parsed in v1
			}
			store := filepath.Join(projects, slug.Name(), session.Name(), claudeSubagentsDir)
			parent := live[session.Name()]
			storeChildren, storeSpawns, anchor := s.scanStore(now, store, session.Name(), parent)
			if len(storeChildren) == 0 {
				continue
			}
			children = append(children, storeChildren...)
			spawns = append(spawns, storeSpawns...)
			if anchor.location == "" {
				// Every child here carries a terminal the core will drop, so a
				// session synthesized for them would outlive every child it
				// exists for, with no edge, no state and no terminal to end it.
				continue
			}
			s.appendSessionParent(session.Name(), anchor, parent == nil, emitted, nodes)
		}
	}
	return children, spawns
}

// claudeStoreAnchor is what one subagents directory says about its enclosing
// session: which file to date the session from, and when anything in the store
// was last touched. location stays empty unless the store accepted at least one
// child the core will actually publish, which is what keeps a session from
// being synthesized for children that are all past the exit window.
type claudeStoreAnchor struct {
	location       string
	newestActivity time.Time
}

// scanStore reads one subagents directory. Its anchor names the first
// admissible meta file it accepted; directory order is sorted, so that choice
// is stable across ticks.
func (s *claudeScanner) scanStore(now time.Time, store, session string, parent *claudeSidecar) ([]NodeSighting, []SpawnSighting, claudeStoreAnchor) {
	var anchor claudeStoreAnchor
	entries, err := os.ReadDir(store)
	if err != nil {
		return nil, nil, anchor
	}
	var children []NodeSighting
	var spawns []SpawnSighting
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, claudeAgentPrefix) || !strings.HasSuffix(name, claudeMetaSuffix) {
			continue
		}
		// The child's identity is this agent id. Inside the sibling transcript
		// the sessionId field holds the PARENT's session, so a child identified
		// from its own transcript collapses onto its parent.
		agent := strings.TrimSuffix(strings.TrimPrefix(name, claudeAgentPrefix), claudeMetaSuffix)
		if agent == "" {
			s.skippedAgents.Add(1)
			continue
		}
		info, err := entry.Info()
		if err != nil {
			s.skippedAgents.Add(1)
			continue
		}
		metaPath := filepath.Join(store, name)
		transcriptPath := filepath.Join(store, claudeAgentPrefix+agent+claudeLogSuffix)
		activity := info.ModTime()
		var transcriptAt time.Time
		if transcript, err := os.Stat(transcriptPath); err == nil {
			transcriptAt = transcript.ModTime()
			if transcriptAt.After(activity) {
				activity = transcriptAt
			}
		}
		if now.Sub(activity) > nativeHorizon {
			continue
		}
		body, err := os.ReadFile(metaPath)
		if err != nil {
			s.skippedAgents.Add(1)
			continue
		}
		var meta claudeAgentMeta
		if err := json.Unmarshal(body, &meta); err != nil {
			s.skippedAgents.Add(1)
			continue
		}
		childID, err := graph.ClaudeAgentID(session, agent)
		if err != nil {
			s.skippedAgents.Add(1)
			continue
		}

		childActivity := activity
		sighting := NodeSighting{
			ID:         childID,
			SessionID:  agent,
			Runtime:    types.RuntimeClaude,
			Role:       types.RoleSubagent,
			Name:       firstNonEmpty(meta.Name, meta.AgentType),
			Model:      meta.Model,
			TaskName:   meta.Description,
			ActivityAt: &childActivity,
			Location:   metaPath,
		}
		if parent != nil {
			sighting.Project = join.ProjectName(parent.CWD)
			if !transcriptAt.IsZero() && now.Sub(transcriptAt) < claudeChildActiveWindow {
				sighting.State = graph.StateActive
			}
		} else {
			// In-process children die with their parent, so a session whose
			// roster entry is gone took its children with it. The terminal is
			// dated at the last activity; the core drops it if that is older
			// than the exit window, rather than publishing a node it would
			// evict moments later.
			vanishedAt := activity
			sighting.Exit = graph.OutcomeVanished
			sighting.ExitAt = &vanishedAt
		}
		children = append(children, sighting)
		if activity.After(anchor.newestActivity) {
			anchor.newestActivity = activity
		}
		if anchor.location == "" && !terminalOutsideWindow(sighting, now) {
			anchor.location = metaPath
		}

		parentID, err := claudeParentID(session, meta.ParentAgentID)
		if err != nil {
			s.skippedSpawns.Add(1)
			continue
		}
		parentShort := meta.ParentAgentID
		if parentShort == "" {
			parentShort = session
		}
		spawns = append(spawns, SpawnSighting{
			ParentID:        parentID,
			ParentSessionID: parentShort,
			ChildID:         childID,
			ChildSessionID:  agent,
			Relationship:    graph.RelationshipID(agent),
			Location:        metaPath,
		})
	}
	return children, spawns, anchor
}

// appendSessionParent publishes the enclosing session when no roster entry did,
// so the spawn edge rooted at it has both endpoints. orphaned says the session
// has no roster entry at all.
func (s *claudeScanner) appendSessionParent(session string, anchor claudeStoreAnchor, orphaned bool, emitted map[graph.NodeID]bool, nodes *[]NodeSighting) {
	id, err := graph.ClaudeSessionID(session)
	if err != nil {
		s.skippedSpawns.Add(1)
		return
	}
	if emitted[id] {
		return
	}
	emitted[id] = true
	sighting := NodeSighting{
		ID:         id,
		SessionID:  session,
		Runtime:    types.RuntimeClaude,
		Role:       types.RolePrimary,
		ActivityAt: &anchor.newestActivity,
		Location:   anchor.location,
	}
	if orphaned {
		// The same absence that proves the children died proves the session
		// did: there is no roster entry, and in-process children cannot
		// outlive their session. Without a terminal this node is immortal,
		// because the reconciler dates eviction from a terminal state and
		// nothing else would ever end it. A sidecar that is merely stale is
		// not death evidence, so a session that still has one stands as it is.
		vanishedAt := anchor.newestActivity
		sighting.Exit = graph.OutcomeVanished
		sighting.ExitAt = &vanishedAt
	}
	*nodes = append(*nodes, sighting)
}

// claudeParentID resolves a child's parent endpoint: the sibling agent named by
// parentAgentId when there is one, otherwise the enclosing session.
func claudeParentID(session, parentAgent string) (graph.NodeID, error) {
	if parentAgent != "" {
		return graph.ClaudeAgentID(session, parentAgent)
	}
	return graph.ClaudeSessionID(session)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func epochMillis(millis int64) *time.Time {
	if millis <= 0 {
		return nil
	}
	at := time.UnixMilli(millis).UTC()
	return &at
}
