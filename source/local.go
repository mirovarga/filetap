package source

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/charmbracelet/log"
	"github.com/samber/lo"
)

// LocalSource scans files from a local directory.
type LocalSource struct {
	rootDir string
}

// NewLocal creates a LocalSource rooted at the given directory.
func NewLocal(dir string) (*LocalSource, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving '%s': %w", dir, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("accessing '%s': %w", absPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", absPath)
	}

	return &LocalSource{rootDir: absPath}, nil
}

// List scans the root directory up to maxDepth and returns file metadata.
func (s *LocalSource) List(ctx context.Context, logger *log.Logger, maxDepth int) ([]*FileInfo, error) {
	fs := os.DirFS(s.rootDir)
	pattern := buildPattern(maxDepth)

	logger.Info("starting scan", "dir", s.rootDir, "depth", maxDepth)
	start := time.Now()

	filePaths, err := doublestar.Glob(fs, pattern, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
	if err != nil {
		return nil, fmt.Errorf("globbing '%s': %w", pattern, err)
	}

	files := make([]*FileInfo, 0, len(filePaths))
	for _, filePath := range filePaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		file, err := scanFile(s.rootDir, filePath)
		if err != nil {
			logger.Warn("skipping file", "path", filePath, "error", err)
			continue
		}

		files = append(files, file)
		logger.Debug("scanned file", "path", file.Path)
	}

	logger.Info("scan complete", "files", len(files), "duration", time.Since(start))
	return files, nil
}

// RawURL returns a file:// URL pointing to the file on disk.
func (s *LocalSource) RawURL(_ context.Context, file *FileInfo) (string, error) {
	absPath := filepath.Join(s.rootDir, file.Path)
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String(), nil
}

func scanFile(rootDir, relativePath string) (*FileInfo, error) {
	absPath := filepath.Join(rootDir, relativePath)

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("accessing '%s': %w", absPath, err)
	}

	return newFileInfo(relativePath, info.Size(), info.ModTime()), nil
}

func buildPattern(maxDepth int) string {
	if maxDepth == 0 {
		return "**"
	}
	if maxDepth == 1 {
		return "*"
	}
	parts := lo.Times(maxDepth, func(i int) string {
		return strings.Repeat("*/", i) + "*"
	})
	return "{" + strings.Join(parts, ",") + "}"
}
