//go:build unix

package cron

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockFileName = "cron.pid"

// PIDLock is a file-based lock using the current process's PID.
// It allows detecting stale locks left by crashed processes.
type PIDLock struct {
	path string
}

// NewPIDLock creates a PIDLock whose file lives in dir.
func NewPIDLock(dir string) *PIDLock {
	return &PIDLock{
		path: filepath.Join(dir, lockFileName),
	}
}

// TryAcquire attempts to acquire the lock for the current process.
// If a lock file exists and the recorded PID is still alive, it returns (false, nil).
// If the recorded process is dead, the stale lock is removed and a new one is written.
// Returns (true, nil) on success.
func (l *PIDLock) TryAcquire() (bool, error) {
	data, err := os.ReadFile(l.path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("cron lock read: %w", err)
	}

	if err == nil {
		// Lock file exists — check if the owning process is alive.
		pidStr := strings.TrimSpace(string(data))
		pid, parseErr := strconv.Atoi(pidStr)
		if parseErr == nil && isProcessAlive(pid) {
			if pid == os.Getpid() {
				// We already own the lock.
				return true, nil
			}
			return false, nil
		}
		// Stale lock — remove it.
		if rmErr := os.Remove(l.path); rmErr != nil && !os.IsNotExist(rmErr) {
			return false, fmt.Errorf("cron lock remove stale: %w", rmErr)
		}
	}

	// Atomically create lock file with O_CREATE|O_EXCL to prevent race.
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return false, fmt.Errorf("cron lock mkdir: %w", err)
	}
	pidData := []byte(strconv.Itoa(os.Getpid()))
	f, createErr := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if createErr != nil {
		// Another process won the race — re-read to confirm.
		return false, nil
	}
	defer f.Close()
	if _, writeErr := f.Write(pidData); writeErr != nil {
		os.Remove(l.path) // best-effort cleanup
		return false, fmt.Errorf("cron lock write: %w", writeErr)
	}
	return true, nil
}

// Release deletes the PID lock file if it is owned by the current process.
func (l *PIDLock) Release() error {
	if !l.IsOwner() {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cron lock release: %w", err)
	}
	return nil
}

// IsOwner reports whether the current process holds the lock.
func (l *PIDLock) IsOwner() bool {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}
	return pid == os.Getpid()
}

// isProcessAlive sends signal 0 to pid to test whether the process exists.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
