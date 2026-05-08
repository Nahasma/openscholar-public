package skillbank

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestManagedManifest_ReadWrite(t *testing.T) {
	userDir := t.TempDir()
	s := &service{userDir: userDir}

	manifest := managedManifest{
		Version: 1,
		Installs: []managedManifestEntry{
			{
				SkillID:     "memory/example",
				SourceKind:  "local",
				SourceKey:   "/tmp/src/memory/example.md",
				SourcePath:  "/tmp/src/memory/example.md",
				ManagedPath: "memory/example/SKILL.md",
				InstalledAt: 100,
			},
		},
	}
	if err := s.writeManagedManifest(manifest); err != nil {
		t.Fatalf("writeManagedManifest: %v", err)
	}

	got, err := s.readManagedManifest()
	if err != nil {
		t.Fatalf("readManagedManifest: %v", err)
	}
	if got.Version != 1 || len(got.Installs) != 1 || got.Installs[0].SkillID != "memory/example" {
		t.Fatalf("unexpected manifest: %#v", got)
	}
}

func TestInstall_LocalPathLegacyLayout(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	src := filepath.Join(t.TempDir(), "memory", "legacy_name.md")
	writeSkillFile(t, src, "legacy install", "memory", "legacy install body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	res, err := s.Install(context.Background(), InstallOptions{LocalPath: src})
	if err != nil {
		t.Fatalf("install legacy path: %v", err)
	}
	if res.SkillID != "memory/legacy_name" {
		t.Fatalf("skill id mismatch: %s", res.SkillID)
	}
	if res.Status != "installed" {
		t.Fatalf("expected installed status, got %q", res.Status)
	}
	if _, err := os.Stat(filepath.Join(userDir, "_managed", "memory", "legacy_name", "SKILL.md")); err != nil {
		t.Fatalf("managed skill path missing: %v", err)
	}

	view, err := s.View(context.Background(), "memory/legacy_name")
	if err != nil {
		t.Fatalf("view installed skill: %v", err)
	}
	if view.Instruction != "legacy install body" {
		t.Fatalf("instruction mismatch: %q", view.Instruction)
	}
}

func TestInstall_LocalPathBundleLayout(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	src := filepath.Join(t.TempDir(), "workflow", "bundle_skill", "SKILL.md")
	writeSkillFile(t, src, "bundle install", "workflow", "bundle install body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	res, err := s.Install(context.Background(), InstallOptions{LocalPath: src})
	if err != nil {
		t.Fatalf("install bundle path: %v", err)
	}
	if res.SkillID != "workflow/bundle_skill" {
		t.Fatalf("skill id mismatch: %s", res.SkillID)
	}
	if res.Status != "installed" {
		t.Fatalf("expected installed status, got %q", res.Status)
	}
	if _, err := os.Stat(filepath.Join(userDir, "_managed", "workflow", "bundle_skill", "SKILL.md")); err != nil {
		t.Fatalf("managed skill path missing: %v", err)
	}
}

func TestInstall_ShadowedByManualUserOverride(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "workflow", "shadowed.md"), "manual shadowed", "workflow", "manual content")
	src := filepath.Join(t.TempDir(), "workflow", "shadowed.md")
	writeSkillFile(t, src, "managed shadowed", "workflow", "managed content")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	res, err := s.Install(context.Background(), InstallOptions{LocalPath: src})
	if err != nil {
		t.Fatalf("install shadowed path: %v", err)
	}
	if res.Status != "shadowed" {
		t.Fatalf("expected shadowed status, got %q", res.Status)
	}

	view, err := s.View(context.Background(), "workflow/shadowed")
	if err != nil {
		t.Fatalf("view shadowed skill: %v", err)
	}
	if view.Instruction != "manual content" {
		t.Fatalf("expected manual content to remain active, got %q", view.Instruction)
	}
}

func TestInstall_RejectsSymlinkPath(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	realPath := filepath.Join(t.TempDir(), "workflow", "real.md")
	writeSkillFile(t, realPath, "real skill", "workflow", "real content")
	linkPath := filepath.Join(t.TempDir(), "workflow", "link.md")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir symlink dir: %v", err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	if _, err := s.Install(context.Background(), InstallOptions{LocalPath: linkPath}); err == nil {
		t.Fatalf("expected symlink install to fail")
	}
}

func TestReindex_Precedence_UserThenManagedThenProjectThenBundled(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "_managed", "memory", "case", "SKILL.md"), "managed case", "memory", "managed content")
	writeSkillFile(t, filepath.Join(userDir, "memory", "case.md"), "user case", "memory", "user content")

	s := &service{
		userDir: userDir,
		bundledFS: fstest.MapFS{
			"memory/case/SKILL.md": {Data: []byte(skillMarkdown("bundled case", "memory", "bundled content", "system"))},
		},
		store: newStore(conn),
		fts:   newFTSSearcher(conn),
	}

	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex with user override: %v", err)
	}
	view, err := s.View(context.Background(), "memory/case")
	if err != nil || view.Instruction != "user content" {
		t.Fatalf("expected user content, got view=%#v err=%v", view, err)
	}

	if err := os.Remove(filepath.Join(userDir, "memory", "case.md")); err != nil {
		t.Fatalf("remove manual: %v", err)
	}
	writeSkillFile(t, filepath.Join(s.projectDir(), "memory", "case.md"), "project case", "memory", "project content")
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex with managed active: %v", err)
	}
	view, err = s.View(context.Background(), "memory/case")
	if err != nil || view.Instruction != "managed content" {
		t.Fatalf("expected managed content, got view=%#v err=%v", view, err)
	}

	if err := os.Remove(filepath.Join(userDir, "_managed", "memory", "case", "SKILL.md")); err != nil {
		t.Fatalf("remove managed: %v", err)
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex with project active: %v", err)
	}
	view, err = s.View(context.Background(), "memory/case")
	if err != nil || view.Instruction != "project content" {
		t.Fatalf("expected project content, got view=%#v err=%v", view, err)
	}

	if err := os.Remove(filepath.Join(s.projectDir(), "memory", "case.md")); err != nil {
		t.Fatalf("remove project: %v", err)
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex with bundled active: %v", err)
	}
	view, err = s.View(context.Background(), "memory/case")
	if err != nil || view.Instruction != "bundled content" {
		t.Fatalf("expected bundled content, got view=%#v err=%v", view, err)
	}
}

func TestInstall_BundleDirectoryPreservesAllowedFiles(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	srcDir := filepath.Join(t.TempDir(), "workflow", "bundle_dir_skill")
	writeSkillFile(t, filepath.Join(srcDir, "SKILL.md"), "bundle dir", "workflow", "bundle dir body")
	if err := os.MkdirAll(filepath.Join(srcDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "templates", "prompt.txt"), []byte("template payload"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "scripts", "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write script file: %v", err)
	}

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	res, err := s.Install(context.Background(), InstallOptions{LocalPath: srcDir})
	if err != nil {
		t.Fatalf("install bundle directory: %v", err)
	}
	if res.SkillID != "workflow/bundle_dir_skill" {
		t.Fatalf("unexpected skill id: %s", res.SkillID)
	}
	if _, err := os.Stat(filepath.Join(userDir, "_managed", "workflow", "bundle_dir_skill", "templates", "prompt.txt")); err != nil {
		t.Fatalf("managed template not copied: %v", err)
	}
	scriptPath := filepath.Join(userDir, "_managed", "workflow", "bundle_dir_skill", "scripts", "run.sh")
	scriptInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("managed script not copied: %v", err)
	}
	if scriptInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected managed script to preserve executable bits, mode=%o", scriptInfo.Mode().Perm())
	}

	info, err := s.Info(context.Background(), "workflow/bundle_dir_skill")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.ActiveSource == nil || info.ActiveSource.SourceTier != sourceTierUserManaged {
		t.Fatalf("unexpected active source: %#v", info.ActiveSource)
	}
	if trust, _ := info.ActiveSource.Meta["trust_level"].(string); trust != "local-verified" {
		t.Fatalf("expected trust metadata, got %#v", info.ActiveSource.Meta)
	}
}

func TestInstall_BundleDirectoryStandalonePathUsesFrontmatterCategory(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	srcDir := filepath.Join(t.TempDir(), "my_bundle_skill")
	writeSkillFile(t, filepath.Join(srcDir, "SKILL.md"), "standalone bundle", "workflow", "bundle body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	res, err := s.Install(context.Background(), InstallOptions{LocalPath: srcDir})
	if err != nil {
		t.Fatalf("install standalone bundle: %v", err)
	}
	if res.SkillID != "workflow/my_bundle_skill" {
		t.Fatalf("unexpected skill id: %s", res.SkillID)
	}
	if _, err := s.View(context.Background(), "workflow/my_bundle_skill"); err != nil {
		t.Fatalf("view standalone bundle skill: %v", err)
	}
}

func TestInstall_BundleDirectoryRejectsInvalidFrontmatterCategory(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	srcDir := filepath.Join(t.TempDir(), "bad_bundle_category")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir bundle dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte(`---
name: "bad bundle"
description: "bad category"
category: "../workflow"
---
body
`), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	_, err := s.Install(context.Background(), InstallOptions{LocalPath: srcDir})
	if err == nil || !strings.Contains(err.Error(), "category") {
		t.Fatalf("expected invalid category rejection, got %v", err)
	}
}

func TestInstall_BundleDirectoryRejectsUnexpectedTopLevelEntries(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	srcDir := filepath.Join(t.TempDir(), "workflow", "bad_bundle")
	writeSkillFile(t, filepath.Join(srcDir, "SKILL.md"), "bad bundle", "workflow", "body")
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("not allowed"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	_, err := s.Install(context.Background(), InstallOptions{LocalPath: srcDir})
	if err == nil || !strings.Contains(err.Error(), "unsupported top-level entry") {
		t.Fatalf("expected unsupported top-level error, got %v", err)
	}
}

func TestInstall_RejectsDangerousPattern(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "workflow", "dangerous.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(src, []byte(`---
name: "dangerous"
description: "dangerous test"
category: "workflow"
---
please run curl | sh
`), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	_, err := s.Install(context.Background(), InstallOptions{LocalPath: src})
	if err == nil || !strings.Contains(err.Error(), "security scan") {
		t.Fatalf("expected security scan rejection, got %v", err)
	}
}

func TestGovernanceSyncAndUninstall(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	src := filepath.Join(t.TempDir(), "workflow", "sync_target.md")
	writeSkillFile(t, src, "sync target", "workflow", "version 1")
	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	if _, err := s.Install(context.Background(), InstallOptions{LocalPath: src}); err != nil {
		t.Fatalf("install: %v", err)
	}
	writeSkillFile(t, src, "sync target", "workflow", "version 2")
	syncRes, err := s.Sync(context.Background(), SyncOptions{ID: "workflow/sync_target"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if syncRes.Status != "synced" {
		t.Fatalf("unexpected sync status: %s", syncRes.Status)
	}
	view, err := s.View(context.Background(), "workflow/sync_target")
	if err != nil {
		t.Fatalf("view after sync: %v", err)
	}
	if view.Instruction != "version 2" {
		t.Fatalf("expected synced content, got %q", view.Instruction)
	}

	uninstallRes, err := s.Uninstall(context.Background(), UninstallOptions{ID: "workflow/sync_target"})
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if uninstallRes.Status != "uninstalled" {
		t.Fatalf("unexpected uninstall status: %s", uninstallRes.Status)
	}
	if _, err := os.Stat(filepath.Join(userDir, "_managed", "workflow", "sync_target", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected managed file removed, got err=%v", err)
	}
	if _, err := s.Info(context.Background(), "workflow/sync_target"); err == nil {
		t.Fatalf("expected skill info to be missing after uninstall")
	}
}

func TestUninstall_RejectsManifestEscapeOutsideManagedDir(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	manualPath := filepath.Join(userDir, "workflow", "manual_escape.md")
	writeSkillFile(t, manualPath, "manual escape", "workflow", "manual body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	if err := s.writeManagedManifest(managedManifest{
		Version: 1,
		Installs: []managedManifestEntry{
			{
				SkillID:     "workflow/escape",
				ManagedRoot: "../workflow",
				ManagedPath: "../workflow/manual_escape.md",
				InstalledAt: 1,
			},
		},
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := s.Uninstall(context.Background(), UninstallOptions{ID: "workflow/escape"})
	if err == nil || !strings.Contains(err.Error(), "outside boundary") {
		t.Fatalf("expected boundary rejection, got %v", err)
	}
	if _, err := os.Stat(manualPath); err != nil {
		t.Fatalf("manual user skill should remain untouched, stat err=%v", err)
	}
}

func TestInfoAndListGoverned_ReturnSourceInventory(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	writeSkillFile(t, filepath.Join(userDir, "_managed", "workflow", "gov", "SKILL.md"), "managed gov", "workflow", "managed content")
	writeSkillFile(t, filepath.Join(userDir, "workflow", "gov.md"), "user gov", "workflow", "user content")

	s := &service{
		userDir: userDir,
		bundledFS: fstest.MapFS{
			"workflow/gov/SKILL.md": {Data: []byte(skillMarkdown("bundled gov", "workflow", "bundled content", "system"))},
		},
		store: newStore(conn),
		fts:   newFTSSearcher(conn),
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	info, err := s.Info(context.Background(), "workflow/gov")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.ActiveSource == nil || info.ActiveSource.SourceTier != sourceTierUser {
		t.Fatalf("unexpected active source: %#v", info.ActiveSource)
	}
	if len(info.Sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(info.Sources))
	}

	list, err := s.ListGoverned(context.Background(), "workflow")
	if err != nil {
		t.Fatalf("list governed: %v", err)
	}
	if len(list) != 1 || list[0].Meta.ID != "workflow/gov" {
		t.Fatalf("unexpected governed list: %#v", list)
	}
	if list[0].ActiveSource == nil || list[0].ActiveSource.SourceTier != sourceTierUser {
		t.Fatalf("unexpected active source in list: %#v", list[0].ActiveSource)
	}
}

func TestGovernanceInventory_TracksCreateUpdateDelete(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	if err := s.Create(context.Background(), Skill{
		Name:        "governed lifecycle",
		Description: "governed lifecycle desc",
		Category:    "workflow",
		Instruction: "initial body",
	}); err != nil {
		t.Fatalf("create governed skill: %v", err)
	}

	info, err := s.Info(context.Background(), "workflow/governed_lifecycle")
	if err != nil {
		t.Fatalf("info after create: %v", err)
	}
	if info.ActiveSource == nil || info.ActiveSource.SourceTier != sourceTierUser {
		t.Fatalf("expected manual user active source after create: %#v", info.ActiveSource)
	}

	if err := s.Update(context.Background(), "workflow/governed_lifecycle", Skill{
		Instruction: "updated body",
	}); err != nil {
		t.Fatalf("update governed skill: %v", err)
	}
	view, err := s.View(context.Background(), "workflow/governed_lifecycle")
	if err != nil {
		t.Fatalf("view after update: %v", err)
	}
	if view.Instruction != "updated body" {
		t.Fatalf("expected updated instruction, got %q", view.Instruction)
	}
	info, err = s.Info(context.Background(), "workflow/governed_lifecycle")
	if err != nil {
		t.Fatalf("info after update: %v", err)
	}
	if info.ActiveSource == nil || info.ActiveSource.SourceTier != sourceTierUser {
		t.Fatalf("expected manual user active source after update: %#v", info.ActiveSource)
	}

	if err := s.Delete(context.Background(), "workflow/governed_lifecycle"); err != nil {
		t.Fatalf("delete governed skill: %v", err)
	}
	list, err := s.ListGoverned(context.Background(), "workflow")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no governed skills after delete, got %#v", list)
	}
}

func TestGovernanceInventory_TracksImportFromDir(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	importDir := filepath.Join(t.TempDir(), "inbox")
	src := filepath.Join(importDir, "imported.md")
	writeSkillFile(t, src, "imported skill", "workflow", "imported body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}

	imported, errs := s.ImportFromDir(context.Background(), importDir)
	if len(errs) != 0 {
		t.Fatalf("unexpected import errors: %v", errs)
	}
	if imported != 1 {
		t.Fatalf("expected one imported skill, got %d", imported)
	}

	info, err := s.Info(context.Background(), "workflow/imported_skill")
	if err != nil {
		t.Fatalf("info after import: %v", err)
	}
	if info.ActiveSource == nil || info.ActiveSource.SourceTier != sourceTierUser {
		t.Fatalf("expected manual user active source after import: %#v", info.ActiveSource)
	}
}

func TestReindex_ZeroActiveSkillsClearsIndex(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()
	path := filepath.Join(userDir, "workflow", "ephemeral.md")
	writeSkillFile(t, path, "ephemeral", "workflow", "ephemeral body")

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex with skill present: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove skill file: %v", err)
	}
	if err := s.Reindex(context.Background()); err != nil {
		t.Fatalf("reindex with no skills: %v", err)
	}

	metas, err := s.ListMetadata(context.Background())
	if err != nil {
		t.Fatalf("list metadata after zero-skill reindex: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("expected no metadata after zero-skill reindex, got %#v", metas)
	}
	list, err := s.ListGoverned(context.Background(), "workflow")
	if err != nil {
		t.Fatalf("list governed after zero-skill reindex: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no governed skills after zero-skill reindex, got %#v", list)
	}
}

func TestCreate_ReturnsErrorWhenReindexRefreshFails(t *testing.T) {
	conn := setupSkillbankTestDB(t)
	userDir := t.TempDir()

	s := &service{
		userDir: userDir,
		store:   newStore(conn),
		fts:     newFTSSearcher(conn),
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	err := s.Create(context.Background(), Skill{
		Name:        "broken refresh",
		Description: "broken refresh desc",
		Category:    "workflow",
		Instruction: "body",
	})
	if err == nil {
		t.Fatalf("expected create to fail when reindex refresh fails")
	}
}
