package task

import (
	"context"
	"sync"
	"time"

	"github.com/openscholar/openscholar/internal/pubsub"
)

// gcThreshold is the minimum time after EndedAt before a completed/failed,
// notified task is eligible for garbage collection.
const gcThreshold = 5 * time.Minute

// Registry is a concurrency-safe, in-process task registry backed by a pubsub
// broker so that observers can subscribe to lifecycle events.
type Registry struct {
	mu     sync.RWMutex
	tasks  map[string]State
	broker *pubsub.Broker[RegistryEvent]

	// gcStop is closed to signal the background GC goroutine to exit.
	gcStop chan struct{}
	gcOnce sync.Once
}

// NewRegistry creates an initialised Registry ready for use.
func NewRegistry() *Registry {
	return &Registry{
		tasks:  make(map[string]State),
		broker: pubsub.NewBroker[RegistryEvent](),
		gcStop: make(chan struct{}),
	}
}

// Register stores the state under its task ID and publishes a "registered"
// event. If a task with the same ID already exists it is silently overwritten.
func (r *Registry) Register(state State) {
	meta := state.TaskMeta()

	r.mu.Lock()
	r.tasks[meta.ID] = state
	r.mu.Unlock()

	r.broker.Publish(pubsub.CreatedEvent, RegistryEvent{
		Action: "registered",
		TaskID: meta.ID,
		Kind:   meta.Kind,
		Status: meta.Status,
	})
}

// Update calls fn with the task's Meta under the write lock, then publishes an
// "updated" event. It is a no-op when no task with the given id exists.
func (r *Registry) Update(id string, fn func(*Meta)) {
	r.Mutate(id, func(state State) {
		fn(state.TaskMeta())
	})
}

// Mutate calls fn with the task state under the write lock, then publishes an
// "updated" event. It is a no-op when no task with the given id exists.
func (r *Registry) Mutate(id string, fn func(State)) {
	r.mu.Lock()
	state, ok := r.tasks[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	fn(state)
	meta := *state.TaskMeta() // snapshot for event (avoids holding lock during Publish)
	r.mu.Unlock()

	r.broker.Publish(pubsub.UpdatedEvent, RegistryEvent{
		Action: "updated",
		TaskID: meta.ID,
		Kind:   meta.Kind,
		Status: meta.Status,
	})
}

// Remove deletes the task from the registry and publishes a "removed" event.
// It is a no-op when no task with the given id exists.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	state, ok := r.tasks[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.tasks, id)
	meta := *state.TaskMeta()
	r.mu.Unlock()

	r.broker.Publish(pubsub.DeletedEvent, RegistryEvent{
		Action: "removed",
		TaskID: meta.ID,
		Kind:   meta.Kind,
		Status: meta.Status,
	})
}

// Get returns the State for the given id together with a presence boolean.
func (r *Registry) Get(id string) (State, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.tasks[id]
	if !ok {
		return nil, false
	}
	return CloneState(s), true
}

// BackgroundTasks returns all tasks that are currently backgrounded and running.
func (r *Registry) BackgroundTasks() []State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []State
	for _, s := range r.tasks {
		m := s.TaskMeta()
		if m.IsBackgrounded && m.Status == StatusRunning {
			out = append(out, CloneState(s))
		}
	}
	return out
}

// BackgroundTasksForSession returns running background tasks owned by the
// provided parent/session id.
func (r *Registry) BackgroundTasksForSession(sessionID string) []State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []State
	for _, s := range r.tasks {
		m := s.TaskMeta()
		if !m.IsBackgrounded || m.Status != StatusRunning {
			continue
		}
		if taskSessionID(s) == sessionID {
			out = append(out, CloneState(s))
		}
	}
	return out
}

func taskSessionID(s State) string {
	if sub, ok := s.(*SubtaskState); ok && sub.ParentSessionID != "" {
		return sub.ParentSessionID
	}
	return s.TaskMeta().SessionID
}

// All returns a snapshot of every registered State.
func (r *Registry) All() []State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]State, 0, len(r.tasks))
	for _, s := range r.tasks {
		out = append(out, CloneState(s))
	}
	return out
}

// TryClaimNotification atomically claims a terminal, undelivered task
// notification and returns a detached snapshot for delivery.
func (r *Registry) TryClaimNotification(id string) (State, bool) {
	r.mu.Lock()
	state, ok := r.tasks[id]
	if !ok {
		r.mu.Unlock()
		return nil, false
	}
	meta := state.TaskMeta()
	if meta.Notified || meta.NotifyClaimed || !isTerminalStatus(meta.Status) {
		r.mu.Unlock()
		return nil, false
	}
	meta.NotifyClaimed = true
	snapshot := CloneState(state)
	eventMeta := *meta
	r.mu.Unlock()

	r.broker.Publish(pubsub.UpdatedEvent, RegistryEvent{
		Action: "updated",
		TaskID: eventMeta.ID,
		Kind:   eventMeta.Kind,
		Status: eventMeta.Status,
	})
	return snapshot, true
}

// MarkNotificationDelivered marks a previously claimed task notification as
// delivered. It is safe to call for missing tasks.
func (r *Registry) MarkNotificationDelivered(id string) {
	r.Mutate(id, func(state State) {
		meta := state.TaskMeta()
		meta.Notified = true
		meta.NotifyClaimed = false
	})
}

// ReleaseNotificationClaim releases an in-flight notification claim after a
// delivery failure so a later drain can retry.
func (r *Registry) ReleaseNotificationClaim(id string) {
	r.Mutate(id, func(state State) {
		meta := state.TaskMeta()
		if !meta.Notified {
			meta.NotifyClaimed = false
		}
	})
}

func isTerminalStatus(status Status) bool {
	return status == StatusCompleted || status == StatusFailed || status == StatusCanceled
}

// Subscribe returns a channel that receives RegistryEvent values published by
// the broker. The channel is closed when ctx is cancelled.
func (r *Registry) Subscribe(ctx context.Context) <-chan pubsub.Event[RegistryEvent] {
	return r.broker.Subscribe(ctx)
}

// GC removes tasks that are eligible for garbage collection:
//   - Status is Completed, Failed, or Canceled
//   - Notified is true
//   - EndedAt is set and now.Sub(*EndedAt) > 5 minutes
func (r *Registry) GC(now time.Time) {
	r.mu.Lock()
	var eligible []string
	for id, s := range r.tasks {
		m := s.TaskMeta()
		if (m.Status == StatusCompleted || m.Status == StatusFailed || m.Status == StatusCanceled) &&
			m.Notified &&
			m.EndedAt != nil &&
			now.Sub(*m.EndedAt) > gcThreshold {
			eligible = append(eligible, id)
		}
	}
	for _, id := range eligible {
		delete(r.tasks, id)
	}
	r.mu.Unlock()

	// Publish removal events outside the lock.
	for _, id := range eligible {
		r.broker.Publish(pubsub.DeletedEvent, RegistryEvent{
			Action: "removed",
			TaskID: id,
		})
	}
}

// StartGC launches a background goroutine that calls GC every interval.
// The goroutine exits when ctx is cancelled or Shutdown is called.
// Calling StartGC more than once is safe — only the first call has effect.
func (r *Registry) StartGC(ctx context.Context, interval time.Duration) {
	r.gcOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					r.GC(time.Now())
				case <-ctx.Done():
					return
				case <-r.gcStop:
					return
				}
			}
		}()
	})
}

// Shutdown closes the underlying pubsub broker and stops any background GC
// goroutine started via StartGC.
func (r *Registry) Shutdown() {
	// Signal the GC goroutine to stop (idempotent via select).
	select {
	case <-r.gcStop:
		// already closed
	default:
		close(r.gcStop)
	}
	r.broker.Shutdown()
}
