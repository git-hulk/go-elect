package elector

import (
	"context"
	"errors"
	"go-elect/internal"
	"go.uber.org/atomic"
	"sync"
	"time"

	"go-elect/elector/engine"

	"github.com/google/uuid"
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
	runner  Runner
	session engine.Session
	state   atomic.Int32

	wg         sync.WaitGroup
	shutdownCh chan struct{}
}

// New is used to create an elector instance
func New(store *engine.SessionStore, key string, sessionTimeout time.Duration, runner Runner) (*Elector, error) {
	if store == nil || runner == nil {
		return nil, errors.New("store and runner cannot be nil")
	}
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	sessionID := uuid.NewString()
	session, err := store.Create(context.Background(), key, sessionID, sessionTimeout)
	if err != nil {
		return nil, err
	}
	elector := &Elector{
		runner:     runner,
		session:    session,
		shutdownCh: make(chan struct{}),
	}
	elector.state.Store(electStateNone)
	return elector, nil
}

// Run is used to start the elector instance
func (e *Elector) Run(ctx context.Context) error {
	if !e.state.CompareAndSwap(electStateNone, electStateRunning) {
		return errors.New("elector already started")
	}

	go e.loop(ctx)
	return nil
}

// IsLeader is used to check if the elector is leader
func (e *Elector) IsLeader() bool {
	return e.session.IsLeader()
}

func (e *Elector) loop(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()

	var role string
	var err error
	for {
		select {
		case <-e.shutdownCh:
			return
		default:
			if e.session.IsLeader() {
				role = "leader"
				err = e.runner.RunAsLeader(ctx)
			} else {
				role = "observer"
				err = e.runner.RunAsObserver(ctx)
			}
			if err != nil {
				internal.GetLogger().Printf("[%s] run error: %v", role, err)
			}
		}
	}
}

// Delete is used to release the lock if held and session will be kept
func (e *Elector) Resign(ctx context.Context) error {
	return e.session.Resign(ctx)
}

// Release is used to stop the elector instance and release the session
func (e *Elector) Release() error {
	if e.state.Load() == electStateStopped {
		return nil
	}
	e.state.Store(electStateStopped)

	close(e.shutdownCh)
	e.wg.Wait()
	return e.session.Release(context.Background())
}
