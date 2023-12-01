package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

type SessionStore struct {
	client SessionClient

	sessions sync.Map
}

func newSessionStore(c SessionClient) *SessionStore {
	return &SessionStore{
		client: c,
	}
}

// Acquire is used to elect a leader, it will return ErrLeaderElected if already elected
func (store *SessionStore) Acquire(ctx context.Context, key, sessionID string, sessionTimeout time.Duration) (Session, error) {
	if v, ok := store.sessions.Load(key); ok {
		session := v.(*LockSession)
		if session.Owner() == sessionID {
			return nil, errors.New("you're running as a leader now")
		}
	}

	session, err := store.client.Create(ctx, sessionID, key, sessionTimeout)
	if err != nil {
		return nil, err
	}
	// Become leader
	store.sessions.Store(key, session)
	return session, nil
}

// Resign is used to release the lock, it will return ErrNoLockHolder if not held
func (store *SessionStore) Resign(_ context.Context, key string) error {
	v, ok := store.sessions.LoadAndDelete(key)
	if !ok {
		return ErrNoLockHolder
	}

	return v.(Session).Release()
}

// Stop is used to stop the leader election and release the lock if held
func (store *SessionStore) Stop(ctx context.Context) error {
	store.sessions.Range(func(key, _ any) bool {
		if err := store.Resign(ctx, key.(string)); err != nil {
			// TODO: log error
		}
		return true
	})
	return nil
}
