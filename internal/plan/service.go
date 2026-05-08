package plan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// PlanFile describes the session-bound plan file metadata.
type PlanFile struct {
	SessionID string
	Slug      string
	Path      string
	Exists    bool
}

// Service manages session-bound plan files.
type Service interface {
	Ensure(ctx context.Context, sessionID string) (PlanFile, error)
	Read(ctx context.Context, sessionID string) (PlanFile, string, error)
	Write(ctx context.Context, sessionID string, content string) (PlanFile, error)
	IsPlanFile(ctx context.Context, sessionID string, path string) bool
}

// NewService creates a local filesystem-backed plan service rooted at dir.
func NewService(dir string) Service {
	return &localService{dir: dir}
}

type localService struct {
	dir string
}

func (s *localService) Ensure(_ context.Context, sessionID string) (PlanFile, error) {
	if strings.TrimSpace(sessionID) == "" {
		return PlanFile{}, errors.New("session id is required")
	}

	baseDir, err := filepath.Abs(filepath.Clean(s.dir))
	if err != nil {
		return PlanFile{}, err
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return PlanFile{}, err
	}

	slug := slugForSession(sessionID)
	path := filepath.Join(baseDir, slug+".md")
	info, err := os.Lstat(path)
	exists := err == nil && !info.IsDir()
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return PlanFile{}, errors.New("plan file path is a symlink")
	}
	if err == nil && info.IsDir() {
		return PlanFile{}, errors.New("plan file path is a directory")
	}
	if err != nil && !os.IsNotExist(err) {
		return PlanFile{}, err
	}

	return PlanFile{SessionID: sessionID, Slug: slug, Path: path, Exists: exists}, nil
}

func (s *localService) Read(ctx context.Context, sessionID string) (PlanFile, string, error) {
	meta, err := s.Ensure(ctx, sessionID)
	if err != nil {
		return PlanFile{}, "", err
	}

	content, err := os.ReadFile(meta.Path)
	if os.IsNotExist(err) {
		return meta, "", nil
	}
	if err != nil {
		return PlanFile{}, "", err
	}
	return meta, string(content), nil
}

func (s *localService) Write(ctx context.Context, sessionID string, content string) (PlanFile, error) {
	meta, err := s.Ensure(ctx, sessionID)
	if err != nil {
		return PlanFile{}, err
	}
	if !s.IsPlanFile(ctx, sessionID, meta.Path) {
		return PlanFile{}, errors.New("plan file path failed safety check")
	}
	tmp, err := os.CreateTemp(filepath.Dir(meta.Path), ".plan-*.tmp")
	if err != nil {
		return PlanFile{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return PlanFile{}, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return PlanFile{}, err
	}
	if err := tmp.Close(); err != nil {
		return PlanFile{}, err
	}
	if err := os.Rename(tmpPath, meta.Path); err != nil {
		return PlanFile{}, err
	}
	return s.Ensure(ctx, sessionID)
}

func (s *localService) IsPlanFile(ctx context.Context, sessionID string, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	meta, err := s.Ensure(ctx, sessionID)
	if err != nil {
		return false
	}

	candidateAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	expectedAbs := filepath.Clean(meta.Path)
	if candidateAbs != expectedAbs {
		return false
	}
	info, err := os.Lstat(expectedAbs)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		realCandidate, err := realPath(candidateAbs)
		if err != nil {
			return false
		}
		realExpected, err := realPath(expectedAbs)
		if err != nil {
			return false
		}
		return realCandidate == realExpected
	case os.IsNotExist(err):
		realCandidate, err := realPath(candidateAbs)
		if err != nil {
			return false
		}
		realExpected, err := realPath(expectedAbs)
		if err != nil {
			return false
		}
		return realCandidate == realExpected
	default:
		return false
	}
}

func slugForSession(sessionID string) string {
	clean := make([]rune, 0, len(sessionID))
	for _, r := range strings.ToLower(strings.TrimSpace(sessionID)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return "plan-session"
	}
	if len(clean) > 8 {
		clean = clean[:8]
	}
	return "plan-" + string(clean)
}

func realPath(path string) (string, error) {
	rp, err := filepath.EvalSymlinks(path)
	if err == nil {
		return rp, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent, perr := filepath.EvalSymlinks(filepath.Dir(path))
	if perr != nil {
		return "", perr
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
