package supervisor

/*
Task 10 Phase B risk map (GF-T10):

  Invariants: unique control-free UTF-8 names, nonnil Err, cloned sorted
  Failures, sibling failure does not cancel, owned-cancel only.
  State transitions: open/stopping/stopped, reserve-before-launch, Shutdown
  once then wait, Go rejected after cancel/stopping.
  Boundaries: name 0/1/128/129, controls, invalid UTF-8, duplicates.
  Malformed: invalid names, nil run, duplicate Go.
  Concurrency: completion channels, never sleep-sync.
  Persistence: N/A.
  Integration: Supervisor API only in this package.
  Regression traps: after-launch reservation, distinct concurrent Shutdown
  results, unsorted aliases, unowned cancel suppression, Go while stopping.
*/

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

const supervisorTestTimeout = 5 * time.Second

func waitSignal(t *testing.T, ch <-chan struct{}, rule string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(supervisorTestTimeout):
		t.Fatalf("%s rule violated: signal=false timeout=%s", rule, supervisorTestTimeout)
	}
}

func waitErr(t *testing.T, ch <-chan error, rule string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(supervisorTestTimeout):
		t.Fatalf("%s rule violated: result=false timeout=%s", rule, supervisorTestTimeout)
	}
	return nil
}

func waitFailures(t *testing.T, ch <-chan []Failure, rule string) []Failure {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(supervisorTestTimeout):
		t.Fatalf("%s rule violated: result=false timeout=%s", rule, supervisorTestTimeout)
	}
	return nil
}

func requireFailureName(t *testing.T, name string, rule string) {
	t.Helper()
	if name == "" || len(name) > 128 || !utf8.ValidString(name) {
		t.Fatalf("%s rule violated: name=%q bytes=%d validUTF8=%t", rule, name, len(name), utf8.ValidString(name))
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			t.Fatalf("%s rule violated: name=%q control=%U", rule, name, r)
		}
	}
}

func TestSupervisorNamedTaskFailureDoesNotCancelSiblings(t *testing.T) {
	s := New(context.Background())
	boomErr := errors.New("supervisor-sibling-boom-secret")
	boomDone := make(chan struct{})
	heldEntered := make(chan struct{})
	heldFinished := make(chan struct{})
	if err := s.Go("boom", func(context.Context) error {
		close(boomDone)
		return boomErr
	}); err != nil {
		t.Fatalf("supervisor-named-task-failure-does-not-cancel-siblings rule violated: boom Go err=%v", err)
	}
	if err := s.Go("held", func(ctx context.Context) error {
		close(heldEntered)
		<-ctx.Done()
		close(heldFinished)
		return ctx.Err()
	}); err != nil {
		t.Fatalf("supervisor-named-task-failure-does-not-cancel-siblings rule violated: held Go err=%v", err)
	}
	waitSignal(t, boomDone, "supervisor-named-task-failure-does-not-cancel-siblings boom-finished")
	waitSignal(t, heldEntered, "supervisor-named-task-failure-does-not-cancel-siblings held-entered")
	if err := s.Context().Err(); err != nil {
		t.Fatalf("supervisor-named-task-failure-does-not-cancel-siblings rule violated: supervisor context canceled by sibling failure err=%v", err)
	}
	select {
	case <-heldFinished:
		t.Fatalf("supervisor-named-task-failure-does-not-cancel-siblings rule violated: held returned after boom without Shutdown heldFinished=true supervisorErr=%v", s.Context().Err())
	default:
	}
	got := s.Shutdown()
	waitSignal(t, heldFinished, "supervisor-named-task-failure-does-not-cancel-siblings held-after-shutdown")
	if len(got) != 1 || got[0].Name != "boom" || !errors.Is(got[0].Err, boomErr) {
		t.Fatalf("supervisor-named-task-failure-does-not-cancel-siblings rule violated: failures=%+v want name=boom err=%v", got, boomErr)
	}
}

func TestSupervisorShutdownCancelsOnceAndWaits(t *testing.T) {
	s := New(context.Background())
	canceled := make(chan struct{})
	release := make(chan struct{})
	if err := s.Go("slow", func(ctx context.Context) error {
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	}); err != nil {
		t.Fatalf("supervisor-shutdown-cancels-once-and-waits rule violated: Go err=%v", err)
	}
	first := make(chan []Failure, 1)
	go func() { first <- s.Shutdown() }()
	waitSignal(t, canceled, "supervisor-shutdown-cancels-once-and-waits task-canceled")
	select {
	case got := <-first:
		t.Fatalf("supervisor-shutdown-cancels-once-and-waits rule violated: Shutdown returned before task finished failures=%+v", got)
	default:
	}
	close(release)
	got := waitFailures(t, first, "supervisor-shutdown-cancels-once-and-waits first-shutdown")
	if len(got) != 0 {
		t.Fatalf("supervisor-shutdown-cancels-once-and-waits rule violated: owned cancel recorded as failure failures=%+v", got)
	}
	second := s.Shutdown()
	if len(second) != 0 {
		t.Fatalf("supervisor-shutdown-cancels-once-and-waits rule violated: second Shutdown failures=%+v want empty", second)
	}
}

func TestSupervisorRejectsDuplicateTaskName(t *testing.T) {
	s := New(context.Background())
	block := make(chan struct{})
	entered := make(chan struct{})
	if err := s.Go("same", func(context.Context) error {
		close(entered)
		<-block
		return nil
	}); err != nil {
		t.Fatalf("supervisor-rejects-duplicate-task-name rule violated: first Go err=%v", err)
	}
	waitSignal(t, entered, "supervisor-rejects-duplicate-task-name first-entered")
	var launched atomic.Bool
	err := s.Go("same", func(context.Context) error {
		launched.Store(true)
		return nil
	})
	if err == nil || launched.Load() {
		t.Fatalf("supervisor-rejects-duplicate-task-name rule violated: err=%v launched=%t", err, launched.Load())
	}
	if err := s.Go("other", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("supervisor-rejects-duplicate-task-name rule violated: distinct name Go err=%v", err)
	}
	close(block)
	s.Shutdown()
}

func TestSupervisorConcurrentShutdownSharesResult(t *testing.T) {
	s := New(context.Background())
	alphaErr := errors.New("supervisor-concurrent-alpha-secret")
	zetaErr := errors.New("supervisor-concurrent-zeta-secret")
	if err := s.Go("zeta", func(context.Context) error { return zetaErr }); err != nil {
		t.Fatalf("supervisor-concurrent-shutdown-shares-result rule violated: zeta Go err=%v", err)
	}
	if err := s.Go("alpha", func(context.Context) error { return alphaErr }); err != nil {
		t.Fatalf("supervisor-concurrent-shutdown-shares-result rule violated: alpha Go err=%v", err)
	}
	const n = 8
	var started sync.WaitGroup
	started.Add(n)
	results := make(chan []Failure, n)
	for i := 0; i < n; i++ {
		go func() {
			started.Done()
			results <- s.Shutdown()
		}()
	}
	started.Wait()
	var got [][]Failure
	for i := 0; i < n; i++ {
		got = append(got, waitFailures(t, results, "supervisor-concurrent-shutdown-shares-result caller"))
	}
	if len(got[0]) != 2 || got[0][0].Name != "alpha" || got[0][1].Name != "zeta" {
		t.Fatalf("supervisor-concurrent-shutdown-shares-result rule violated: first=%+v want sorted [alpha zeta]", got[0])
	}
	if !errors.Is(got[0][0].Err, alphaErr) || !errors.Is(got[0][1].Err, zetaErr) {
		t.Fatalf("supervisor-concurrent-shutdown-shares-result rule violated: err identity first=%+v", got[0])
	}
	got[0][0].Name = "mutated"
	for i := 1; i < n; i++ {
		if len(got[i]) != 2 || got[i][0].Name != "alpha" || got[i][1].Name != "zeta" {
			t.Fatalf("supervisor-concurrent-shutdown-shares-result rule violated: caller=%d result=%+v want cloned [alpha zeta]", i, got[i])
		}
		if !errors.Is(got[i][0].Err, alphaErr) || !errors.Is(got[i][1].Err, zetaErr) {
			t.Fatalf("supervisor-concurrent-shutdown-shares-result rule violated: caller=%d err identity result=%+v", i, got[i])
		}
	}
}

func TestSupervisorGoRejectedAfterStopping(t *testing.T) {
	s := New(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	if err := s.Go("held", func(ctx context.Context) error {
		close(entered)
		<-release
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatalf("supervisor-go-rejected-after-stopping rule violated: held Go err=%v", err)
	}
	waitSignal(t, entered, "supervisor-go-rejected-after-stopping held-entered")
	shutDone := make(chan []Failure, 1)
	go func() { shutDone <- s.Shutdown() }()
	select {
	case <-s.Context().Done():
	case <-time.After(supervisorTestTimeout):
		t.Fatalf("supervisor-go-rejected-after-stopping rule violated: Shutdown did not cancel timeout=%s", supervisorTestTimeout)
	}
	var launched atomic.Bool
	err := s.Go("late", func(context.Context) error {
		launched.Store(true)
		return nil
	})
	if err == nil || launched.Load() {
		t.Fatalf("supervisor-go-rejected-after-stopping rule violated: Go while stopping err=%v launched=%t", err, launched.Load())
	}
	close(release)
	waitFailures(t, shutDone, "supervisor-go-rejected-after-stopping shutdown")
	launched.Store(false)
	err = s.Go("after", func(context.Context) error {
		launched.Store(true)
		return nil
	})
	if err == nil || launched.Load() {
		t.Fatalf("supervisor-go-rejected-after-stopping rule violated: Go after Shutdown err=%v launched=%t", err, launched.Load())
	}

	parent, cancel := context.WithCancel(context.Background())
	canceled := New(parent)
	cancel()
	err = canceled.Go("canceled-parent", func(context.Context) error {
		launched.Store(true)
		return nil
	})
	if err == nil || launched.Load() {
		t.Fatalf("supervisor-go-rejected-after-stopping rule violated: Go after parent cancel err=%v launched=%t", err, launched.Load())
	}
	canceled.Shutdown()
}

func TestSupervisorFailuresSortedAndImmutable(t *testing.T) {
	s := New(context.Background())
	zErr := errors.New("supervisor-sort-zeta-secret")
	aErr := errors.New("supervisor-sort-alpha-secret")
	if err := s.Go("zeta", func(context.Context) error { return zErr }); err != nil {
		t.Fatalf("supervisor-failures-sorted-and-immutable rule violated: zeta Go err=%v", err)
	}
	if err := s.Go("alpha", func(context.Context) error { return aErr }); err != nil {
		t.Fatalf("supervisor-failures-sorted-and-immutable rule violated: alpha Go err=%v", err)
	}
	got := s.Shutdown()
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("supervisor-failures-sorted-and-immutable rule violated: shutdown=%+v want sorted [alpha zeta]", got)
	}
	got[0].Name = "mutated"
	again := s.Failures()
	if len(again) != 2 || again[0].Name != "alpha" || again[1].Name != "zeta" {
		t.Fatalf("supervisor-failures-sorted-and-immutable rule violated: Failures after mutate=%+v want [alpha zeta]", again)
	}
	second := s.Shutdown()
	if len(second) != 2 || second[0].Name != "alpha" || second[1].Name != "zeta" {
		t.Fatalf("supervisor-failures-sorted-and-immutable rule violated: second Shutdown=%+v want [alpha zeta]", second)
	}
}

func TestSupervisorSuppressesOnlyOwnedCancellation(t *testing.T) {
	s := New(context.Background())
	unowned := context.Canceled
	realErr := errors.New("supervisor-unowned-real-secret")
	if err := s.Go("unowned", func(context.Context) error { return unowned }); err != nil {
		t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: unowned Go err=%v", err)
	}
	deadline := time.After(supervisorTestTimeout)
	for {
		open := s.Failures()
		found := false
		for _, f := range open {
			if f.Name == "unowned" && errors.Is(f.Err, context.Canceled) {
				found = true
				break
			}
		}
		if found {
			break
		}
		if s.Context().Err() != nil {
			t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: supervisor canceled before unowned failure was recorded failures=%+v", open)
		}
		select {
		case <-deadline:
			t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: unowned failure not recorded while open failures=%+v", open)
		default:
			runtime.Gosched()
		}
	}
	if err := s.Go("owned", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: owned Go err=%v", err)
	}
	if err := s.Go("real", func(ctx context.Context) error {
		<-ctx.Done()
		return realErr
	}); err != nil {
		t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: real Go err=%v", err)
	}
	got := s.Shutdown()
	if len(got) != 2 || got[0].Name != "real" || got[1].Name != "unowned" {
		t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: failures=%+v want [real unowned] without owned cancel", got)
	}
	if !errors.Is(got[0].Err, realErr) || !errors.Is(got[1].Err, context.Canceled) {
		t.Fatalf("supervisor-suppresses-only-owned-cancellation rule violated: err identity failures=%+v", got)
	}
}

func TestSupervisorReservesNameBeforeLaunch(t *testing.T) {
	s := New(context.Background())
	block := make(chan struct{})
	if err := s.Go("held", func(context.Context) error {
		<-block
		return nil
	}); err != nil {
		t.Fatalf("supervisor-reserves-name-before-launch rule violated: first Go err=%v", err)
	}
	var launched atomic.Bool
	err := s.Go("held", func(context.Context) error {
		launched.Store(true)
		return nil
	})
	if err == nil || launched.Load() {
		t.Fatalf("supervisor-reserves-name-before-launch rule violated: duplicate Go before first run returned err=%v launched=%t", err, launched.Load())
	}
	close(block)
	s.Shutdown()
}

func TestFailureShapeBoundsSortAndClone(t *testing.T) {
	s := New(context.Background())
	var launched atomic.Int32
	run := func(context.Context) error {
		launched.Add(1)
		return nil
	}
	oversize := strings.Repeat("n", 129)
	invalidUTF8 := string([]byte{0xff, 0xfe, 0xfd})
	cases := []struct {
		name  string
		class string
	}{
		{"", "empty"},
		{"\x01", "control"},
		{"a\nb", "control"},
		{invalidUTF8, "invalid-utf8"},
		{oversize, "too-long"},
	}
	for _, tc := range cases {
		before := launched.Load()
		err := s.Go(tc.name, run)
		if err == nil || launched.Load() != before {
			t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: nameBytes=%d class=%s err=%v launchedDelta=%d", len(tc.name), tc.class, err, launched.Load()-before)
		}
		text := err.Error()
		if !strings.Contains(text, "class="+tc.class) {
			t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: class=%s err=%q", tc.class, text)
		}
		if strings.Contains(text, oversize) || strings.Contains(text, invalidUTF8) {
			t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: rejected bytes echoed err=%q", text)
		}
	}
	if err := s.Go(strings.Repeat("a", 1), func(context.Context) error { return errors.New("one") }); err != nil {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: 1-byte name rejected err=%v", err)
	}
	if err := s.Go(strings.Repeat("b", 128), func(context.Context) error { return errors.New("max") }); err != nil {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: 128-byte name rejected err=%v", err)
	}
	if err := s.Go("nilerr", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: nil-error task Go err=%v", err)
	}
	dup := s.Go("a", run)
	if dup == nil {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: duplicate name admitted launched=%d", launched.Load())
	}
	got := s.Shutdown()
	if len(got) != 2 {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: failures=%+v want 2 nonnil errors excluding nil-return task", got)
	}
	if got[0].Name > got[1].Name {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: unsorted names=%q,%q", got[0].Name, got[1].Name)
	}
	for i, f := range got {
		requireFailureName(t, f.Name, "failure-shape-bounds-sort-and-clone")
		if f.Err == nil {
			t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: index=%d nil Err name=%q", i, f.Name)
		}
	}
	savedName := got[0].Name
	got[0].Name = "mutated"
	cloned := s.Failures()
	if len(cloned) != 2 || cloned[0].Name != savedName || cloned[1].Name != got[1].Name {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: clone not independent saved=%q cloned=%+v mutated=%+v", savedName, cloned, got)
	}
	if cloned[0].Name == "mutated" || cloned[1].Name == "mutated" {
		t.Fatalf("failure-shape-bounds-sort-and-clone rule violated: clone shares backing cloned=%+v", cloned)
	}
}
