package elector

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/atomic"

	"github.com/git-hulk/go-elect/elector/engine"
	"github.com/git-hulk/go-elect/internal"
)

const (
	electStateNone = iota + 1
	electStateRunning
	electStateStopped
)

type Runner interface {
	Run(ctx context.Context) error
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

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.loop(ctx)
	}()
	return nil
}

// IsLeader is used to check if the elector is leader
func (e *Elector) IsLeader() bool {
	return e.session.IsLeader()
}

func (e *Elector) loop(ctx context.Context) {
	var role string
	var err error
	for {
		select {
		case <-e.shutdownCh:
			internal.GetLogger().Printf("elector[%s] on key[%s] was stopped while receiving shutdown signal",
				e.session.ID(),
				e.session.Key())
			return
		default:
			if e.session.IsLeader() {
				role = "leader"
				err = e.runner.Run(ctx)
			}
			if err != nil {
				internal.GetLogger().Printf("[%s:%s] run error: %v", e.session.ID(), role, err)
			}
		}
	}
}

// Resign is used to resign the leader if the elector is leader role now
func (e *Elector) Resign(ctx context.Context) error {
	return e.session.Resign(ctx)
}

// Release is used to stop the elector instance and release the session
func (e *Elector) Release(ctx context.Context) error {
	if !e.state.CompareAndSwap(electStateRunning, electStateStopped) {
		return nil
	}

	close(e.shutdownCh)
	return e.session.Release(ctx)
}

func (e *Elector) Wait() {
	e.wg.Wait()
}
