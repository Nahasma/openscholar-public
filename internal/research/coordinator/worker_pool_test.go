package coordinator

import (
	"sync"
	"testing"
	"time"
)

func TestWorkerPool_ReadConcurrencyLimit(t *testing.T) {
	pool := NewWorkerPool(2)

	acquired := make(chan struct{}, 3)
	release := make(chan struct{})

	startReader := func() {
		go func() {
			pool.AcquireRead()
			acquired <- struct{}{}
			<-release
			pool.ReleaseRead()
		}()
	}

	startReader()
	startReader()

	for i := 0; i < 2; i++ {
		select {
		case <-acquired:
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("reader %d did not acquire in time", i+1)
		}
	}

	startReader()

	select {
	case <-acquired:
		t.Fatal("third reader acquired before a slot was released")
	case <-time.After(120 * time.Millisecond):
		// blocked as expected
	}

	release <- struct{}{}

	select {
	case <-acquired:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("third reader did not acquire after release")
	}

	release <- struct{}{}
	release <- struct{}{}
}

func TestWorkerPool_WriteLockSerializesSamePath(t *testing.T) {
	pool := NewWorkerPool(4)

	first := pool.AcquireWrite([]string{"paper/intro.tex"})
	secondAcquired := make(chan struct{})
	released := make(chan struct{})

	go func() {
		defer close(released)
		second := pool.AcquireWrite([]string{"paper/intro.tex"})
		close(secondAcquired)
		pool.ReleaseAll(second)
	}()

	select {
	case <-secondAcquired:
		t.Fatal("second writer acquired lock before first writer released it")
	case <-time.After(120 * time.Millisecond):
		// blocked as expected
	}

	pool.ReleaseAll(first)

	select {
	case <-secondAcquired:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("second writer did not acquire lock after first release")
	}

	<-released
}

func TestWorkerPool_WriteLockSortedAvoidsDeadlock(t *testing.T) {
	pool := NewWorkerPool(2)

	var wg sync.WaitGroup
	wg.Add(2)

	start := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer wg.Done()
		<-start
		releases := pool.AcquireWrite([]string{"paper/b.tex", "paper/a.tex"})
		time.Sleep(50 * time.Millisecond)
		pool.ReleaseAll(releases)
	}()

	go func() {
		defer wg.Done()
		<-start
		releases := pool.AcquireWrite([]string{"paper/a.tex", "paper/b.tex"})
		time.Sleep(50 * time.Millisecond)
		pool.ReleaseAll(releases)
	}()

	close(start)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writers likely deadlocked while acquiring sorted path locks")
	}
}

func TestNormalizeWritePaths(t *testing.T) {
	paths := normalizeWritePaths([]string{" ./paper/a.tex ", "paper/../paper/a.tex", "paper/b.tex", ""})
	if len(paths) != 2 {
		t.Fatalf("expected 2 unique normalized paths, got %d (%v)", len(paths), paths)
	}
	if paths[0] != "paper/a.tex" || paths[1] != "paper/b.tex" {
		t.Fatalf("unexpected normalized paths: %v", paths)
	}

	global := normalizeWritePaths(nil)
	if len(global) != 1 || global[0] != globalWritePath {
		t.Fatalf("expected global write fallback path, got %v", global)
	}
}
