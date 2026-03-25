package server

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/mirovarga/filetap/db"
)

type queryValidationError struct {
	Message string
}

func (validationError *queryValidationError) Error() string {
	return validationError.Message
}

const (
	maxLimit        = 1000
	maxSelectFields = 20
	maxFilters      = 20
	maxSortFields   = 5
	maxInValues     = 100
)

var paramPattern = regexp.MustCompile(`^([a-zA-Z]+)\[([a-zA-Z]+)]$`)

var reservedParams = map[string]bool{
	"skip":   true,
	"limit":  true,
	"order":  true,
	"select": true,
}

func parseFileQuery(queryParams url.Values) (*db.FileQuery, error) {
	query := db.NewFileQuery()

	fields, err := parseSelectParams(queryParams)
	if err != nil {
		return nil, err
	}
	query.Select(fields...)

	filters, err := parseFilterParams(queryParams)
	if err != nil {
		return nil, err
	}
	for _, filter := range filters {
		query.Where(filter.Field, filter.Operator, filter.Value)
	}

	sorts, err := parseOrderParams(queryParams)
	if err != nil {
		return nil, err
	}
	for _, sort := range sorts {
		query.OrderBy(sort.Field, sort.Descending)
	}

	page, err := parsePaginationParams(queryParams)
	if err != nil {
		return nil, err
	}
	query.Page(page.Skip, page.Limit)

	return query, nil
}

func parseSelectParams(queryParams url.Values) ([]db.QueryField, error) {
	selectParam := queryParams.Get("select")
	if selectParam == "" {
		return nil, nil
	}

	fieldNames := strings.Split(selectParam, ",")
	if len(fieldNames) > maxSelectFields {
		return nil, &queryValidationError{Message: fmt.Sprintf("too many select fields: %d (max %d)", len(fieldNames), maxSelectFields)}
	}
	fields := make([]db.QueryField, 0, len(fieldNames))
	for _, fieldName := range fieldNames {
		fieldName = strings.TrimSpace(fieldName)
		field, ok := db.NewQueryField(fieldName)
		if !ok {
			return nil, &queryValidationError{Message: fmt.Sprintf("unknown select field: '%s'", fieldName)}
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func parseFilterParams(queryParams url.Values) ([]db.Filter, error) {
	var filters []db.Filter
	for param, values := range queryParams {
		if len(filters) >= maxFilters {
			return nil, &queryValidationError{Message: fmt.Sprintf("too many filters: max %d", maxFilters)}
		}
		if reservedParams[param] {
			continue
		}

		// Repeated query params use the first value only.
		value := values[0]

		if matches := paramPattern.FindStringSubmatch(param); matches != nil {
			fieldName := matches[1]
			operatorName := matches[2]

			field, ok := db.NewQueryField(fieldName)
			if !ok {
				return nil, &queryValidationError{Message: fmt.Sprintf("unknown filter field: '%s'", fieldName)}
			}
			operator, ok := db.NewOperator(operatorName)
			if !ok {
				return nil, &queryValidationError{Message: fmt.Sprintf("unknown operator: '%s'", operatorName)}
			}
			if err := validateOperatorField(operator, field); err != nil {
				return nil, err
			}
			if (operator == db.OpIn || operator == db.OpNin || operator == db.OpAll) && len(strings.Split(value, ",")) > maxInValues {
				return nil, &queryValidationError{Message: fmt.Sprintf("too many values for '%s' operator: max %d", operator.String(), maxInValues)}
			}
			filters = append(filters, db.Filter{Field: field, Operator: operator, Value: value})
			continue
		}

		field, ok := db.NewQueryField(param)
		if !ok {
			return nil, &queryValidationError{Message: fmt.Sprintf("unknown parameter: '%s'", param)}
		}
		filters = append(filters, db.Filter{Field: field, Operator: db.OpEq, Value: value})
	}
	return filters, nil
}

func parseOrderParams(queryParams url.Values) ([]db.Sorting, error) {
	orderParam := queryParams.Get("order")
	if orderParam == "" {
		return nil, nil
	}

	sortFields := strings.Split(orderParam, ",")
	if len(sortFields) > maxSortFields {
		return nil, &queryValidationError{Message: fmt.Sprintf("too many sort fields: %d (max %d)", len(sortFields), maxSortFields)}
	}
	sorts := make([]db.Sorting, 0, len(sortFields))
	for _, sortField := range sortFields {
		sortField = strings.TrimSpace(sortField)
		descending := false
		if strings.HasPrefix(sortField, "-") {
			descending = true
			sortField = sortField[1:]
		}
		field, ok := db.NewQueryField(sortField)
		if !ok {
			return nil, &queryValidationError{Message: fmt.Sprintf("unknown sort field: '%s'", sortField)}
		}
		if field == db.FieldDirs {
			return nil, &queryValidationError{Message: "sorting by 'dirs' is not supported"}
		}
		sorts = append(sorts, db.Sorting{Field: field, Descending: descending})
	}
	return sorts, nil
}

func parsePaginationParams(queryParams url.Values) (db.Pagination, error) {
	skip, err := parseQueryInt(queryParams.Get("skip"), 0)
	if err != nil || skip < 0 {
		return db.Pagination{}, &queryValidationError{Message: fmt.Sprintf("invalid skip parameter: '%s'", queryParams.Get("skip"))}
	}

	limit, err := parseQueryInt(queryParams.Get("limit"), db.DefaultLimit)
	if err != nil || limit < 0 {
		return db.Pagination{}, &queryValidationError{Message: fmt.Sprintf("invalid limit parameter: '%s'", queryParams.Get("limit"))}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	return db.Pagination{Skip: skip, Limit: limit}, nil
}

func parseQueryInt(value string, defaultValue int) (int, error) {
	if value == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(value)
}

func validateOperatorField(operator db.Operator, field db.QueryField) error {
	isArray := field == db.FieldDirs

	switch operator {
	case db.OpAll:
		if !isArray {
			return &queryValidationError{Message: fmt.Sprintf("operator '%s' is not supported for field '%s'", operator.String(), field.Column())}
		}
	case db.OpGt, db.OpGte, db.OpLt, db.OpLte:
		if isArray {
			return &queryValidationError{Message: fmt.Sprintf("operator '%s' is not supported for field '%s'", operator.String(), field.Column())}
		}
	case db.OpMatch, db.OpGlob, db.OpNglob:
		if isArray {
			return &queryValidationError{Message: fmt.Sprintf("operator '%s' is not supported for field '%s'", operator.String(), field.Column())}
		}
	}
	return nil
}
