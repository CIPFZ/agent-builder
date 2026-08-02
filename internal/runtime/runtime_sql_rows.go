package runtime

import (
	"context"
	"database/sql"
)

// consumeRuntimeRows centralizes iterator cleanup and terminal error checks.
func consumeRuntimeRows(rows *sql.Rows, visit func(*sql.Rows) error) error {
	defer rows.Close()
	for rows.Next() {
		if err := visit(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func queryRuntimeRows(ctx context.Context, db *sql.DB, query string, visit func(*sql.Rows) error, args ...any) error {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return consumeRuntimeRows(rows, visit)
}
