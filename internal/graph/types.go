package graph

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"aitop/internal/types"
)

const maxJSONSafeInteger = 1<<53 - 1

const (
	SourceAITopReceiver      SourceID = "aitop:receiver"
	SourceAITopStoreNormal   SourceID = "aitop:store:normal"
	SourceAITopStoreCritical SourceID = "aitop:store:critical"
	SourceAITopStoreState    SourceID = "aitop:store:state"
	SourceAITopGapLedger     SourceID = "aitop:reconciler:gaps"
)

type State string

const (
	StateUnknown   State = "unknown"
	StateIdle      State = "idle"
	StateActive    State = "active"
	StateThinking  State = "thinking"
	StateTool      State = "tool"
	StateShell     State = "shell"
	StateWaiting   State = "waiting"
	StateApproval  State = "approval"
	StateBlocked   State = "blocked"
	StateError     State = "error"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateVanished  State = "vanished"
)

type Authority uint8

const (
	AuthorityPassive Authority = iota + 1
	AuthorityNative
	AuthorityHook
)

type EdgeType string

const (
	EdgeSpawn   EdgeType = "spawn"
	EdgeLaunch  EdgeType = "launch"
	EdgeMessage EdgeType = "message"
	EdgeService EdgeType = "service"
)

type Provenance string

const (
	ProvenanceNative         Provenance = "native"
	ProvenanceAITopSidecar   Provenance = "aitop-sidecar"
	ProvenanceTraceHandshake Provenance = "trace-handshake"
)

type Delivery string

const (
	DeliveryUnknown  Delivery = "unknown"
	DeliveryEmitted  Delivery = "emitted"
	DeliveryReceived Delivery = "received"
	DeliveryFailed   Delivery = "failed"
)

type MessageKind string

const (
	MessageDirect               MessageKind = "direct"
	MessageBroadcast            MessageKind = "broadcast"
	MessageShutdownRequest      MessageKind = "shutdown_request"
	MessageShutdownResponse     MessageKind = "shutdown_response"
	MessagePlanApprovalResponse MessageKind = "plan_approval_response"
)

type EdgeLifecycle string

const (
	LifecycleActive EdgeLifecycle = "active"
	LifecycleGhost  EdgeLifecycle = "ghost"
)

type ExitOutcome string

const (
	OutcomeCompleted ExitOutcome = "completed"
	OutcomeFailed    ExitOutcome = "failed"
	OutcomeVanished  ExitOutcome = "vanished"
)

type Capability string

const (
	CapabilityIdentity Capability = "identity"
	CapabilityState    Capability = "state"
	CapabilityMetrics  Capability = "metrics"
	CapabilitySpawn    Capability = "spawn"
	CapabilityMessage  Capability = "message"
	CapabilityTerminal Capability = "terminal"
	CapabilityService  Capability = "service"
)

type GapKind string

const (
	GapCollector  GapKind = "collector"
	GapSchema     GapKind = "schema"
	GapSequence   GapKind = "sequence"
	GapSaturation GapKind = "saturation"
	GapCollision  GapKind = "collision"
	GapResource   GapKind = "resource"
)

type SourceRef struct {
	ID          SourceID
	Runtime     types.Runtime
	Incarnation SourceIncarnationID
	Authority   Authority
}

type StateEvidence struct {
	Value        State
	Source       SourceRef
	ObservedAt   time.Time
	ValidUntil   time.Time
	Relationship RelationshipID
	Sequence     *uint64
}

type TokenUsage struct {
	Input      int64
	CacheRead  int64
	CacheWrite int64
	Output     int64
}

type Metrics struct {
	Usage         *TokenUsage
	TokenRate     *float64
	ContextUsed   *int64
	ContextWindow *int64
	ContextFill   *float64
	CacheUse      *float64
	CostUSD       *float64
	CostSource    string
}

type NodeState struct {
	Value      State
	Source     SourceRef
	Since      time.Time
	ValidUntil time.Time
	Stale      bool
}

type Transition struct {
	At     time.Time
	State  State
	Source SourceRef
}

type DeliveryCounts struct {
	Unknown  uint64
	Emitted  uint64
	Received uint64
	Failed   uint64
	Latest   Delivery
}

type Node struct {
	ID             NodeID
	Incarnation    IncarnationID
	Runtime        types.Runtime
	Role           types.Role
	ProvenName     string
	Model          string
	Project        string
	Worktree       string
	TaskName       string
	Process        *ProcessIdentity
	State          NodeState
	Metrics        Metrics
	StartedAt      *time.Time
	CompletedAt    *time.Time
	FailedAt       *time.Time
	GhostExpiresAt *time.Time
	Pinned         bool
	TelemetryAt    time.Time
	Partial        bool
	Transitions    []Transition
}

type Edge struct {
	Key          EdgeKey
	Source       NodeID
	Target       NodeID
	Type         EdgeType
	Provenance   Provenance
	Relationship RelationshipID
	CreatedAt    time.Time
	LastActivity time.Time
	EventCount   uint64
	Lifecycle    EdgeLifecycle
	Trace        *TraceID
	MessageKind  MessageKind
	Delivery     *DeliveryCounts
	Partial      bool
}

type Gap struct {
	Source     SourceID
	Capability *Capability
	Kind       GapKind
	At         time.Time
	Count      uint64
}

type Snapshot struct {
	At                 time.Time
	Nodes              []Node
	Edges              []Edge
	Gaps               []Gap
	TopologyRevision   uint64
	VisibilityRevision uint64
	StateRevision      uint64
	MetricsRevision    uint64
}

func RelationshipEdgeKey(typ EdgeType, source, target NodeID, relationship RelationshipID) EdgeKey {
	return hashedEdgeKey(string(typ), "relationship", string(source), string(target), string(relationship))
}

func MessageEdgeKey(source, target NodeID, kind MessageKind) EdgeKey {
	return hashedEdgeKey(string(EdgeMessage), "message", string(source), string(target), string(kind))
}

func hashedEdgeKey(prefix string, parts ...string) EdgeKey {
	encoded := appendLengthPrefixed(nil, prefix)
	for _, part := range parts {
		encoded = appendLengthPrefixed(encoded, part)
	}
	sum := sha256.Sum256(encoded)
	return EdgeKey(prefix + ":" + hex.EncodeToString(sum[:]))
}

func appendLengthPrefixed(dst []byte, value string) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func (d *DeliveryCounts) Observe(v Delivery) error {
	if d == nil {
		return fmt.Errorf("delivery counts receiver is nil")
	}

	var count *uint64
	switch v {
	case DeliveryUnknown:
		count = &d.Unknown
	case DeliveryEmitted:
		count = &d.Emitted
	case DeliveryReceived:
		count = &d.Received
	case DeliveryFailed:
		count = &d.Failed
	default:
		return fmt.Errorf("delivery %q is invalid", v)
	}
	if *count >= maxJSONSafeInteger {
		return fmt.Errorf("delivery %q count reached JSON-safe integer ceiling %d", v, uint64(maxJSONSafeInteger))
	}
	*count = *count + 1
	d.Latest = v
	return nil
}

func CloneSnapshot(in *Snapshot) *Snapshot {
	if in == nil {
		return &Snapshot{Nodes: []Node{}, Edges: []Edge{}, Gaps: []Gap{}}
	}

	out := *in
	out.Nodes = make([]Node, len(in.Nodes))
	for i := range in.Nodes {
		out.Nodes[i] = cloneNode(in.Nodes[i])
	}
	out.Edges = make([]Edge, len(in.Edges))
	for i := range in.Edges {
		out.Edges[i] = cloneEdge(in.Edges[i])
	}
	out.Gaps = make([]Gap, len(in.Gaps))
	for i := range in.Gaps {
		out.Gaps[i] = in.Gaps[i]
		out.Gaps[i].Capability = clonePointer(in.Gaps[i].Capability)
	}
	return &out
}

func cloneNode(in Node) Node {
	out := in
	out.Process = clonePointer(in.Process)
	out.Metrics.Usage = clonePointer(in.Metrics.Usage)
	out.Metrics.TokenRate = clonePointer(in.Metrics.TokenRate)
	out.Metrics.ContextUsed = clonePointer(in.Metrics.ContextUsed)
	out.Metrics.ContextWindow = clonePointer(in.Metrics.ContextWindow)
	out.Metrics.ContextFill = clonePointer(in.Metrics.ContextFill)
	out.Metrics.CacheUse = clonePointer(in.Metrics.CacheUse)
	out.Metrics.CostUSD = clonePointer(in.Metrics.CostUSD)
	out.StartedAt = clonePointer(in.StartedAt)
	out.CompletedAt = clonePointer(in.CompletedAt)
	out.FailedAt = clonePointer(in.FailedAt)
	out.GhostExpiresAt = clonePointer(in.GhostExpiresAt)
	if in.Transitions != nil {
		out.Transitions = append([]Transition{}, in.Transitions...)
	}
	return out
}

func cloneEdge(in Edge) Edge {
	out := in
	out.Trace = clonePointer(in.Trace)
	out.Delivery = clonePointer(in.Delivery)
	return out
}

func clonePointer[T any](in *T) *T {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func SortSnapshot(in *Snapshot) *Snapshot {
	out := CloneSnapshot(in)
	sort.Slice(out.Nodes, func(i, j int) bool {
		return out.Nodes[i].ID < out.Nodes[j].ID
	})
	sort.Slice(out.Edges, func(i, j int) bool {
		return out.Edges[i].Key < out.Edges[j].Key
	})
	sort.Slice(out.Gaps, func(i, j int) bool {
		if out.Gaps[i].Source != out.Gaps[j].Source {
			return out.Gaps[i].Source < out.Gaps[j].Source
		}
		left := out.Gaps[i].Capability
		right := out.Gaps[j].Capability
		if left == nil && right != nil {
			return true
		}
		if left != nil && right == nil {
			return false
		}
		if left != nil && right != nil && *left != *right {
			return *left < *right
		}
		return out.Gaps[i].Kind < out.Gaps[j].Kind
	})
	return out
}
