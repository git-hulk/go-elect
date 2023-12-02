package engine

import (
	"context"
	"time"
)

type Lock interface {
	Owner() string
	Release(ctx context.Context) error
	Refresh(ctx context.Context, lease time.Duration) error
}

type LockClient interface {
	TryLock(ctx context.Context, id, key string, timeout time.Duration) (Lock, error)
}
