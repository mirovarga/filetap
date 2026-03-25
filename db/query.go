package db

import (
	"fmt"
	"strings"
)

// DefaultLimit is the default number of results per query.
const DefaultLimit = 100

const dirSubquery = "SELECT 1 FROM file_dirs fd WHERE fd.file_hash = files.hash AND fd.source_id = files.source_id"

// QueryField represents a typed, validated column name.
type QueryField int

// QueryField values for each supported file attribute.
const (
	_ QueryField = iota
	FieldHash
	FieldPath
	FieldDirs
	FieldName
	FieldBaseName
	FieldExt
	FieldSize
	FieldModifiedAt
	FieldMime
)

var fieldNames = map[QueryField]string{
	FieldHash:       "hash",
	FieldPath:       "path",
	FieldDirs:       "dirs",
	FieldName:       "name",
	FieldBaseName:   "baseName",
	FieldExt:        "ext",
	FieldSize:       "size",
	FieldModifiedAt: "modifiedAt",
	FieldMime:       "mime",
}

var fieldsByName = initFieldNames(fieldNames)

func initFieldNames(forward map[QueryField]string) map[string]QueryField {
	reverse := make(map[string]QueryField, len(forward))
	for field, name := range forward {
		reverse[name] = field
	}
	return reverse
}

// NewQueryField returns the QueryField for the given name, or false if unknown.
func NewQueryField(name string) (QueryField, bool) {
	field, ok := fieldsByName[name]
	return field, ok
}

// Column returns the SQL column name (matches JSON field name).
func (field QueryField) Column() string {
	return fieldNames[field]
}

// Operator represents a filter comparison operator.
type Operator int

// Operator values for each supported comparison.
const (
	_ Operator = iota
	OpEq
	OpNe
	OpGt
	OpGte
	OpLt
	OpLte
	OpIn
	OpNin
	OpAll
	OpExists
	OpMatch
	OpGlob
	OpNglob
)

var operatorNames = map[Operator]string{
	OpEq:     "eq",
	OpNe:     "ne",
	OpGt:     "gt",
	OpGte:    "gte",
	OpLt:     "lt",
	OpLte:    "lte",
	OpIn:     "in",
	OpNin:    "nin",
	OpAll:    "all",
	OpExists: "exists",
	OpMatch:  "match",
	OpGlob:   "glob",
	OpNglob:  "nglob",
}

var operatorsByName = initOperatorNames(operatorNames)

var likeEscaper = strings.NewReplacer("%", "\\%", "_", "\\_")

func initOperatorNames(forward map[Operator]string) map[string]Operator {
	reverse := make(map[string]Operator, len(forward))
	for operator, name := range forward {
		reverse[name] = operator
	}
	return reverse
}

// String returns the operator's string representation (e.g. "eq", "gt").
func (operator Operator) String() string {
	if name, ok := operatorNames[operator]; ok {
		return name
	}
	return ""
}

// NewOperator returns the Operator for the given name, or false if unknown.
func NewOperator(name string) (Operator, bool) {
	operator, ok := operatorsByName[name]
	return operator, ok
}

// Filter represents a single field-operator-value condition.
type Filter struct {
	Field    QueryField
	Operator Operator
	Value    string
}

// Sorting represents a field ordering directive.
type Sorting struct {
	Field      QueryField
	Descending bool
}

// Pagination holds skip and limit values for paged results.
type Pagination struct {
	Skip  int
	Limit int
}

// FileQuery is a pure query builder. Build() produces SQL + args.
type FileQuery struct {
	fields   []QueryField
	filters  []Filter
	sortings []Sorting
	paging   Pagination
}

// NewFileQuery returns a FileQuery with default pagination.
func NewFileQuery() *FileQuery {
	return &FileQuery{paging: Pagination{Limit: DefaultLimit}}
}

// Paging returns the current pagination settings.
func (query *FileQuery) Paging() Pagination {
	return query.paging
}

// Select adds fields to the projection list.
func (query *FileQuery) Select(fields ...QueryField) *FileQuery {
	query.fields = append(query.fields, fields...)
	return query
}

// Where adds a filter condition to the query.
func (query *FileQuery) Where(field QueryField, operator Operator, value string) *FileQuery {
	query.filters = append(query.filters, Filter{Field: field, Operator: operator, Value: value})
	return query
}

// OrderBy adds a sort directive to the query.
func (query *FileQuery) OrderBy(field QueryField, desc bool) *FileQuery {
	query.sortings = append(query.sortings, Sorting{Field: field, Descending: desc})
	return query
}

// Page sets the skip and limit for pagination.
func (query *FileQuery) Page(skip, limit int) *FileQuery {
	query.paging = Pagination{Skip: skip, Limit: limit}
	return query
}

// Build produces the SQL query string and bound parameters.
func (query *FileQuery) Build() (string, []any, error) {
	whereClause, args, err := query.buildWhereClause()
	if err != nil {
		return "", nil, err
	}

	querySQL := "SELECT hash, source_id, path, name, baseName, ext, size, modifiedAt, mime FROM files" + whereClause

	// ORDER BY clause
	if len(query.sortings) > 0 {
		orderParts := make([]string, len(query.sortings))
		for i, sort := range query.sortings {
			direction := " ASC"
			if sort.Descending {
				direction = " DESC"
			}
			orderParts[i] = sort.Field.Column() + direction
		}
		querySQL += " ORDER BY " + strings.Join(orderParts, ", ")
	}

	// LIMIT/OFFSET clause
	querySQL += " LIMIT ? OFFSET ?"
	args = append(args, query.paging.Limit, query.paging.Skip)

	return querySQL, args, nil
}

// BuildWithDirs wraps Build() output in a CTE and LEFT JOINs file_dirs,
// so the caller receives file columns + dir in a single query.
func (query *FileQuery) BuildWithDirs() (string, []any, error) {
	innerSQL, args, err := query.Build()
	if err != nil {
		return "", nil, err
	}

	var outerOrderParts []string
	for _, sort := range query.sortings {
		direction := " ASC"
		if sort.Descending {
			direction = " DESC"
		}
		outerOrderParts = append(outerOrderParts, "pf."+sort.Field.Column()+direction)
	}
	outerOrderParts = append(outerOrderParts, "pf.hash", "fd.position")

	result := "WITH paginated_files AS (" + innerSQL + ") " +
		"SELECT pf.hash, pf.path, pf.name, pf.baseName, pf.ext, pf.size, pf.modifiedAt, pf.mime, fd.dir " +
		"FROM paginated_files pf " +
		"LEFT JOIN file_dirs fd ON fd.file_hash = pf.hash AND fd.source_id = pf.source_id " +
		"ORDER BY " + strings.Join(outerOrderParts, ", ")

	return result, args, nil
}

// BuildCount produces a COUNT(*) query with the same WHERE clause.
func (query *FileQuery) BuildCount() (string, []any, error) {
	whereClause, args, err := query.buildWhereClause()
	if err != nil {
		return "", nil, err
	}
	return "SELECT COUNT(*) FROM files" + whereClause, args, nil
}

func (query *FileQuery) buildWhereClause() (string, []any, error) {
	var args []any
	var whereParts []string
	for _, filter := range query.filters {
		clause, filterArgs, err := buildFilterSQL(filter)
		if err != nil {
			return "", nil, err
		}
		whereParts = append(whereParts, clause)
		args = append(args, filterArgs...)
	}
	if len(whereParts) > 0 {
		return " WHERE " + strings.Join(whereParts, " AND "), args, nil
	}
	return "", nil, nil
}

func buildFilterSQL(filter Filter) (string, []any, error) {
	column := filter.Field.Column()
	isArray := filter.Field == FieldDirs

	switch filter.Operator {
	case OpEq:
		if isArray {
			return "EXISTS (" + dirSubquery + " AND fd.dir = ?)", []any{filter.Value}, nil
		}
		return column + " = ?", []any{filter.Value}, nil
	case OpNe:
		if isArray {
			return "NOT EXISTS (" + dirSubquery + " AND fd.dir = ?)", []any{filter.Value}, nil
		}
		return column + " != ?", []any{filter.Value}, nil
	case OpGt:
		return column + " > ?", []any{filter.Value}, nil
	case OpGte:
		return column + " >= ?", []any{filter.Value}, nil
	case OpLt:
		return column + " < ?", []any{filter.Value}, nil
	case OpLte:
		return column + " <= ?", []any{filter.Value}, nil
	case OpIn:
		return buildInFilterSQL(column, isArray, filter.Value, false)
	case OpNin:
		return buildInFilterSQL(column, isArray, filter.Value, true)
	case OpAll:
		values := strings.Split(filter.Value, ",")
		clauses := make([]string, len(values))
		args := make([]any, len(values))
		for i, value := range values {
			clauses[i] = "EXISTS (" + dirSubquery + " AND fd.dir = ?)"
			args[i] = strings.TrimSpace(value)
		}
		return "(" + strings.Join(clauses, " AND ") + ")", args, nil
	case OpExists:
		if isArray {
			if strings.ToLower(filter.Value) == "true" {
				return "EXISTS (" + dirSubquery + ")", nil, nil
			}
			return "NOT EXISTS (" + dirSubquery + ")", nil, nil
		}
		if strings.ToLower(filter.Value) == "true" {
			return column + " != ''", nil, nil
		}
		return column + " = ''", nil, nil
	case OpMatch:
		escaped := likeEscaper.Replace(filter.Value)
		return column + " LIKE ? ESCAPE '\\'", []any{"%" + escaped + "%"}, nil
	case OpGlob:
		return column + " GLOB ?", []any{filter.Value}, nil
	case OpNglob:
		return column + " NOT GLOB ?", []any{filter.Value}, nil
	default:
		return "", nil, fmt.Errorf("unhandled operator: '%s'", filter.Operator)
	}
}

func buildInFilterSQL(column string, isArray bool, value string, negate bool) (string, []any, error) {
	values := strings.Split(value, ",")
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = strings.TrimSpace(v)
	}
	joined := strings.Join(placeholders, ", ")
	if isArray {
		not := ""
		if negate {
			not = "NOT "
		}
		return not + "EXISTS (" + dirSubquery + " AND fd.dir IN (" + joined + "))", args, nil
	}
	if negate {
		return column + " NOT IN (" + joined + ")", args, nil
	}
	return column + " IN (" + joined + ")", args, nil
}

// HasFieldSelection returns true if the query has explicit field selection.
func (query *FileQuery) HasFieldSelection() bool {
	return len(query.fields) > 0
}

// Fields returns the selected fields.
func (query *FileQuery) Fields() []QueryField {
	return query.fields
}
