package server

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mirovarga/filetap/db"
)

func TestParseFileQuery(t *testing.T) {
	t.Run("empty query", func(t *testing.T) {
		query, err := parseFileQuery(url.Values{})
		require.NoError(t, err)
		assert.Equal(t, db.Pagination{Skip: 0, Limit: db.DefaultLimit}, query.Paging())
	})

	t.Run("simple eq filter", func(t *testing.T) {
		query, err := parseFileQuery(url.Values{"ext": {"go"}})
		require.NoError(t, err)
		assert.NotNil(t, query)
	})

	t.Run("filter with operator", func(t *testing.T) {
		query, err := parseFileQuery(url.Values{"size[gt]": {"100"}})
		require.NoError(t, err)
		assert.NotNil(t, query)
	})

	t.Run("unknown field", func(t *testing.T) {
		_, err := parseFileQuery(url.Values{"unknown": {"val"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown parameter")
	})

	t.Run("unknown operator", func(t *testing.T) {
		_, err := parseFileQuery(url.Values{"ext[bad]": {"go"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operator")
	})

	t.Run("unknown filter field", func(t *testing.T) {
		_, err := parseFileQuery(url.Values{"bad[eq]": {"val"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown filter field")
	})
}

func TestParseSelectParams(t *testing.T) {
	t.Run("valid fields", func(t *testing.T) {
		fields, err := parseSelectParams(url.Values{"select": {"name,size,ext"}})
		require.NoError(t, err)
		assert.Len(t, fields, 3)
	})

	t.Run("empty", func(t *testing.T) {
		fields, err := parseSelectParams(url.Values{})
		require.NoError(t, err)
		assert.Nil(t, fields)
	})

	t.Run("unknown field", func(t *testing.T) {
		_, err := parseSelectParams(url.Values{"select": {"name,bad"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown select field")
	})

	t.Run("too many fields", func(t *testing.T) {
		fields := make([]string, maxSelectFields+1)
		for i := range fields {
			fields[i] = "name"
		}
		_, err := parseSelectParams(url.Values{"select": {strings.Join(fields, ",")}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many select fields")
	})
}

func TestParseOrderParams(t *testing.T) {
	t.Run("ascending", func(t *testing.T) {
		sorts, err := parseOrderParams(url.Values{"order": {"name"}})
		require.NoError(t, err)
		require.Len(t, sorts, 1)
		assert.False(t, sorts[0].Descending)
	})

	t.Run("descending", func(t *testing.T) {
		sorts, err := parseOrderParams(url.Values{"order": {"-size"}})
		require.NoError(t, err)
		require.Len(t, sorts, 1)
		assert.True(t, sorts[0].Descending)
	})

	t.Run("dirs not sortable", func(t *testing.T) {
		_, err := parseOrderParams(url.Values{"order": {"dirs"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})

	t.Run("unknown sort field", func(t *testing.T) {
		_, err := parseOrderParams(url.Values{"order": {"bad"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown sort field")
	})

	t.Run("too many sort fields", func(t *testing.T) {
		fields := make([]string, maxSortFields+1)
		for i := range fields {
			fields[i] = "name"
		}
		_, err := parseOrderParams(url.Values{"order": {strings.Join(fields, ",")}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many sort fields")
	})
}

func TestParsePaginationParams(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		page, err := parsePaginationParams(url.Values{})
		require.NoError(t, err)
		assert.Equal(t, 0, page.Skip)
		assert.Equal(t, db.DefaultLimit, page.Limit)
	})

	t.Run("custom values", func(t *testing.T) {
		page, err := parsePaginationParams(url.Values{"skip": {"10"}, "limit": {"50"}})
		require.NoError(t, err)
		assert.Equal(t, 10, page.Skip)
		assert.Equal(t, 50, page.Limit)
	})

	t.Run("limit capped at max", func(t *testing.T) {
		page, err := parsePaginationParams(url.Values{"limit": {"9999"}})
		require.NoError(t, err)
		assert.Equal(t, maxLimit, page.Limit)
	})

	t.Run("limit=0 returns zero", func(t *testing.T) {
		page, err := parsePaginationParams(url.Values{"limit": {"0"}})
		require.NoError(t, err)
		assert.Equal(t, 0, page.Limit)
	})

	t.Run("negative skip", func(t *testing.T) {
		_, err := parsePaginationParams(url.Values{"skip": {"-1"}})
		assert.Error(t, err)
	})

	t.Run("negative limit", func(t *testing.T) {
		_, err := parsePaginationParams(url.Values{"limit": {"-1"}})
		assert.Error(t, err)
	})

	t.Run("invalid skip", func(t *testing.T) {
		_, err := parsePaginationParams(url.Values{"skip": {"abc"}})
		assert.Error(t, err)
	})

	t.Run("invalid limit", func(t *testing.T) {
		_, err := parsePaginationParams(url.Values{"limit": {"abc"}})
		assert.Error(t, err)
	})
}

func TestValidateOperatorField(t *testing.T) {
	t.Run("all on non-array", func(t *testing.T) {
		err := validateOperatorField(db.OpAll, db.FieldName)
		assert.Error(t, err)
	})

	t.Run("all on array", func(t *testing.T) {
		err := validateOperatorField(db.OpAll, db.FieldDirs)
		assert.NoError(t, err)
	})

	t.Run("gt on array", func(t *testing.T) {
		err := validateOperatorField(db.OpGt, db.FieldDirs)
		assert.Error(t, err)
	})

	t.Run("match on array", func(t *testing.T) {
		err := validateOperatorField(db.OpMatch, db.FieldDirs)
		assert.Error(t, err)
	})

	t.Run("glob on array", func(t *testing.T) {
		err := validateOperatorField(db.OpGlob, db.FieldDirs)
		assert.Error(t, err)
	})

	t.Run("eq on non-array", func(t *testing.T) {
		err := validateOperatorField(db.OpEq, db.FieldName)
		assert.NoError(t, err)
	})

	t.Run("in on array", func(t *testing.T) {
		err := validateOperatorField(db.OpIn, db.FieldDirs)
		assert.NoError(t, err)
	})
}

func TestFilterInValuesLimit(t *testing.T) {
	values := make([]string, maxInValues+1)
	for i := range values {
		values[i] = "v"
	}
	_, err := parseFileQuery(url.Values{"ext[in]": {strings.Join(values, ",")}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many values")
}

func TestFilterCountLimit(t *testing.T) {
	params := url.Values{}
	for i := 0; i <= maxFilters; i++ {
		params.Set("name[eq]", "val")
	}
	// url.Values deduplicates, so we need different keys
	// Using different field[op] combinations
	params = url.Values{}
	fields := []string{"hash", "path", "name", "baseName", "ext", "size", "mime"}
	ops := []string{"eq", "ne", "match"}
	count := 0
	for _, f := range fields {
		for _, op := range ops {
			params.Set(f+"["+op+"]", "val")
			count++
			if count > maxFilters {
				break
			}
		}
		if count > maxFilters {
			break
		}
	}
	_, err := parseFileQuery(params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many filters")
}
