package engine

import (
	"context"
	"time"
)

type LeaderChangeFn func(isLeader bool)

type SessionClient interface {
	Create(ctx context.Context, id, key string, timeout time.Duration) (Session, error)
}

type Session interface {
	IsLeader() bool
	Resign(ctx context.Context) error
	Release(ctx context.Context) error
	SetLeaderChangeFn(fn LeaderChangeFn)
}
