package engine

import (
	"context"
	"time"
)

type Mutex interface {
	ID() string
	Key() string
	Timeout() time.Duration
	IsLocked() bool
	TryLock(ctx context.Context) error
	Release(ctx context.Context) error
	Refresh(ctx context.Context, lease time.Duration) error
}
