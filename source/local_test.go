package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocal(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		src, err := NewLocal(".")
		require.NoError(t, err)
		require.NotNil(t, src)
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := NewLocal("/nonexistent/path")
		assert.Error(t, err)
	})

	t.Run("file instead of directory", func(t *testing.T) {
		f, err := os.CreateTemp("", "filetap-test-*")
		require.NoError(t, err)
		defer os.Remove(f.Name())
		f.Close()

		_, err = NewLocal(f.Name())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestLocalSourceList(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "nested.go"), []byte("package main"), 0o644))

	src, err := NewLocal(dir)
	require.NoError(t, err)

	logger := log.NewWithOptions(os.Stderr, log.Options{})

	t.Run("depth 1 returns only root files", func(t *testing.T) {
		files, err := src.List(context.Background(), logger, 1)
		require.NoError(t, err)
		assert.Len(t, files, 1)
		assert.Equal(t, "root.txt", files[0].Name)
	})

	t.Run("depth 0 (unlimited) returns all files", func(t *testing.T) {
		files, err := src.List(context.Background(), logger, 0)
		require.NoError(t, err)
		assert.Len(t, files, 2)
	})

	t.Run("depth 2 returns all files", func(t *testing.T) {
		files, err := src.List(context.Background(), logger, 2)
		require.NoError(t, err)
		assert.Len(t, files, 2)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := src.List(ctx, logger, 0)
		assert.Error(t, err)
	})
}

func TestLocalSourceRawURL(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0o644))

	src, err := NewLocal(dir)
	require.NoError(t, err)

	file := &FileInfo{Path: "test.txt"}
	rawURL, err := src.RawURL(context.Background(), file)
	require.NoError(t, err)
	assert.Contains(t, rawURL, "file://")
	assert.Contains(t, rawURL, "test.txt")
}

func TestBuildPattern(t *testing.T) {
	tests := []struct {
		depth int
		want  string
	}{
		{0, "**"},
		{1, "*"},
		{2, "{*,*/*}"},
		{3, "{*,*/*,*/*/*}"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("depth_%d", tt.depth), func(t *testing.T) {
			assert.Equal(t, tt.want, buildPattern(tt.depth))
		})
	}
}
