package coordinator

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const globalWritePath = ".__coordinator_global_write_lock__"

// WorkerPool coordinates bounded read concurrency and path-scoped writes.
//
// Reads are limited by a semaphore. Writes are locked per normalized path, and
// write-lock acquisition order is deterministic (sorted) to avoid AB-BA deadlock.
type WorkerPool struct {
	readSem chan struct{}

	mu         sync.Mutex
	writeLocks map[string]*pathWriteLock
}

type pathWriteLock struct {
	mu       sync.Mutex
	refCount int
}

// NewWorkerPool creates a WorkerPool.
// maxReadConcurrency <= 0 means unbounded read concurrency.
func NewWorkerPool(maxReadConcurrency int) *WorkerPool {
	pool := &WorkerPool{
		writeLocks: make(map[string]*pathWriteLock),
	}
	if maxReadConcurrency > 0 {
		pool.readSem = make(chan struct{}, maxReadConcurrency)
	}
	return pool
}

// AcquireRead acquires one read slot.
func (p *WorkerPool) AcquireRead() {
	if p.readSem == nil {
		return
	}
	p.readSem <- struct{}{}
}

// ReleaseRead releases one read slot.
func (p *WorkerPool) ReleaseRead() {
	if p.readSem == nil {
		return
	}
	<-p.readSem
}

// AcquireWrite acquires path-scoped write locks in sorted order.
//
// The returned release callbacks should be executed in reverse order.
func (p *WorkerPool) AcquireWrite(paths []string) (releases []func()) {
	normalizedPaths := normalizeWritePaths(paths)
	locks := make([]*pathWriteLock, 0, len(normalizedPaths))

	for _, path := range normalizedPaths {
		locks = append(locks, p.getOrCreateWriteLock(path))
	}

	for _, lock := range locks {
		lock.mu.Lock()
	}

	releases = make([]func(), 0, len(normalizedPaths))
	for i := len(normalizedPaths) - 1; i >= 0; i-- {
		path := normalizedPaths[i]
		lock := locks[i]
		releases = append(releases, func() {
			lock.mu.Unlock()
			p.releaseWriteLockRef(path)
		})
	}
	return releases
}

// ReleaseAll runs release callbacks from AcquireWrite.
func (p *WorkerPool) ReleaseAll(releases []func()) {
	for _, release := range releases {
		release()
	}
}

func (p *WorkerPool) getOrCreateWriteLock(path string) *pathWriteLock {
	p.mu.Lock()
	defer p.mu.Unlock()

	lock, ok := p.writeLocks[path]
	if !ok {
		lock = &pathWriteLock{}
		p.writeLocks[path] = lock
	}
	lock.refCount++
	return lock
}

func (p *WorkerPool) releaseWriteLockRef(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	lock, ok := p.writeLocks[path]
	if !ok {
		return
	}
	lock.refCount--
	if lock.refCount <= 0 {
		delete(p.writeLocks, path)
	}
}

func normalizeWritePaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{globalWritePath}
	}

	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := strings.TrimSpace(path)
		if cleaned == "" {
			continue
		}
		cleaned = filepath.ToSlash(filepath.Clean(cleaned))
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}

	if len(normalized) == 0 {
		return []string{globalWritePath}
	}

	sort.Strings(normalized)
	return normalized
}
