package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mirovarga/filetap/db"
	"github.com/mirovarga/filetap/source"
)

type mockSource struct {
	rootDir string
}

func (m *mockSource) List(_ context.Context, _ *log.Logger, _ int) ([]*source.FileInfo, error) {
	return nil, nil
}

func (m *mockSource) RawURL(_ context.Context, file *source.FileInfo) (string, error) {
	if m.rootDir != "" {
		return "file://" + filepath.Join(m.rootDir, file.Path), nil
	}
	return "https://example.com/raw/" + file.Path, nil
}

func setupTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	database, err := db.NewInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	files := []*source.FileInfo{
		{
			Hash: source.Hash("src/main.go"), Path: "src/main.go",
			Dirs: []string{"src"}, Name: "main.go", BaseName: "main", Ext: "go",
			Size: 100, Mime: "text/x-go",
		},
		{
			Hash: source.Hash("README.md"), Path: "README.md",
			Dirs: []string{}, Name: "README.md", BaseName: "README", Ext: "md",
			Size: 200, Mime: "text/markdown",
		},
	}
	require.NoError(t, database.Insert(context.Background(), "default", files))

	logger := log.NewWithOptions(os.Stderr, log.Options{})
	src := &mockSource{}
	srv := New(3000, nil, database, src, logger)
	return srv, database
}

func doRequest(t *testing.T, srv *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)
	return rr
}

func TestHandleGetFiles(t *testing.T) {
	srv, _ := setupTestServer(t)

	t.Run("list all files", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files")
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		meta := resp["meta"].(map[string]any)
		assert.Equal(t, float64(2), meta["total"])
		data := resp["data"].([]any)
		assert.Len(t, data, 2)
	})

	t.Run("filter by ext", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files?ext[eq]=go")
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		meta := resp["meta"].(map[string]any)
		assert.Equal(t, float64(1), meta["total"])
	})

	t.Run("select fields", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files?select=name,size")
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		data := resp["data"].([]any)
		require.NotEmpty(t, data)
		first := data[0].(map[string]any)
		assert.Contains(t, first, "name")
		assert.Contains(t, first, "size")
		assert.Contains(t, first, "hash")
		assert.Contains(t, first, "links")
		assert.NotContains(t, first, "path")
	})

	t.Run("invalid filter", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files?bad[eq]=val")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("pagination", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files?limit=1&skip=0")
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		meta := resp["meta"].(map[string]any)
		assert.Equal(t, float64(2), meta["total"])
		assert.Equal(t, float64(1), meta["limit"])
		data := resp["data"].([]any)
		assert.Len(t, data, 1)
	})

	t.Run("order by size desc", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files?order=-size")
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		data := resp["data"].([]any)
		first := data[0].(map[string]any)
		assert.Equal(t, float64(200), first["size"])
	})

	t.Run("limit=0 returns empty", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files?limit=0")
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		meta := resp["meta"].(map[string]any)
		assert.Equal(t, float64(2), meta["total"])
		assert.Equal(t, float64(0), meta["limit"])
		data := resp["data"].([]any)
		assert.Len(t, data, 0)
	})
}

func TestHandleGetFile(t *testing.T) {
	srv, _ := setupTestServer(t)

	t.Run("found", func(t *testing.T) {
		hash := source.Hash("src/main.go")
		rr := doRequest(t, srv, "GET", "/api/files/"+hash)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		data := resp["data"].(map[string]any)
		assert.Equal(t, "src/main.go", data["path"])
		assert.Contains(t, data, "links")
	})

	t.Run("not found", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/files/nonexistenthash1")
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestHandleGetFileRaw(t *testing.T) {
	t.Run("redirect for non-file URL", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		hash := source.Hash("src/main.go")
		rr := doRequest(t, srv, "GET", "/api/files/"+hash+"/raw")
		assert.Equal(t, http.StatusTemporaryRedirect, rr.Code)
		assert.Contains(t, rr.Header().Get("Location"), "example.com/raw/src/main.go")
	})

	t.Run("serves local file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644))

		database, err := db.NewInMemory()
		require.NoError(t, err)
		t.Cleanup(func() { database.Close() })

		files := []*source.FileInfo{
			{
				Hash: source.Hash("test.txt"), Path: "test.txt",
				Dirs: []string{}, Name: "test.txt", BaseName: "test", Ext: "txt",
				Size: 5, Mime: "text/plain",
			},
		}
		require.NoError(t, database.Insert(context.Background(), "default", files))

		logger := log.NewWithOptions(os.Stderr, log.Options{})
		src := &mockSource{rootDir: dir}
		srv := New(3000, nil, database, src, logger)

		hash := source.Hash("test.txt")
		rr := doRequest(t, srv, "GET", "/api/files/"+hash+"/raw")
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "hello", rr.Body.String())
	})

	t.Run("returns 404 for directory path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))

		database, err := db.NewInMemory()
		require.NoError(t, err)
		t.Cleanup(func() { database.Close() })

		files := []*source.FileInfo{
			{
				Hash: source.Hash("subdir"), Path: "subdir",
				Dirs: []string{}, Name: "subdir", BaseName: "subdir", Ext: "",
				Size: 0, Mime: "application/octet-stream",
			},
		}
		require.NoError(t, database.Insert(context.Background(), "default", files))

		logger := log.NewWithOptions(os.Stderr, log.Options{})
		src := &mockSource{rootDir: dir}
		srv := New(3000, nil, database, src, logger)

		hash := source.Hash("subdir")
		rr := doRequest(t, srv, "GET", "/api/files/"+hash+"/raw")
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("not found hash", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		rr := doRequest(t, srv, "GET", "/api/files/nonexistent12345/raw")
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestPingEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)
	rr := doRequest(t, srv, "GET", "/api/ping")
	assert.Equal(t, http.StatusOK, rr.Code)
}
