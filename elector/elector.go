package elector

import (
	"context"
	"errors"
	"sync"
	"time"

	"go-elect/elector/store"

	"github.com/google/uuid"
	"go.uber.org/atomic"
)

const (
	maxHeartbeatInterval = 10 * time.Second
	minHeartbeatInterval = 100 * time.Millisecond

	// the heartbeat interval will be sessionTimeout / sessionCheckCount
	sessionCheckCount = 5
)

const (
	electStateNone = iota + 1
	electStateRunning
	electStateStopped
)

type Runner interface {
	RunAsLeader(ctx context.Context) error
	RunAsObserver(ctx context.Context) error
}

type Elector struct {
	state  atomic.Int32
	runner Runner
	engine store.Engine

	key            string
	sessionID      string
	sessionTimeout time.Duration
	lastResigned   time.Time

	isLeader        atomic.Bool
	leaderChangedCh chan struct{}

	wg         sync.WaitGroup
	shutdownCh chan struct{}
}

// New is used to create an elector instance
func New(e store.Engine, key string, sessionTimeout time.Duration, runner Runner) (*Elector, error) {
	if e == nil || runner == nil {
		return nil, errors.New("engine and runner cannot be nil")
	}
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}
	if sessionTimeout < sessionCheckCount*minHeartbeatInterval {
		return nil, errors.New("session timeout is too short")
	}
	elector := &Elector{
		runner:          runner,
		engine:          e,
		key:             key,
		sessionID:       uuid.NewString(),
		sessionTimeout:  sessionTimeout,
		leaderChangedCh: make(chan struct{}),
		shutdownCh:      make(chan struct{}),
	}
	elector.state.Store(electStateNone)
	return elector, nil
}

// Run is used to start the elector instance and send heartbeats periodically
func (e *Elector) Run(ctx context.Context) error {
	if !e.state.CompareAndSwap(electStateNone, electStateRunning) {
		return errors.New("elector already started")
	}

	isLeader, err := e.tryElect(ctx, e.key, e.sessionTimeout)
	if err != nil {
		return err
	}
	e.isLeader.Store(isLeader)

	go e.runLoop(ctx)
	return nil
}

// IsLeader is used to check if the elector is leader
func (e *Elector) IsLeader() bool {
	return e.isLeader.Load()
}

func (e *Elector) tryElect(ctx context.Context, key string, sessionTimeout time.Duration) (bool, error) {
	err := e.engine.Elect(ctx, key, e.sessionID, sessionTimeout)
	if err != nil && err != store.ErrLeaderElected {
		return false, err
	}
	return err == nil, nil
}

func (e *Elector) notifyLeaderChanged() {
	if e.state.Load() != electStateRunning {
		return
	}
	select {
	case e.leaderChangedCh <- struct{}{}:
	default:
	}
}

func (e *Elector) runLoop(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()

	var err error
	for {
		select {
		case <-e.shutdownCh:
			return
		case <-e.leaderChangedCh:
			// TODO: log leader changed
		default:
			if e.isLeader.Load() {
				err = e.runner.RunAsLeader(ctx)
			} else {
				err = e.runner.RunAsObserver(ctx)
			}
			if err != nil {
				// TODO: log error
			}
		}
	}
}

// Resign is used to resign the leader, it will return ErrNoLockHolder if not leader
func (e *Elector) Resign() error {
	if !e.isLeader.Load() {
		return store.ErrNoLockHolder
	}
	if err := e.engine.Resign(context.Background(), e.key); err != nil {
		if errors.Is(err, store.ErrNoLockHolder) {
			return store.ErrNoLockHolder
		}
		return err
	}

	e.lastResigned = time.Now()
	e.isLeader.Store(false)
	e.notifyLeaderChanged()
	return nil
}

// Stop is used to stop the elector instance
func (e *Elector) Stop() error {
	if e.state.Load() == electStateStopped {
		return nil
	}
	e.state.Store(electStateStopped)

	close(e.shutdownCh)
	close(e.leaderChangedCh)
	e.wg.Wait()

	if e.isLeader.Load() {
		return e.engine.Resign(context.Background(), e.key)
	}
	return nil
}
