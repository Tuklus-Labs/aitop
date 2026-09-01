package graph

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
)

type shadowRuntime struct {
	Clock                storeClock
	NewSourceIncarnation func() (SourceIncarnationID, error)
}

type shadowChildResult struct {
	handled chan struct{}
	err     error
	once    sync.Once
}

type shadowRunRecord struct {
	mu              sync.Mutex
	used            bool
	ctx             context.Context
	store, registry shadowChildResult
}

type Shadow struct {
	store    *Store
	registry *Registry
	run      *shadowRunRecord
}

var errShadowAlreadyRun = errors.New("graph shadow lifecycle rule violated: already-run")

var errShadowNil = errors.New("graph shadow lifecycle rule violated: shadow=nil")

var errShadowNilContext = errors.New("graph shadow context rule violated: context=nil")

func newShadow(reconcileConfig ReconcileConfig, storeConfig StoreConfig, runtime shadowRuntime, collectors ...Collector) (*Shadow, error) {
	if isTypedNil(runtime.Clock) {
		return nil, shadowDependencyError("Clock", runtime.Clock != nil)
	}
	if isTypedNil(runtime.NewSourceIncarnation) {
		return nil, shadowDependencyError("NewSourceIncarnation", false)
	}

	reconciler, err := NewReconciler(reconcileConfig)
	if err != nil {
		return nil, err
	}
	clock := runtime.Clock
	store, err := newStore(storeConfig, reconciler, storeRuntime{Clock: clock})
	if err != nil {
		return nil, err
	}
	registry, err := newRegistry(store, registryRuntime{
		Clock:                clock,
		NewSourceIncarnation: runtime.NewSourceIncarnation,
	}, collectors...)
	if err != nil {
		return nil, err
	}

	return &Shadow{
		store:    store,
		registry: registry,
		run: &shadowRunRecord{
			store:    shadowChildResult{handled: make(chan struct{})},
			registry: shadowChildResult{handled: make(chan struct{})},
		},
	}, nil
}

func shadowDependencyError(field string, typed bool) error {
	class := "nil"
	if typed {
		class = "typed-nil"
	}
	return fmt.Errorf("graph shadow dependency rule violated: field=%s class=%s", field, class)
}

func NewShadow(reconcileConfig ReconcileConfig, storeConfig StoreConfig, collectors ...Collector) (*Shadow, error) {
	return newShadow(reconcileConfig, storeConfig, shadowRuntime{
		Clock: realStoreClock{},
		NewSourceIncarnation: func() (SourceIncarnationID, error) {
			return randomSourceIncarnation(rand.Reader)
		},
	}, collectors...)
}

func (s *Shadow) Run(ctx context.Context) error {
	if isTypedNil(ctx) {
		return errShadowNilContext
	}
	if s == nil || s.run == nil || s.store == nil || s.registry == nil {
		return errShadowNil
	}

	record := s.run
	record.mu.Lock()
	if record.used {
		record.mu.Unlock()
		return errShadowAlreadyRun
	}
	record.used = true
	childCtx, cancel := context.WithCancel(ctx)
	record.ctx = childCtx
	record.mu.Unlock()

	runChild := func(label string, result *shadowChildResult, run func(context.Context) error) {
		go func() {
			finish := func(err error) {
				err = shadowNormalizeChildError(childCtx, err)
				result.once.Do(func() {
					if err != nil {
						cancel()
					}
					record.mu.Lock()
					result.err = err
					record.mu.Unlock()
					close(result.handled)
				})
			}
			defer finish(fmt.Errorf("graph shadow child lifecycle rule violated: child=%s class=incomplete", label))
			finish(run(childCtx))
		}()
	}
	runChild("store", &record.store, s.store.Run)
	runChild("registry", &record.registry, s.registry.Run)

	<-record.store.handled
	<-record.registry.handled
	record.mu.Lock()
	storeErr, registryErr := record.store.err, record.registry.err
	record.mu.Unlock()
	cancel()
	if storeErr == nil && registryErr == nil {
		return nil
	}
	return errors.Join(storeErr, registryErr)
}

func (s *Shadow) Snapshot() *Snapshot {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Snapshot()
}

// Flush publishes the Store prefix already accepted by this Shadow.
func (s *Shadow) Flush(ctx context.Context) error {
	if s == nil || s.store == nil {
		return errShadowNil
	}
	return s.store.flush(ctx)
}

const shadowErrorTreeDepthLimit = 64

func shadowNormalizeChildError(ctx context.Context, err error) error {
	if err == nil || isTypedNil(ctx) || ctx.Err() == nil {
		return err
	}
	if shadowCancellationOnly(err, 0) {
		return nil
	}
	return err
}

func shadowCancellationOnly(err error, depth int) (cancellationOnly bool) {
	defer func() {
		if recover() != nil {
			cancellationOnly = false
		}
	}()
	if err == nil || isTypedNil(err) || depth >= shadowErrorTreeDepthLimit {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		children := many.Unwrap()
		found := false
		for _, child := range children {
			if child == nil {
				continue
			}
			found = true
			if !shadowCancellationOnly(child, depth+1) {
				return false
			}
		}
		return found
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		child := one.Unwrap()
		return child != nil && shadowCancellationOnly(child, depth+1)
	}
	return false
}
