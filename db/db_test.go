package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mirovarga/filetap/source"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := NewInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func sampleFiles() []*source.FileInfo {
	return []*source.FileInfo{
		{
			Hash: source.Hash("src/main.go"), Path: "src/main.go",
			Dirs: []string{"src"}, Name: "main.go", BaseName: "main", Ext: "go",
			Size: 100, ModifiedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Mime: "text/x-go",
		},
		{
			Hash: source.Hash("docs/README.md"), Path: "docs/README.md",
			Dirs: []string{"docs"}, Name: "README.md", BaseName: "README", Ext: "md",
			Size: 200, ModifiedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Mime: "text/markdown",
		},
		{
			Hash: source.Hash("Makefile"), Path: "Makefile",
			Dirs: []string{}, Name: "Makefile", BaseName: "Makefile", Ext: "",
			Size: 50, ModifiedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Mime: "application/octet-stream",
		},
	}
}

func TestNewInMemory(t *testing.T) {
	database, err := NewInMemory()
	require.NoError(t, err)
	require.NotNil(t, database)
	assert.NoError(t, database.Close())
}

func TestInsert(t *testing.T) {
	t.Run("insert and retrieve", func(t *testing.T) {
		database := setupTestDB(t)
		files := sampleFiles()

		err := database.Insert(context.Background(), "default", files)
		require.NoError(t, err)
	})

	t.Run("insert empty list", func(t *testing.T) {
		database := setupTestDB(t)
		err := database.Insert(context.Background(), "default", nil)
		require.NoError(t, err)
	})

	t.Run("duplicate hash is ignored", func(t *testing.T) {
		database := setupTestDB(t)
		files := []*source.FileInfo{
			{
				Hash: "samehash1234abcd", Path: "a.txt",
				Dirs: []string{}, Name: "a.txt", BaseName: "a", Ext: "txt",
				Size: 10, Mime: "text/plain",
			},
			{
				Hash: "samehash1234abcd", Path: "b.txt",
				Dirs: []string{}, Name: "b.txt", BaseName: "b", Ext: "txt",
				Size: 20, Mime: "text/plain",
			},
		}
		err := database.Insert(context.Background(), "default", files)
		require.NoError(t, err)

		file, found, err := database.FindByHash(context.Background(), "samehash1234abcd")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "a.txt", file.Path)
	})
}

func TestFindByHash(t *testing.T) {
	database := setupTestDB(t)
	files := sampleFiles()
	require.NoError(t, database.Insert(context.Background(), "default", files))

	t.Run("found", func(t *testing.T) {
		hash := source.Hash("src/main.go")
		file, found, err := database.FindByHash(context.Background(), hash)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "src/main.go", file.Path)
		assert.Equal(t, []string{"src"}, file.Dirs)
		assert.Equal(t, "main.go", file.Name)
		assert.Equal(t, int64(100), file.Size)
	})

	t.Run("not found", func(t *testing.T) {
		_, found, err := database.FindByHash(context.Background(), "nonexistenthash1")
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("file with no dirs", func(t *testing.T) {
		hash := source.Hash("Makefile")
		file, found, err := database.FindByHash(context.Background(), hash)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, []string{}, file.Dirs)
	})
}

func TestFind(t *testing.T) {
	database := setupTestDB(t)
	files := sampleFiles()
	require.NoError(t, database.Insert(context.Background(), "default", files))

	t.Run("no filters returns all", func(t *testing.T) {
		query := NewFileQuery()
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, results, 3)
	})

	t.Run("filter by ext eq", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpEq, "go")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, results, 1)
		assert.Equal(t, "main.go", results[0].Name)
	})

	t.Run("filter by size gt", func(t *testing.T) {
		query := NewFileQuery().Where(FieldSize, OpGt, "100")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "README.md", results[0].Name)
	})

	t.Run("filter by name match", func(t *testing.T) {
		query := NewFileQuery().Where(FieldName, OpMatch, "main")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "main.go", results[0].Name)
	})

	t.Run("filter by ext in", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpIn, "go,md")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, results, 2)
	})

	t.Run("filter by ext nin", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpNin, "go,md")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Makefile", results[0].Name)
	})

	t.Run("filter by dirs eq", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpEq, "src")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "main.go", results[0].Name)
	})

	t.Run("filter by dirs exists true", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpExists, "true")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, results, 2)
	})

	t.Run("filter by dirs exists false", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpExists, "false")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Makefile", results[0].Name)
	})

	t.Run("filter by name glob", func(t *testing.T) {
		query := NewFileQuery().Where(FieldName, OpGlob, "*.go")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "main.go", results[0].Name)
	})

	t.Run("filter by name nglob", func(t *testing.T) {
		query := NewFileQuery().Where(FieldName, OpNglob, "*.go")
		_, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
	})

	t.Run("filter by ext ne", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpNe, "go")
		_, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
	})

	t.Run("filter by ext exists true", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpExists, "true")
		_, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
	})

	t.Run("filter by ext exists false", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpExists, "false")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Makefile", results[0].Name)
	})

	t.Run("order by size asc", func(t *testing.T) {
		query := NewFileQuery().OrderBy(FieldSize, false)
		results, _, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, results, 3)
		assert.Equal(t, int64(50), results[0].Size)
		assert.Equal(t, int64(100), results[1].Size)
		assert.Equal(t, int64(200), results[2].Size)
	})

	t.Run("order by size desc", func(t *testing.T) {
		query := NewFileQuery().OrderBy(FieldSize, true)
		results, _, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, results, 3)
		assert.Equal(t, int64(200), results[0].Size)
		assert.Equal(t, int64(100), results[1].Size)
		assert.Equal(t, int64(50), results[2].Size)
	})

	t.Run("pagination skip", func(t *testing.T) {
		query := NewFileQuery().OrderBy(FieldSize, false).Page(1, 100)
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, results, 2)
	})

	t.Run("pagination limit", func(t *testing.T) {
		query := NewFileQuery().OrderBy(FieldSize, false).Page(0, 2)
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, results, 2)
	})

	t.Run("dirs are reconstructed correctly", func(t *testing.T) {
		query := NewFileQuery().Where(FieldPath, OpEq, "src/main.go")
		results, _, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, []string{"src"}, results[0].Dirs)
	})

	t.Run("filter by dirs ne", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpNe, "src")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, results, 2)
	})

	t.Run("filter by dirs in", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpIn, "src,docs")
		_, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
	})

	t.Run("filter by dirs nin", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpNin, "src,docs")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Makefile", results[0].Name)
	})

	t.Run("filter by dirs all", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpAll, "src")
		results, total, err := database.Find(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "main.go", results[0].Name)
	})
}
