// Package act is the control-plane Actor: a fourth clock behind a bounded
// intent queue. Paint never lives here. The TUI only Enqueues; adapters exec
// off-tick.
package act

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aitop/internal/types"
)

const maxInFlight = 4

var errActorBusy = errors.New("actor busy")

type Op string

const (
	OpFork       Op = "fork"
	OpClone      Op = "clone"
	OpMessage    Op = "message"
	OpRestart    Op = "restart"
	OpKill       Op = "kill"
	OpPromote    Op = "promote"
	OpBudget     Op = "budget"
	OpMerge      Op = "merge"
	OpFanout     Op = "fanout"
	OpTranscript Op = "transcript"
)

type Intent struct {
	Op        Op
	Target    Target
	Args      string // prompt text, model id, "ctx,np", merge winner key
	N         int    // fanout count
	Confirmed bool
	Parent    Target // merge
	Winner    Target
	Loser     Target
}

type Result struct {
	Key string
	Op  Op
	Err string
	At  time.Time
}

type Actor struct {
	adapters map[types.Runtime]Adapter
	ch       chan Intent
	bound    int // max outstanding data-path intents (waiting plus in flight); default 16
	results  atomic.Pointer[Result]

	// CapsuleDir is where Fork/Clone write capsule.json+md before adapter.Fork.
	// Empty skips the write so occupancy-only tests stay file-free; production
	// sets CapsuleRoot().
	CapsuleDir string

	// ForksDir is where a successful Fork/Clone writes <child-session>.json
	// so join can nest before the runtime records parent_session_id. Empty
	// skips the write; production sets ForksRoot().
	ForksDir string

	// Merger runs git merge --no-ff for OpMerge. nil uses exec git.
	Merger *Merger

	mu       sync.Mutex
	started  bool
	stopped  bool
	reserved int
	keyMus   map[string]*sync.Mutex
	sem      chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(ads map[types.Runtime]Adapter) *Actor {
	copied := make(map[types.Runtime]Adapter, len(ads))
	for k, v := range ads {
		copied[k] = v
	}
	return &Actor{
		adapters: copied,
		bound:    16,
		keyMus:   make(map[string]*sync.Mutex),
	}
}

func (a *Actor) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return
	}
	if a.bound <= 0 {
		a.bound = 16
	}
	a.ch = make(chan Intent, a.bound)
	a.sem = make(chan struct{}, maxInFlight)
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	a.cancel = cancel
	a.started = true
	a.wg.Add(1)
	go a.loop()
}

func (a *Actor) Stop() {
	a.mu.Lock()
	if !a.started || a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	cancel := a.cancel
	a.mu.Unlock()
	cancel()
	a.wg.Wait()
}

func (a *Actor) LastResult() *Result {
	return a.results.Load()
}

func (a *Actor) Enqueue(in Intent) error {
	if in.Op == OpKill && in.Confirmed {
		return a.enqueueConfirmedKill(in)
	}
	a.mu.Lock()
	if !a.started || a.stopped || a.ch == nil {
		a.mu.Unlock()
		return errActorBusy
	}
	if a.reserved >= a.bound {
		a.mu.Unlock()
		return errActorBusy
	}
	a.reserved++
	ch := a.ch
	ctx := a.ctx
	a.mu.Unlock()

	select {
	case ch <- in:
		return nil
	case <-ctx.Done():
		a.releaseTicket()
		return errActorBusy
	}
}

func (a *Actor) enqueueConfirmedKill(in Intent) error {
	a.mu.Lock()
	if !a.started || a.stopped {
		a.mu.Unlock()
		return errActorBusy
	}
	// Confirmed kill bypasses the bounded data queue so a wedged occupancy
	// board (queue full, adapters blocked) can still SIGINT. The one-shot
	// goroutine still takes the per-key lock so two intents for the same
	// row cannot overlap.
	a.wg.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		a.run(in)
	}()
	return nil
}

func (a *Actor) loop() {
	defer a.wg.Done()
	for {
		select {
		case <-a.ctx.Done():
			a.dropQueued()
			return
		case in := <-a.ch:
			a.mu.Lock()
			if a.stopped {
				a.mu.Unlock()
				a.releaseTicket()
				a.dropQueued()
				return
			}
			a.wg.Add(1)
			a.mu.Unlock()
			go func(in Intent) {
				defer a.wg.Done()
				defer a.releaseTicket()
				a.run(in)
			}(in)
		}
	}
}

func (a *Actor) dropQueued() {
	for {
		select {
		case <-a.ch:
			a.releaseTicket()
		default:
			return
		}
	}
}

func (a *Actor) releaseTicket() {
	a.mu.Lock()
	if a.reserved > 0 {
		a.reserved--
	}
	a.mu.Unlock()
}

func (a *Actor) keyMu(key string) *sync.Mutex {
	if key == "" {
		key = "_"
	}
	a.mu.Lock()
	m := a.keyMus[key]
	if m == nil {
		m = &sync.Mutex{}
		a.keyMus[key] = m
	}
	a.mu.Unlock()
	return m
}

func (a *Actor) run(in Intent) {
	km := a.keyMu(in.Target.Key)
	km.Lock()
	defer km.Unlock()

	select {
	case a.sem <- struct{}{}:
		defer func() { <-a.sem }()
	case <-a.ctx.Done():
		a.storeResult(in, a.ctx.Err())
		return
	}
	a.dispatch(in)
}

func (a *Actor) dispatch(in Intent) {
	ad, ok := a.adapters[in.Target.Runtime]
	if !ok || ad == nil {
		a.storeResult(in, fmt.Errorf("unsupported: no adapter for %s", in.Target.Runtime))
		return
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	switch in.Op {
	case OpFork:
		var cap Capsule
		cap, err = a.writeCapsule(in)
		if err != nil {
			a.storeResult(in, err)
			return
		}
		var spawned Spawned
		spawned, err = ad.Fork(ctx, in.Target, cap, in.Args)
		if err == nil {
			err = a.writeForkSidecar(in, cap, spawned)
		}
	case OpClone:
		var cap Capsule
		cap, err = a.writeCapsule(in)
		if err != nil {
			a.storeResult(in, err)
			return
		}
		var spawned Spawned
		spawned, err = ad.Clone(ctx, in.Target, cap)
		if err == nil {
			err = a.writeForkSidecar(in, cap, spawned)
		}
	case OpMessage:
		err = ad.Message(ctx, in.Target, in.Args)
	case OpRestart:
		err = ad.Restart(ctx, in.Target)
	case OpKill:
		err = ad.Kill(ctx, in.Target)
	case OpPromote:
		err = ad.Promote(ctx, in.Target, in.Args)
	case OpBudget:
		err = ad.Budget(ctx, in.Target, in.Args)
	case OpMerge:
		err = ad.Merge(ctx, in.Parent, in.Winner, in.Loser)
		if isUnsupported(err) {
			mer := a.Merger
			if mer == nil {
				mer = &Merger{}
			}
			err = mer.Merge(in.Parent, in.Winner, in.Loser)
		}
		if err == nil {
			parent := in.Parent
			if parent.Runtime == "" {
				parent = in.Target
			}
			msg := mergeMessage(in.Winner)
			msgErr := ad.Message(ctx, parent, msg)
			if isUnsupported(msgErr) {
				_ = WriteMergeNote(parent, mergeNoteID(in.Winner), msg)
			} else if msgErr != nil {
				err = msgErr
			}
		}
	case OpFanout:
		err = ad.Fanout(ctx, in.Target, in.N)
	case OpTranscript:
		_, err = ad.Transcript(ctx, in.Target)
	default:
		err = fmt.Errorf("unsupported: unknown op %s", in.Op)
	}
	a.storeResult(in, err)
}

func isUnsupported(err error) bool {
	return err != nil && (errors.Is(err, ErrUnsupported) || strings.Contains(err.Error(), "unsupported"))
}

func (a *Actor) storeResult(in Intent, err error) {
	r := &Result{Key: in.Target.Key, Op: in.Op, At: time.Now()}
	if err != nil {
		r.Err = err.Error()
	}
	a.results.Store(r)
}
