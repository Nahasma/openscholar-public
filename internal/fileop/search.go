package fileop

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type SearchOptions struct {
	Pattern         string
	Path            string
	Glob            string
	Type            string
	Include         string
	OutputMode      string
	HeadLimit       int
	Offset          int
	ContextBefore   int
	ContextAfter    int
	ContextCombined int
	IgnoreCase      bool
	Multiline       bool
	Timeout         time.Duration
	MaxBufferBytes  int
}

type SearchResult struct {
	Lines     []string
	Paths     []string
	Count     int
	Truncated bool
}

var errSearchLimitReached = errors.New("search result limit reached")

func RunSearch(opts SearchOptions) (SearchResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	if opts.MaxBufferBytes <= 0 {
		opts.MaxBufferBytes = 20 * 1024 * 1024
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	if _, err := exec.LookPath("rg"); err == nil {
		return runRG(ctx, opts)
	}
	return runGoSearchContext(ctx, opts)
}

func runRG(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	args := []string{"--line-number", "--with-filename", "--color", "never"}
	if opts.IgnoreCase {
		args = append(args, "-i")
	}
	if opts.Multiline {
		args = append(args, "-U")
	}
	if opts.ContextCombined > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", opts.ContextCombined))
	} else {
		if opts.ContextBefore > 0 {
			args = append(args, "-B", fmt.Sprintf("%d", opts.ContextBefore))
		}
		if opts.ContextAfter > 0 {
			args = append(args, "-A", fmt.Sprintf("%d", opts.ContextAfter))
		}
	}
	if g := firstNonEmpty(opts.Glob, opts.Include); g != "" {
		args = append(args, "-g", g)
	}
	if opts.Type != "" {
		args = append(args, "-t", opts.Type)
	}
	switch opts.OutputMode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	}
	args = append(args, opts.Pattern, opts.Path)
	cmd := exec.CommandContext(ctx, "rg", args...)
	stdout := &limitedBuffer{limit: opts.MaxBufferBytes}
	cmd.Stdout = stdout
	cmd.Stderr = &bytes.Buffer{}
	err := cmd.Run()
	if ctx.Err() != nil {
		return SearchResult{}, ctx.Err()
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return SearchResult{}, nil
		}
		return SearchResult{}, err
	}
	res := parseSearchOutput(stdout.String(), opts)
	if stdout.truncated {
		res.Truncated = true
	}
	return res, nil
}

func runGoSearch(opts SearchOptions) (SearchResult, error) {
	ctx := context.Background()
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	return runGoSearchContext(ctx, opts)
}

func runGoSearchContext(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	reFlags := ""
	if opts.IgnoreCase {
		reFlags = "(?i)"
	}
	re, err := regexp.Compile(reFlags + opts.Pattern)
	if err != nil {
		return SearchResult{}, err
	}
	var lines []string
	var paths []string
	count := 0
	counts := make(map[string]int)
	maxCollected := opts.Offset + opts.HeadLimit
	if maxCollected <= 0 {
		maxCollected = 100
	}
	collectedContent := 0
	truncated := false
	err = filepath.Walk(opts.Path, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if (opts.OutputMode == "content" || opts.OutputMode == "") && collectedContent >= maxCollected {
			return errSearchLimitReached
		}
		if opts.OutputMode == "count" && len(paths) >= maxCollected {
			return errSearchLimitReached
		}
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if g := firstNonEmpty(opts.Glob, opts.Include); g != "" {
			ok, _ := filepath.Match(g, filepath.Base(path))
			if !ok {
				return nil
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		ln := 0
		matched := false
		for s.Scan() {
			ln++
			line := s.Text()
			if re.MatchString(line) {
				matched = true
				count++
				counts[path]++
				if opts.OutputMode == "content" || opts.OutputMode == "" {
					lines = append(lines, fmt.Sprintf("%s:%d:%s", path, ln, strings.TrimSpace(line)))
					collectedContent++
					if collectedContent >= maxCollected {
						return errSearchLimitReached
					}
				}
			}
		}
		if matched {
			paths = append(paths, path)
			if opts.OutputMode == "files_with_matches" && len(paths) >= maxCollected {
				return errSearchLimitReached
			}
		}
		return nil
	})
	if errors.Is(err, errSearchLimitReached) {
		truncated = true
	} else if err != nil {
		return SearchResult{}, err
	}
	if opts.OutputMode == "count" {
		for _, p := range paths {
			lines = append(lines, fmt.Sprintf("%s:%d", p, counts[p]))
		}
		sort.Strings(lines)
	}
	res := SearchResult{Lines: lines, Paths: paths, Count: count, Truncated: truncated}
	return paginateSearch(res, opts), nil
}

func parseSearchOutput(out string, opts SearchOptions) SearchResult {
	res := SearchResult{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch opts.OutputMode {
		case "files_with_matches":
			res.Paths = append(res.Paths, line)
		case "count":
			res.Lines = append(res.Lines, line)
		default:
			res.Lines = append(res.Lines, line)
		}
	}
	if opts.OutputMode == "count" {
		res.Count = len(res.Lines)
	}
	return paginateSearch(res, opts)
}

func paginateSearch(in SearchResult, opts SearchOptions) SearchResult {
	if len(in.Paths) > 0 {
		sort.Strings(in.Paths)
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.HeadLimit <= 0 {
		opts.HeadLimit = 100
	}
	in.Paths = sliceWindow(in.Paths, opts.Offset, opts.HeadLimit)
	in.Lines = sliceWindow(in.Lines, opts.Offset, opts.HeadLimit)
	return in
}

func sliceWindow[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		_, _ = b.Buffer.Write(p)
		return len(p), nil
	}
	remaining := b.limit - b.Buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
