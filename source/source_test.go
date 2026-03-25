package source

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHash(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		h1 := Hash("some/path.txt")
		h2 := Hash("some/path.txt")
		assert.Equal(t, h1, h2)
	})

	t.Run("length is 16 hex chars", func(t *testing.T) {
		h := Hash("any/file.go")
		assert.Len(t, h, hashTruncationLength)
	})

	t.Run("different paths produce different hashes", func(t *testing.T) {
		h1 := Hash("a.txt")
		h2 := Hash("b.txt")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("empty path", func(t *testing.T) {
		h := Hash("")
		assert.Len(t, h, hashTruncationLength)
	})
}

func TestSplitNameExt(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantBase string
		wantExt  string
	}{
		{"simple", "file.txt", "file", "txt"},
		{"no extension", "Makefile", "Makefile", ""},
		{"dotfile", ".gitignore", ".gitignore", ""},
		{"dotfile with ext", ".env.local", ".env", "local"},
		{"multiple dots", "archive.tar.gz", "archive.tar", "gz"},
		{"only dot", ".", ".", ""},
		{"trailing dot", "file.", "file", ""},
		{"double ext", "file.test.js", "file.test", "js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseName, ext := SplitNameExt(tt.fileName)
			assert.Equal(t, tt.wantBase, baseName)
			assert.Equal(t, tt.wantExt, ext)
		})
	}
}

func TestSplitDirs(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"file in root", "file.txt", []string{}},
		{"one level deep", "src/file.txt", []string{"src"}},
		{"two levels deep", "src/pkg/file.txt", []string{"src", "pkg"}},
		{"three levels deep", "a/b/c/file.txt", []string{"a", "b", "c"}},
		{"forward slashes", "docs/api/v2/spec.yaml", []string{"docs", "api", "v2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitDirs(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewFileInfo(t *testing.T) {
	info := newFileInfo("src/main.go", 1024, time.Time{})

	require.NotNil(t, info)
	assert.Equal(t, "src/main.go", info.Path)
	assert.Equal(t, "main.go", info.Name)
	assert.Equal(t, "main", info.BaseName)
	assert.Equal(t, "go", info.Ext)
	assert.Equal(t, int64(1024), info.Size)
	assert.Equal(t, []string{"src"}, info.Dirs)
	assert.Equal(t, Hash("src/main.go"), info.Hash)
	assert.NotEmpty(t, info.Mime)
	assert.True(t, info.ModifiedAt.IsZero())
}
