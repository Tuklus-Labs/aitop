package graph

import (
	"fmt"
	"time"
)

const (
	defaultStateValidity = 5 * time.Second
	maximumStateValidity = 15 * time.Second
)

func (s State) Valid() bool {
	switch s {
	case StateUnknown, StateIdle, StateActive, StateThinking, StateTool,
		StateShell, StateWaiting, StateApproval, StateBlocked, StateError,
		StateCompleted, StateFailed, StateVanished:
		return true
	default:
		return false
	}
}

func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateVanished
}

func (s State) Protected() bool {
	switch s {
	case StateActive, StateThinking, StateTool, StateShell, StateWaiting,
		StateApproval, StateBlocked, StateError, StateFailed:
		return true
	default:
		return false
	}
}

func DefaultValidity(s State) time.Duration {
	switch s {
	case StateThinking, StateTool, StateShell, StateWaiting:
		return defaultStateValidity
	default:
		return 0
	}
}

func NormalizeValidity(s State, requested time.Duration) (time.Duration, error) {
	if !s.Valid() {
		return 0, fmt.Errorf("state-validity state rule violated: stateBytes=%d class=unsupported", len(s))
	}
	if requested < 0 {
		return 0, fmt.Errorf("state-validity duration rule violated: class=negative")
	}
	if requested == 0 {
		return DefaultValidity(s), nil
	}
	if s.Terminal() || s == StateApproval || s == StateBlocked {
		return 0, fmt.Errorf("state-validity duration rule violated: stateBytes=%d class=forbidden", len(s))
	}
	if requested > maximumStateValidity {
		return maximumStateValidity, nil
	}
	return requested, nil
}

func NormalizePassive(procState byte, cpuActive bool) State {
	if procState == 'R' || cpuActive {
		return StateActive
	}
	return StateUnknown
}

func NormalizeGeneric(status string) (State, bool) {
	if status == "busy" {
		return StateActive, true
	}
	return StateUnknown, false
}

func ValidateStateEvidence(e StateEvidence, now time.Time) error {
	if !e.Value.Valid() {
		return fmt.Errorf("state-evidence value rule violated: field=Value bytes=%d class=unsupported", len(e.Value))
	}

	sourcePresent := e.Source != (SourceRef{})
	timePresent := !e.ObservedAt.IsZero()
	if sourcePresent != timePresent {
		return fmt.Errorf("state-evidence source-time pair rule violated: sourcePresent=%t observedAtPresent=%t", sourcePresent, timePresent)
	}
	if sourcePresent {
		if err := validateStateSource(e.Source); err != nil {
			return err
		}
	}

	if e.Value.Terminal() && !sourcePresent {
		return fmt.Errorf("state-evidence terminal-source rule violated: sourcePresent=false observedAtPresent=false")
	}
	if e.Relationship != "" {
		if err := validateRelationshipID(e.Relationship); err != nil {
			return fmt.Errorf("state-evidence relationship rule violated: %w", err)
		}
	}
	if (e.Value == StateApproval || e.Value == StateBlocked) && e.Relationship == "" {
		return fmt.Errorf("state-evidence relationship rule violated: stateBytes=%d class=missing", len(e.Value))
	}

	if e.Source.Authority == AuthorityPassive {
		if e.Value != StateUnknown && e.Value != StateActive && e.Value != StateVanished {
			return fmt.Errorf("state-evidence passive-state rule violated: stateBytes=%d class=forbidden", len(e.Value))
		}
		if e.Relationship != "" || e.Sequence != nil || !e.ValidUntil.IsZero() {
			return fmt.Errorf("state-evidence passive-fields rule violated: relationshipPresent=%t sequencePresent=%t validUntilPresent=%t", e.Relationship != "", e.Sequence != nil, !e.ValidUntil.IsZero())
		}
	}

	if e.Sequence != nil {
		if !sourcePresent {
			return fmt.Errorf("state-evidence sequence-source rule violated: sourcePresent=false observedAtPresent=false")
		}
		if *e.Sequence == 0 {
			return fmt.Errorf("state-evidence sequence rule violated: class=present-zero")
		}
	}

	if !e.ValidUntil.IsZero() {
		if !sourcePresent {
			return fmt.Errorf("state-evidence validity-source rule violated: sourcePresent=false observedAtPresent=false")
		}
		if e.Value.Terminal() || e.Value == StateApproval || e.Value == StateBlocked {
			return fmt.Errorf("state-evidence validity-state rule violated: stateBytes=%d class=forbidden", len(e.Value))
		}
		if e.Source.Authority != AuthorityNative && e.Source.Authority != AuthorityHook {
			return fmt.Errorf("state-evidence validity-authority rule violated: authority=%d class=forbidden", e.Source.Authority)
		}
		if !e.ValidUntil.After(e.ObservedAt) {
			return fmt.Errorf("state-evidence validity-order rule violated: validUntilAfterObservedAt=false")
		}
		if e.ValidUntil.Sub(e.ObservedAt) > maximumStateValidity {
			return fmt.Errorf("state-evidence validity-duration rule violated: limit=%s class=above-maximum", maximumStateValidity)
		}
		if !now.Before(e.ValidUntil) {
			return fmt.Errorf("state-evidence validity-expiry rule violated: validUntilAfterNow=false")
		}
	}

	return nil
}

func PreferState(current, candidate StateEvidence, now time.Time) StateEvidence {
	currentEligible := stateEvidenceEligible(current, now)
	candidateEligible := stateEvidenceEligible(candidate, now)
	if !currentEligible {
		if candidateEligible {
			return candidate
		}
		return StateEvidence{}
	}
	if !candidateEligible {
		return current
	}

	currentRank, candidateRank := terminalRank(current.Value), terminalRank(candidate.Value)
	if currentRank != candidateRank {
		if candidateRank > currentRank {
			return candidate
		}
		return current
	}
	if current.Source.Authority != candidate.Source.Authority {
		if candidate.Source.Authority > current.Source.Authority {
			return candidate
		}
		return current
	}

	sameLane := sameCompleteSource(current.Source, candidate.Source)
	if sameLane && current.Sequence != nil && candidate.Sequence != nil {
		if *current.Sequence != *candidate.Sequence {
			if *candidate.Sequence > *current.Sequence {
				return candidate
			}
			return current
		}
	}
	if !current.ObservedAt.Equal(candidate.ObservedAt) {
		if candidate.ObservedAt.After(current.ObservedAt) {
			return candidate
		}
		return current
	}
	if sameLane {
		return current
	}
	return candidate
}

func validateStateSource(source SourceRef) error {
	if err := validateSourceID(source.ID); err != nil {
		return fmt.Errorf("state-evidence source rule violated: %w", err)
	}
	if err := validateRuntime(source.Runtime); err != nil {
		return fmt.Errorf("state-evidence source-runtime rule violated: runtimeBytes=%d class=unsupported", len(source.Runtime))
	}
	if source.Incarnation == 0 {
		return fmt.Errorf("state-evidence source-incarnation rule violated: class=zero")
	}
	if !validAuthority(source.Authority) {
		return fmt.Errorf("state-evidence source-authority rule violated: authority=%d class=unsupported", source.Authority)
	}
	return nil
}

func stateEvidenceEligible(e StateEvidence, now time.Time) bool {
	return e.ValidUntil.IsZero() || now.Before(e.ValidUntil)
}

func terminalRank(state State) uint8 {
	switch state {
	case StateCompleted, StateFailed:
		return 2
	case StateVanished:
		return 1
	default:
		return 0
	}
}

func sameCompleteSource(left, right SourceRef) bool {
	return left == right && validateStateSource(left) == nil
}
