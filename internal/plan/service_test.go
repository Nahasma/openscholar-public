package plan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAndReadStablePath(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	first, err := svc.Ensure(context.Background(), "Session-ABC123456")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	second, err := svc.Ensure(context.Background(), "Session-ABC123456")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	if first.Path != second.Path {
		t.Fatalf("expected stable path, got %q and %q", first.Path, second.Path)
	}
	if first.Exists {
		t.Fatalf("expected Ensure to reserve a path without creating the file")
	}
	if filepath.Ext(first.Path) != ".md" {
		t.Fatalf("expected .md path, got %q", first.Path)
	}

	meta, content, err := svc.Read(context.Background(), "Session-ABC123456")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if meta.Path != first.Path {
		t.Fatalf("expected stable read path")
	}
	if content != "" {
		t.Fatalf("expected empty content for new plan file")
	}

	written, err := svc.Write(context.Background(), "Session-ABC123456", "approved plan")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if !written.Exists {
		t.Fatalf("expected Write to create the file")
	}
	_, content, err = svc.Read(context.Background(), "Session-ABC123456")
	if err != nil {
		t.Fatalf("Read after Write failed: %v", err)
	}
	if content != "approved plan" {
		t.Fatalf("unexpected content %q", content)
	}
}

func TestIsPlanFileExactAndSessionBound(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)
	ctx := context.Background()

	planA, err := svc.Ensure(ctx, "session-A")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if ok := svc.IsPlanFile(ctx, "session-A", planA.Path); !ok {
		t.Fatalf("expected exact path to pass")
	}

	if ok := svc.IsPlanFile(ctx, "session-A", planA.Path+".bak"); ok {
		t.Fatalf("expected similar prefix path to fail")
	}
	if ok := svc.IsPlanFile(ctx, "session-A", filepath.Join(filepath.Dir(planA.Path), "..", filepath.Base(planA.Path))); ok {
		t.Fatalf("expected non-canonical ../ path to fail")
	}
	if ok := svc.IsPlanFile(ctx, "session-B", planA.Path); ok {
		t.Fatalf("expected different session to fail")
	}
}

func TestIsPlanFileRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)
	ctx := context.Background()

	planA, err := svc.Ensure(ctx, "session-A")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside failed: %v", err)
	}

	if err := os.Symlink(outside, planA.Path); err != nil {
		t.Fatalf("symlink failed: %v", err)
	}

	if ok := svc.IsPlanFile(ctx, "session-A", planA.Path); ok {
		t.Fatalf("expected symlink-escaped plan path to fail")
	}
	if _, _, err := svc.Read(ctx, "session-A"); err == nil {
		t.Fatalf("expected Read to reject symlinked plan path")
	}
	if _, err := svc.Write(ctx, "session-A", "edited"); err == nil {
		t.Fatalf("expected Write to reject symlinked plan path")
	}
}
