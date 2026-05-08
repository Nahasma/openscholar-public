package picker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileEntry represents a file or directory in the index.
type FileEntry struct {
	RelPath string // relative to root (e.g., "papers/attention.pdf")
	IsDir   bool
	Depth   int
}

// IndexConfig controls which files are indexed.
type IndexConfig struct {
	MaxDepth    int // 0 means unlimited
	MaxEntries  int
	Timeout     time.Duration
	CacheTTL    time.Duration
	PreferGit   bool
	IgnoreDirs  map[string]bool
	IgnoreFiles []string
}

// DefaultIndexConfig returns sensible defaults for academic projects.
func DefaultIndexConfig() IndexConfig {
	return IndexConfig{
		MaxDepth:   32,
		MaxEntries: 20000,
		Timeout:    2 * time.Second,
		CacheTTL:   2 * time.Second,
		PreferGit:  true,
		IgnoreDirs: map[string]bool{
			".git":         true,
			".openscholar": true,
			"node_modules": true,
			"__pycache__":  true,
			".venv":        true,
			"venv":         true,
			"vendor":       true,
			".idea":        true,
			".vscode":      true,
			".DS_Store":    true,
		},
		IgnoreFiles: []string{".gitignore", ".ignore", ".rgignore"},
	}
}

// IndexProvider builds file indexes for a workspace.
type IndexProvider interface {
	Index(ctx context.Context, root string, cfg IndexConfig) ([]FileEntry, error)
}

type defaultIndexProvider struct{}

// NewIndexProvider returns the default git-aware index provider.
func NewIndexProvider() IndexProvider {
	return defaultIndexProvider{}
}

// IndexFiles walks root and returns all files and directories up to MaxDepth.
// It keeps the legacy API, but now uses a git-aware cached provider.
func IndexFiles(root string, cfg IndexConfig) []FileEntry {
	entries, _ := IndexFilesContext(context.Background(), root, cfg)
	return entries
}

// IndexFilesContext indexes root with cancellation support.
func IndexFilesContext(ctx context.Context, root string, cfg IndexConfig) ([]FileEntry, error) {
	return NewIndexProvider().Index(ctx, root, cfg)
}

// IndexFilesShallow returns a fast best-effort index suitable for immediate UI
// feedback while a full IndexFiles call refreshes in the background.
func IndexFilesShallow(root string, cfg IndexConfig) []FileEntry {
	cfg = normalizeIndexConfig(cfg)
	if entries, ok := cachedIndex(root, cfg); ok {
		return entries
	}
	cfg.PreferGit = false
	if cfg.MaxDepth == 0 || cfg.MaxDepth > 2 {
		cfg.MaxDepth = 2
	}
	if cfg.MaxEntries == 0 || cfg.MaxEntries > 500 {
		cfg.MaxEntries = 500
	}
	cfg.Timeout = 75 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	entries, _ := walkIndex(ctx, root, cfg)
	return entries
}

func (defaultIndexProvider) Index(ctx context.Context, root string, cfg IndexConfig) ([]FileEntry, error) {
	cfg = normalizeIndexConfig(cfg)
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	root = filepath.Clean(root)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = absRoot

	if entries, ok := cachedIndex(root, cfg); ok {
		return entries, nil
	}

	var entries []FileEntry
	if cfg.PreferGit {
		if gitEntries, err := gitIndex(ctx, root, cfg); err == nil {
			entries = gitEntries
		}
	}
	if entries == nil {
		entries, err = walkIndex(ctx, root, cfg)
		if err != nil && !errors.Is(err, errIndexLimitReached) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			return entries, err
		}
	}
	sortEntries(entries)
	storeIndex(root, cfg, entries)
	return entries, nil
}

func normalizeIndexConfig(cfg IndexConfig) IndexConfig {
	defaults := DefaultIndexConfig()
	if cfg.MaxDepth < 0 {
		cfg.MaxDepth = defaults.MaxDepth
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = defaults.MaxEntries
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaults.CacheTTL
	}
	if cfg.IgnoreDirs == nil {
		cfg.IgnoreDirs = defaults.IgnoreDirs
	}
	if cfg.IgnoreFiles == nil {
		cfg.IgnoreFiles = defaults.IgnoreFiles
	}
	return cfg
}

func gitIndex(ctx context.Context, root string, cfg IndexConfig) ([]FileEntry, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-co", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]FileEntry, 0, min(len(lines)*2, cfg.MaxEntries))
	seen := make(map[string]struct{}, len(lines)*2)
	add := func(rel string, isDir bool) bool {
		rel = cleanRelPath(rel)
		if rel == "" || rel == "." {
			return true
		}
		depth := relDepth(rel)
		if cfg.MaxDepth > 0 && depth >= cfg.MaxDepth {
			return true
		}
		key := rel
		if isDir {
			key += "/"
		}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
		entries = append(entries, FileEntry{RelPath: rel, IsDir: isDir, Depth: depth})
		return len(entries) < cfg.MaxEntries
	}
	for _, line := range lines {
		rel := cleanRelPath(line)
		if rel == "" {
			continue
		}
		parts := strings.Split(rel, "/")
		for i := 1; i < len(parts); i++ {
			if !add(strings.Join(parts[:i], "/"), true) {
				return entries, errIndexLimitReached
			}
		}
		if !add(rel, false) {
			return entries, errIndexLimitReached
		}
	}
	sortEntries(entries)
	return entries, nil
}

var errIndexLimitReached = errors.New("picker index limit reached")

func walkIndex(ctx context.Context, root string, cfg IndexConfig) ([]FileEntry, error) {
	root = filepath.Clean(root)
	matcher := loadIgnoreMatcher(root, cfg)
	entries := make([]FileEntry, 0, min(1024, cfg.MaxEntries))

	err := filepath.WalkDir(root, func(filePath string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(root, filePath)
		if err != nil || rel == "." {
			return nil
		}
		rel = cleanRelPath(rel)

		base := path.Base(rel)
		if d.IsDir() && (cfg.IgnoreDirs[base] || matcher.ignored(rel, true)) {
			return filepath.SkipDir
		}
		if !d.IsDir() && matcher.ignored(rel, false) {
			return nil
		}

		depth := relDepth(rel)
		if cfg.MaxDepth > 0 && depth >= cfg.MaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		entries = append(entries, FileEntry{
			RelPath: rel,
			IsDir:   d.IsDir(),
			Depth:   depth,
		})
		if len(entries) >= cfg.MaxEntries {
			return errIndexLimitReached
		}
		return nil
	})
	sortEntries(entries)
	return entries, err
}

func sortEntries(entries []FileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Depth != entries[j].Depth {
			return entries[i].Depth < entries[j].Depth
		}
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].RelPath < entries[j].RelPath
	})
}

func cleanRelPath(rel string) string {
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if rel == "." {
		return ""
	}
	return strings.TrimPrefix(rel, "./")
}

func relDepth(rel string) int {
	if rel == "" {
		return 0
	}
	return strings.Count(rel, "/")
}

type ignoreMatcher struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	raw     string
	dirOnly bool
	hasPath bool
}

func loadIgnoreMatcher(root string, cfg IndexConfig) ignoreMatcher {
	var patterns []ignorePattern
	for _, name := range cfg.IgnoreFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
				continue
			}
			line = strings.TrimPrefix(line, "/")
			dirOnly := strings.HasSuffix(line, "/")
			line = strings.TrimSuffix(line, "/")
			if line == "" {
				continue
			}
			patterns = append(patterns, ignorePattern{
				raw:     filepath.ToSlash(line),
				dirOnly: dirOnly,
				hasPath: strings.Contains(line, "/"),
			})
		}
	}
	return ignoreMatcher{patterns: patterns}
}

func (m ignoreMatcher) ignored(rel string, isDir bool) bool {
	rel = cleanRelPath(rel)
	base := path.Base(rel)
	for _, p := range m.patterns {
		if p.dirOnly && !isDir && !strings.HasPrefix(rel, p.raw+"/") {
			continue
		}
		if p.hasPath {
			if rel == p.raw || strings.HasPrefix(rel, p.raw+"/") {
				return true
			}
			if ok, _ := path.Match(p.raw, rel); ok {
				return true
			}
			continue
		}
		if base == p.raw {
			return true
		}
		if ok, _ := path.Match(p.raw, base); ok {
			return true
		}
	}
	return false
}

type indexCacheEntry struct {
	key     string
	entries []FileEntry
	expires time.Time
}

var indexCache = struct {
	sync.Mutex
	items map[string]indexCacheEntry
}{items: make(map[string]indexCacheEntry)}

func cachedIndex(root string, cfg IndexConfig) ([]FileEntry, bool) {
	key := cacheKey(root, cfg)
	indexCache.Lock()
	defer indexCache.Unlock()
	entry, ok := indexCache.items[root]
	if !ok || entry.key != key || time.Now().After(entry.expires) {
		return nil, false
	}
	return cloneEntries(entry.entries), true
}

func storeIndex(root string, cfg IndexConfig, entries []FileEntry) {
	key := cacheKey(root, cfg)
	indexCache.Lock()
	defer indexCache.Unlock()
	indexCache.items[root] = indexCacheEntry{
		key:     key,
		entries: cloneEntries(entries),
		expires: time.Now().Add(cfg.CacheTTL),
	}
}

func cacheKey(root string, cfg IndexConfig) string {
	root = filepath.Clean(root)
	floor := time.Now().Unix() / int64(max(1, int(cfg.CacheTTL/time.Second)))
	return fmt.Sprintf("%s|depth=%d|limit=%d|git=%s|ignore=%s|root=%s|floor=%d",
		root,
		cfg.MaxDepth,
		cfg.MaxEntries,
		fileModFingerprint(filepath.Join(root, ".git", "index")),
		ignoreFingerprint(root, cfg.IgnoreFiles),
		fileModFingerprint(root),
		floor,
	)
}

func ignoreFingerprint(root string, names []string) string {
	var parts []string
	for _, name := range names {
		parts = append(parts, name+"="+fileModFingerprint(filepath.Join(root, name)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func fileModFingerprint(filePath string) string {
	info, err := os.Stat(filePath)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

func entriesSignature(entries []FileEntry) string {
	h := sha1.New()
	for _, entry := range entries {
		_, _ = h.Write([]byte(entry.RelPath))
		if entry.IsDir {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cloneEntries(entries []FileEntry) []FileEntry {
	if entries == nil {
		return nil
	}
	out := make([]FileEntry, len(entries))
	copy(out, entries)
	return out
}

// ClearIndexCacheForTest clears process-local picker index cache.
func ClearIndexCacheForTest() {
	indexCache.Lock()
	defer indexCache.Unlock()
	indexCache.items = make(map[string]indexCacheEntry)
}

// IndexSignatureForTest exposes a stable signature for cache-focused tests.
func IndexSignatureForTest(entries []FileEntry) string {
	return entriesSignature(entries)
}
