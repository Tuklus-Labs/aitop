package graph

import (
	"context"
	"testing"
	"time"

	"aitop/internal/types"
)

func TestOccupancyEventsFromRowsIdleIsUnknownPassive(t *testing.T) {
	now := time.Date(2026, 8, 31, 6, 0, 0, 0, time.UTC)
	rows := []types.Row{{
		Process:   types.Process{PID: 42, StartTime: 1001, Runtime: types.RuntimeGrok, Role: types.RolePrimary},
		Overlay:   types.Overlay{Status: "idle"},
		OverlayOK: true,
	}}
	events := OccupancyEventsFromRows(rows, now)
	found := false
	for _, ev := range events {
		if ev.Kind != EventStateObserved {
			continue
		}
		found = true
		if err := ev.Validate(); err != nil {
			t.Fatalf("occupancy-idle-state-validate rule violated: err=%v", err)
		}
		data, ok := ev.Data.(StateObserved)
		if !ok || data.State != StateUnknown {
			t.Fatalf("occupancy-passive-idle-is-unknown rule violated: state=%v", ev.Data)
		}
	}
	if !found {
		t.Fatalf("occupancy-passive-idle-is-unknown rule violated: missing state event")
	}
}

func TestOccupancyEventsFromRowsEmitsPassiveProcessNode(t *testing.T) {
	now := time.Date(2026, 8, 31, 6, 0, 0, 0, time.UTC)
	rows := []types.Row{{
		Process: types.Process{
			PID:       42,
			StartTime: 1001,
			Runtime:   types.RuntimeGrok,
			Role:      types.RolePrimary,
			NameHint:  "grok",
		},
		Overlay: types.Overlay{
			ProvenName: "Grok",
			Project:    "aitop",
			Model:      "grok-4.5",
			Status:     "idle",
		},
		OverlayOK: true,
	}}
	events := OccupancyEventsFromRows(rows, now)
	if len(events) < 2 {
		t.Fatalf("occupancy-events-from-rows rule violated: want node+state events got=%d", len(events))
	}
	var node Event
	found := false
	for _, ev := range events {
		if ev.Kind == EventNodeObserved {
			node = ev
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("occupancy-events-from-rows node-observed rule violated: events=%d kinds missing node_observed", len(events))
	}
	if err := node.Validate(); err != nil {
		t.Fatalf("occupancy-event-validate rule violated: err=%v actor=%s", err, node.Actor)
	}
	want, err := PassiveProcessID(42, 1001)
	if err != nil {
		t.Fatalf("passive-id constructor rule violated: err=%v", err)
	}
	if node.Actor != want {
		t.Fatalf("occupancy-passive-node-id rule violated: actor=%s want=%s", node.Actor, want)
	}
	if node.Source.Mode != SourceOccupancy {
		t.Fatalf("occupancy-source-mode rule violated: mode=%s want=%s", node.Source.Mode, SourceOccupancy)
	}
	if node.Source.Ref.Authority != AuthorityPassive {
		t.Fatalf("occupancy-source-authority rule violated: authority=%d want=%d", node.Source.Ref.Authority, AuthorityPassive)
	}
	data, ok := node.Data.(NodeObserved)
	if !ok {
		t.Fatalf("occupancy-node-payload rule violated: type=%T", node.Data)
	}
	if data.Runtime != types.RuntimeGrok || data.Role != types.RolePrimary {
		t.Fatalf("occupancy-node-runtime-role rule violated: runtime=%s role=%s", data.Runtime, data.Role)
	}
}

func TestOccupancyEventsFromRowsSkipsIgnoreAndUnknownRuntime(t *testing.T) {
	now := time.Date(2026, 8, 31, 6, 0, 0, 0, time.UTC)
	rows := []types.Row{
		{Process: types.Process{PID: 1, StartTime: 1, Runtime: types.RuntimeGrok, Role: types.RoleIgnore}},
		{Process: types.Process{PID: 2, StartTime: 2, Role: types.RolePrimary}},
	}
	events := OccupancyEventsFromRows(rows, now)
	if len(events) != 0 {
		t.Fatalf("occupancy-skip-ignore-unknown-runtime rule violated: events=%d want=0", len(events))
	}
}

func TestOccupancyEventsFromRowsWalksChildren(t *testing.T) {
	now := time.Date(2026, 8, 31, 6, 0, 0, 0, time.UTC)
	rows := []types.Row{{
		Process: types.Process{PID: 10, StartTime: 10, Runtime: types.RuntimeClaude, Role: types.RolePrimary},
		Children: []types.Row{{
			Process: types.Process{PID: 11, StartTime: 11, Runtime: types.RuntimeClaude, Role: types.RoleSubagent},
		}},
	}}
	events := OccupancyEventsFromRows(rows, now)
	actors := map[NodeID]bool{}
	for _, ev := range events {
		if ev.Kind == EventNodeObserved {
			actors[ev.Actor] = true
		}
	}
	parent, _ := PassiveProcessID(10, 10)
	child, _ := PassiveProcessID(11, 11)
	if !actors[parent] || !actors[child] {
		t.Fatalf("occupancy-walks-children rule violated: actors=%v parent=%s child=%s", actors, parent, child)
	}
}

func TestOccupancyEventsFromRowsUsesSessionIDWhenValid(t *testing.T) {
	now := time.Date(2026, 8, 31, 6, 0, 0, 0, time.UTC)
	rows := []types.Row{{
		Process:   types.Process{PID: 7, StartTime: 77, Runtime: types.RuntimeClaude, Role: types.RolePrimary},
		Overlay:   types.Overlay{SessionID: "abc123", Runtime: types.RuntimeClaude, ProvenName: "Heph"},
		OverlayOK: true,
	}}
	events := OccupancyEventsFromRows(rows, now)
	want, err := ClaudeSessionID("abc123")
	if err != nil {
		t.Fatalf("claude-session-id constructor rule violated: err=%v", err)
	}
	found := false
	for _, ev := range events {
		if ev.Kind == EventNodeObserved && ev.Actor == want {
			found = true
			if err := ev.Validate(); err != nil {
				t.Fatalf("occupancy-session-node-validate rule violated: err=%v", err)
			}
		}
	}
	if !found {
		t.Fatalf("occupancy-session-id-node rule violated: want actor=%s", want)
	}
}

func TestOccupancyCollectorPublishesIntoShadow(t *testing.T) {
	rows := []types.Row{{
		Process:   types.Process{PID: 42, StartTime: 1001, Runtime: types.RuntimeGrok, Role: types.RolePrimary},
		Overlay:   types.Overlay{ProvenName: "Grok", Status: "idle"},
		OverlayOK: true,
	}}
	occ := NewOccupancyCollector(func() []types.Row { return rows })
	shadow, err := NewShadow(DefaultReconcileConfig(), DefaultStoreConfig(), occ)
	if err != nil {
		t.Fatalf("occupancy-shadow-construct rule violated: err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- shadow.Run(ctx) }()
	deadline := time.After(2 * time.Second)
	for {
		snap := shadow.Snapshot()
		if snap != nil && len(snap.Nodes) > 0 {
			want, _ := PassiveProcessID(42, 1001)
			found := false
			for _, node := range snap.Nodes {
				if node.ID == want {
					found = true
					if node.Runtime != types.RuntimeGrok {
						t.Fatalf("occupancy-shadow-node-runtime rule violated: runtime=%s", node.Runtime)
					}
					break
				}
			}
			if !found {
				t.Fatalf("occupancy-shadow-node-id rule violated: want=%s nodes=%d", want, len(snap.Nodes))
			}
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("occupancy-shadow-run-return rule violated: Run did not return after cancel")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("occupancy-shadow-publish rule violated: nodes still empty after wait snapshot=%v", snap)
		case err := <-done:
			t.Fatalf("occupancy-shadow-run-early-exit rule violated: err=%v", err)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
