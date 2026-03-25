package source

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/google/go-github/v68/github"
)

// GitHubSource scans files from a GitHub repository via the API.
type GitHubSource struct {
	client        *github.Client
	owner         string
	repo          string
	ref           string
	enterpriseURL string
}

// NewGitHub creates a GitHubSource for the given owner/repo string.
func NewGitHub(repo, ref, token, enterpriseURL string) (*GitHubSource, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid repo format: %q (expected owner/repo)", repo)
	}
	owner, repoName := parts[0], parts[1]

	client := github.NewClient(nil)

	if enterpriseURL != "" {
		var err error
		client, err = client.WithEnterpriseURLs(enterpriseURL, enterpriseURL)
		if err != nil {
			return nil, fmt.Errorf("configuring GitHub Enterprise URL: %w", err)
		}
	}

	if token != "" {
		client = client.WithAuthToken(token)
	}

	return &GitHubSource{
		client:        client,
		owner:         owner,
		repo:          repoName,
		ref:           ref,
		enterpriseURL: enterpriseURL,
	}, nil
}

// List fetches the repository tree and returns file metadata up to maxDepth.
func (s *GitHubSource) List(ctx context.Context, logger *log.Logger, maxDepth int) ([]*FileInfo, error) {
	ref := s.ref
	if ref == "" {
		repository, _, err := s.client.Repositories.Get(ctx, s.owner, s.repo)
		if err != nil {
			return nil, fmt.Errorf("fetching default branch for %s/%s: %w", s.owner, s.repo, err)
		}
		ref = repository.GetDefaultBranch()
		s.ref = ref
	}

	logger.Info("fetching tree from GitHub", "repo", s.owner+"/"+s.repo, "ref", ref)
	start := time.Now()

	tree, _, err := s.client.Git.GetTree(ctx, s.owner, s.repo, ref, true)
	if err != nil {
		return nil, fmt.Errorf("fetching tree for %s/%s@%s: %w", s.owner, s.repo, ref, err)
	}

	if tree.GetTruncated() {
		return nil, fmt.Errorf("repository tree is too large (truncated by GitHub API); consider using --depth to limit scope")
	}

	files := make([]*FileInfo, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}

		entryPath := entry.GetPath()

		if maxDepth > 0 {
			depth := strings.Count(entryPath, "/") + 1
			if depth > maxDepth {
				continue
			}
		}

		files = append(files, newFileInfo(entryPath, int64(entry.GetSize()), time.Time{}))
	}

	logger.Info("tree fetched", "files", len(files), "duration", time.Since(start))
	return files, nil
}

// RawURL returns a URL to the raw file content on GitHub.
func (s *GitHubSource) RawURL(_ context.Context, file *FileInfo) (string, error) {
	encodedPath := encodePathSegments(file.Path)
	if s.enterpriseURL != "" {
		return fmt.Sprintf("%s/%s/%s/raw/%s/%s", s.enterpriseURL, s.owner, s.repo, s.ref, encodedPath), nil
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", s.owner, s.repo, s.ref, encodedPath), nil
}

func encodePathSegments(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}
