package engine

import (
	"context"
	"go-elect/internal"
	"sync"
	"time"
)

type SessionStore struct {
	client SessionClient

	sessions sync.Map
}

func NewSessionStore(c SessionClient) *SessionStore {
	return &SessionStore{
		client: c,
	}
}

func (store *SessionStore) Create(ctx context.Context, key, id string, timeout time.Duration) (Session, error) {
	if v, ok := store.sessions.Load(key); ok {
		session := v.(*LockSession)
		if session.ID() == id {
			return session, nil
		}
	}

	session, err := store.client.Create(ctx, id, key, timeout)
	if err != nil {
		return nil, err
	}
	// Become leader
	store.sessions.Store(key, session)
	return session, nil
}

// Delete will release the lock if held and session will be kept
func (store *SessionStore) Delete(ctx context.Context, key string) error {
	v, ok := store.sessions.LoadAndDelete(key)
	if !ok {
		return ErrNotLockHolder
	}

	return v.(Session).Release(ctx)
}

// Close all sessions and release the lock if held by any session
func (store *SessionStore) Close(ctx context.Context) error {
	store.sessions.Range(func(key, _ any) bool {
		if err := store.Delete(ctx, key.(string)); err != nil {
			internal.GetLogger().Printf("Failed to delete session[%s], err: %v", key.(string), err)
		}
		return true
	})
	return nil
}
