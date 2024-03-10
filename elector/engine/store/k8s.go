package store

import (
	"context"
	"github.com/git-hulk/go-elect/internal"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/clock"
	"sync"
	"time"

	"github.com/git-hulk/go-elect/elector/engine"

	clientset "k8s.io/client-go/kubernetes"
)

type K8sMutex struct {
	client     *clientset.Clientset
	rwmu       sync.RWMutex
	lock       *resourcelock.LeaseLock
	clock      clock.Clock
	record     *resourcelock.LeaderElectionRecord
	recordTime time.Time
	id         string
	key        string
	namespace  string
	timeout    time.Duration
}

func NewK8sMutex(client *clientset.Clientset, id, key, namespace string, timeout time.Duration) *K8sMutex {
	mu := &K8sMutex{
		client:    client,
		clock:     clock.RealClock{},
		id:        id,
		key:       key,
		namespace: namespace,
		timeout:   timeout,
	}
	mu.lock = mu.getNewLock()
	return mu
}

func (mu *K8sMutex) ID() string {
	return mu.id
}

func (mu *K8sMutex) Key() string {
	return mu.key
}

func (mu *K8sMutex) Timeout() time.Duration {
	return mu.timeout
}

func (mu *K8sMutex) IsLocked() bool {
	mu.rwmu.RLock()
	defer mu.rwmu.RUnlock()
	return mu.record.HolderIdentity == mu.id
}

func (mu *K8sMutex) getNewLock() *resourcelock.LeaseLock {
	return &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      mu.key,
			Namespace: mu.namespace,
		},
		Client: mu.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: mu.id,
		},
	}
}

func (mu *K8sMutex) TryLock(ctx context.Context) error {
	now := metav1.NewTime(mu.clock.Now())
	leaderElectionRecord := resourcelock.LeaderElectionRecord{
		HolderIdentity:       mu.lock.Identity(),
		LeaseDurationSeconds: int(mu.timeout.Seconds()),
		RenewTime:            now,
		AcquireTime:          now,
	}

	// 1. obtain or create the ElectionRecord
	oldLeaderElectionRecord, _, err := mu.lock.Get(ctx)
	if err != nil {
		if !errors.IsNotFound(err) {
			internal.GetLogger().Printf("Failed to try lock, err: %v", err)
			return engine.ErrNotLockHolder
		}
		if err = mu.lock.Create(ctx, leaderElectionRecord); err != nil {
			internal.GetLogger().Printf("error initially creating leader election record, err: %v", err)
			return engine.ErrNotLockHolder
		}
		mu.setRecord(&leaderElectionRecord)
		return nil
	}

	// 2. Record obtained, check the Identity & Time
	if mu.record == nil || mu.record.HolderIdentity != oldLeaderElectionRecord.HolderIdentity {
		mu.setRecord(oldLeaderElectionRecord)
		internal.GetLogger().Printf("update record time %v", mu.recordTime)
	}
	if len(oldLeaderElectionRecord.HolderIdentity) > 0 &&
		//mu.recordTime.Add(time.Second*time.Duration(oldLeaderElectionRecord.LeaseDurationSeconds)).After(now.Time) &&
		oldLeaderElectionRecord.RenewTime.Add(time.Second*time.Duration(oldLeaderElectionRecord.LeaseDurationSeconds)).After(now.Time) &&
		!mu.IsLocked() {
		internal.GetLogger().Printf("lock is held by %v and has not yet expired recordTime:%v now:%v leaseSeconds:%v",
			oldLeaderElectionRecord.HolderIdentity, mu.recordTime, now.Time, oldLeaderElectionRecord.LeaseDurationSeconds)
		return engine.ErrLeaderElected
	}

	// 3. We're going to try to update. The leaderElectionRecord is set to it's default
	// here. Let's correct it before updating.
	if mu.IsLocked() {
		leaderElectionRecord.AcquireTime = oldLeaderElectionRecord.AcquireTime
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions
	} else {
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions + 1
	}

	// update the lock itself
	if err = mu.lock.Update(ctx, leaderElectionRecord); err != nil {
		internal.GetLogger().Printf("Failed to update lock: %v", err)
		return engine.ErrLeaderElected
	}
	mu.setRecord(&leaderElectionRecord)
	return nil
}

func (mu *K8sMutex) Release(ctx context.Context) error {
	if !mu.IsLocked() {
		return nil
	}
	now := metav1.NewTime(mu.clock.Now())
	leaderElectionRecord := resourcelock.LeaderElectionRecord{
		LeaderTransitions:    mu.record.LeaderTransitions,
		LeaseDurationSeconds: 1,
		RenewTime:            now,
		AcquireTime:          now,
	}
	if err := mu.lock.Update(context.TODO(), leaderElectionRecord); err != nil {
		internal.GetLogger().Printf("Failed to update lock: %v", err)
		return err
	}
	mu.setRecord(&leaderElectionRecord)
	return nil
}

func (mu *K8sMutex) Refresh(ctx context.Context, lease time.Duration) error {
	if !mu.IsLocked() {
		return engine.ErrNotLockHolder
	}
	now := metav1.NewTime(mu.clock.Now())
	leaderElectionRecord := resourcelock.LeaderElectionRecord{
		LeaderTransitions:    mu.record.LeaderTransitions,
		LeaseDurationSeconds: mu.record.LeaseDurationSeconds,
		RenewTime:            now,
		AcquireTime:          mu.record.AcquireTime,
	}
	if err := mu.lock.Update(context.TODO(), leaderElectionRecord); err != nil {
		internal.GetLogger().Printf("Failed to update lock: %v", err)
		return err
	}
	mu.setRecord(&leaderElectionRecord)
	return nil
}

func (mu *K8sMutex) setRecord(record *resourcelock.LeaderElectionRecord) {
	mu.rwmu.Lock()
	defer mu.rwmu.Unlock()
	mu.record = record
	mu.recordTime = mu.clock.Now()
}

type K8sEngine struct {
	client    *clientset.Clientset
	namespace string
}

func (c *K8sEngine) Create(ctx context.Context, id, key string, timeout time.Duration) (engine.Session, error) {
	return engine.NewLockSession(ctx, NewK8sMutex(c.client, id, key, c.namespace, timeout))
}

func NewK8sStore(c *clientset.Clientset, namespace string) *engine.SessionStore {
	k8sEngine := &K8sEngine{
		client:    c,
		namespace: namespace,
	}
	return engine.NewSessionStore(k8sEngine)
}
