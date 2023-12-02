package engine

import "errors"

var (
	ErrLeaderElected = errors.New("leader already elected")
	ErrNotLockHolder = errors.New("you're not lock holder")
)
