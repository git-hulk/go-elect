package store

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

const K8sConfigPath = "../../../k8s.yaml"

func TestNewK8sStore(t *testing.T) {
	k8sConfig, err := clientcmd.BuildConfigFromFlags("", K8sConfigPath)
	require.NoError(t, err)
	k8sClient := clientset.NewForConfigOrDie(k8sConfig)
	store := NewK8sStore(k8sClient, "default")
	ctx := context.Background()
	key := "test-k8s-key"
	timeout := 3 * time.Second

	id1 := uuid.NewString()
	session1, err := store.Create(ctx, key, id1, timeout)
	require.NoError(t, err)
	//if lock a exists lease resource, need to wait timeout for elected
	//if !session1.IsLeader() {
	//	time.Sleep(timeout + time.Second*5)
	//}
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
