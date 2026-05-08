//go:build !unix

package app

import (
	"context"

	"github.com/Nahasma/openscholar-public/internal/task"
)

// initCronSchedulerIfAvailable is a no-op on non-unix platforms.
func initCronSchedulerIfAvailable(_ context.Context, _ *task.Registry) CronSchedulerService {
	return nil
}
