package engine

import (
	"context"
	"errors"
	"go.uber.org/atomic"
	"sync"
	"time"
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

type LeaderChangeFn func(isLeader bool)

type SessionClient interface {
	Create(ctx context.Context, id, key string, timeout time.Duration) (Session, error)
}

type Session interface {
	IsLeader() bool
	Resign(ctx context.Context) error
	Release(ctx context.Context) error
}

type LockSession struct {
	id      string
	key     string
	timeout time.Duration

	state           atomic.Int32
	lock            Lock
	client          LockClient
	leaderChangedFn LeaderChangeFn
	lastResigned    time.Time

	wg         sync.WaitGroup
	shutdownCh chan struct{}
}

func NewLockSession(ctx context.Context, client LockClient, id, key string, timeout time.Duration) Session {
	l, err := client.TryLock(ctx, id, key, timeout)
	if err != nil {
		// TODO log error
	}
	session := &LockSession{
		client:  client,
		id:      id,
		lock:    l,
		timeout: timeout,

		shutdownCh: make(chan struct{}),
	}
	session.state.Store(sessionStateInit)
	go session.keepalive(ctx)
	return session
}

func (s *LockSession) IsLeader() bool {
	return s.lock != nil
}

func (s *LockSession) Owner() string {
	if s.lock == nil || s.state.Load() == sessionStateReleased {
		return ""
	}
	return s.lock.Owner()
}

func (s *LockSession) Timeout() time.Duration {
	return s.timeout
}

func (s *LockSession) GetHeartbeatInterval() time.Duration {
	interval := s.timeout / sessionCheckCount
	if interval > maxHeartbeatInterval {
		interval = maxHeartbeatInterval
	}
	if interval < minHeartbeatInterval {
		interval = minHeartbeatInterval
	}
	return interval
}

// keepalive will try to elect leader if not leader, or extend lease if leader
func (s *LockSession) keepalive(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()

	interval := s.GetHeartbeatInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCh:
			return
		case <-ticker.C:
			// Don't try to elect leader again before elapsed timeout if it resigned recently.
			if time.Since(s.lastResigned) < s.timeout {
				continue
			}
			if s.lock == nil {
				l, err := s.client.TryLock(ctx, s.id, s.key, s.timeout)
				if err != nil {
					if !errors.Is(err, ErrLeaderElected) {
						// TODO: log error
					}
					continue
				}
				s.lock = l
				s.notifyLeaderChange()
			} else {
				if err := s.lock.Refresh(ctx, s.timeout); err != nil {
					if errors.Is(err, ErrNotLockHolder) {
						// TODO: notify leader lost
					} else {
						// TODO: log error
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

	if s.lock == nil {
		return ErrNotLockHolder
	}
	if err := s.lock.Release(ctx); err != nil {
		if errors.Is(err, ErrNotLockHolder) {
			return ErrNotLockHolder
		}
		return err
	}
	s.lock = nil
	s.lastResigned = time.Now()
	return nil
}

func (s *LockSession) Release(ctx context.Context) error {
	if s.state.Load() == sessionStateReleased {
		return nil
	}
	s.state.Store(sessionStateReleased)

	close(s.shutdownCh)
	s.wg.Wait()
	if s.lock != nil {
		return s.Resign(ctx)
	}
	return nil
}
