package graph

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"aitop/internal/present"
	"aitop/internal/types"
)

const SourceAITopOccupancy SourceID = "aitop:occupancy"

const occupancySourceIncarnation SourceIncarnationID = 1

// OccupancyEventsFromRows maps live occupancy rows into passive graph events.
// It does not parse runtime transcripts. Session IDs from overlays are used
// when they already satisfy canonical ID rules; otherwise the node stays a
// process or unit identity.
func OccupancyEventsFromRows(rows []types.Row, now time.Time) []Event {
	if now.IsZero() {
		return nil
	}
	var events []Event
	var walk func([]types.Row)
	walk = func(list []types.Row) {
		for _, row := range list {
			events = append(events, occupancyEventsForRow(row, now)...)
			if len(row.Children) > 0 {
				walk(row.Children)
			}
		}
	}
	walk(rows)
	return events
}

func occupancyEventsForRow(row types.Row, now time.Time) []Event {
	runtime := occupancyRuntime(row)
	role := occupancyRole(row)
	if runtime == types.RuntimeUnknown || !validRole(role) {
		return nil
	}
	actor, incarnation, process, ok := occupancyIdentity(row, runtime)
	if !ok {
		return nil
	}
	source := EventSource{
		Ref: SourceRef{
			ID:          SourceAITopOccupancy,
			Runtime:     types.RuntimeLocal,
			Incarnation: occupancySourceIncarnation,
			Authority:   AuthorityPassive,
		},
		Mode: SourceOccupancy,
	}
	node := Event{
		Schema:           1,
		Source:           source,
		ID:               occupancyEventID(EventNodeObserved, actor, incarnation, now, 0),
		ReceivedAt:       now,
		Kind:             EventNodeObserved,
		Actor:            actor,
		ActorIncarnation: incarnation,
		Data: NodeObserved{
			Runtime:    runtime,
			Role:       role,
			ProvenName: occupancyDisplay(present.Name(row)),
			Model:      occupancyDisplay(row.Overlay.Model),
			Project:    occupancyDisplay(row.Overlay.Project),
			Worktree:   occupancyDisplay(row.Overlay.Worktree),
			Process:    process,
		},
	}
	if !row.Overlay.StartedAt.IsZero() {
		started := row.Overlay.StartedAt.UTC()
		data := node.Data.(NodeObserved)
		data.StartedAt = &started
		node.Data = data
	}
	state := occupancyState(row)
	stateEvent := Event{
		Schema:           1,
		Source:           source,
		ID:               occupancyEventID(EventStateObserved, actor, incarnation, now, 1),
		ReceivedAt:       now,
		Kind:             EventStateObserved,
		Actor:            actor,
		ActorIncarnation: incarnation,
		Data:             StateObserved{State: state},
	}
	out := []Event{node, stateEvent}
	if metrics, ok := occupancyMetrics(row.Overlay); ok {
		out = append(out, Event{
			Schema:           1,
			Source:           source,
			ID:               occupancyEventID(EventMetricsObserved, actor, incarnation, now, 2),
			ReceivedAt:       now,
			Kind:             EventMetricsObserved,
			Actor:            actor,
			ActorIncarnation: incarnation,
			Data:             MetricsObserved{Metrics: metrics},
		})
	}
	return out
}

func occupancyRuntime(row types.Row) types.Runtime {
	if validateRuntime(row.Process.Runtime) == nil {
		return row.Process.Runtime
	}
	if validateRuntime(row.Overlay.Runtime) == nil {
		return row.Overlay.Runtime
	}
	return types.RuntimeUnknown
}

func occupancyRole(row types.Row) types.Role {
	if validRole(row.Process.Role) {
		return row.Process.Role
	}
	return types.RoleDrop
}

func occupancyIdentity(row types.Row, runtime types.Runtime) (NodeID, IncarnationID, *ProcessIdentity, bool) {
	if row.OverlayOK && row.Overlay.SessionID != "" {
		if id, inc, ok := occupancySessionIdentity(runtime, row.Overlay.SessionID, row); ok {
			return id, inc, occupancyProcess(row), true
		}
	}
	if row.OverlayOnly && row.Overlay.Dark && row.Overlay.SessionName != "" {
		id, err := LocalUnitID(row.Overlay.SessionName)
		if err != nil {
			return "", "", nil, false
		}
		inc, err := InvocationIncarnation(runtime, row.Overlay.SessionName)
		if err != nil {
			return "", "", nil, false
		}
		return id, inc, nil, true
	}
	if row.Process.PID <= 0 || row.Process.StartTime == 0 {
		return "", "", nil, false
	}
	id, err := PassiveProcessID(row.Process.PID, row.Process.StartTime)
	if err != nil {
		return "", "", nil, false
	}
	proc := ProcessIdentity{PID: row.Process.PID, StartTicks: row.Process.StartTime}
	inc, err := ProcessIncarnation(runtime, proc)
	if err != nil {
		return "", "", nil, false
	}
	return id, inc, &proc, true
}

func occupancySessionIdentity(runtime types.Runtime, session string, row types.Row) (NodeID, IncarnationID, bool) {
	var (
		id  NodeID
		err error
	)
	switch runtime {
	case types.RuntimeClaude:
		id, err = ClaudeSessionID(session)
	case types.RuntimeCodex:
		id, err = CodexThreadID(session)
	case types.RuntimeGrok:
		id, err = GrokSessionID(session)
	default:
		return "", "", false
	}
	if err != nil {
		return "", "", false
	}
	proc := occupancyProcess(row)
	var inc IncarnationID
	if proc != nil {
		inc, err = ProcessIncarnation(runtime, *proc)
	} else {
		inc, err = InvocationIncarnation(runtime, session)
	}
	if err != nil {
		return "", "", false
	}
	return id, inc, true
}

func occupancyProcess(row types.Row) *ProcessIdentity {
	if row.Process.PID <= 0 || row.Process.StartTime == 0 {
		return nil
	}
	p := ProcessIdentity{PID: row.Process.PID, StartTicks: row.Process.StartTime}
	if err := validateProcessIdentity(p); err != nil {
		return nil
	}
	return &p
}

func occupancyDisplay(value string) string {
	if len(value) <= maxEventDisplayBytes {
		return value
	}
	return value[:maxEventDisplayBytes]
}

func occupancyState(row types.Row) State {
	switch present.Status(row) {
	case present.StatusBusy:
		return StateActive
	case present.StatusIdle:
		return StateIdle
	case present.StatusWait:
		return StateWaiting
	case present.StatusShell:
		return StateShell
	case present.StatusError:
		return StateError
	case present.StatusDone:
		return StateCompleted
	case present.StatusCancelled:
		return StateVanished
	default:
		return StateUnknown
	}
}

func occupancyMetrics(o types.Overlay) (Metrics, bool) {
	var metrics Metrics
	present := false
	if o.Usage.Known {
		metrics.Usage = &TokenUsage{
			Input:      o.Usage.Input,
			CacheRead:  o.Usage.CacheRead,
			CacheWrite: o.Usage.CacheWrite,
			Output:     o.Usage.Output,
		}
		present = true
	}
	if o.TokPerSec != nil {
		metrics.TokenRate = o.TokPerSec
		present = true
	}
	if o.TokensUsed != nil {
		metrics.ContextUsed = o.TokensUsed
		present = true
	}
	if o.ContextWindow != nil {
		metrics.ContextWindow = o.ContextWindow
		present = true
	}
	if o.ContextFill != nil {
		metrics.ContextFill = o.ContextFill
		present = true
	}
	if o.CostUSD != nil {
		metrics.CostUSD = o.CostUSD
		metrics.CostSource = o.CostSource
		present = true
	}
	return metrics, present
}

func occupancyEventID(kind EventKind, actor NodeID, incarnation IncarnationID, now time.Time, salt byte) EventID {
	encoded := appendLengthPrefixed(nil, "aitop.graph.occupancy-event-id.v1")
	encoded = appendLengthPrefixed(encoded, string(kind))
	encoded = appendLengthPrefixed(encoded, string(actor))
	encoded = appendLengthPrefixed(encoded, string(incarnation))
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(now.UnixNano()))
	encoded = append(encoded, stamp[:]...)
	encoded = append(encoded, salt)
	digest := sha256.Sum256(encoded)
	var id EventID
	copy(id[:], digest[:len(id)])
	return id
}

// OccupancyCollector publishes occupancy-derived graph events into a Shadow.
type OccupancyCollector struct {
	latest func() []types.Row
	notify chan struct{}
	now    func() time.Time
}

func NewOccupancyCollector(latest func() []types.Row) *OccupancyCollector {
	return &OccupancyCollector{
		latest: latest,
		notify: make(chan struct{}, 1),
		now:    time.Now,
	}
}

func (c *OccupancyCollector) Descriptor() CollectorDescriptor {
	return CollectorDescriptor{
		ID:      SourceAITopOccupancy,
		Runtime: types.RuntimeLocal,
		Schemas: []InputSchema{{Name: "aitop-occupancy", Version: 1}},
		Capabilities: []Capability{
			CapabilityIdentity,
			CapabilityMetrics,
			CapabilityState,
		},
	}
}

func (c *OccupancyCollector) Notify() {
	if c == nil {
		return
	}
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func (c *OccupancyCollector) Run(ctx context.Context, sink EventSink) error {
	if ctx == nil {
		return fmt.Errorf("graph occupancy context rule violated: context=nil")
	}
	if sink == nil {
		return fmt.Errorf("graph occupancy sink rule violated: sink=nil")
	}
	interval := 100 * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	c.publish(sink)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.notify:
			c.publish(sink)
		case <-ticker.C:
			c.publish(sink)
		}
	}
}

func (c *OccupancyCollector) publish(sink EventSink) {
	if c == nil || c.latest == nil || sink == nil {
		return
	}
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	for _, event := range OccupancyEventsFromRows(c.latest(), now) {
		_, _ = sink.Publish(event)
	}
}
