//go:build unix

package app

import (
	"context"
	"log"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/cron"
	"github.com/Nahasma/openscholar-public/internal/task"
)

// initCronSchedulerIfAvailable creates and starts the cron scheduler on unix platforms.
// It registers fired tasks into the task.Registry for unified lifecycle tracking.
func initCronSchedulerIfAvailable(ctx context.Context, registry *task.Registry) CronSchedulerService {
	stateDir := config.StatePath()
	runtimeDir := config.RuntimePath()
	if stateDir == "" || runtimeDir == "" {
		return nil
	}

	onFire := func(ct cron.Task) {
		// Truncate prompt to avoid leaking sensitive content into logs.
		logPrompt := ct.Prompt
		if len(logPrompt) > 60 {
			logPrompt = logPrompt[:60] + "..."
		}
		log.Printf("[cron] fired task %s: %s", ct.ID, logPrompt)

		// Register into task.Registry so TUI and other observers can track it.
		if registry != nil {
			now := time.Now()
			registry.Register(&task.BaseState{
				M: task.Meta{
					ID:        "cron-" + ct.ID + "-" + now.Format("20060102-150405"),
					Label:     ct.Prompt,
					SessionID: ct.SessionID,
					Kind:      task.KindCron,
					Status:    task.StatusRunning,
					StartedAt: now,
				},
			})
			// Note: actual agent execution will be wired in Wave 1 (CR2).
			// For now we mark it completed immediately after registration.
			taskID := "cron-" + ct.ID + "-" + now.Format("20060102-150405")
			endTime := time.Now()
			registry.Update(taskID, func(m *task.Meta) {
				m.Status = task.StatusCompleted
				m.EndedAt = &endTime
				m.Notified = true
			})
		}
	}

	// isLoading — for now always false; Wave 1 will wire to session loading state
	isLoading := func() bool { return false }

	storage := cron.NewStorage(stateDir)
	scheduler := cron.NewScheduler(storage, runtimeDir, onFire, isLoading)
	scheduler.Start(ctx)
	log.Printf("[cron] scheduler started (state=%s runtime=%s)", stateDir, runtimeDir)
	return scheduler
}
