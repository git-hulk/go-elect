package store

import "errors"

var (
	ErrLeaderElected = errors.New("leader already elected")
	ErrNoLockHolder  = errors.New("no lock holder")
)
