package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// QueryMaps is intentionally limited to SQL assembled by the backend. Callers
// must pass fixed table/column fragments, never user input.
func (s *Store) QueryMaps(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for index, name := range columns {
			if bytes, ok := values[index].([]byte); ok {
				item[name] = string(bytes)
			} else {
				item[name] = values[index]
			}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) InsertMap(ctx context.Context, table string, values map[string]any) (int64, error) {
	columns := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for column := range values {
		columns = append(columns, column)
	}
	sortStrings(columns)
	for _, column := range columns {
		args = append(args, values[column])
	}
	placeholders := make([]string, len(columns))
	for index := range placeholders {
		placeholders[index] = "?"
	}
	query := "INSERT INTO " + s.quote(table) + " (" + s.columns(columns...) + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	result, err := s.db.ExecContext(ctx, s.bind(query), args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UpdateMap(ctx context.Context, table string, id int64, values map[string]any) error {
	columns := make([]string, 0, len(values))
	for column := range values {
		columns = append(columns, column)
	}
	sortStrings(columns)
	assignments := make([]string, len(columns))
	args := make([]any, 0, len(columns)+1)
	for index, column := range columns {
		assignments[index] = s.quote(column) + "=?"
		args = append(args, values[column])
	}
	args = append(args, id)
	query := "UPDATE " + s.quote(table) + " SET " + strings.Join(assignments, ", ") + " WHERE " + s.quote("id") + "=?"
	result, err := s.db.ExecContext(ctx, s.bind(query), args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows != 1 {
		return sql.ErrNoRows
	}
	return err
}

func (s *Store) DeleteByID(ctx context.Context, table string, id int64) error {
	result, err := s.db.ExecContext(ctx, s.bind("DELETE FROM "+s.quote(table)+" WHERE "+s.quote("id")+"=?"), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows != 1 {
		return sql.ErrNoRows
	}
	return err
}

func commonCreatedValues() map[string]any {
	now := time.Now().UnixMilli()
	return map[string]any{"created_time": now, "updated_time": now, "status": 1}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func asInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case string:
		var parsed int64
		_, err := fmt.Sscan(typed, &parsed)
		return parsed, err
	default:
		return 0, fmt.Errorf("invalid id")
	}
}

func AsInt64(value any) (int64, error) { return asInt64(value) }
