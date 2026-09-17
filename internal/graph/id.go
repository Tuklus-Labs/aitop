package graph

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

const maxCanonicalIDBytes = 192

type NodeID string
type IncarnationID string
type SourceID string
type SourceIncarnationID uint64
type EventID [16]byte
type TraceID [16]byte
type RelationshipID string
type EdgeKey string
type ObservationKey string
type RevisionDigest [32]byte

type ProcessIdentity struct {
	PID        int32
	StartTicks uint64
}

func ClaudeSessionID(session string) (NodeID, error) {
	id, err := canonical("claude:session", session)
	return NodeID(id), err
}

func ClaudeAgentID(rootSession, agent string) (NodeID, error) {
	id, err := canonical("claude:agent", rootSession, agent)
	return NodeID(id), err
}

func CodexThreadID(thread string) (NodeID, error) {
	id, err := canonical("codex:thread", thread)
	return NodeID(id), err
}

func GrokSessionID(session string) (NodeID, error) {
	id, err := canonical("grok:session", session)
	return NodeID(id), err
}

func LocalUnitID(unit string) (NodeID, error) {
	id, err := canonical("local:unit", strings.TrimSuffix(unit, ".service"))
	return NodeID(id), err
}

func LocalProcessID(pid int32, startTicks uint64) (NodeID, error) {
	p := ProcessIdentity{PID: pid, StartTicks: startTicks}
	if err := validateProcessIdentity(p); err != nil {
		return "", err
	}
	id, err := canonical("local:pid", strconv.FormatInt(int64(pid), 10), strconv.FormatUint(startTicks, 10))
	return NodeID(id), err
}

func PassiveProcessID(pid int32, startTicks uint64) (NodeID, error) {
	p := ProcessIdentity{PID: pid, StartTicks: startTicks}
	if err := validateProcessIdentity(p); err != nil {
		return "", err
	}
	id, err := canonical("proc", strconv.FormatInt(int64(pid), 10), strconv.FormatUint(startTicks, 10))
	return NodeID(id), err
}

func ProcessIncarnation(runtime types.Runtime, p ProcessIdentity) (IncarnationID, error) {
	if err := validateRuntime(runtime); err != nil {
		return "", err
	}
	if err := validateProcessIdentity(p); err != nil {
		return "", err
	}
	id, err := canonical(string(runtime)+":proc", strconv.FormatInt(int64(p.PID), 10), strconv.FormatUint(p.StartTicks, 10))
	return IncarnationID(id), err
}

func InvocationIncarnation(runtime types.Runtime, invocation string) (IncarnationID, error) {
	if err := validateRuntime(runtime); err != nil {
		return "", err
	}
	id, err := canonical(string(runtime)+":invocation", invocation)
	return IncarnationID(id), err
}

func canonical(prefix string, parts ...string) (string, error) {
	id := prefix
	for index, part := range parts {
		if part == "" {
			return "", fmt.Errorf("canonical ID component rule violated: prefix=%s component=%d bytes=0 class=empty", prefix, index)
		}
		if !utf8.ValidString(part) {
			return "", fmt.Errorf("canonical ID component rule violated: prefix=%s component=%d bytes=%d class=invalid-utf8; component is not valid UTF-8", prefix, index, len(part))
		}
		if strings.ContainsRune(part, ':') {
			return "", fmt.Errorf("canonical ID component rule violated: prefix=%s component=%d bytes=%d class=delimiter; contains delimiter ':'", prefix, index, len(part))
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return "", fmt.Errorf("canonical ID component rule violated: prefix=%s component=%d bytes=%d class=control; contains control rune U+%04X", prefix, index, len(part), r)
			}
		}
		id += ":" + part
	}
	if len(id) > maxCanonicalIDBytes {
		return "", fmt.Errorf("canonical ID length rule violated: prefix=%s is %d bytes; maximum is %d; class=too-long", prefix, len(id), maxCanonicalIDBytes)
	}
	return id, nil
}

func validateProcessIdentity(p ProcessIdentity) error {
	if p.PID <= 0 {
		return fmt.Errorf("process-identity rule violated: field=PID class=nonpositive")
	}
	if p.StartTicks == 0 {
		return fmt.Errorf("process-identity rule violated: field=StartTicks class=nonpositive")
	}
	return nil
}

func validateRuntime(runtime types.Runtime) error {
	switch runtime {
	case types.RuntimeGrok,
		types.RuntimeClaude,
		types.RuntimeCodex,
		types.RuntimeHermes,
		types.RuntimeParlor,
		types.RuntimeForge,
		types.RuntimeLocal:
		return nil
	default:
		return fmt.Errorf("canonical ID runtime rule violated: field=Runtime bytes=%d class=unsupported; unknown or unsupported runtime", len(runtime))
	}
}
