package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

const tasksFileName = "scheduled_tasks.json"

// Storage handles persistent storage of Tasks as a JSON file.
type Storage struct {
	path string
}

// NewStorage creates a Storage backed by a JSON file in dir.
func NewStorage(dir string) *Storage {
	return &Storage{
		path: filepath.Join(dir, tasksFileName),
	}
}

// Load reads all tasks from the JSON file.
// If the file does not exist, an empty slice is returned (fail-open).
func (s *Storage) Load() ([]Task, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, fmt.Errorf("cron storage load: %w", err)
	}
	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("cron storage parse: %w", err)
	}
	return tasks, nil
}

// Save atomically writes tasks to the JSON file.
// It writes to a temporary file first, then renames it to avoid partial writes.
func (s *Storage) Save(tasks []Task) error {
	data, err := json.Marshal(tasks)
	if err != nil {
		return fmt.Errorf("cron storage marshal: %w", err)
	}

	if err := fileop.WriteFileAtomic(s.path, data, 0o644); err != nil {
		return fmt.Errorf("cron storage write: %w", err)
	}
	return nil
}

// AddTask appends a single task and persists the updated list.
func (s *Storage) AddTask(task Task) error {
	tasks, err := s.Load()
	if err != nil {
		return err
	}
	tasks = append(tasks, task)
	return s.Save(tasks)
}

// RemoveTask removes the task with the given ID and persists the updated list.
// It returns an error if no task with that ID exists.
func (s *Storage) RemoveTask(id string) error {
	tasks, err := s.Load()
	if err != nil {
		return err
	}
	filtered := tasks[:0]
	found := false
	for _, t := range tasks {
		if t.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return fmt.Errorf("cron storage: task %q not found", id)
	}
	return s.Save(filtered)
}

// UpdateTask applies fn to the task with the given ID and persists the result.
func (s *Storage) UpdateTask(id string, fn func(*Task)) error {
	tasks, err := s.Load()
	if err != nil {
		return err
	}
	found := false
	for i := range tasks {
		if tasks[i].ID == id {
			fn(&tasks[i])
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("cron storage: task %q not found", id)
	}
	return s.Save(tasks)
}
