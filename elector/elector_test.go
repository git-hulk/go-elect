package elector

import (
	"context"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/git-hulk/go-elect/elector/engine/store"
)

type CountRunner struct {
	count atomic.Int32
}

func (r *CountRunner) RunAsLeader(_ context.Context) error {
	r.count.Inc()
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (r *CountRunner) RunAsObserver(_ context.Context) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}

func TestElector(t *testing.T) {
	ctx := context.Background()
	redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	redisStore := store.NewRedisStore(redisClient)
	defer func() {
		require.NoError(t, redisClient.Close())
		require.NoError(t, redisStore.Close(context.Background()))
	}()

	key := "test-elector1-key"
	sessionTimeout := 3 * time.Second
	runner := &CountRunner{}

	// basic elect test
	elector1, err := New(redisStore, key, sessionTimeout, runner)
	defer func() {
		require.NoError(t, elector1.Release(ctx))
		elector1.Wait()
	}()
	require.NoError(t, err)
	require.NoError(t, elector1.Run(context.Background()))
	require.True(t, elector1.IsLeader())

	elector2, err := New(redisStore, key, sessionTimeout, runner)
	defer func() {
		require.NoError(t, elector2.Release(ctx))
		elector2.Wait()
	}()
	require.NoError(t, err)
	require.NoError(t, elector2.Run(context.Background()))
	require.False(t, elector2.IsLeader())

	t.Run("check count", func(t *testing.T) {
		time.Sleep(1 * time.Second)
		require.GreaterOrEqual(t, runner.count.Load(), int32(8))
	})

	t.Run("resign", func(t *testing.T) {
		require.NoError(t, elector1.Resign(context.Background()))
		require.False(t, elector1.IsLeader())
		require.Eventually(t, func() bool {
			return elector2.IsLeader()
		}, sessionTimeout, 100*time.Millisecond)
	})

	t.Run("stop", func(t *testing.T) {
		require.NoError(t, elector2.Release(ctx))

		// need to wait for a longer time since the elector1 may be still in resign yield period
		require.Eventually(t, func() bool {
			return elector1.IsLeader()
		}, sessionTimeout*3, 100*time.Millisecond)
	})
}
