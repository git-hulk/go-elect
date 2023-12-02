package engine

import (
	"context"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const redisAddr = "localhost:6379"

func TestNewRedisStore(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	store := NewRedisStore(redisClient)
	ctx := context.Background()
	key := "test-redis-key"
	timeout := 3 * time.Second

	id1 := uuid.NewString()
	session1, err := store.Create(ctx, key, id1, timeout)
	require.NoError(t, err)
	require.True(t, session1.IsLeader())
	defer func() {
		require.NoError(t, session1.Release(ctx))
	}()

	id2 := uuid.NewString()
	session2, err := store.Create(ctx, key, id2, timeout)
	require.NoError(t, err)
	require.False(t, session2.IsLeader())
	defer func() {
		require.NoError(t, session2.Release(ctx))
	}()

	require.NoError(t, session1.Resign(ctx))
	require.Eventually(t, func() bool {
		return session2.IsLeader()
	}, timeout, 100*time.Millisecond)
}
