package elector

import (
	"context"
	"testing"
	"time"

	"github.com/git-hulk/go-elect/elector/engine"
	"github.com/git-hulk/go-elect/elector/engine/store"
	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/atomic"

	"github.com/stretchr/testify/require"
)

type CountRunner struct {
	count atomic.Int32
}

func (r *CountRunner) Run(_ context.Context) error {
	r.count.Inc()
	time.Sleep(100 * time.Millisecond)
	return nil
}

func TestRedisElector(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{})
	defer func() {
		require.NoError(t, redisClient.Close())
	}()

	testElector(t, store.NewRedisStore(redisClient))
}

func TestEtcdElector(t *testing.T) {
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"localhost:2379"},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, etcdClient.Close())
	}()
	testElector(t, store.NewEtcdStore(etcdClient))
}

func testElector(t *testing.T, store *engine.SessionStore) {
	ctx := context.Background()

	key := "test-elector1-key"
	sessionTimeout := 3 * time.Second
	runner := &CountRunner{}

	// basic elect test
	elector1, err := New(store, key, sessionTimeout, runner)
	defer func() {
		require.NoError(t, elector1.Release(ctx))
		elector1.Wait()
	}()
	require.NoError(t, err)
	require.NoError(t, elector1.Run(context.Background()))
	require.Eventually(t, func() bool {
		return elector1.IsLeader()
	}, sessionTimeout, 100*time.Millisecond)

	elector2, err := New(store, key, sessionTimeout, runner)
	defer func() {
		require.NoError(t, elector2.Release(ctx))
		elector2.Wait()
	}()
	require.NoError(t, err)

	require.NoError(t, elector2.Run(ctx))
	require.False(t, elector2.IsLeader())

	t.Run("check count", func(t *testing.T) {
		time.Sleep(1 * time.Second)
		require.GreaterOrEqual(t, runner.count.Load(), int32(8))
	})

	t.Run("resign", func(t *testing.T) {
		require.NoError(t, elector1.Resign(context.Background()))
		require.Eventually(t, func() bool {
			return elector2.IsLeader() && !elector1.IsLeader()
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
