package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitHub(t *testing.T) {
	t.Run("valid repo", func(t *testing.T) {
		src, err := NewGitHub("owner/repo", "main", "", "")
		require.NoError(t, err)
		assert.Equal(t, "owner", src.owner)
		assert.Equal(t, "repo", src.repo)
		assert.Equal(t, "main", src.ref)
	})

	t.Run("empty owner", func(t *testing.T) {
		_, err := NewGitHub("/repo", "", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid repo format")
	})

	t.Run("empty repo", func(t *testing.T) {
		_, err := NewGitHub("owner/", "", "", "")
		assert.Error(t, err)
	})

	t.Run("no slash", func(t *testing.T) {
		_, err := NewGitHub("justrepo", "", "", "")
		assert.Error(t, err)
	})

	t.Run("with enterprise URL", func(t *testing.T) {
		src, err := NewGitHub("org/repo", "", "", "https://ghe.company.com")
		require.NoError(t, err)
		assert.Equal(t, "https://ghe.company.com", src.enterpriseURL)
	})
}

func TestGitHubSourceRawURL(t *testing.T) {
	t.Run("github.com URL encoding", func(t *testing.T) {
		src, err := NewGitHub("owner/repo", "main", "", "")
		require.NoError(t, err)

		file := &FileInfo{Path: "docs/my file.md"}
		rawURL, err := src.RawURL(context.Background(), file)
		require.NoError(t, err)
		assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/main/docs/my%20file.md", rawURL)
	})

	t.Run("github.com simple path", func(t *testing.T) {
		src, err := NewGitHub("owner/repo", "v1.0", "", "")
		require.NoError(t, err)

		file := &FileInfo{Path: "src/main.go"}
		rawURL, err := src.RawURL(context.Background(), file)
		require.NoError(t, err)
		assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0/src/main.go", rawURL)
	})

	t.Run("enterprise URL", func(t *testing.T) {
		src, err := NewGitHub("org/repo", "develop", "", "https://ghe.company.com")
		require.NoError(t, err)

		file := &FileInfo{Path: "README.md"}
		rawURL, err := src.RawURL(context.Background(), file)
		require.NoError(t, err)
		assert.Equal(t, "https://ghe.company.com/org/repo/raw/develop/README.md", rawURL)
	})

	t.Run("ref resolved after List persists to RawURL", func(t *testing.T) {
		src, err := NewGitHub("owner/repo", "", "", "")
		require.NoError(t, err)
		// Simulate what List does: resolve the default branch and store it
		src.ref = "main"

		file := &FileInfo{Path: "README.md"}
		rawURL, err := src.RawURL(context.Background(), file)
		require.NoError(t, err)
		assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/main/README.md", rawURL)
	})

	t.Run("special characters in path", func(t *testing.T) {
		src, err := NewGitHub("owner/repo", "main", "", "")
		require.NoError(t, err)

		file := &FileInfo{Path: "path/with spaces/and#hash.txt"}
		rawURL, err := src.RawURL(context.Background(), file)
		require.NoError(t, err)
		assert.Contains(t, rawURL, "with%20spaces")
		assert.Contains(t, rawURL, "and%23hash.txt")
	})
}

func TestEncodePathSegments(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"simple", "src/main.go", "src/main.go"},
		{"with space", "my docs/file.md", "my%20docs/file.md"},
		{"with hash", "dir/file#1.txt", "dir/file%231.txt"},
		{"with question", "dir/file?.txt", "dir/file%3F.txt"},
		{"nested spaces", "a b/c d/e f.txt", "a%20b/c%20d/e%20f.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, encodePathSegments(tt.path))
		})
	}
}
