package store

import (
	"context"
	"testing"
	"time"

	"github.com/git-hulk/go-elect/elector/engine"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestRedisSessionStore(t *testing.T) {
	store := NewRedisStore(redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	}))
	runBasicStoreTest(t, store)
}

func TestEtcdSessionStore(t *testing.T) {
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"localhost:2379"},
	})
	require.NoError(t, err)
	store := NewEtcdStore(etcdClient)
	runBasicStoreTest(t, store)
}

func runBasicStoreTest(t *testing.T, store *engine.SessionStore) {
	ctx := context.Background()
	key := "test-redis-key"
	timeout := 3 * time.Second

	id1 := uuid.NewString()
	session1, err := store.Create(ctx, key, id1, timeout)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return session1.IsLeader()
	}, timeout, 100*time.Millisecond)
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

	// session1 resigns, session2 should become leader
	require.NoError(t, session1.Resign(ctx))
	require.Eventually(t, func() bool {
		return session2.IsLeader()
	}, timeout, 100*time.Millisecond)

	// session2 resigns, session1 should become leader
	require.NoError(t, session2.Resign(ctx))
	require.Eventually(t, func() bool {
		return session1.IsLeader()
	}, timeout, 100*time.Millisecond)
}
