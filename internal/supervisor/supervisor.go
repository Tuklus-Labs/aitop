package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"unicode"
	"unicode/utf8"
)

const maxTaskNameBytes = 128

type supervisorState int

const (
	stateOpen supervisorState = iota
	stateStopping
	stateStopped
)

type Failure struct {
	Name string
	Err  error
}

type Supervisor struct {
	mu       sync.Mutex
	state    supervisorState
	ctx      context.Context
	cancel   context.CancelFunc
	names    map[string]struct{}
	failures []Failure
	result   []Failure
	wg       sync.WaitGroup
	done     chan struct{}
}

func New(parent context.Context) *Supervisor {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Supervisor{
		ctx:    ctx,
		cancel: cancel,
		names:  make(map[string]struct{}),
		done:   make(chan struct{}),
	}
}

func (s *Supervisor) Context() context.Context {
	return s.ctx
}

func (s *Supervisor) Go(name string, run func(context.Context) error) error {
	if err := validateTaskName(name); err != nil {
		return err
	}
	if run == nil {
		return errNilTask
	}
	s.mu.Lock()
	if s.state != stateOpen || s.ctx.Err() != nil {
		s.mu.Unlock()
		return errGoAfterStopping
	}
	if _, exists := s.names[name]; exists {
		s.mu.Unlock()
		return errDuplicateTaskName
	}
	s.names[name] = struct{}{}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		err := run(s.ctx)
		s.record(name, err)
	}()
	return nil
}

func (s *Supervisor) Failures() []Failure {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == stateStopped {
		return cloneFailures(s.result)
	}
	return sortCloneFailures(s.failures)
}

func (s *Supervisor) Shutdown() []Failure {
	s.mu.Lock()
	switch s.state {
	case stateStopped:
		out := cloneFailures(s.result)
		s.mu.Unlock()
		return out
	case stateStopping:
		done := s.done
		s.mu.Unlock()
		<-done
		s.mu.Lock()
		out := cloneFailures(s.result)
		s.mu.Unlock()
		return out
	default:
		s.state = stateStopping
		done := s.done
		s.mu.Unlock()
		s.cancel()
		s.wg.Wait()
		s.mu.Lock()
		s.result = sortCloneFailures(s.failures)
		s.state = stateStopped
		out := cloneFailures(s.result)
		close(done)
		s.mu.Unlock()
		return out
	}
}

func (s *Supervisor) record(name string, err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Suppress only the Supervisor-owned cancellation after stopping begins.
	// An arbitrary context.Canceled while still open is a recorded failure.
	if s.state != stateOpen {
		if ctxErr := s.ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return
		}
	}
	s.failures = append(s.failures, Failure{Name: name, Err: err})
}

func validateTaskName(name string) error {
	if name == "" {
		return taskNameError(0, "empty")
	}
	if !utf8.ValidString(name) {
		return taskNameError(len(name), "invalid-utf8")
	}
	if len(name) > maxTaskNameBytes {
		return taskNameError(len(name), "too-long")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return taskNameError(len(name), "control")
		}
	}
	return nil
}

func taskNameError(n int, class string) error {
	return fmt.Errorf("supervisor task name rule violated: bytes=%d limit=%d class=%s", n, maxTaskNameBytes, class)
}

func sortCloneFailures(in []Failure) []Failure {
	out := cloneFailures(in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func cloneFailures(in []Failure) []Failure {
	if in == nil {
		return []Failure{}
	}
	out := make([]Failure, len(in))
	copy(out, in)
	return out
}

var (
	errDuplicateTaskName = errors.New("supervisor duplicate task name rule violated")
	errGoAfterStopping   = errors.New("supervisor go after stopping rule violated")
	errNilTask           = errors.New("supervisor nil task rule violated")
)
