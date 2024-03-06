package engine

import (
	"context"
	"sync"
	"time"

	"github.com/git-hulk/go-elect/internal"
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
		session, _ := v.(Session)
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
	session, _ := v.(Session)

	return session.Release(ctx)
}

// Close all sessions and release the lock if held by any session
func (store *SessionStore) Close(ctx context.Context) error {
	store.sessions.Range(func(k, _ any) bool {
		key, _ := k.(string)
		if err := store.Delete(ctx, key); err != nil {
			internal.GetLogger().Printf("Failed to delete session[%s], err: %v", key, err)
		}
		return true
	})
	return nil
}
