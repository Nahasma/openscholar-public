//go:build unix

package cron

import (
	"path/filepath"
	"testing"
)

func TestPIDLockCreatesMissingParentDir(t *testing.T) {
	lockDir := filepath.Join(t.TempDir(), "runtime")
	lock := NewPIDLock(lockDir)

	ok, err := lock.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire error: %v", err)
	}
	if !ok {
		t.Fatal("TryAcquire should acquire lock when parent directory is missing")
	}
	if !lock.IsOwner() {
		t.Fatal("current process should own acquired lock")
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release error: %v", err)
	}
}
