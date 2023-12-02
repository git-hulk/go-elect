package engine

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

func wrapRedisLock(l *redislock.Lock) Lock {
	return &redisLock{
		internal: l,
	}
}

func (l *redisLock) Owner() string {
	return l.internal.Token()
}

func (l *redisLock) Release(ctx context.Context) error {
	if err := l.internal.Release(ctx); err != nil {
		if errors.Is(err, redislock.ErrLockNotHeld) {
			return ErrNotLockHolder
		}
	}
	return nil
}

func (l *redisLock) Refresh(ctx context.Context, lease time.Duration) error {
	if err := l.internal.Refresh(ctx, lease, nil); err != nil {
		if errors.Is(err, redislock.ErrLockNotHeld) {
			return ErrNotLockHolder
		}
	}
	return nil
}

type redisSessionClient struct {
	client *redislock.Client
}

func (c *redisSessionClient) TryLock(ctx context.Context, id, key string, timeout time.Duration) (Lock, error) {
	lock, err := c.client.Obtain(ctx, key, timeout, &redislock.Options{
		Token: id,
		// No retry strategy to achieve a non-blocking lock
		RetryStrategy: redislock.NoRetry(),
	})
	if err != nil {
		if errors.Is(err, redislock.ErrNotObtained) {
			return nil, ErrLeaderElected
		}
		return nil, err
	}
	return wrapRedisLock(lock), nil
}

func (c *redisSessionClient) Create(ctx context.Context, id, key string, timeout time.Duration) (Session, error) {
	return NewLockSession(ctx, c, id, key, timeout), nil
}
