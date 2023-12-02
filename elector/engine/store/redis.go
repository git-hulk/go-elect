package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/bsm/redislock"

	"go-elect/elector/engine"
)

// RedisMutex implements engine.Mutex, it uses redislock to implement distributed mutex.
type RedisMutex struct {
	client *redislock.Client

	// rwmu is used to protect lock field
	rwmu sync.RWMutex
	lock *redislock.Lock

	id      string
	key     string
	timeout time.Duration
}

// NewRedisMutex creates a new RedisMutex instance.
func NewRedisMutex(client *redislock.Client, id, key string, timeout time.Duration) *RedisMutex {
	return &RedisMutex{
		client:  client,
		id:      id,
		key:     key,
		timeout: timeout,
	}
}

// ID returns the id of the Redis mutex.
func (mu *RedisMutex) ID() string {
	return mu.id
}

// Key returns the key of the Redis mutex.
func (mu *RedisMutex) Key() string {
	return mu.key
}

// Timeout returns the timeout of the Redis mutex.
// It's used to set the timeout of the Redis lock.
func (mu *RedisMutex) Timeout() time.Duration {
	return mu.timeout
}

// IsLocked returns true if the Redis mutex is locked.
func (mu *RedisMutex) IsLocked() bool {
	mu.rwmu.RLock()
	defer mu.rwmu.RUnlock()
	return mu.lock != nil
}

// TryLock tries to obtain the Redis lock, it will return ErrLeaderElected
// if the Redis lock is already held.
func (mu *RedisMutex) TryLock(ctx context.Context) error {
	lock, err := mu.client.Obtain(ctx, mu.key, mu.timeout, &redislock.Options{
		Token: mu.id,
		// No retry strategy to achieve a non-blocking mu
		RetryStrategy: redislock.NoRetry(),
	})
	if err != nil {
		if errors.Is(err, redislock.ErrNotObtained) {
			return engine.ErrLeaderElected
		}
		return err
	}

	mu.rwmu.Lock()
	defer mu.rwmu.Unlock()
	mu.lock = lock
	return nil
}

// Release releases the Redis lock. It will return ErrNotLockHolder if the Redis lock
// is not held.
func (mu *RedisMutex) Release(ctx context.Context) error {
	mu.rwmu.Lock()

	if mu.lock == nil {
		mu.rwmu.Unlock()
		return engine.ErrNotLockHolder
	}
	lock := mu.lock
	mu.lock = nil
	mu.rwmu.Unlock()

	if err := lock.Release(ctx); err != nil {
		if errors.Is(err, redislock.ErrLockNotHeld) {
			return engine.ErrNotLockHolder
		}
		return err
	}
	return nil
}

// Refresh refreshes the Redis lock. It will return ErrNotLockHolder if the Redis lock
// is not held.
func (mu *RedisMutex) Refresh(ctx context.Context, lease time.Duration) error {
	mu.rwmu.RLock()
	lock := mu.lock
	mu.rwmu.RUnlock()
	if lock == nil {
		return engine.ErrNotLockHolder
	}
	if err := lock.Refresh(ctx, lease, nil); err != nil {
		if errors.Is(err, redislock.ErrLockNotHeld) {
			return engine.ErrNotLockHolder
		}
	}
	return nil
}

// RedisEngine implements engine.SessionClient, it's used to create a new Redis session.
type RedisEngine struct {
	client *redislock.Client
}

// Create creates a new Redis session.
func (r *RedisEngine) Create(ctx context.Context, id, key string, timeout time.Duration) (engine.Session, error) {
	return engine.NewLockSession(ctx, NewRedisMutex(r.client, id, key, timeout))
}

// NewRedisStore creates a new Redis session store.
func NewRedisStore(c redislock.RedisClient) *engine.SessionStore {
	redisEngine := &RedisEngine{
		client: redislock.New(c),
	}
	return engine.NewSessionStore(redisEngine)
}
