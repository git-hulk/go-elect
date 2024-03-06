## go-elect

go-elect is a library for making the election process easier. It provides a simple interface for managing your distributed exclusive job execution,
and it supports multiple backends for storing the election state:

- [x] [Redis](https://redis.io/)
- [ ] [Etcd](https://etcd.io/)
- [ ] [Zookeeper](https://zookeeper.apache.org/)
- [ ] [MySQL](https://www.mysql.com/)
- [ ] [PostgreSQL](https://www.postgresql.org/)

And more to come...

## How to use

### Redis

```go
package main

import (
    "github.com/redis/go-redis/v9"
    "github.com/git-hulk/go-elect/elector/engine/store"
    "github.com/git-hulk/go-elect/elector"
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

func main() {
    ctx := context.Background()
    redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    redisStore := store.NewRedisStore(redisClient)
	
    elector, err := elector.New(redisStore, "test-elector1-key", 3 * time.Second,  &CountRunner{})
    if err := elector.Run(ctx);  err != nil {
        // handle error
    }
    elector.Wait()
    // use elector.Release() to release the election
}
```

## License
