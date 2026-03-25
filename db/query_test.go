package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQueryField(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"hash", true},
		{"path", true},
		{"dirs", true},
		{"name", true},
		{"baseName", true},
		{"ext", true},
		{"size", true},
		{"modifiedAt", true},
		{"mime", true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := NewQueryField(tt.name)
			assert.Equal(t, tt.valid, ok)
			if ok {
				assert.Equal(t, tt.name, field.Column())
			}
		})
	}
}

func TestNewOperator(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"eq", true},
		{"ne", true},
		{"gt", true},
		{"gte", true},
		{"lt", true},
		{"lte", true},
		{"in", true},
		{"nin", true},
		{"all", true},
		{"exists", true},
		{"match", true},
		{"glob", true},
		{"nglob", true},
		{"bad", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := NewOperator(tt.name)
			assert.Equal(t, tt.valid, ok)
		})
	}
}

func TestOperatorString(t *testing.T) {
	assert.Equal(t, "eq", OpEq.String())
	assert.Equal(t, "glob", OpGlob.String())
	assert.Equal(t, "", Operator(999).String())
}

func TestFileQueryBuild(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		query := NewFileQuery()
		sql, args, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "SELECT hash, source_id, path, name, baseName, ext, size, modifiedAt, mime FROM files")
		assert.Contains(t, sql, "LIMIT ? OFFSET ?")
		assert.Equal(t, []any{DefaultLimit, 0}, args)
	})

	t.Run("with eq filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpEq, "go")
		sql, args, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "WHERE ext = ?")
		assert.Contains(t, args, "go")
	})

	t.Run("with ne filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpNe, "go")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "WHERE ext != ?")
	})

	t.Run("with gt filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldSize, OpGt, "100")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "WHERE size > ?")
	})

	t.Run("with match filter escapes wildcards", func(t *testing.T) {
		query := NewFileQuery().Where(FieldName, OpMatch, "test%file")
		sql, args, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "LIKE ? ESCAPE")
		assert.Equal(t, "%test\\%file%", args[0])
	})

	t.Run("with glob filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldName, OpGlob, "*.go")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "GLOB ?")
	})

	t.Run("with nglob filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldName, OpNglob, "*.go")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT GLOB ?")
	})

	t.Run("with in filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpIn, "go,md")
		sql, args, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "IN (?, ?)")
		assert.Contains(t, args, "go")
		assert.Contains(t, args, "md")
	})

	t.Run("with nin filter", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpNin, "go,md")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT IN (?, ?)")
	})

	t.Run("with order by", func(t *testing.T) {
		query := NewFileQuery().OrderBy(FieldSize, true)
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY size DESC")
	})

	t.Run("with multiple order by", func(t *testing.T) {
		query := NewFileQuery().OrderBy(FieldSize, true).OrderBy(FieldName, false)
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY size DESC, name ASC")
	})

	t.Run("with pagination", func(t *testing.T) {
		query := NewFileQuery().Page(10, 50)
		sql, args, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "LIMIT ? OFFSET ?")
		assert.Equal(t, 50, args[len(args)-2])
		assert.Equal(t, 10, args[len(args)-1])
	})

	t.Run("dirs eq uses EXISTS subquery", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpEq, "src")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "file_dirs")
	})

	t.Run("dirs ne uses NOT EXISTS", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpNe, "src")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT EXISTS")
	})

	t.Run("dirs all uses multiple EXISTS", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpAll, "src,pkg")
		sql, args, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, args, "src")
		assert.Contains(t, args, "pkg")
	})

	t.Run("dirs exists true", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpExists, "true")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "EXISTS")
	})

	t.Run("dirs exists false", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpExists, "false")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT EXISTS")
	})

	t.Run("non-array exists true", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpExists, "true")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "ext != ''")
	})

	t.Run("non-array exists false", func(t *testing.T) {
		query := NewFileQuery().Where(FieldExt, OpExists, "false")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "ext = ''")
	})

	t.Run("dirs in", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpIn, "src,docs")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "EXISTS")
		assert.Contains(t, sql, "IN (?, ?)")
	})

	t.Run("dirs nin", func(t *testing.T) {
		query := NewFileQuery().Where(FieldDirs, OpNin, "src,docs")
		sql, _, err := query.Build()
		require.NoError(t, err)
		assert.Contains(t, sql, "NOT EXISTS")
		assert.Contains(t, sql, "IN (?, ?)")
	})
}

func TestFileQueryBuildCount(t *testing.T) {
	query := NewFileQuery().Where(FieldExt, OpEq, "go")
	sql, args, err := query.BuildCount()
	require.NoError(t, err)
	assert.Contains(t, sql, "SELECT COUNT(*) FROM files")
	assert.Contains(t, sql, "WHERE ext = ?")
	assert.Contains(t, args, "go")
}

func TestFileQueryBuildWithDirs(t *testing.T) {
	query := NewFileQuery().OrderBy(FieldSize, true)
	sql, _, err := query.BuildWithDirs()
	require.NoError(t, err)
	assert.Contains(t, sql, "WITH paginated_files AS")
	assert.Contains(t, sql, "LEFT JOIN file_dirs fd")
	assert.Contains(t, sql, "pf.size DESC")
	assert.Contains(t, sql, "fd.position")
}

func TestFileQueryFieldSelection(t *testing.T) {
	query := NewFileQuery()
	assert.False(t, query.HasFieldSelection())
	assert.Empty(t, query.Fields())

	query.Select(FieldName, FieldSize)
	assert.True(t, query.HasFieldSelection())
	assert.Equal(t, []QueryField{FieldName, FieldSize}, query.Fields())
}

func TestFileQueryPaging(t *testing.T) {
	query := NewFileQuery()
	assert.Equal(t, Pagination{Skip: 0, Limit: DefaultLimit}, query.Paging())

	query.Page(5, 25)
	assert.Equal(t, Pagination{Skip: 5, Limit: 25}, query.Paging())
}
