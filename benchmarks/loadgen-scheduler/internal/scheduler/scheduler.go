package scheduler

import (
	"context"
	"time"
)

type Runner interface {
	Name() string
	Cleanup(context.Context) error
	Deploy(context.Context, int) (time.Duration, error)
	Scale(context.Context, int) (time.Duration, error)
	Recover(context.Context) (time.Duration, error)
}
