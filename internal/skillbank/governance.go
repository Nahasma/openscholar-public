package skillbank

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

const (
	sourceTierBundled     = "bundled"
	sourceTierProject     = "project"
	sourceTierUserManaged = "user-managed"
	sourceTierUser        = "user"
)

type GovernanceService interface {
	Service
	Install(ctx context.Context, opts InstallOptions) (*InstallResult, error)
	Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error)
	Uninstall(ctx context.Context, opts UninstallOptions) (*UninstallResult, error)
	ListGoverned(ctx context.Context, category string) ([]GovernedSkill, error)
	Info(ctx context.Context, id string) (*GovernedSkill, error)
}

type InstallOptions struct {
	LocalPath string
}

type SyncOptions struct {
	ID string
}

type UninstallOptions struct {
	ID string
}

type InstallResult struct {
	SkillID       string
	InstalledPath string
	ManifestPath  string
	Status        string
}

type SyncResult struct {
	SkillID       string
	InstalledPath string
	ManifestPath  string
	Status        string
}

type UninstallResult struct {
	SkillID      string
	RemovedPath  string
	ManifestPath string
	Status       string
}

type SkillSource struct {
	ID          string
	SkillID     string
	SourceTier  string
	SourceKind  string
	SourceKey   string
	DisplayName string
	FilePath    string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Meta        map[string]any
}

type GovernedSkill struct {
	Meta         SkillMeta
	ActiveSource *SkillSource
	Sources      []SkillSource
}

type managedManifest struct {
	Version  int                    `json:"version"`
	Installs []managedManifestEntry `json:"installs"`
}

type managedManifestEntry struct {
	SkillID       string   `json:"skill_id"`
	SourceKind    string   `json:"source_kind"`
	SourceKey     string   `json:"source_key"`
	SourcePath    string   `json:"source_path"`
	SourceType    string   `json:"source_type,omitempty"` // "file" | "bundle"
	SourceRoot    string   `json:"source_root,omitempty"` // bundle root for directory installs
	ManagedPath   string   `json:"managed_path"`
	ManagedRoot   string   `json:"managed_root,omitempty"` // relative: category/name
	ContentSHA256 string   `json:"content_sha256,omitempty"`
	ScanStatus    string   `json:"scan_status,omitempty"`     // "passed" | "failed"
	TrustLevel    string   `json:"trust_level,omitempty"`     // "local-verified" | "rejected"
	ScanSummary   string   `json:"scan_summary,omitempty"`    // brief scan summary
	ScanMatches   []string `json:"scan_matches,omitempty"`    // matched dangerous patterns
	ScannedAtUnix int64    `json:"scanned_at_unix,omitempty"` // unix seconds
	InstalledAt   int64    `json:"installed_at"`
	LastSyncUnix  int64    `json:"last_sync_unix"`
}

type sourceCandidate struct {
	Skill      Skill
	SourceTier string
	SourceKind string
	SourceKey  string
	FilePath   string
	SourceMeta string
}

type installSourceSpec struct {
	SourceType string // "file" | "bundle"
	SourcePath string // abs local input path
	SourceRoot string // abs directory root (for bundle) or dir(file)
	SkillPath  string // abs skill markdown source path
	Category   string
	Name       string
	Skill      Skill
	Scan       governanceScanResult
}

type governanceScanResult struct {
	Status    string
	Trust     string
	Summary   string
	Matches   []string
	ScannedAt int64
}

func (s *service) managedDir() string {
	return filepath.Join(s.userDir, "_managed")
}

func (s *service) managedManifestPath() string {
	return filepath.Join(s.managedDir(), "manifest.json")
}

func (s *service) projectDir() string {
	clean := filepath.Clean(s.userDir)
	if filepath.Base(clean) == "skills" {
		return filepath.Join(filepath.Dir(clean), "project-skills")
	}
	return filepath.Join(clean, "_project")
}

func (s *service) readManagedManifest() (managedManifest, error) {
	path := s.managedManifestPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return managedManifest{Version: 1, Installs: nil}, nil
		}
		return managedManifest{}, err
	}
	var manifest managedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return managedManifest{}, fmt.Errorf("parse managed manifest: %w", err)
	}
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	return manifest, nil
}

func (s *service) writeManagedManifest(manifest managedManifest) error {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	if manifest.Installs == nil {
		manifest.Installs = []managedManifestEntry{}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal managed manifest: %w", err)
	}
	path := s.managedManifestPath()
	absUserDir, absErr := filepath.Abs(s.userDir)
	if absErr != nil {
		absUserDir = s.userDir
	}
	return fileop.SafeWrite(path, append(data, '\n'), fileop.WithBoundary(absUserDir), fileop.WithMkdir())
}

func (s *service) Install(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	localPath := strings.TrimSpace(opts.LocalPath)
	if localPath == "" {
		return nil, fmt.Errorf("local_path is required")
	}
	spec, err := resolveInstallSource(localPath)
	if err != nil {
		return nil, err
	}
	if len(spec.Scan.Matches) > 0 {
		return nil, fmt.Errorf("local skill rejected by security scan: %s", strings.Join(spec.Scan.Matches, ", "))
	}

	manifest, err := s.readManagedManifest()
	if err != nil {
		return nil, err
	}
	entry := managedManifestEntry{}
	existed := false
	for i := range manifest.Installs {
		if manifest.Installs[i].SkillID == spec.Skill.ID {
			entry = manifest.Installs[i]
			existed = true
			break
		}
	}
	res, updatedEntry, err := s.writeManagedFromSourceLocked(ctx, spec, entry)
	if err != nil {
		return nil, err
	}

	updated := false
	for i := range manifest.Installs {
		if manifest.Installs[i].SkillID == res.SkillID {
			manifest.Installs[i] = updatedEntry
			updated = true
			break
		}
	}
	if !updated {
		manifest.Installs = append(manifest.Installs, updatedEntry)
	}
	sort.Slice(manifest.Installs, func(i, j int) bool {
		return manifest.Installs[i].SkillID < manifest.Installs[j].SkillID
	})
	if err := s.writeManagedManifest(manifest); err != nil {
		return nil, err
	}

	if err := s.reindexLocked(ctx); err != nil {
		return nil, err
	}
	status := "installed"
	if existed || updated {
		status = "updated"
	}
	if info, infoErr := s.Info(ctx, res.SkillID); infoErr == nil && info.ActiveSource != nil {
		managedSourceID := sourceID(res.SkillID, sourceTierUserManaged, updatedEntry.SourceKey)
		if info.ActiveSource.ID != managedSourceID {
			status = "shadowed"
		}
	}

	return &InstallResult{
		SkillID:       res.SkillID,
		InstalledPath: res.InstalledPath,
		ManifestPath:  s.managedManifestPath(),
		Status:        status,
	}, nil
}

func (s *service) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	manifest, err := s.readManagedManifest()
	if err != nil {
		return nil, err
	}
	var existing managedManifestEntry
	found := false
	for _, entry := range manifest.Installs {
		if entry.SkillID == id {
			existing = entry
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("managed skill not found: %s", id)
	}

	sourcePath := strings.TrimSpace(existing.SourcePath)
	if sourcePath == "" {
		sourcePath = strings.TrimSpace(existing.SourceKey)
	}
	if sourcePath == "" {
		return nil, fmt.Errorf("managed skill has no source path: %s", id)
	}
	spec, err := resolveInstallSource(sourcePath)
	if err != nil {
		return nil, err
	}
	if spec.Skill.ID != id {
		return nil, fmt.Errorf("sync source id mismatch: manifest=%s source=%s", id, spec.Skill.ID)
	}
	if len(spec.Scan.Matches) > 0 {
		return nil, fmt.Errorf("local skill rejected by security scan: %s", strings.Join(spec.Scan.Matches, ", "))
	}

	res, updatedEntry, err := s.writeManagedFromSourceLocked(ctx, spec, existing)
	if err != nil {
		return nil, err
	}
	for i := range manifest.Installs {
		if manifest.Installs[i].SkillID == id {
			manifest.Installs[i] = updatedEntry
			break
		}
	}
	if err := s.writeManagedManifest(manifest); err != nil {
		return nil, err
	}
	if err := s.reindexLocked(ctx); err != nil {
		return nil, err
	}
	status := "synced"
	if info, infoErr := s.Info(ctx, res.SkillID); infoErr == nil && info.ActiveSource != nil {
		managedSourceID := sourceID(res.SkillID, sourceTierUserManaged, updatedEntry.SourceKey)
		if info.ActiveSource.ID != managedSourceID {
			status = "shadowed"
		}
	}
	return &SyncResult{
		SkillID:       res.SkillID,
		InstalledPath: res.InstalledPath,
		ManifestPath:  s.managedManifestPath(),
		Status:        status,
	}, nil
}

func (s *service) Uninstall(ctx context.Context, opts UninstallOptions) (*UninstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	manifest, err := s.readManagedManifest()
	if err != nil {
		return nil, err
	}

	idx := -1
	entry := managedManifestEntry{}
	for i := range manifest.Installs {
		if manifest.Installs[i].SkillID == id {
			idx = i
			entry = manifest.Installs[i]
			break
		}
	}
	if idx < 0 {
		return &UninstallResult{
			SkillID:      id,
			ManifestPath: s.managedManifestPath(),
			Status:       "not_found",
		}, nil
	}

	relRoot := strings.TrimSpace(entry.ManagedRoot)
	if relRoot == "" {
		relPath := filepath.Clean(strings.TrimSpace(entry.ManagedPath))
		if relPath == "." || relPath == "" {
			return nil, fmt.Errorf("invalid managed path in manifest for %s", id)
		}
		relRoot = filepath.Dir(relPath)
	}
	removedPath := filepath.Join(s.managedDir(), filepath.FromSlash(relRoot))
	absManagedDir, absErr := filepath.Abs(s.managedDir())
	if absErr != nil {
		absManagedDir = s.managedDir()
	}
	if err := fileop.ValidateBoundary(removedPath, absManagedDir); err != nil {
		return nil, fmt.Errorf("remove managed skill: %w", err)
	}
	if err := os.RemoveAll(removedPath); err != nil {
		return nil, fmt.Errorf("remove managed skill: %w", err)
	}
	manifest.Installs = append(manifest.Installs[:idx], manifest.Installs[idx+1:]...)
	if err := s.writeManagedManifest(manifest); err != nil {
		return nil, err
	}
	if err := s.reindexLocked(ctx); err != nil {
		return nil, err
	}
	return &UninstallResult{
		SkillID:      id,
		RemovedPath:  removedPath,
		ManifestPath: s.managedManifestPath(),
		Status:       "uninstalled",
	}, nil
}

func validateInstallLocalPath(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat local skill: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("install path must not be a symlink")
	}
	return info, nil
}

func resolveInstallSource(localPath string) (*installSourceSpec, error) {
	absLocalPath, err := filepath.Abs(localPath)
	if err != nil {
		absLocalPath = localPath
	}
	info, err := validateInstallLocalPath(absLocalPath)
	if err != nil {
		return nil, err
	}

	spec := &installSourceSpec{
		SourcePath: filepath.Clean(absLocalPath),
	}
	if info.IsDir() {
		spec.SourceType = "bundle"
		spec.SourceRoot = spec.SourcePath
		skillPath, err := validateBundleDirectory(spec.SourceRoot)
		if err != nil {
			return nil, err
		}
		spec.SkillPath = skillPath
	} else {
		spec.SourceType = "file"
		spec.SourceRoot = filepath.Dir(spec.SourcePath)
		spec.SkillPath = spec.SourcePath
		if !strings.HasSuffix(strings.ToLower(spec.SkillPath), ".md") {
			return nil, fmt.Errorf("install path must be a .md skill file")
		}
	}

	raw, err := os.ReadFile(spec.SkillPath)
	if err != nil {
		return nil, fmt.Errorf("read local skill: %w", err)
	}
	skill, err := ParseSkillContent("", raw)
	if err != nil {
		return nil, fmt.Errorf("parse local skill: %w", err)
	}
	category, name, err := resolveSkillIdentity(spec, skill)
	if err != nil {
		return nil, err
	}
	skillID := category + "/" + name
	skill.ID = skillID
	skill.Category = category
	spec.Category = category
	spec.Name = name
	spec.Skill = *skill

	scan, err := runGovernanceScan(spec)
	if err != nil {
		return nil, err
	}
	spec.Scan = scan
	if len(spec.Scan.Matches) > 0 {
		spec.Scan.Status = "failed"
		spec.Scan.Trust = "rejected"
		if spec.Scan.Summary == "" {
			spec.Scan.Summary = "blocked dangerous content patterns"
		}
		return spec, nil
	}
	spec.Scan.Status = "passed"
	spec.Scan.Trust = "local-verified"
	if spec.Scan.Summary == "" {
		spec.Scan.Summary = "basic local security scan passed"
	}
	return spec, nil
}

func skillIDFromInstallPath(path string) (category string, name string, err error) {
	path = filepath.Clean(path)
	base := filepath.Base(path)
	if strings.EqualFold(base, "SKILL.md") {
		nameDir := filepath.Base(filepath.Dir(path))
		categoryDir := filepath.Base(filepath.Dir(filepath.Dir(path)))
		if nameDir == "." || categoryDir == "." || nameDir == "" || categoryDir == "" {
			return "", "", fmt.Errorf("install path must be category/name/SKILL.md")
		}
		return normalizeCategory(categoryDir), sanitizeFilename(nameDir), nil
	}
	if strings.HasSuffix(strings.ToLower(base), ".md") {
		name = strings.TrimSuffix(base, filepath.Ext(base))
		categoryDir := filepath.Base(filepath.Dir(path))
		if categoryDir == "." || categoryDir == "" {
			return "", "", fmt.Errorf("install path must be category/name.md")
		}
		return normalizeCategory(categoryDir), sanitizeFilename(name), nil
	}
	return "", "", fmt.Errorf("install path must be a .md skill file")
}

func resolveSkillIdentity(spec *installSourceSpec, skill *Skill) (category string, name string, err error) {
	if spec == nil || skill == nil {
		return "", "", fmt.Errorf("install source is incomplete")
	}
	if spec.SourceType == "bundle" {
		category = normalizeCategory(strings.TrimSpace(skill.Category))
		if category == "" {
			return "", "", fmt.Errorf("bundle SKILL.md must declare frontmatter category")
		}
		if !validCategories[category] {
			return "", "", fmt.Errorf("bundle SKILL.md category %q is invalid", skill.Category)
		}
		name = sanitizeFilename(filepath.Base(spec.SourceRoot))
		if name == "" || name == "." {
			return "", "", fmt.Errorf("bundle directory name is invalid")
		}
		return category, name, nil
	}
	return skillIDFromInstallPath(spec.SkillPath)
}

func (s *service) ListGoverned(ctx context.Context, category string) ([]GovernedSkill, error) {
	rows, err := s.store.ListByCategory(ctx, category, 500)
	if err != nil {
		return nil, fmt.Errorf("list governed rows: %w", err)
	}

	allSources, err := s.store.ListSkillSources(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("list governed sources: %w", err)
	}
	srcBySkill := make(map[string][]skillSourceRow, len(allSources))
	for _, src := range allSources {
		srcBySkill[src.SkillID] = append(srcBySkill[src.SkillID], src)
	}

	out := make([]GovernedSkill, 0, len(rows))
	for _, row := range rows {
		g := GovernedSkill{Meta: skillMetaFromRow(row)}
		for _, src := range srcBySkill[row.ID] {
			ss := sourceFromRow(src, row.ResolvedSourceID)
			if ss.IsActive {
				c := ss
				g.ActiveSource = &c
			}
			g.Sources = append(g.Sources, ss)
		}
		sortSources(g.Sources)
		out = append(out, g)
	}
	return out, nil
}

func (s *service) Info(ctx context.Context, id string) (*GovernedSkill, error) {
	row, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	sources, err := s.store.ListSkillSources(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list skill sources: %w", err)
	}
	out := &GovernedSkill{
		Meta:    skillMetaFromRow(*row),
		Sources: make([]SkillSource, 0, len(sources)),
	}
	for _, src := range sources {
		ss := sourceFromRow(src, row.ResolvedSourceID)
		if ss.IsActive {
			c := ss
			out.ActiveSource = &c
		}
		out.Sources = append(out.Sources, ss)
	}
	sortSources(out.Sources)
	return out, nil
}

func sourceFromRow(r skillSourceRow, activeID string) SkillSource {
	return SkillSource{
		ID:          r.ID,
		SkillID:     r.SkillID,
		SourceTier:  r.SourceTier,
		SourceKind:  r.SourceKind,
		SourceKey:   r.SourceKey,
		DisplayName: r.DisplayName,
		FilePath:    r.FilePath,
		IsActive:    r.ID == activeID,
		CreatedAt:   time.Unix(r.CreatedAt, 0),
		UpdatedAt:   time.Unix(r.UpdatedAt, 0),
		Meta:        parseSourceMetaJSON(r.MetaJSON),
	}
}

func sortSources(sources []SkillSource) {
	sort.Slice(sources, func(i, j int) bool {
		pi := sourceTierPriority(sources[i].SourceTier)
		pj := sourceTierPriority(sources[j].SourceTier)
		if pi != pj {
			return pi > pj
		}
		if sources[i].UpdatedAt.Unix() != sources[j].UpdatedAt.Unix() {
			return sources[i].UpdatedAt.Unix() > sources[j].UpdatedAt.Unix()
		}
		return sources[i].ID < sources[j].ID
	})
}

func sourceTierPriority(tier string) int {
	switch tier {
	case sourceTierUser:
		return 4
	case sourceTierUserManaged:
		return 3
	case sourceTierProject:
		return 2
	case sourceTierBundled:
		return 1
	default:
		return 0
	}
}

func sourceID(skillID string, tier string, key string) string {
	sum := sha1.Sum([]byte(skillID + "|" + tier + "|" + key))
	return hex.EncodeToString(sum[:])
}

type managedWriteResult struct {
	SkillID       string
	InstalledPath string
}

func (s *service) writeManagedFromSourceLocked(ctx context.Context, spec *installSourceSpec, existing managedManifestEntry) (*managedWriteResult, managedManifestEntry, error) {
	managedRootAbs := filepath.Join(s.managedDir(), spec.Category, spec.Name)
	managedSkillPath := filepath.Join(managedRootAbs, "SKILL.md")
	absUserDir, absErr := filepath.Abs(s.userDir)
	if absErr != nil {
		absUserDir = s.userDir
	}

	if spec.SourceType == "bundle" {
		if err := copyManagedBundle(absUserDir, spec.SourceRoot, managedRootAbs); err != nil {
			return nil, managedManifestEntry{}, err
		}
	} else {
		if err := fileop.ValidateBoundary(managedRootAbs, absUserDir); err != nil {
			return nil, managedManifestEntry{}, fmt.Errorf("clear existing managed skill: %w", err)
		}
		if err := os.RemoveAll(managedRootAbs); err != nil {
			return nil, managedManifestEntry{}, fmt.Errorf("clear existing managed skill: %w", err)
		}
		managedContent := []byte(spec.Skill.ToMarkdown())
		if err := fileop.SafeWrite(managedSkillPath, managedContent, fileop.WithBoundary(absUserDir), fileop.WithMkdir()); err != nil {
			return nil, managedManifestEntry{}, fmt.Errorf("write managed skill: %w", err)
		}
		writtenRaw, err := os.ReadFile(managedSkillPath)
		if err != nil {
			return nil, managedManifestEntry{}, fmt.Errorf("read managed skill: %w", err)
		}
		writtenDigest := sha256.Sum256(writtenRaw)
		managedDigest := sha256.Sum256(managedContent)
		if writtenDigest != managedDigest {
			return nil, managedManifestEntry{}, fmt.Errorf("managed install digest mismatch after copy")
		}
	}

	managedDigest, err := hashManagedBundle(managedRootAbs)
	if err != nil {
		return nil, managedManifestEntry{}, err
	}
	now := time.Now().Unix()
	installedAt := existing.InstalledAt
	if installedAt == 0 {
		installedAt = now
	}
	sourceKind := "local-file"
	if spec.SourceType == "bundle" {
		sourceKind = "local-bundle"
	}
	entry := managedManifestEntry{
		SkillID:       spec.Skill.ID,
		SourceKind:    sourceKind,
		SourceType:    spec.SourceType,
		SourceKey:     filepath.Clean(spec.SourcePath),
		SourcePath:    filepath.Clean(spec.SourcePath),
		SourceRoot:    filepath.Clean(spec.SourceRoot),
		ManagedPath:   filepath.ToSlash(filepath.Join(spec.Category, spec.Name, "SKILL.md")),
		ManagedRoot:   filepath.ToSlash(filepath.Join(spec.Category, spec.Name)),
		ContentSHA256: managedDigest,
		ScanStatus:    spec.Scan.Status,
		TrustLevel:    spec.Scan.Trust,
		ScanSummary:   spec.Scan.Summary,
		ScanMatches:   append([]string(nil), spec.Scan.Matches...),
		ScannedAtUnix: spec.Scan.ScannedAt,
		InstalledAt:   installedAt,
		LastSyncUnix:  now,
	}
	return &managedWriteResult{
		SkillID:       spec.Skill.ID,
		InstalledPath: managedSkillPath,
	}, entry, nil
}

func parseSourceMetaJSON(metaJSON string) map[string]any {
	metaJSON = strings.TrimSpace(metaJSON)
	if metaJSON == "" {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &obj); err != nil {
		return nil
	}
	return obj
}

func sourceMetaJSONFromManagedEntry(entry managedManifestEntry) string {
	meta := map[string]any{
		"source_type":     entry.SourceType,
		"source_path":     entry.SourcePath,
		"source_root":     entry.SourceRoot,
		"managed_root":    entry.ManagedRoot,
		"scan_status":     entry.ScanStatus,
		"trust_level":     entry.TrustLevel,
		"scan_summary":    entry.ScanSummary,
		"scanned_at_unix": entry.ScannedAtUnix,
	}
	if len(entry.ScanMatches) > 0 {
		meta["scan_matches"] = append([]string(nil), entry.ScanMatches...)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func validateBundleDirectory(bundleDir string) (string, error) {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return "", fmt.Errorf("read bundle directory: %w", err)
	}
	allowed := map[string]bool{
		"SKILL.md":   true,
		"references": true,
		"templates":  true,
		"scripts":    true,
		"assets":     true,
	}
	hasSkill := false
	for _, entry := range entries {
		name := entry.Name()
		if !allowed[name] {
			return "", fmt.Errorf("bundle directory contains unsupported top-level entry: %s", name)
		}
		full := filepath.Join(bundleDir, name)
		info, err := os.Lstat(full)
		if err != nil {
			return "", fmt.Errorf("stat bundle entry %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("bundle entry must not be a symlink: %s", name)
		}
		if name == "SKILL.md" {
			if info.IsDir() {
				return "", fmt.Errorf("bundle SKILL.md must be a file")
			}
			hasSkill = true
			continue
		}
		if !info.IsDir() {
			return "", fmt.Errorf("bundle top-level entry must be a directory: %s", name)
		}
	}
	if !hasSkill {
		return "", fmt.Errorf("bundle directory must contain SKILL.md")
	}
	return filepath.Join(bundleDir, "SKILL.md"), nil
}

func runGovernanceScan(spec *installSourceSpec) (governanceScanResult, error) {
	if spec == nil {
		return governanceScanResult{}, fmt.Errorf("nil install source")
	}
	scan := governanceScanResult{
		ScannedAt: time.Now().Unix(),
	}

	if spec.SourceType == "bundle" {
		paths, err := collectBundleScanPaths(spec.SourceRoot)
		if err != nil {
			return scan, err
		}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				return scan, fmt.Errorf("read bundle content for scan: %w", err)
			}
			if !isLikelyTextualFile(p, data) {
				continue
			}
			scan.Matches = append(scan.Matches, detectDangerousPatterns(data)...)
		}
	} else {
		data, err := os.ReadFile(spec.SkillPath)
		if err != nil {
			return scan, fmt.Errorf("read local skill for scan: %w", err)
		}
		if isLikelyTextualFile(spec.SkillPath, data) {
			scan.Matches = append(scan.Matches, detectDangerousPatterns(data)...)
		}
	}
	scan.Matches = dedupeStrings(scan.Matches)
	return scan, nil
}

func collectBundleScanPaths(bundleRoot string) ([]string, error) {
	allowedDirs := map[string]bool{
		"references": true,
		"templates":  true,
		"scripts":    true,
		"assets":     true,
	}
	entries, err := os.ReadDir(bundleRoot)
	if err != nil {
		return nil, fmt.Errorf("read bundle root: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		full := filepath.Join(bundleRoot, name)
		info, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("stat bundle entry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("bundle entry must not be a symlink: %s", name)
		}
		if name == "SKILL.md" {
			paths = append(paths, full)
			continue
		}
		if !allowedDirs[name] {
			continue
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("bundle top-level entry must be directory: %s", name)
		}
		if err := filepath.WalkDir(full, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			dinfo, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if dinfo.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("bundle content must not be a symlink: %s", path)
			}
			if d.IsDir() {
				return nil
			}
			paths = append(paths, path)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func copyManagedBundle(absUserDir string, sourceRoot string, managedRootAbs string) error {
	if err := os.RemoveAll(managedRootAbs); err != nil {
		return fmt.Errorf("clear existing managed bundle: %w", err)
	}
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return fmt.Errorf("read source bundle: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		srcPath := filepath.Join(sourceRoot, name)
		dstPath := filepath.Join(managedRootAbs, name)
		info, err := os.Lstat(srcPath)
		if err != nil {
			return fmt.Errorf("stat source bundle entry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle content must not be a symlink: %s", srcPath)
		}
		if !info.IsDir() {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("read source file: %w", err)
			}
			if err := fileop.SafeWrite(dstPath, data, fileop.WithBoundary(absUserDir), fileop.WithMkdir()); err != nil {
				return fmt.Errorf("write managed file: %w", err)
			}
			if err := os.Chmod(dstPath, info.Mode().Perm()); err != nil {
				return fmt.Errorf("chmod managed file: %w", err)
			}
			continue
		}
		if err := filepath.WalkDir(srcPath, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("bundle content must not be a symlink: %s", path)
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(sourceRoot, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
				dstFile := filepath.Join(managedRootAbs, rel)
				if err := fileop.SafeWrite(dstFile, data, fileop.WithBoundary(absUserDir), fileop.WithMkdir(), fileop.WithPerm(info.Mode().Perm())); err != nil {
					return err
				}
				return os.Chmod(dstFile, info.Mode().Perm())
			}); err != nil {
				return fmt.Errorf("copy bundle directory: %w", err)
			}
	}
	return nil
}

func hashManagedBundle(managedRootAbs string) (string, error) {
	var files []string
	err := filepath.WalkDir(managedRootAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk managed bundle: %w", err)
	}
	sort.Strings(files)
	hasher := sha256.New()
	for _, f := range files {
		rel, err := filepath.Rel(managedRootAbs, f)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("read managed bundle file: %w", err)
		}
		hasher.Write([]byte(filepath.ToSlash(rel)))
		hasher.Write([]byte{0})
		hasher.Write(data)
		hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func isLikelyTextualFile(path string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".sh", ".bash", ".zsh", ".py", ".js", ".ts", ".tsx", ".go", ".sql", ".rb", ".ps1":
		return true
	default:
		// Heuristic fallback: skip obvious binary.
		return !strings.Contains(string(data), "\x00")
	}
}

var governanceDangerPatterns = []string{
	"rm -rf /",
	"rm -rf ~",
	"curl | sh",
	"wget | sh",
	"powershell -enc",
	"os.removeall(",
	"subprocess.popen(",
	"eval(",
}

func detectDangerousPatterns(data []byte) []string {
	content := strings.ToLower(string(data))
	var matches []string
	for _, pat := range governanceDangerPatterns {
		if strings.Contains(content, pat) {
			matches = append(matches, pat)
		}
	}
	return matches
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
