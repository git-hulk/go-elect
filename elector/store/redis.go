package store

import (
	"context"
	"errors"
	"time"

	"github.com/bsm/redislock"
)

func NewRedisStore(c redislock.RedisClient) *SessionStore {
	sessionClient := &redisSessionClient{
		client: redislock.New(c),
	}
	return newSessionStore(sessionClient)
}

type redisLock struct {
	internal *redislock.Lock
}

func newRedisLock(l *redislock.Lock) Lock {
	return &redisLock{
		internal: l,
	}
}

func (l *redisLock) Owner() string {
	return l.internal.Token()
}

func (l *redisLock) Release(ctx context.Context) error {
	return l.internal.Release(ctx)
}

func (l *redisLock) Refresh(ctx context.Context, lease time.Duration) error {
	return l.internal.Refresh(ctx, lease, nil)
}

type redisSessionClient struct {
	client *redislock.Client
}

func (c *redisSessionClient) TryLock(ctx context.Context, id, key string, timeout time.Duration) (Lock, error) {
	lock, err := c.client.Obtain(ctx, key, timeout, &redislock.Options{
		// No retry strategy to achieve a non-blocking lock
		Token:         id,
		RetryStrategy: redislock.NoRetry(),
	})
	if err != nil {
		if errors.Is(err, redislock.ErrNotObtained) {
			return nil, ErrLeaderElected
		}
		return nil, err
	}
	return newRedisLock(lock), nil
}

func (c *redisSessionClient) Create(ctx context.Context, id, key string, timeout time.Duration) (Session, error) {
	lock, err := c.TryLock(ctx, id, key, timeout)
	if err != nil {
		return nil, err
	}
	return NewLockSession(ctx, c, id, key, timeout), lock.Release(ctx)
}
