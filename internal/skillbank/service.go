package skillbank

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/fileop"
)

// BundledFS holds embedded system skills, injected from main.go.
var BundledFS fs.FS

// SkillMeta is a lightweight view of skill metadata for prompt injection.
// Unlike Skill, it does not require disk I/O to load .md content.
type SkillMeta struct {
	ID                     string
	Name                   string
	Description            string
	Category               string
	Tags                   []string
	WhenToUse              string
	AllowedTools           []string
	Agent                  string
	Context                string
	Effort                 string
	Model                  string
	Exposure               SkillExposure
	UserInvocable          bool
	DisableModelInvocation bool
	Paths                  []string
	Platforms              []string
	Source                 string
	UsageCount             int
}

func (m SkillMeta) IsUserInvocable() bool {
	if m.Exposure == "" {
		return m.UserInvocable
	}
	return m.Exposure == SkillExposureExplicit || m.Exposure == SkillExposureBoth
}

func (m SkillMeta) IsModelInvocable() bool {
	if m.Exposure == "" {
		return !m.DisableModelInvocation
	}
	return m.Exposure == SkillExposureImplicit || m.Exposure == SkillExposureBoth
}

func (m SkillMeta) IsDisabled() bool {
	if m.Exposure == "" {
		return !m.UserInvocable && m.DisableModelInvocation
	}
	return m.Exposure == SkillExposureNone
}

// SkillSearchHit is an L2 view: indexed metadata + snippet, without loading full file content.
type SkillSearchHit struct {
	Meta    SkillMeta
	Snippet string
	Rank    float64
}

// Service is the general-purpose skill bank interface.
type Service interface {
	// Query
	Search(ctx context.Context, opts QueryOptions) ([]Skill, error)
	SearchMetadata(ctx context.Context, opts QueryOptions) ([]SkillSearchHit, error)
	View(ctx context.Context, id string) (*Skill, error)
	Get(ctx context.Context, id string) (*Skill, error)
	List(ctx context.Context, category string) ([]Skill, error)
	ListMetadata(ctx context.Context) ([]SkillMeta, error)

	// CRUD
	Create(ctx context.Context, skill Skill) error
	// Update replaces mutable body/metadata fields with the provided full skill object.
	Update(ctx context.Context, id string, skill Skill) error
	Delete(ctx context.Context, id string) error

	// Stats
	RecordUsage(ctx context.Context, id string, success bool) error

	// Import
	ImportFromDir(ctx context.Context, dir string) (imported int, errs []error)

	// Lifecycle
	Reindex(ctx context.Context) error

	// Config
	SetLLMCaller(caller LLMCaller)
}

// LLMCaller makes a simple LLM call. Used to classify unstructured skill files.
type LLMCaller func(ctx context.Context, prompt string) (string, error)

type service struct {
	userDir   string // absolute path to .openscholar/skills/
	bundledFS fs.FS  // embedded system skills (may be nil)
	store     *store
	fts       *ftsSearcher
	callLLM   LLMCaller
	mu        sync.Mutex // protects file writes
}

// NewService creates a new SkillBank service.
// userDir is the root directory for user skill files (e.g., ".openscholar/skills").
// callLLM is optional; if provided, non-standard .md files are classified via LLM.
func NewService(conn *sql.DB, userDir string, callLLM LLMCaller) Service {
	s := &service{
		userDir:   userDir,
		bundledFS: BundledFS, // read package-level variable injected from main.go
		store:     newStore(conn),
		fts:       newFTSSearcher(conn),
		callLLM:   callLLM,
	}

	// Ensure user directory and _inbox exist
	os.MkdirAll(userDir, 0o755)
	os.MkdirAll(filepath.Join(userDir, "_inbox"), 0o755)
	os.MkdirAll(filepath.Join(userDir, "_managed"), 0o755)
	os.MkdirAll(s.projectDir(), 0o755)

	// Migrate legacy flat .md files to category subdirectories
	migrateLegacySkills(userDir)

	// Reindex on startup (dual-source: bundled + user)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Reindex(ctx); err != nil {
		log.Printf("[skillbank] reindex failed: %v", err)
	}

	// Auto-import from _inbox directory
	inboxDir := filepath.Join(userDir, "_inbox")
	if imported, errs := s.ImportFromDir(ctx, inboxDir); imported > 0 {
		log.Printf("[skillbank] auto-imported %d skills from _inbox", imported)
		for _, e := range errs {
			log.Printf("[skillbank] import error: %v", e)
		}
	}

	return s
}

func (s *service) Search(ctx context.Context, opts QueryOptions) ([]Skill, error) {
	if opts.Limit <= 0 {
		opts.Limit = 5
	}

	// If there's a free-text query, use FTS5
	if opts.Query != "" {
		results, err := s.fts.Search(ctx, opts.Query, opts.Category, opts.Limit*2)
		if err != nil {
			return nil, fmt.Errorf("fts search: %w", err)
		}

		// Load full skill content from disk for matched results
		var skills []Skill
		for _, r := range results {
			skill, err := s.loadSkillByID(r.ID)
			if err != nil {
				continue
			}
			skill.UsageCount = r.row.UsageCount
			skills = append(skills, *skill)
		}

		// Apply tag filter if specified
		if len(opts.Tags) > 0 {
			skills = filterByTags(skills, opts.Tags)
		}

		if len(skills) > opts.Limit {
			skills = skills[:opts.Limit]
		}
		return skills, nil
	}

	// No free-text query: list by category with optional tag filter
	rows, err := s.store.ListByCategory(ctx, opts.Category, opts.Limit*2)
	if err != nil {
		return nil, fmt.Errorf("list by category: %w", err)
	}

	var skills []Skill
	for _, row := range rows {
		skill, err := s.loadSkillByID(row.ID)
		if err != nil {
			continue
		}
		skill.UsageCount = row.UsageCount
		skills = append(skills, *skill)
	}

	if len(opts.Tags) > 0 {
		skills = filterByTags(skills, opts.Tags)
	}

	if len(skills) > opts.Limit {
		skills = skills[:opts.Limit]
	}
	return skills, nil
}

func (s *service) SearchMetadata(ctx context.Context, opts QueryOptions) ([]SkillSearchHit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 5
	}
	if opts.Query == "" {
		return nil, nil
	}

	results, err := s.fts.Search(ctx, opts.Query, opts.Category, opts.Limit*2)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}

	hits := make([]SkillSearchHit, 0, len(results))
	for _, r := range results {
		meta := skillMetaFromRow(r.row)
		if len(opts.Tags) > 0 && !rowMatchesTags(r.row, opts.Tags) {
			continue
		}
		hits = append(hits, SkillSearchHit{
			Meta:    meta,
			Snippet: r.Snippet,
			Rank:    r.Rank,
		})
		if len(hits) >= opts.Limit {
			break
		}
	}
	return hits, nil
}

func (s *service) View(ctx context.Context, id string) (*Skill, error) {
	skill, err := s.loadSkillByID(id)
	if err != nil {
		return nil, fmt.Errorf("view skill %s: %w", id, err)
	}

	row, dbErr := s.store.Get(ctx, id)
	if dbErr == nil {
		skill.UsageCount = row.UsageCount
		applyMetadataJSON(skill, row.MetaJSON)
	}
	return skill, nil
}

func (s *service) Get(ctx context.Context, id string) (*Skill, error) {
	return s.View(ctx, id)
}

func (s *service) List(ctx context.Context, category string) ([]Skill, error) {
	rows, err := s.store.ListByCategory(ctx, category, 100)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	var skills []Skill
	for _, row := range rows {
		skill, err := s.loadSkillByID(row.ID)
		if err != nil {
			continue
		}
		skill.UsageCount = row.UsageCount
		skills = append(skills, *skill)
	}
	return skills, nil
}

func (s *service) ListMetadata(ctx context.Context) ([]SkillMeta, error) {
	rows, err := s.store.ListByCategory(ctx, "", 200)
	if err != nil {
		return nil, fmt.Errorf("list metadata: %w", err)
	}
	metas := make([]SkillMeta, 0, len(rows))
	for _, r := range rows {
		metas = append(metas, skillMetaFromRow(r))
	}
	return metas, nil
}

func (s *service) Create(ctx context.Context, skill Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if skill.Category == "" {
		return fmt.Errorf("skill category is required")
	}
	if skill.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	skill.Exposure = normalizeSkillExposure(skill, sourceTierUser, "manual-user")
	skill.UserInvocable = skill.IsUserInvocable()
	skill.DisableModelInvocation = !skill.IsModelInvocable()

	// Generate ID and file path (always write to userDir)
	safeName := sanitizeFilename(skill.Name)
	skill.ID = skill.Category + "/" + safeName

	absUserDir, err := filepath.Abs(s.userDir)
	if err != nil {
		absUserDir = s.userDir
	}
	skill.FilePath = filepath.Join(absUserDir, skill.Category, safeName+".md")

	// Write .md file (SafeWrite creates category dir + validates boundary)
	if err := fileop.SafeWrite(skill.FilePath, []byte(skill.ToMarkdown()), fileop.WithBoundary(absUserDir), fileop.WithMkdir()); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	return s.reindexLocked(ctx)
}

func (s *service) Update(ctx context.Context, id string, skill Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.View(ctx, id)
	if err != nil {
		return fmt.Errorf("skill %s not found: %w", id, err)
	}
	if row, rowErr := s.store.Get(ctx, id); rowErr == nil {
		if existing.FilePath == "" {
			existing.FilePath = row.FilePath
		}
		if existing.CreatedAt.IsZero() {
			existing.CreatedAt = time.Unix(row.CreatedAt, 0)
		}
		if existing.Version == 0 {
			existing.Version = row.Version
		}
	}

	// If the existing skill is from bundledFS (no user file on disk), create a user override
	absUserDir, absErr := filepath.Abs(s.userDir)
	if absErr != nil {
		absUserDir = s.userDir
	}

	isBundled := false
	if s.bundledFS != nil && s.bundledSkillPath(id) != "" {
		isBundled = true
	}

	// Check if user override file already exists
	userPath := filepath.Join(absUserDir, id+".md")
	hasUserFile := false
	for _, p := range userSkillPaths(absUserDir, id) {
		if _, statErr := os.Stat(p); statErr == nil {
			hasUserFile = true
			break
		}
	}

	// If it's a bundled skill without a user override, create the override in userDir
	if isBundled && !hasUserFile {
		// Ensure category directory exists
		parts := strings.SplitN(id, "/", 2)
		if len(parts) == 2 {
			catDir := filepath.Join(absUserDir, parts[0])
			os.MkdirAll(catDir, 0o755)
		}
		existing.FilePath = userPath
	}

	// Core identity/content fields keep patch semantics for compatibility.
	if skill.Name != "" {
		existing.Name = skill.Name
	}
	if skill.Description != "" {
		existing.Description = skill.Description
	}
	if skill.Tags != nil {
		existing.Tags = skill.Tags
	}
	if skill.Instruction != "" {
		existing.Instruction = skill.Instruction
	}

	// Rich metadata is replacement-based so callers can clear strings and false booleans.
	existing.WhenToUse = skill.WhenToUse
	if skill.AllowedTools != nil {
		existing.AllowedTools = skill.AllowedTools
	}
	existing.Agent = skill.Agent
	existing.Context = skill.Context
	existing.Effort = skill.Effort
	existing.Model = skill.Model
	nextExposure := skill.Exposure
	if nextExposure == "" {
		if skill.UserInvocable || skill.DisableModelInvocation {
			nextExposure = SkillExposureFromLegacy(skill.UserInvocable, skill.DisableModelInvocation)
		} else {
			nextExposure = existing.Exposure
		}
	}
	if nextExposure == "" {
		nextExposure = normalizeSkillExposure(*existing, sourceTierUser, "manual-user")
	}
	existing.Exposure = nextExposure
	existing.UserInvocable = existing.IsUserInvocable()
	existing.DisableModelInvocation = !existing.IsModelInvocable()
	if skill.Paths != nil {
		existing.Paths = skill.Paths
	}
	if skill.Platforms != nil {
		existing.Platforms = skill.Platforms
	}
	existing.Source = skill.Source
	if existing.CreatedAt.IsZero() {
		existing.CreatedAt = time.Now()
	}
	existing.Version++
	existing.UpdatedAt = time.Now()

	// Write back to disk (with boundary validation against userDir)
	if err := fileop.SafeWrite(existing.FilePath, []byte(existing.ToMarkdown()), fileop.WithBoundary(absUserDir), fileop.WithMkdir()); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	return s.reindexLocked(ctx)
}

func (s *service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if it's a bundled system skill
	isBundled := false
	bundledPath := ""
	if s.bundledFS != nil {
		bundledPath = s.bundledSkillPath(id)
		isBundled = bundledPath != ""
	}

	// Check if there is a user override file
	absUserDir, absErr := filepath.Abs(s.userDir)
	if absErr != nil {
		absUserDir = s.userDir
	}
	userPaths := userSkillPaths(absUserDir, id)
	hasUserFile := false
	for _, p := range userPaths {
		if _, err := os.Stat(p); err == nil {
			hasUserFile = true
			break
		}
	}

	if isBundled && !hasUserFile {
		return fmt.Errorf("cannot delete built-in system skill %s", id)
	}

	if hasUserFile {
		for _, p := range userPaths {
			_ = os.Remove(p)
		}
	}

	if isBundled {
		return s.reindexLocked(ctx)
	}

	// Pure user skill: remove from active inventory.
	return s.reindexLocked(ctx)
}

func (s *service) RecordUsage(ctx context.Context, id string, success bool) error {
	return s.store.RecordUsage(ctx, id, success)
}

// SetLLMCaller sets the LLM caller for classifying non-standard skill files.
// Called after the LLM provider is initialized.
func (s *service) SetLLMCaller(caller LLMCaller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callLLM = caller
}

// ImportFromDir scans a directory for .md skill files, parses them, moves them
// to the correct category subdirectory, and indexes them.
// Files without proper YAML frontmatter are classified via LLM (if available).
// Successfully imported files are removed from the source directory.
// If dir is empty, defaults to userDir/_inbox.
func (s *service) ImportFromDir(ctx context.Context, dir string) (imported int, errs []error) {
	if dir == "" {
		dir = filepath.Join(s.userDir, "_inbox")
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(files) == 0 {
		return 0, nil
	}

	for _, f := range files {
		skill, err := ParseSkillFile(f)
		if err != nil {
			// Non-standard file: try LLM classification
			if s.callLLM == nil {
				errs = append(errs, fmt.Errorf("parse %s: %w (no LLM available for auto-classification)", filepath.Base(f), err))
				continue
			}

			raw, readErr := os.ReadFile(f)
			if readErr != nil {
				errs = append(errs, fmt.Errorf("read %s: %w", filepath.Base(f), readErr))
				continue
			}

			classified, llmErr := s.classifyWithLLM(ctx, filepath.Base(f), string(raw))
			if llmErr != nil {
				errs = append(errs, fmt.Errorf("classify %s: %w", filepath.Base(f), llmErr))
				continue
			}
			skill = classified
		}

		// Default category
		if skill.Category == "" {
			skill.Category = "workflow"
		}
		if skill.Name == "" {
			skill.Name = strings.TrimSuffix(filepath.Base(f), ".md")
		}

		// Create via normal path (writes to userDir category dir + indexes)
		if err := s.Create(ctx, *skill); err != nil {
			errs = append(errs, fmt.Errorf("import %s: %w", skill.Name, err))
			continue
		}

		// Remove original file from inbox
		if err := os.Remove(f); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", filepath.Base(f), err))
		}

		imported++
	}

	return imported, errs
}

const classifyPrompt = `You are a skill classifier. Given the filename and raw content of a document, extract structured metadata for a skill bank entry.

Filename: %s

Content:
%s

Respond with ONLY a JSON object (no markdown, no explanation):
{
  "name": "short_snake_case_name",
  "description": "一句话描述这个技能的用途",
  "category": "one of: memory, writing, research, tool_usage, domain, workflow, prompt, debug",
  "tags": ["tag1", "tag2"],
  "instruction": "cleaned up skill body text (When to use / How to apply / Constraints)"
}`

// classifyWithLLM uses an LLM to extract skill metadata from raw unstructured content.
func (s *service) classifyWithLLM(ctx context.Context, filename string, rawContent string) (*Skill, error) {
	// Truncate very long content to avoid excessive token usage
	content := rawContent
	if len(content) > 4000 {
		content = content[:4000] + "\n...(truncated)"
	}

	prompt := fmt.Sprintf(classifyPrompt, filename, content)
	output, err := s.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Extract JSON from response (handle markdown code blocks)
	output = strings.TrimSpace(output)
	if strings.HasPrefix(output, "```") {
		// Strip markdown code block
		lines := strings.Split(output, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		output = strings.Join(jsonLines, "\n")
	}

	var result struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Category    string   `json:"category"`
		Tags        []string `json:"tags"`
		Instruction string   `json:"instruction"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w\nraw output: %s", err, output)
	}

	skill := &Skill{
		Name:        result.Name,
		Description: result.Description,
		Category:    result.Category,
		Tags:        result.Tags,
		Version:     1,
		Author:      "agent",
		Instruction: result.Instruction,
	}

	// If LLM didn't return instruction, use original content as-is
	if skill.Instruction == "" {
		skill.Instruction = rawContent
	}

	return skill, nil
}

func (s *service) Reindex(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reindexLocked(ctx)
}

// loadSkillByID loads a skill from disk or bundled FS given its ID (e.g., "memory/insert").
// User directory takes priority over bundled FS.
func (s *service) loadSkillByID(id string) (*Skill, error) {
	if row, err := s.store.Get(context.Background(), id); err == nil {
		if srcs, srcErr := s.store.ListSkillSources(context.Background(), id); srcErr == nil && row.ResolvedSourceID != "" {
			for _, src := range srcs {
				if src.ID != row.ResolvedSourceID || src.FilePath == "" {
					continue
				}
				if src.SourceTier == sourceTierBundled {
					if data, readErr := fs.ReadFile(s.bundledFS, src.SourceKey); readErr == nil {
						return ParseSkillContent(id, data)
					}
					continue
				}
				if skill, parseErr := ParseSkillFile(src.FilePath); parseErr == nil {
					skill.ID = id
					return skill, nil
				}
			}
		}
	}

	// 1. Try user directory first (priority)
	absUserDir, absErr := filepath.Abs(s.userDir)
	if absErr != nil {
		absUserDir = s.userDir
	}
	for _, userPath := range userSkillPaths(absUserDir, id) {
		if skill, err := ParseSkillFile(userPath); err == nil {
			skill.ID = id
			return skill, nil
		}
	}

	// 2. Try bundled FS
	if s.bundledFS != nil {
		if bundledPath := s.bundledSkillPath(id); bundledPath != "" {
			data, err := fs.ReadFile(s.bundledFS, bundledPath)
			if err == nil {
				return ParseSkillContent(id, data)
			}
		}
	}

	// 3. Fallback: check DB for stored file_path (handles legacy paths)
	row, err := s.store.Get(context.Background(), id)
	if err == nil && row.FilePath != "" {
		skill, err := ParseSkillFile(row.FilePath)
		if err == nil {
			skill.ID = id
			return skill, nil
		}
	}

	return nil, fmt.Errorf("skill %s not found", id)
}

func (s *service) reindexLocked(ctx context.Context) error {
	candidates := make([]sourceCandidate, 0, 64)
	bundledCount := 0
	projectCount := 0
	managedCount := 0
	userCount := 0

	if s.bundledFS != nil {
		bundled, _ := scanBundledFSDetailed(s.bundledFS)
		bundledCount = len(bundled)
		for _, sk := range bundled {
			candidates = append(candidates, sourceCandidate{
				Skill:      sk.Skill,
				SourceTier: sourceTierBundled,
				SourceKind: "bundled",
				SourceKey:  filepath.ToSlash(sk.Relative),
				FilePath:   "",
			})
		}
	}

	project, _ := scanUserDirDetailed(s.projectDir())
	projectCount = len(project)
	for _, sk := range project {
		candidates = append(candidates, sourceCandidate{
			Skill:      sk.Skill,
			SourceTier: sourceTierProject,
			SourceKind: "project-local",
			SourceKey:  sk.Relative,
			FilePath:   sk.SourcePath,
			SourceMeta: "{}",
		})
	}

	managed, _ := scanManagedDir(s.managedDir())
	managedCount = len(managed)
	manifest, _ := s.readManagedManifest()
	managedByID := map[string]managedManifestEntry{}
	for _, entry := range manifest.Installs {
		managedByID[entry.SkillID] = entry
	}
	for _, sk := range managed {
		entry := managedByID[sk.Skill.ID]
		sourceKey := sk.Relative
		if entry.SourceKey != "" {
			sourceKey = entry.SourceKey
		}
		sourceKind := strings.TrimSpace(entry.SourceKind)
		if sourceKind == "" {
			sourceKind = "local"
		}
		candidates = append(candidates, sourceCandidate{
			Skill:      sk.Skill,
			SourceTier: sourceTierUserManaged,
			SourceKind: sourceKind,
			SourceKey:  sourceKey,
			FilePath:   sk.SourcePath,
			SourceMeta: sourceMetaJSONFromManagedEntry(entry),
		})
	}

	user, _ := scanUserDirDetailed(s.userDir)
	userCount = len(user)
	for _, sk := range user {
		candidates = append(candidates, sourceCandidate{
			Skill:      sk.Skill,
			SourceTier: sourceTierUser,
			SourceKind: "manual-user",
			SourceKey:  sk.Relative,
			FilePath:   sk.SourcePath,
			SourceMeta: "{}",
		})
	}

	now := time.Now().Unix()
	resolved := map[string]sourceCandidate{}
	resolvedSourceIDs := map[string]string{}
	sourceRows := make([]skillSourceRow, 0, len(candidates))
	for _, cand := range candidates {
		cand.Skill.Exposure = normalizeSkillExposure(cand.Skill, cand.SourceTier, cand.SourceKind)
		cand.Skill.UserInvocable = cand.Skill.IsUserInvocable()
		cand.Skill.DisableModelInvocation = !cand.Skill.IsModelInvocable()
		sid := sourceID(cand.Skill.ID, cand.SourceTier, cand.SourceKey)
		sourceRows = append(sourceRows, skillSourceRow{
			ID:          sid,
			SkillID:     cand.Skill.ID,
			SourceTier:  cand.SourceTier,
			SourceKind:  cand.SourceKind,
			SourceKey:   cand.SourceKey,
			DisplayName: cand.Skill.Name,
			FilePath:    cand.FilePath,
			MetaJSON:    cand.SourceMeta,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		existing, ok := resolved[cand.Skill.ID]
		if !ok || sourceTierPriority(cand.SourceTier) >= sourceTierPriority(existing.SourceTier) {
			resolved[cand.Skill.ID] = cand
			resolvedSourceIDs[cand.Skill.ID] = sid
		}
	}

	if err := s.store.ReplaceSkillSources(ctx, sourceRows); err != nil {
		return fmt.Errorf("replace skill_sources: %w", err)
	}

	activeIDs := make([]string, 0, len(resolved))
	var firstErr error
	for id, cand := range resolved {
		activeIDs = append(activeIDs, id)
		if err := s.store.Upsert(ctx, skillRow{
			ID:               cand.Skill.ID,
			Name:             cand.Skill.Name,
			Description:      cand.Skill.Description,
			Category:         cand.Skill.Category,
			Tags:             cand.Skill.Tags,
			MetaJSON:         cand.Skill.metadataJSON(),
			ResolvedSourceID: resolvedSourceIDs[cand.Skill.ID],
			Version:          cand.Skill.Version,
			Author:           cand.Skill.Author,
			FilePath:         cand.FilePath,
			CreatedAt:        now,
			UpdatedAt:        now,
			Content:          cand.Skill.searchDocument(),
		}); err != nil {
			log.Printf("[skillbank] index %s failed: %v", id, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("index %s: %w", id, err)
			}
		}
	}

	if err := s.store.DeleteNotIn(ctx, activeIDs); err != nil {
		log.Printf("[skillbank] cleanup stale skills: %v", err)
		if firstErr == nil {
			firstErr = fmt.Errorf("cleanup stale skills: %w", err)
		}
	}
	if firstErr != nil {
		return firstErr
	}

	log.Printf("[skillbank] indexed %d skills (%d bundled, %d project, %d managed, %d user)", len(resolved), bundledCount, projectCount, managedCount, userCount)
	return nil
}

func userSkillPaths(absUserDir string, id string) []string {
	return []string{
		filepath.Join(absUserDir, id, "SKILL.md"),
		filepath.Join(absUserDir, id+".md"),
	}
}

func (s *service) bundledSkillPath(id string) string {
	if s.bundledFS == nil {
		return ""
	}
	candidates := []string{
		filepath.ToSlash(filepath.Join(id, "SKILL.md")),
		id + ".md",
	}
	for _, p := range candidates {
		if _, err := fs.ReadFile(s.bundledFS, p); err == nil {
			return p
		}
	}
	return ""
}

func filterByTags(skills []Skill, tags []string) []Skill {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[strings.ToLower(t)] = true
	}

	var filtered []Skill
	for _, s := range skills {
		for _, st := range s.Tags {
			if tagSet[strings.ToLower(st)] {
				filtered = append(filtered, s)
				break
			}
		}
	}
	return filtered
}

func sanitizeFilename(name string) string {
	// Replace spaces and special chars with underscores
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, name)
	if name == "" {
		name = uuid.New().String()[:8]
	}
	return name
}

func skillMetaFromRow(r skillRow) SkillMeta {
	meta := Skill{
		WhenToUse:              "",
		AllowedTools:           nil,
		Agent:                  "",
		Context:                "",
		Effort:                 "",
		Model:                  "",
		Exposure:               "",
		UserInvocable:          false,
		DisableModelInvocation: false,
		Paths:                  nil,
		Platforms:              nil,
		Source:                 "",
	}
	applyMetadataJSON(&meta, r.MetaJSON)
	return SkillMeta{
		ID:                     r.ID,
		Name:                   r.Name,
		Description:            r.Description,
		Category:               r.Category,
		Tags:                   r.Tags,
		WhenToUse:              meta.WhenToUse,
		AllowedTools:           meta.AllowedTools,
		Agent:                  meta.Agent,
		Context:                meta.Context,
		Effort:                 meta.Effort,
		Model:                  meta.Model,
		Exposure:               meta.Exposure,
		UserInvocable:          meta.UserInvocable,
		DisableModelInvocation: meta.DisableModelInvocation,
		Paths:                  meta.Paths,
		Platforms:              meta.Platforms,
		Source:                 meta.Source,
		UsageCount:             r.UsageCount,
	}
}

func rowMatchesTags(row skillRow, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[strings.ToLower(t)] = true
	}
	for _, st := range row.Tags {
		if tagSet[strings.ToLower(st)] {
			return true
		}
	}
	return false
}

func normalizeSkillExposure(skill Skill, sourceTier string, sourceKind string) SkillExposure {
	if skill.Exposure == SkillExposureImplicit || skill.Exposure == SkillExposureExplicit || skill.Exposure == SkillExposureBoth || skill.Exposure == SkillExposureNone {
		return skill.Exposure
	}
	if skill.UserInvocable || skill.DisableModelInvocation {
		return SkillExposureFromLegacy(skill.UserInvocable, skill.DisableModelInvocation)
	}
	if strings.EqualFold(sourceKind, "manual-user") || strings.EqualFold(sourceKind, "project-local") {
		return SkillExposureBoth
	}
	if sourceTier == sourceTierUser || sourceTier == sourceTierProject {
		return SkillExposureBoth
	}
	if sourceTier == sourceTierBundled || sourceTier == sourceTierUserManaged || sourceKind == "bundled" || sourceKind == "policy" {
		return SkillExposureImplicit
	}
	return SkillExposureBoth
}
