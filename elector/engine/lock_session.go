package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"go-elect/internal"

	"go.uber.org/atomic"
)

const (
	maxHeartbeatInterval = 10 * time.Second
	minHeartbeatInterval = 100 * time.Millisecond

	sessionCheckCount = 5
)

const (
	sessionStateInit = iota + 1
	sessionStateReleased
)

type LockSession struct {
	Mutex

	state           atomic.Int32
	leaderChangedFn LeaderChangeFn
	lastResigned    atomic.Time

	wg         sync.WaitGroup
	shutdownCh chan struct{}
}

func NewLockSession(ctx context.Context, mu Mutex) (Session, error) {
	err := mu.TryLock(ctx)
	if err != nil && errors.Is(err, ErrNotLockHolder) {
		return nil, err
	}
	session := &LockSession{
		Mutex: mu,

		shutdownCh: make(chan struct{}),
	}
	session.state.Store(sessionStateInit)

	session.wg.Add(1)
	go func() {
		defer session.wg.Done()
		session.refresh(ctx)
	}()

	return session, nil
}

func (s *LockSession) IsLeader() bool {
	return s.Mutex.IsLocked()
}

func (s *LockSession) SetLeaderChangeFn(fn LeaderChangeFn) {
	s.leaderChangedFn = fn
}

func (s *LockSession) GetHeartbeatInterval() time.Duration {
	interval := s.Mutex.Timeout() / sessionCheckCount
	if interval > maxHeartbeatInterval {
		interval = maxHeartbeatInterval
	}
	if interval < minHeartbeatInterval {
		interval = minHeartbeatInterval
	}
	return interval
}

// refresh will try to elect leader if not leader, or extend lease if leader
func (s *LockSession) refresh(ctx context.Context) {
	interval := s.GetHeartbeatInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCh:
			return
		case <-ticker.C:
			// Don't try to elect leader again before elapsed timeout if it resigned recently.
			if time.Since(s.lastResigned.Load()) < s.Mutex.Timeout() {
				continue
			}
			if !s.Mutex.IsLocked() {
				err := s.Mutex.TryLock(ctx)
				if err != nil {
					if !errors.Is(err, ErrLeaderElected) {
						internal.GetLogger().Printf("Failed to try lock, err: %v", err)
					}
					continue
				}
				s.notifyLeaderChange()
			} else {
				if err := s.Mutex.Refresh(ctx, s.Mutex.Timeout()); err != nil {
					if errors.Is(err, ErrNotLockHolder) {
						s.notifyLeaderChange()
					} else {
						internal.GetLogger().Printf("Failed to refresh session, err: %v", err)
					}
					continue
				}
				s.notifyLeaderChange()
			}
		}
	}
}

func (s *LockSession) notifyLeaderChange() {
	if s.state.Load() == sessionStateReleased || s.leaderChangedFn == nil {
		return
	}
	s.leaderChangedFn(s.IsLeader())
}

func (s *LockSession) Resign(ctx context.Context) error {
	if s.state.Load() == sessionStateReleased {
		return nil
	}

	if !s.Mutex.IsLocked() {
		return ErrNotLockHolder
	}
	s.lastResigned.Store(time.Now())
	return s.Mutex.Release(ctx)
}

func (s *LockSession) Release(ctx context.Context) error {
	if s.state.Load() == sessionStateReleased {
		return nil
	}
	s.state.Store(sessionStateReleased)

	close(s.shutdownCh)
	s.wg.Wait()
	if s.Mutex.IsLocked() {
		return s.Resign(ctx)
	}
	return nil
}
