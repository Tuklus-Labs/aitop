package snapshot

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aitop/internal/graph"
	"aitop/internal/proc"
	"aitop/internal/types"
)

const (
	maxJSONSafeInteger   = 1<<53 - 1
	maxGraphIDBytes      = 192
	maxGraphDisplayBytes = 128
	maxTransitions       = 256
	maxRelationshipBytes = 64
	traceByteLen         = 16
)

type jsonV2Runtime struct {
	Marshal func(any) ([]byte, error)
}

type dumpV2 struct {
	Schema int         `json:"schema"`
	At     string      `json:"at"`
	Host   dumpHost    `json:"host"`
	Rows   []dumpRow   `json:"rows"`
	Graph  dumpGraphV2 `json:"graph"`
}

type dumpGraphV2 struct {
	At                 string     `json:"at"`
	TopologyRevision   uint64     `json:"topology_revision"`
	VisibilityRevision uint64     `json:"visibility_revision"`
	StateRevision      uint64     `json:"state_revision"`
	MetricsRevision    uint64     `json:"metrics_revision"`
	Nodes              []dumpNode `json:"nodes"`
	Edges              []dumpEdge `json:"edges"`
	Gaps               []dumpGap  `json:"gaps"`
}

type dumpGraphSource struct {
	ID          string `json:"id"`
	Runtime     string `json:"runtime"`
	Incarnation string `json:"incarnation"`
	Authority   string `json:"authority"`
}

type dumpGraphProcess struct {
	PID        int32  `json:"pid"`
	StartTicks uint64 `json:"start_ticks"`
}

type dumpGraphUsage struct {
	Input      int64 `json:"input"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Output     int64 `json:"output"`
}

type dumpGraphMetrics struct {
	Usage         *dumpGraphUsage `json:"usage,omitempty"`
	TokenRate     *float64        `json:"token_rate,omitempty"`
	ContextUsed   *int64          `json:"context_used,omitempty"`
	ContextWindow *int64          `json:"context_window,omitempty"`
	ContextFill   *float64        `json:"context_fill,omitempty"`
	CacheUse      *float64        `json:"cache_use,omitempty"`
	CostUSD       *float64        `json:"cost_usd,omitempty"`
	CostSource    string          `json:"cost_source,omitempty"`
}

type dumpGraphState struct {
	Value      string           `json:"value"`
	Source     *dumpGraphSource `json:"source,omitempty"`
	Since      string           `json:"since,omitempty"`
	ValidUntil string           `json:"valid_until,omitempty"`
	Stale      bool             `json:"stale"`
}

type dumpGraphTransition struct {
	At     string          `json:"at"`
	State  string          `json:"state"`
	Source dumpGraphSource `json:"source"`
}

type dumpNode struct {
	ID             string                `json:"id"`
	Incarnation    string                `json:"incarnation"`
	Runtime        string                `json:"runtime"`
	Role           string                `json:"role"`
	ProvenName     string                `json:"proven_name,omitempty"`
	Model          string                `json:"model,omitempty"`
	Project        string                `json:"project,omitempty"`
	Worktree       string                `json:"worktree,omitempty"`
	TaskName       string                `json:"task_name,omitempty"`
	Process        *dumpGraphProcess     `json:"process,omitempty"`
	State          dumpGraphState        `json:"state"`
	Metrics        dumpGraphMetrics      `json:"metrics"`
	StartedAt      string                `json:"started_at,omitempty"`
	CompletedAt    string                `json:"completed_at,omitempty"`
	FailedAt       string                `json:"failed_at,omitempty"`
	GhostExpiresAt string                `json:"ghost_expires_at,omitempty"`
	Pinned         bool                  `json:"pinned"`
	TelemetryAt    string                `json:"telemetry_at"`
	Partial        bool                  `json:"partial"`
	Transitions    []dumpGraphTransition `json:"transitions"`
}

type dumpDelivery struct {
	Unknown  uint64 `json:"unknown"`
	Emitted  uint64 `json:"emitted"`
	Received uint64 `json:"received"`
	Failed   uint64 `json:"failed"`
	Latest   string `json:"latest"`
}

type dumpEdge struct {
	Key          string        `json:"key"`
	Source       string        `json:"source"`
	Target       string        `json:"target"`
	Type         string        `json:"type"`
	Provenance   string        `json:"provenance"`
	Relationship string        `json:"relationship,omitempty"`
	Trace        string        `json:"trace,omitempty"`
	CreatedAt    string        `json:"created_at"`
	LastActivity string        `json:"last_activity"`
	EventCount   uint64        `json:"event_count"`
	Lifecycle    string        `json:"lifecycle"`
	MessageKind  string        `json:"message_kind,omitempty"`
	Delivery     *dumpDelivery `json:"delivery,omitempty"`
	Partial      bool          `json:"partial"`
}

type dumpGap struct {
	Source     string  `json:"source"`
	Capability *string `json:"capability,omitempty"`
	Kind       string  `json:"kind"`
	At         string  `json:"at"`
	Count      uint64  `json:"count"`
}

func WriteJSON(s *Snapshot, w io.Writer, now time.Time) error {
	return writeJSONWithRuntime(s, w, now, jsonV2Runtime{})
}

func writeJSONWithRuntime(s *Snapshot, w io.Writer, now time.Time, rt jsonV2Runtime) error {
	d, err := ToDumpV2(s, now)
	if err != nil {
		return err
	}
	marshal := rt.Marshal
	if marshal == nil {
		marshal = json.Marshal
	}
	body, err := marshal(d)
	if err != nil {
		return err
	}
	payload := append(append([]byte{}, body...), '\n')
	n, err := w.Write(payload)
	if n < len(payload) {
		if err == nil {
			return io.ErrShortWrite
		}
		return errors.Join(io.ErrShortWrite, err)
	}
	return err
}

func ToDumpV2(s *Snapshot, now time.Time) (dumpV2, error) {
	if s == nil {
		return dumpV2{}, fmt.Errorf("schema-2-snapshot rule violated: field=snapshot class=nil")
	}
	at, err := formatCanonicalTime("at", s.At, true)
	if err != nil {
		return dumpV2{}, err
	}
	d := dumpV2{
		Schema: 2,
		At:     at,
		Host:   dumpHostFromSample(s.Host),
		Rows:   []dumpRow{},
		Graph: dumpGraphV2{
			Nodes: []dumpNode{},
			Edges: []dumpEdge{},
			Gaps:  []dumpGap{},
		},
	}
	for _, row := range s.Rows {
		d.Rows = append(d.Rows, flattenWith(row, s.Host, now))
	}
	graphAt := s.At
	if s.Graph != nil {
		graphAt = s.Graph.At
		d.Graph.TopologyRevision = s.Graph.TopologyRevision
		d.Graph.VisibilityRevision = s.Graph.VisibilityRevision
		d.Graph.StateRevision = s.Graph.StateRevision
		d.Graph.MetricsRevision = s.Graph.MetricsRevision
		for i := range s.Graph.Nodes {
			node, err := dumpNodeFrom(s.Graph.Nodes[i])
			if err != nil {
				return dumpV2{}, err
			}
			d.Graph.Nodes = append(d.Graph.Nodes, node)
		}
		for i := range s.Graph.Edges {
			edge, err := dumpEdgeFrom(s.Graph.Edges[i])
			if err != nil {
				return dumpV2{}, err
			}
			d.Graph.Edges = append(d.Graph.Edges, edge)
		}
		for i := range s.Graph.Gaps {
			gap, err := dumpGapFrom(s.Graph.Gaps[i])
			if err != nil {
				return dumpV2{}, err
			}
			d.Graph.Gaps = append(d.Graph.Gaps, gap)
		}
	}
	formattedGraphAt, err := formatCanonicalTime("graph.at", graphAt, true)
	if err != nil {
		return dumpV2{}, err
	}
	d.Graph.At = formattedGraphAt
	sortDumpV2(&d)
	if err := validateDumpV2(d); err != nil {
		return dumpV2{}, err
	}
	return d, nil
}

func dumpHostFromSample(host proc.HostSample) dumpHost {
	out := dumpHost{NumCPU: host.NumCPU, MemTotal: host.MemTotal, MemAvail: host.MemAvail, Uptime: host.Uptime}
	if host.CPUKnown {
		v := host.CPUBusyPct
		out.CPUPct = &v
	}
	return out
}

func dumpNodeFrom(n graph.Node) (dumpNode, error) {
	if err := checkPublicRole(string(n.Role)); err != nil {
		return dumpNode{}, err
	}
	if err := checkRuntime(string(n.Runtime)); err != nil {
		return dumpNode{}, err
	}
	state, err := dumpStateFrom(n.State)
	if err != nil {
		return dumpNode{}, err
	}
	metrics, err := dumpMetricsFrom(n.Metrics)
	if err != nil {
		return dumpNode{}, err
	}
	telemetry, err := formatCanonicalTime("telemetry_at", n.TelemetryAt, true)
	if err != nil {
		return dumpNode{}, err
	}
	out := dumpNode{
		ID:          string(n.ID),
		Incarnation: string(n.Incarnation),
		Runtime:     string(n.Runtime),
		Role:        string(n.Role),
		ProvenName:  n.ProvenName,
		Model:       n.Model,
		Project:     n.Project,
		Worktree:    n.Worktree,
		TaskName:    n.TaskName,
		State:       state,
		Metrics:     metrics,
		Pinned:      n.Pinned,
		TelemetryAt: telemetry,
		Partial:     n.Partial,
		Transitions: []dumpGraphTransition{},
	}
	if n.Process != nil {
		out.Process = &dumpGraphProcess{PID: n.Process.PID, StartTicks: n.Process.StartTicks}
	}
	if out.StartedAt, err = formatOptionalTime("started_at", n.StartedAt); err != nil {
		return dumpNode{}, err
	}
	if out.CompletedAt, err = formatOptionalTime("completed_at", n.CompletedAt); err != nil {
		return dumpNode{}, err
	}
	if out.FailedAt, err = formatOptionalTime("failed_at", n.FailedAt); err != nil {
		return dumpNode{}, err
	}
	if out.GhostExpiresAt, err = formatOptionalTime("ghost_expires_at", n.GhostExpiresAt); err != nil {
		return dumpNode{}, err
	}
	for _, tr := range n.Transitions {
		item, err := dumpTransitionFrom(tr)
		if err != nil {
			return dumpNode{}, err
		}
		out.Transitions = append(out.Transitions, item)
	}
	return out, nil
}

func dumpStateFrom(st graph.NodeState) (dumpGraphState, error) {
	out := dumpGraphState{Value: string(st.Value), Stale: st.Stale}
	src, err := dumpSourcePtr(st.Source)
	if err != nil {
		return dumpGraphState{}, err
	}
	out.Source = src
	if out.Since, err = formatCanonicalTime("state.since", st.Since, false); err != nil {
		return dumpGraphState{}, err
	}
	if out.ValidUntil, err = formatCanonicalTime("state.valid_until", st.ValidUntil, false); err != nil {
		return dumpGraphState{}, err
	}
	return out, nil
}

func dumpSourcePtr(src graph.SourceRef) (*dumpGraphSource, error) {
	if src.ID == "" && src.Runtime == "" && src.Incarnation == 0 && src.Authority == 0 {
		return nil, nil
	}
	out, err := dumpSourceValue(src)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func dumpSourceValue(src graph.SourceRef) (dumpGraphSource, error) {
	auth, err := authorityString(src.Authority)
	if err != nil {
		return dumpGraphSource{}, err
	}
	inc, err := formatSourceIncarnation(src.Incarnation)
	if err != nil {
		return dumpGraphSource{}, err
	}
	return dumpGraphSource{
		ID:          string(src.ID),
		Runtime:     string(src.Runtime),
		Incarnation: inc,
		Authority:   auth,
	}, nil
}

func dumpTransitionFrom(tr graph.Transition) (dumpGraphTransition, error) {
	at, err := formatCanonicalTime("transition.at", tr.At, true)
	if err != nil {
		return dumpGraphTransition{}, err
	}
	src, err := dumpSourceValue(tr.Source)
	if err != nil {
		return dumpGraphTransition{}, err
	}
	return dumpGraphTransition{At: at, State: string(tr.State), Source: src}, nil
}

func dumpMetricsFrom(m graph.Metrics) (dumpGraphMetrics, error) {
	out := dumpGraphMetrics{
		TokenRate:     m.TokenRate,
		ContextUsed:   m.ContextUsed,
		ContextWindow: m.ContextWindow,
		ContextFill:   m.ContextFill,
		CacheUse:      m.CacheUse,
		CostUSD:       m.CostUSD,
		CostSource:    m.CostSource,
	}
	if m.Usage != nil {
		out.Usage = &dumpGraphUsage{
			Input:      m.Usage.Input,
			CacheRead:  m.Usage.CacheRead,
			CacheWrite: m.Usage.CacheWrite,
			Output:     m.Usage.Output,
		}
	}
	return out, nil
}

func dumpEdgeFrom(e graph.Edge) (dumpEdge, error) {
	created, err := formatCanonicalTime("created_at", e.CreatedAt, true)
	if err != nil {
		return dumpEdge{}, err
	}
	last, err := formatCanonicalTime("last_activity", e.LastActivity, true)
	if err != nil {
		return dumpEdge{}, err
	}
	out := dumpEdge{
		Key:          string(e.Key),
		Source:       string(e.Source),
		Target:       string(e.Target),
		Type:         string(e.Type),
		Provenance:   string(e.Provenance),
		CreatedAt:    created,
		LastActivity: last,
		EventCount:   e.EventCount,
		Lifecycle:    string(e.Lifecycle),
		MessageKind:  string(e.MessageKind),
		Partial:      e.Partial,
	}
	if e.Relationship != "" {
		out.Relationship = base64.RawURLEncoding.EncodeToString([]byte(e.Relationship))
	}
	if e.Trace != nil {
		out.Trace = base64.RawURLEncoding.EncodeToString(e.Trace[:])
	}
	if e.Delivery != nil {
		out.Delivery = &dumpDelivery{
			Unknown:  e.Delivery.Unknown,
			Emitted:  e.Delivery.Emitted,
			Received: e.Delivery.Received,
			Failed:   e.Delivery.Failed,
			Latest:   string(e.Delivery.Latest),
		}
	}
	return out, nil
}

func dumpGapFrom(g graph.Gap) (dumpGap, error) {
	at, err := formatCanonicalTime("gap.at", g.At, true)
	if err != nil {
		return dumpGap{}, err
	}
	out := dumpGap{Source: string(g.Source), Kind: string(g.Kind), At: at, Count: g.Count}
	if g.Capability != nil {
		v := string(*g.Capability)
		out.Capability = &v
	}
	return out, nil
}

func sortDumpV2(d *dumpV2) {
	sort.Slice(d.Graph.Nodes, func(i, j int) bool { return d.Graph.Nodes[i].ID < d.Graph.Nodes[j].ID })
	sort.Slice(d.Graph.Edges, func(i, j int) bool { return d.Graph.Edges[i].Key < d.Graph.Edges[j].Key })
	sort.Slice(d.Graph.Gaps, func(i, j int) bool {
		left, right := d.Graph.Gaps[i], d.Graph.Gaps[j]
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Capability == nil && right.Capability != nil {
			return true
		}
		if left.Capability != nil && right.Capability == nil {
			return false
		}
		if left.Capability != nil && right.Capability != nil && *left.Capability != *right.Capability {
			return *left.Capability < *right.Capability
		}
		return left.Kind < right.Kind
	})
}

func validateDumpV2(d dumpV2) error {
	if d.Schema != 2 {
		return fmt.Errorf("schema-2-root rule violated: field=schema class=unsupported")
	}
	if _, err := parseCanonicalTime("at", d.At, true); err != nil {
		return err
	}
	if d.Rows == nil {
		return fmt.Errorf("schema-2-non-null-array rule violated: field=rows class=null")
	}
	if err := validateHost(d.Host); err != nil {
		return err
	}
	for i := range d.Rows {
		if err := validateRow(d.Rows[i]); err != nil {
			return err
		}
	}
	return validateGraph(d.Graph)
}

func validateHost(h dumpHost) error {
	if err := checkOptionalFiniteRange("cpu_pct", h.CPUPct, 0, 100); err != nil {
		return err
	}
	if h.NumCPU != 0 {
		if err := checkSafePositiveInt("ncpu", h.NumCPU); err != nil {
			return err
		}
	}
	if err := checkSafeUint64("mem_total", h.MemTotal); err != nil {
		return err
	}
	if err := checkSafeUint64("mem_avail", h.MemAvail); err != nil {
		return err
	}
	return checkFiniteNonneg("uptime_s", h.Uptime)
}

func validateRow(r dumpRow) error {
	if r.PID != 0 {
		if err := checkPID("pid", r.PID); err != nil {
			return err
		}
	}
	if err := checkSafeUint64("rss", r.RSS); err != nil {
		return err
	}
	if err := checkOptionalFiniteNonneg("cpu", r.CPU); err != nil {
		return err
	}
	if err := checkOptionalSafeInt64("tokens", r.Tokens); err != nil {
		return err
	}
	if err := checkOptionalSafeInt64("ctx_window", r.CtxWindow); err != nil {
		return err
	}
	if err := checkOptionalFiniteRange("ctx_fill", r.CtxFill, 0, 1); err != nil {
		return err
	}
	if err := checkOptionalFiniteNonneg("cost_usd", r.CostUSD); err != nil {
		return err
	}
	if err := checkCostSource("cost_source", r.CostSource, r.CostUSD); err != nil {
		return err
	}
	if r.Usage != nil {
		if err := validateDumpUse(*r.Usage); err != nil {
			return err
		}
	}
	if err := checkOptionalFiniteNonneg("age_s", r.AgeS); err != nil {
		return err
	}
	if err := checkSafeInt("subagents_live", r.SubLive); err != nil {
		return err
	}
	if err := checkSafeInt("subagents_declared", r.SubDeclared); err != nil {
		return err
	}
	if err := checkOptionalFiniteNonneg("tok_per_sec", r.TokPerSec); err != nil {
		return err
	}
	if r.SlotIndex != nil {
		if err := checkSafeInt("slot_index", *r.SlotIndex); err != nil {
			return err
		}
	}
	for i := range r.Children {
		if err := validateRow(r.Children[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateDumpUse(u dumpUse) error {
	for _, item := range []struct {
		name  string
		value int64
	}{
		{"input", u.Input},
		{"cache_read", u.CacheRead},
		{"cache_write", u.CacheWrite},
		{"output", u.Output},
	} {
		if err := checkSafeInt64(item.name, item.value); err != nil {
			return err
		}
	}
	return nil
}

func validateGraph(g dumpGraphV2) error {
	if _, err := parseCanonicalTime("graph.at", g.At, true); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value uint64
	}{
		{"topology_revision", g.TopologyRevision},
		{"visibility_revision", g.VisibilityRevision},
		{"state_revision", g.StateRevision},
		{"metrics_revision", g.MetricsRevision},
	} {
		if err := checkSafeUint64(item.name, item.value); err != nil {
			return err
		}
	}
	if g.Nodes == nil || g.Edges == nil || g.Gaps == nil {
		return fmt.Errorf("schema-2-non-null-array rule violated: field=graph class=null")
	}
	ids := make(map[string]struct{}, len(g.Nodes))
	var lastID string
	for i := range g.Nodes {
		if err := validateNode(g.Nodes[i]); err != nil {
			return err
		}
		if i > 0 && g.Nodes[i].ID < lastID {
			return fmt.Errorf("schema-2-sort rule violated: field=nodes class=order")
		}
		if _, ok := ids[g.Nodes[i].ID]; ok {
			return fmt.Errorf("schema-2-uniqueness rule violated: field=nodes class=duplicate")
		}
		ids[g.Nodes[i].ID] = struct{}{}
		lastID = g.Nodes[i].ID
	}
	var lastKey string
	for i := range g.Edges {
		if err := validateEdge(g.Edges[i], ids); err != nil {
			return err
		}
		if i > 0 && g.Edges[i].Key < lastKey {
			return fmt.Errorf("schema-2-sort rule violated: field=edges class=order")
		}
		if i > 0 && g.Edges[i].Key == lastKey {
			return fmt.Errorf("schema-2-uniqueness rule violated: field=edges class=duplicate")
		}
		lastKey = g.Edges[i].Key
	}
	var lastGap dumpGap
	for i := range g.Gaps {
		if err := validateGap(g.Gaps[i]); err != nil {
			return err
		}
		if i > 0 {
			if gapIdentity(lastGap) == gapIdentity(g.Gaps[i]) {
				return fmt.Errorf("schema-2-uniqueness rule violated: field=gaps class=duplicate")
			}
			if !gapLess(lastGap, g.Gaps[i]) {
				return fmt.Errorf("schema-2-sort rule violated: field=gaps class=order")
			}
		}
		lastGap = g.Gaps[i]
	}
	return nil
}

func validateNode(n dumpNode) error {
	if err := checkID("id", n.ID, maxGraphIDBytes); err != nil {
		return err
	}
	if err := checkID("incarnation", n.Incarnation, maxGraphIDBytes); err != nil {
		return err
	}
	if err := checkRuntime(n.Runtime); err != nil {
		return err
	}
	if err := checkPublicRole(n.Role); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{"proven_name", n.ProvenName},
		{"model", n.Model},
		{"project", n.Project},
		{"worktree", n.Worktree},
		{"task_name", n.TaskName},
	} {
		if item.value == "" {
			continue
		}
		if err := checkID(item.name, item.value, maxGraphDisplayBytes); err != nil {
			return err
		}
	}
	if n.Process != nil {
		if err := checkPID("process.pid", n.Process.PID); err != nil {
			return err
		}
		if err := checkSafePositiveUint64("start_ticks", n.Process.StartTicks); err != nil {
			return err
		}
	}
	if err := validateState(n.State); err != nil {
		return err
	}
	if err := validateMetrics(n.Metrics); err != nil {
		return err
	}
	if _, err := parseCanonicalTime("telemetry_at", n.TelemetryAt, true); err != nil {
		return err
	}
	started, err := parseCanonicalTime("started_at", n.StartedAt, false)
	if err != nil {
		return err
	}
	completed, err := parseCanonicalTime("completed_at", n.CompletedAt, false)
	if err != nil {
		return err
	}
	failed, err := parseCanonicalTime("failed_at", n.FailedAt, false)
	if err != nil {
		return err
	}
	ghost, err := parseCanonicalTime("ghost_expires_at", n.GhostExpiresAt, false)
	if err != nil {
		return err
	}
	if err := validateTerminalTimestamps(n.State.Value, n.CompletedAt, n.FailedAt, n.GhostExpiresAt); err != nil {
		return err
	}
	if err := validateTimestampOrder(n.State, started, completed, failed, ghost); err != nil {
		return err
	}
	if n.Transitions == nil {
		return fmt.Errorf("schema-2-non-null-array rule violated: field=transitions class=null")
	}
	if len(n.Transitions) > maxTransitions {
		return fmt.Errorf("schema-2-transition-limit rule violated: field=transitions count=%d limit=%d class=too-long", len(n.Transitions), maxTransitions)
	}
	var prev time.Time
	for i, tr := range n.Transitions {
		at, err := parseCanonicalTime("transition.at", tr.At, true)
		if err != nil {
			return err
		}
		if !validStateValue(tr.State) {
			return fmt.Errorf("schema-2-state rule violated: field=transition.state class=unsupported")
		}
		if err := validateSourceValue(tr.Source); err != nil {
			return err
		}
		if i > 0 && at.Before(prev) {
			return fmt.Errorf("schema-2-transition-order rule violated: field=transitions class=order")
		}
		prev = at
	}
	return nil
}

func validateState(st dumpGraphState) error {
	if !validStateValue(st.Value) {
		return fmt.Errorf("schema-2-state rule violated: field=state.value class=unsupported")
	}
	hasSource := st.Source != nil
	hasSince := st.Since != ""
	if hasSource != hasSince {
		return fmt.Errorf("schema-2-state-pair rule violated: field=state class=pair")
	}
	if st.ValidUntil != "" && !hasSource {
		return fmt.Errorf("schema-2-state-valid-until rule violated: field=valid_until class=missing-pair")
	}
	if isTerminalState(st.Value) && !hasSource {
		return fmt.Errorf("schema-2-state-pair rule violated: field=state class=terminal-pair")
	}
	if st.ValidUntil != "" && (isTerminalState(st.Value) || st.Value == "approval" || st.Value == "blocked") {
		return fmt.Errorf("schema-2-state-valid-until rule violated: field=valid_until class=forbidden")
	}
	if hasSource {
		if err := validateSourceValue(*st.Source); err != nil {
			return err
		}
		if st.ValidUntil != "" && st.Source.Authority == "passive" {
			return fmt.Errorf("schema-2-state-valid-until rule violated: field=valid_until class=passive")
		}
		if st.ValidUntil != "" && st.Source.Authority != "native" && st.Source.Authority != "hook" {
			return fmt.Errorf("schema-2-state-valid-until rule violated: field=valid_until class=authority")
		}
	}
	since, err := parseCanonicalTime("state.since", st.Since, hasSince)
	if err != nil {
		return err
	}
	until, err := parseCanonicalTime("state.valid_until", st.ValidUntil, st.ValidUntil != "")
	if err != nil {
		return err
	}
	if st.ValidUntil != "" && !until.After(since) {
		return fmt.Errorf("schema-2-state-valid-until rule violated: field=valid_until class=order")
	}
	return nil
}

func validateSourceValue(src dumpGraphSource) error {
	if err := checkID("source.id", src.ID, maxGraphIDBytes); err != nil {
		return err
	}
	if err := checkRuntime(src.Runtime); err != nil {
		return err
	}
	if err := checkSourceIncarnation(src.Incarnation); err != nil {
		return err
	}
	switch src.Authority {
	case "passive", "native", "hook":
		return nil
	default:
		return fmt.Errorf("schema-2-authority rule violated: field=authority class=unsupported")
	}
}

func validateMetrics(m dumpGraphMetrics) error {
	if m.Usage != nil {
		if err := checkSafeInt64("metrics.usage.input", m.Usage.Input); err != nil {
			return err
		}
		if err := checkSafeInt64("metrics.usage.cache_read", m.Usage.CacheRead); err != nil {
			return err
		}
		if err := checkSafeInt64("metrics.usage.cache_write", m.Usage.CacheWrite); err != nil {
			return err
		}
		if err := checkSafeInt64("metrics.usage.output", m.Usage.Output); err != nil {
			return err
		}
	}
	if err := checkOptionalFiniteNonneg("token_rate", m.TokenRate); err != nil {
		return err
	}
	if err := checkOptionalSafeInt64("context_used", m.ContextUsed); err != nil {
		return err
	}
	if err := checkOptionalSafeInt64("context_window", m.ContextWindow); err != nil {
		return err
	}
	if m.ContextUsed != nil && m.ContextWindow != nil && *m.ContextUsed > *m.ContextWindow {
		return fmt.Errorf("schema-2-metrics rule violated: field=context_used class=range")
	}
	if err := checkOptionalFiniteRange("context_fill", m.ContextFill, 0, 1); err != nil {
		return err
	}
	if err := checkOptionalFiniteRange("cache_use", m.CacheUse, 0, 1); err != nil {
		return err
	}
	if err := checkOptionalFiniteNonneg("cost_usd", m.CostUSD); err != nil {
		return err
	}
	return checkCostSource("cost_source", m.CostSource, m.CostUSD)
}

func validateTerminalTimestamps(value, completed, failed, ghost string) error {
	switch value {
	case "completed":
		if completed == "" || ghost == "" || failed != "" {
			return fmt.Errorf("schema-2-terminal-timestamp rule violated: field=completed class=matrix")
		}
	case "failed":
		if failed == "" || ghost == "" || completed != "" {
			return fmt.Errorf("schema-2-terminal-timestamp rule violated: field=failed class=matrix")
		}
	case "vanished":
		if ghost == "" || completed != "" || failed != "" {
			return fmt.Errorf("schema-2-terminal-timestamp rule violated: field=vanished class=matrix")
		}
	default:
		if completed != "" || failed != "" || ghost != "" {
			return fmt.Errorf("schema-2-terminal-timestamp rule violated: field=nonterminal class=matrix")
		}
	}
	return nil
}

func validateTimestampOrder(st dumpGraphState, started, completed, failed, ghost time.Time) error {
	end := completed
	if !failed.IsZero() {
		end = failed
	}
	if !started.IsZero() && !end.IsZero() && end.Before(started) {
		return fmt.Errorf("schema-2-timestamp-order rule violated: field=started_at class=order")
	}
	if !end.IsZero() && !ghost.IsZero() && ghost.Before(end) {
		return fmt.Errorf("schema-2-timestamp-order rule violated: field=ghost_expires_at class=order")
	}
	if st.Value == "vanished" && st.Source != nil && st.Since != "" {
		since, err := parseCanonicalTime("state.since", st.Since, true)
		if err != nil {
			return err
		}
		if !started.IsZero() && since.Before(started) {
			return fmt.Errorf("schema-2-timestamp-order rule violated: field=state.since class=order")
		}
		if !ghost.IsZero() && ghost.Before(since) {
			return fmt.Errorf("schema-2-timestamp-order rule violated: field=ghost_expires_at class=order")
		}
	}
	return nil
}

func validateEdge(e dumpEdge, nodes map[string]struct{}) error {
	if err := checkID("edge.key", e.Key, maxGraphIDBytes); err != nil {
		return err
	}
	if err := checkID("edge.source", e.Source, maxGraphIDBytes); err != nil {
		return err
	}
	if err := checkID("edge.target", e.Target, maxGraphIDBytes); err != nil {
		return err
	}
	if e.Source == e.Target {
		return fmt.Errorf("schema-2-referential-integrity rule violated: field=edge class=self")
	}
	if _, ok := nodes[e.Source]; !ok {
		return fmt.Errorf("schema-2-referential-integrity rule violated: field=edge.source class=missing")
	}
	if _, ok := nodes[e.Target]; !ok {
		return fmt.Errorf("schema-2-referential-integrity rule violated: field=edge.target class=missing")
	}
	created, err := parseCanonicalTime("created_at", e.CreatedAt, true)
	if err != nil {
		return err
	}
	last, err := parseCanonicalTime("last_activity", e.LastActivity, true)
	if err != nil {
		return err
	}
	if last.Before(created) {
		return fmt.Errorf("schema-2-timestamp-order rule violated: field=last_activity class=order")
	}
	if err := checkSafePositiveUint64("event_count", e.EventCount); err != nil {
		return err
	}
	switch e.Lifecycle {
	case "active", "ghost":
	default:
		return fmt.Errorf("schema-2-edge-lifecycle rule violated: field=lifecycle class=unsupported")
	}
	switch e.Type {
	case "spawn", "service":
		if e.Provenance != "native" && e.Provenance != "aitop-sidecar" {
			return fmt.Errorf("schema-2-edge-conditional rule violated: field=provenance class=unsupported")
		}
		if e.Relationship == "" || e.Trace != "" || e.MessageKind != "" || e.Delivery != nil {
			return fmt.Errorf("schema-2-edge-conditional rule violated: field=%s class=shape", e.Type)
		}
		if _, err := decodeRawURL("relationship", e.Relationship, 1, maxRelationshipBytes); err != nil {
			return err
		}
	case "launch":
		if e.Provenance != "trace-handshake" {
			return fmt.Errorf("schema-2-edge-conditional rule violated: field=provenance class=unsupported")
		}
		if e.Relationship == "" || e.Trace == "" || e.MessageKind != "" || e.Delivery != nil {
			return fmt.Errorf("schema-2-edge-conditional rule violated: field=launch class=shape")
		}
		if _, err := decodeRawURL("relationship", e.Relationship, 1, maxRelationshipBytes); err != nil {
			return err
		}
		raw, err := decodeRawURL("trace", e.Trace, traceByteLen, traceByteLen)
		if err != nil {
			return err
		}
		if allZero(raw) {
			return fmt.Errorf("schema-2-trace rule violated: field=trace class=zero")
		}
	case "message":
		if e.Provenance != "native" {
			return fmt.Errorf("schema-2-edge-conditional rule violated: field=provenance class=unsupported")
		}
		if e.Lifecycle != "active" {
			return fmt.Errorf("schema-2-message-lifecycle rule violated: field=lifecycle class=ghost")
		}
		if e.Relationship != "" || e.Trace != "" || e.MessageKind == "" || e.Delivery == nil {
			return fmt.Errorf("schema-2-edge-conditional rule violated: field=message class=shape")
		}
		switch e.MessageKind {
		case "direct", "broadcast", "shutdown_request", "shutdown_response", "plan_approval_response":
		default:
			return fmt.Errorf("schema-2-message-kind rule violated: field=message_kind class=unsupported")
		}
		if err := validateDelivery(*e.Delivery, e.EventCount); err != nil {
			return err
		}
	default:
		return fmt.Errorf("schema-2-edge-type rule violated: field=type class=unsupported")
	}
	return nil
}

func validateDelivery(d dumpDelivery, eventCount uint64) error {
	for _, item := range []struct {
		name  string
		value uint64
	}{
		{"unknown", d.Unknown},
		{"emitted", d.Emitted},
		{"received", d.Received},
		{"failed", d.Failed},
	} {
		if err := checkSafeUint64("delivery."+item.name, item.value); err != nil {
			return err
		}
	}
	sum := d.Unknown + d.Emitted + d.Received + d.Failed
	if sum != eventCount {
		return fmt.Errorf("schema-2-delivery-sum rule violated: field=event_count class=mismatch")
	}
	var selected uint64
	switch d.Latest {
	case "unknown":
		selected = d.Unknown
	case "emitted":
		selected = d.Emitted
	case "received":
		selected = d.Received
	case "failed":
		selected = d.Failed
	default:
		return fmt.Errorf("schema-2-delivery-latest rule violated: field=latest class=unsupported")
	}
	if selected == 0 {
		return fmt.Errorf("schema-2-delivery-latest rule violated: field=latest class=zero")
	}
	return nil
}

func validateGap(g dumpGap) error {
	if err := checkID("gap.source", g.Source, maxGraphIDBytes); err != nil {
		return err
	}
	if g.Capability != nil {
		switch *g.Capability {
		case "identity", "state", "metrics", "spawn", "message", "terminal", "service":
		default:
			return fmt.Errorf("schema-2-gap-capability rule violated: field=capability class=unsupported")
		}
	}
	switch g.Kind {
	case "collector", "schema", "sequence", "saturation", "collision", "resource":
	default:
		return fmt.Errorf("schema-2-gap-kind rule violated: field=kind class=unsupported")
	}
	if _, err := parseCanonicalTime("gap.at", g.At, true); err != nil {
		return err
	}
	return checkSafePositiveUint64("gap.count", g.Count)
}

func formatCanonicalTime(field string, t time.Time, required bool) (string, error) {
	if t.IsZero() {
		if required {
			return "", fmt.Errorf("schema-2-timestamp rule violated: field=%s class=zero", field)
		}
		return "", nil
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

func formatOptionalTime(field string, t *time.Time) (string, error) {
	if t == nil {
		return "", nil
	}
	return formatCanonicalTime(field, *t, true)
}

func parseCanonicalTime(field, s string, required bool) (time.Time, error) {
	if s == "" {
		if required {
			return time.Time{}, fmt.Errorf("schema-2-timestamp rule violated: field=%s class=zero", field)
		}
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("schema-2-timestamp rule violated: field=%s class=parse", field)
	}
	if !strings.HasSuffix(s, "Z") {
		return time.Time{}, fmt.Errorf("schema-2-timestamp rule violated: field=%s class=offset", field)
	}
	if t.UTC().Format(time.RFC3339Nano) != s {
		return time.Time{}, fmt.Errorf("schema-2-timestamp rule violated: field=%s class=noncanonical", field)
	}
	if t.IsZero() {
		return time.Time{}, fmt.Errorf("schema-2-timestamp rule violated: field=%s class=zero", field)
	}
	return t, nil
}

func decodeRawURL(field, s string, min, max int) ([]byte, error) {
	if strings.ContainsRune(s, '=') {
		return nil, fmt.Errorf("schema-2-rawurl rule violated: field=%s class=padding", field)
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("schema-2-rawurl rule violated: field=%s class=decode", field)
	}
	if len(raw) < min || len(raw) > max {
		return nil, fmt.Errorf("schema-2-rawurl rule violated: field=%s bytes=%d limit=%d class=length", field, len(raw), max)
	}
	if base64.RawURLEncoding.EncodeToString(raw) != s {
		return nil, fmt.Errorf("schema-2-rawurl rule violated: field=%s class=noncanonical", field)
	}
	return raw, nil
}

func checkID(field, s string, limit int) error {
	if s == "" {
		return fmt.Errorf("schema-2-id rule violated: field=%s bytes=0 class=empty", field)
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("schema-2-id rule violated: field=%s bytes=%d class=invalid-utf8", field, len(s))
	}
	if len(s) > limit {
		return fmt.Errorf("schema-2-id rule violated: field=%s bytes=%d limit=%d class=too-long", field, len(s), limit)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("schema-2-id rule violated: field=%s bytes=%d class=control", field, len(s))
		}
	}
	return nil
}

func checkPublicRole(role string) error {
	switch types.Role(role) {
	case types.RolePrimary, types.RoleSubagent, types.RoleSidecar, types.RoleDesktop, types.RoleWorkflow, types.RoleMonitor:
		return nil
	default:
		return fmt.Errorf("schema-2-public-role rule violated: field=role class=sentinel")
	}
}

func checkRuntime(runtime string) error {
	switch types.Runtime(runtime) {
	case types.RuntimeGrok, types.RuntimeClaude, types.RuntimeCodex, types.RuntimeHermes, types.RuntimeParlor, types.RuntimeForge, types.RuntimeLocal:
		return nil
	default:
		return fmt.Errorf("schema-2-runtime rule violated: field=runtime class=unsupported")
	}
}

func checkSourceIncarnation(s string) error {
	if len(s) != 16 {
		return fmt.Errorf("schema-2-source-incarnation rule violated: field=incarnation bytes=%d class=length", len(s))
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return fmt.Errorf("schema-2-source-incarnation rule violated: field=incarnation class=encoding")
		}
	}
	if s == "0000000000000000" {
		return fmt.Errorf("schema-2-source-incarnation rule violated: field=incarnation class=zero")
	}
	return nil
}

func formatSourceIncarnation(inc graph.SourceIncarnationID) (string, error) {
	if inc == 0 {
		return "", fmt.Errorf("schema-2-source-incarnation rule violated: field=incarnation class=zero")
	}
	if uint64(inc) > uint64(maxJSONSafeInteger) {
		return "", fmt.Errorf("schema-2-safe-integer rule violated: field=incarnation class=too-large")
	}
	return fmt.Sprintf("%016x", uint64(inc)), nil
}

func authorityString(a graph.Authority) (string, error) {
	switch a {
	case graph.AuthorityPassive:
		return "passive", nil
	case graph.AuthorityNative:
		return "native", nil
	case graph.AuthorityHook:
		return "hook", nil
	default:
		return "", fmt.Errorf("schema-2-authority rule violated: field=authority class=unsupported")
	}
}

func checkPID(field string, pid int32) error {
	if pid <= 0 {
		return fmt.Errorf("schema-2-pid rule violated: field=%s class=nonpositive", field)
	}
	return nil
}

func checkSafeUint64(field string, v uint64) error {
	if v > uint64(maxJSONSafeInteger) {
		return fmt.Errorf("schema-2-safe-integer rule violated: field=%s class=too-large", field)
	}
	return nil
}

func checkSafePositiveUint64(field string, v uint64) error {
	if v == 0 {
		return fmt.Errorf("schema-2-safe-integer rule violated: field=%s class=nonpositive", field)
	}
	return checkSafeUint64(field, v)
}

func checkSafeInt64(field string, v int64) error {
	if v < 0 || uint64(v) > uint64(maxJSONSafeInteger) {
		return fmt.Errorf("schema-2-safe-integer rule violated: field=%s class=range", field)
	}
	return nil
}

func checkOptionalSafeInt64(field string, v *int64) error {
	if v == nil {
		return nil
	}
	return checkSafeInt64(field, *v)
}

func checkSafeInt(field string, v int) error {
	if v < 0 || uint64(v) > uint64(maxJSONSafeInteger) {
		return fmt.Errorf("schema-2-safe-integer rule violated: field=%s class=range", field)
	}
	return nil
}

func checkSafePositiveInt(field string, v int) error {
	if v <= 0 {
		return fmt.Errorf("schema-2-safe-integer rule violated: field=%s class=nonpositive", field)
	}
	return checkSafeInt(field, v)
}

func checkFiniteNonneg(field string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return fmt.Errorf("schema-2-float rule violated: field=%s class=range", field)
	}
	return nil
}

func checkOptionalFiniteNonneg(field string, v *float64) error {
	if v == nil {
		return nil
	}
	return checkFiniteNonneg(field, *v)
}

func checkOptionalFiniteRange(field string, v *float64, min, max float64) error {
	if v == nil {
		return nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < min || *v > max {
		return fmt.Errorf("schema-2-float rule violated: field=%s class=range", field)
	}
	return nil
}

func checkCostSource(field, source string, cost *float64) error {
	if source == "" {
		return nil
	}
	if source != "table:builtin" && source != "table:user" {
		return fmt.Errorf("schema-2-cost-source rule violated: field=%s class=unsupported", field)
	}
	if cost == nil {
		return fmt.Errorf("schema-2-cost-source rule violated: field=%s class=missing-cost", field)
	}
	return nil
}

func validStateValue(v string) bool {
	switch graph.State(v) {
	case graph.StateUnknown, graph.StateIdle, graph.StateActive, graph.StateThinking, graph.StateTool,
		graph.StateShell, graph.StateWaiting, graph.StateApproval, graph.StateBlocked, graph.StateError,
		graph.StateCompleted, graph.StateFailed, graph.StateVanished:
		return true
	default:
		return false
	}
}

func isTerminalState(v string) bool {
	return v == "completed" || v == "failed" || v == "vanished"
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func gapIdentity(g dumpGap) string {
	cap := ""
	if g.Capability != nil {
		cap = *g.Capability
	}
	return g.Source + "\x00" + cap + "\x00" + g.Kind
}

func gapLess(a, b dumpGap) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Capability == nil && b.Capability != nil {
		return true
	}
	if a.Capability != nil && b.Capability == nil {
		return false
	}
	if a.Capability != nil && b.Capability != nil && *a.Capability != *b.Capability {
		return *a.Capability < *b.Capability
	}
	return a.Kind < b.Kind
}
