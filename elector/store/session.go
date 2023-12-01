package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	maxHeartbeatInterval = 10 * time.Second
	minHeartbeatInterval = 100 * time.Millisecond

	sessionCheckCount = 5
)

type SessionClient interface {
	Create(ctx context.Context, id, key string, timeout time.Duration) (Session, error)
}

type Session interface {
	LeaderChanged() <-chan struct{}
	Release() error
}

type LockSession struct {
	client  LockClient
	id      string
	key     string
	lock    Lock
	timeout time.Duration

	wg              sync.WaitGroup
	leaderChangedCh chan struct{}
	shutdownCh      chan struct{}
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

		leaderChangedCh: make(chan struct{}),
		shutdownCh:      make(chan struct{}),
	}
	go session.keepalive(ctx)
	return session
}

func (s *LockSession) Owner() string {
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
			if s.lock == nil {
				l, err := s.client.TryLock(ctx, s.id, s.key, s.timeout)
				if err != nil {
					if !errors.Is(err, ErrLeaderElected) {
						// TODO: log error
					}
					continue
				}
				s.lock = l
				s.leaderChangedCh <- struct{}{}
			} else {
				if err := s.lock.Refresh(ctx, s.timeout); err != nil {
					if errors.Is(err, ErrNoLockHolder) {
						// TODO: notify leader lost
					} else {
						// TODO: log error
					}
					continue
				}
				s.leaderChangedCh <- struct{}{}
			}
		}
	}
}

func (s *LockSession) LeaderChanged() <-chan struct{} {
	return s.leaderChangedCh
}

func (s *LockSession) Release() error {
	close(s.shutdownCh)
	s.wg.Wait()
	return nil
}
