package store

import (
	"context"
	"sync"
	"time"

	"github.com/git-hulk/go-elect/elector/engine"
	"github.com/git-hulk/go-elect/internal"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type EtcdSession struct {
	client         *clientv3.Client
	sessionTimeout time.Duration

	myID     string
	leaderMu sync.RWMutex
	leaderID string

	electPath  string
	electionCh chan *concurrency.Election
	election   *concurrency.Election
	resignCh   chan struct{}

	leaderChangedFn engine.LeaderChangeFn

	wg         sync.WaitGroup
	shutdownCh chan struct{}
}

func New(client *clientv3.Client, id, key string, timeout time.Duration) (engine.Session, error) {
	e := &EtcdSession{
		myID:           id,
		electPath:      key,
		client:         client,
		sessionTimeout: timeout,
		resignCh:       make(chan struct{}),
		shutdownCh:     make(chan struct{}),
		electionCh:     make(chan *concurrency.Election),
	}
	e.wg.Add(2)
	go e.electLoop(context.Background())
	go e.observeLoop(context.Background())

	return e, nil
}

func (e *EtcdSession) Key() string {
	return e.electPath
}

func (e *EtcdSession) ID() string {
	return e.myID
}

func (e *EtcdSession) Timeout() time.Duration {
	return e.sessionTimeout
}

func (e *EtcdSession) IsLeader() bool {
	e.leaderMu.RLock()
	defer e.leaderMu.RUnlock()
	return e.leaderID == e.myID
}

func (e *EtcdSession) Resign(ctx context.Context) error {
	if e.election != nil {
		if err := e.election.Resign(ctx); err != nil {
			return err
		}
		e.resignCh <- struct{}{}
	}
	return nil
}

func (e *EtcdSession) Release(ctx context.Context) (err error) {
	if e.election != nil {
		err = e.election.Resign(ctx)
	}
	close(e.shutdownCh)
	e.wg.Wait()
	return err
}

func (e *EtcdSession) SetLeaderChangeFn(fn engine.LeaderChangeFn) {
	e.leaderChangedFn = fn
}

func (e *EtcdSession) electLoop(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-e.shutdownCh:
			return
		default:
		}

		session, err := concurrency.NewSession(e.client, concurrency.WithTTL(int(e.sessionTimeout.Seconds())))
		if err != nil {
			// TODO: log error
			time.Sleep(e.sessionTimeout / 3)
			continue
		}

		election := concurrency.NewElection(session, e.electPath)
		e.election = election
		e.electionCh <- election
		if err := election.Campaign(ctx, e.myID); err != nil {
			internal.GetLogger().Printf("Failed to campaign, err: %v", err)
			continue
		}

		select {
		case <-e.resignCh:
			// resign the leader, will restart to create a new session and try to elect again
		case <-session.Done():
			// session is expired, will restart to create a new session and try to elect again
		case <-e.shutdownCh:
			if e.IsLeader() {
				_ = election.Resign(ctx)
			}
			_ = session.Close()
			return
		}
		_ = session.Close()
	}
}

func (e *EtcdSession) observeLoop(ctx context.Context) {
	defer e.wg.Done()
	var election *concurrency.Election
	select {
	case elect := <-e.electionCh:
		election = elect
	case <-e.shutdownCh:
		return
	}

	ch := election.Observe(ctx)
	for {
		select {
		case rsp := <-ch:
			if len(rsp.Kvs) > 0 {
				newLeaderID := string(rsp.Kvs[0].Value)
				e.leaderMu.Lock()
				e.leaderID = newLeaderID
				e.leaderMu.Unlock()
				if newLeaderID != "" && newLeaderID == e.leaderID {
					continue
				}
				e.leaderChangedFn(e.IsLeader())
			} else {
				ch = election.Observe(ctx)
			}
		case elect := <-e.electionCh:
			election = elect
			ch = election.Observe(ctx)
		case <-e.shutdownCh:
			return
		}
	}
}

type EtcdEngine struct {
	client *clientv3.Client
}

func NewEtcdEngine(client *clientv3.Client) *EtcdEngine {
	return &EtcdEngine{client: client}
}

func (e *EtcdEngine) Create(ctx context.Context, id, key string, timeout time.Duration) (engine.Session, error) {
	return New(e.client, id, key, timeout)
}

func NewEtcdStore(client *clientv3.Client) *engine.SessionStore {
	return engine.NewSessionStore(NewEtcdEngine(client))
}
