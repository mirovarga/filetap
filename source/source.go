package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/samber/lo"
)

const hashTruncationLength = 16

// FileInfo holds metadata for a single indexed file.
type FileInfo struct {
	Hash       string    `json:"hash"`
	Path       string    `json:"path"`
	Dirs       []string  `json:"dirs"`
	Name       string    `json:"name"`
	BaseName   string    `json:"baseName"`
	Ext        string    `json:"ext"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt,omitzero"`
	Mime       string    `json:"mime"`
}

// Source lists files and provides raw content URLs for a file origin.
type Source interface {
	List(ctx context.Context, logger *log.Logger, maxDepth int) ([]*FileInfo, error)
	RawURL(ctx context.Context, file *FileInfo) (string, error)
}

func newFileInfo(filePath string, size int64, modifiedAt time.Time) *FileInfo {
	name := path.Base(filePath)
	baseName, ext := SplitNameExt(name)
	return &FileInfo{
		Hash:       Hash(filePath),
		Path:       filePath,
		Dirs:       SplitDirs(filePath),
		Name:       name,
		BaseName:   baseName,
		Ext:        ext,
		Size:       size,
		ModifiedAt: modifiedAt,
		Mime:       detectMIME(filePath),
	}
}

// SplitNameExt splits a file name into its base name and extension without the dot.
func SplitNameExt(fileName string) (baseName, ext string) {
	nameForExt := fileName
	if strings.HasPrefix(fileName, ".") {
		nameForExt = fileName[1:]
	}

	rawExt := path.Ext(nameForExt)

	baseName = strings.TrimSuffix(fileName, rawExt)
	ext = strings.TrimPrefix(rawExt, ".")
	return
}

// Hash returns a truncated SHA-256 hex digest of the given path.
func Hash(fullPath string) string {
	sum := sha256.Sum256([]byte(fullPath))
	return hex.EncodeToString(sum[:])[:hashTruncationLength]
}

// SplitDirs returns the individual directory components of a relative path.
func SplitDirs(relativePath string) []string {
	dir := path.Dir(relativePath)
	parts := strings.Split(dir, "/")
	return lo.Filter(parts, func(part string, _ int) bool {
		return part != "" && part != "."
	})
}
