package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubTarget(t *testing.T) {
	t.Run("simple owner/repo", func(t *testing.T) {
		repo, enterprise, err := parseGitHubTarget("owner/repo")
		require.NoError(t, err)
		assert.Equal(t, "owner/repo", repo)
		assert.Empty(t, enterprise)
	})

	t.Run("github.com URL", func(t *testing.T) {
		repo, enterprise, err := parseGitHubTarget("https://github.com/owner/repo")
		require.NoError(t, err)
		assert.Equal(t, "owner/repo", repo)
		assert.Empty(t, enterprise)
	})

	t.Run("www.github.com URL", func(t *testing.T) {
		repo, enterprise, err := parseGitHubTarget("https://www.github.com/owner/repo")
		require.NoError(t, err)
		assert.Equal(t, "owner/repo", repo)
		assert.Empty(t, enterprise)
	})

	t.Run("enterprise URL", func(t *testing.T) {
		repo, enterprise, err := parseGitHubTarget("https://ghe.company.com/org/repo")
		require.NoError(t, err)
		assert.Equal(t, "org/repo", repo)
		assert.Equal(t, "https://ghe.company.com", enterprise)
	})

	t.Run("enterprise URL with extra path", func(t *testing.T) {
		repo, enterprise, err := parseGitHubTarget("https://ghe.company.com/org/repo/extra")
		require.NoError(t, err)
		assert.Equal(t, "org/repo", repo)
		assert.Equal(t, "https://ghe.company.com", enterprise)
	})

	t.Run("missing owner", func(t *testing.T) {
		_, _, err := parseGitHubTarget("https://github.com//repo")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing owner/repo")
	})

	t.Run("missing repo", func(t *testing.T) {
		_, _, err := parseGitHubTarget("https://github.com/owner/")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing owner/repo")
	})

	t.Run("only host", func(t *testing.T) {
		_, _, err := parseGitHubTarget("https://github.com/")
		assert.Error(t, err)
	})
}

func TestValidateDepth(t *testing.T) {
	t.Run("zero is valid", func(t *testing.T) {
		assert.NoError(t, validateDepth(0))
	})

	t.Run("positive is valid", func(t *testing.T) {
		assert.NoError(t, validateDepth(5))
	})

	t.Run("negative is invalid", func(t *testing.T) {
		assert.Error(t, validateDepth(-1))
	})
}
