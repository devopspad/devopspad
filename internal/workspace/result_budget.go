package workspace

import (
	"encoding/json"
	"strconv"
	"time"
)

// Budget the encoded preview before crossing the desktop bridge. Drivers can
// allocate a single row or response before this check; this is not a wire limit.
const ResultByteLimit = 8 << 20

type resultBudget struct{ remaining int }

func newResultBudget(columns []string) *resultBudget {
	b, _ := json.Marshal(columns)
	// Reserve space for the Result envelope and truncation message.
	return &resultBudget{remaining: ResultByteLimit - len(b) - 1024}
}

func (b *resultBudget) append(result *Result, row []any) bool {
	if len(result.Rows) >= RowLimit {
		result.Truncated = true
		return false
	}
	encoded, err := json.Marshal(row)
	if err != nil || len(encoded)+1 > b.remaining {
		result.Truncated = true
		result.Message = "Preview reached the 8 MiB limit. Narrow the query or select fewer fields."
		return false
	}
	b.remaining -= len(encoded) + 1
	result.Rows = append(result.Rows, row)
	return true
}

type sqlRows interface {
	Columns() ([]string, error)
	Next() bool
	Scan(...any) error
	Err() error
}

func readSQLRows(rows sqlRows) (Result, error) {
	result := emptyResult()
	var err error
	result.Columns, err = rows.Columns()
	if err != nil {
		return result, err
	}
	budget := newResultBudget(result.Columns)
	for rows.Next() {
		// Read one extra row to distinguish an exact limit from a partial result.
		if len(result.Rows) >= RowLimit {
			result.Truncated = true
			break
		}
		values := make([]any, len(result.Columns))
		pointers := make([]any, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return result, err
		}
		for i, value := range values {
			switch v := value.(type) {
			case []byte:
				values[i] = string(v)
			case time.Time:
				values[i] = v.UTC().Format(time.RFC3339)
			case int64:
				values[i] = strconv.FormatInt(v, 10)
			case uint64:
				values[i] = strconv.FormatUint(v, 10)
			}
		}
		if !budget.append(&result, values) {
			break
		}
	}
	return result, rows.Err()
}
