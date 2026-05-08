package pubsub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/pubsub"
)

func TestBroker_PublishSubscribe(t *testing.T) {
	b := pubsub.NewBroker[string]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)
	b.Publish(pubsub.CreatedEvent, "hello")

	select {
	case event := <-ch:
		assert.Equal(t, pubsub.CreatedEvent, event.Type)
		assert.Equal(t, "hello", event.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := pubsub.NewBroker[int]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := b.Subscribe(ctx)
	ch2 := b.Subscribe(ctx)
	ch3 := b.Subscribe(ctx)

	b.Publish(pubsub.UpdatedEvent, 42)

	for i, ch := range []<-chan pubsub.Event[int]{ch1, ch2, ch3} {
		select {
		case event := <-ch:
			assert.Equal(t, 42, event.Payload, "subscriber %d", i)
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestBroker_ContextCancellation(t *testing.T) {
	b := pubsub.NewBroker[string]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	ch := b.Subscribe(ctx)

	cancel()

	// Channel should eventually close
	select {
	case _, ok := <-ch:
		if ok {
			// Might get one event, but channel should close soon
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close after context cancellation")
	}
}

func TestBroker_Shutdown(t *testing.T) {
	b := pubsub.NewBroker[string]()
	ctx := context.Background()
	ch := b.Subscribe(ctx)

	b.Shutdown()

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after shutdown")
}

func TestBroker_PublishAfterShutdown(t *testing.T) {
	b := pubsub.NewBroker[string]()
	b.Shutdown()

	// Should not panic
	assert.NotPanics(t, func() {
		b.Publish(pubsub.CreatedEvent, "after shutdown")
	})
}

func TestBroker_SubscribeAfterShutdown(t *testing.T) {
	b := pubsub.NewBroker[string]()
	b.Shutdown()

	ch := b.Subscribe(context.Background())
	// Should return closed channel
	_, ok := <-ch
	assert.False(t, ok, "subscribe after shutdown should return closed channel")
}

func TestBroker_ConcurrentPublish(t *testing.T) {
	b := pubsub.NewBroker[int]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			b.Publish(pubsub.CreatedEvent, val)
		}(i)
	}
	wg.Wait()

	// Drain all received events
	received := 0
	for {
		select {
		case <-ch:
			received++
		default:
			goto done
		}
	}
done:
	// Should receive some events (buffer is 64, sending 50)
	require.Greater(t, received, 0)
}

func TestBroker_DoubleShutdown(t *testing.T) {
	b := pubsub.NewBroker[string]()
	b.Shutdown()
	// Should not panic
	assert.NotPanics(t, func() {
		b.Shutdown()
	})
}
