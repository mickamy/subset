//go:build integration

package tsql

import (
	"context"
	"database/sql"
	"strings"
)

// ResetMySQLTables drops every base table in the current schema so a test
// starts from a clean slate. MySQL has no DROP SCHEMA ... CASCADE.
// `SET FOREIGN_KEY_CHECKS=0` is session-scoped, so we pin a single connection
// to make sure the SELECT / DROP statements see the disabled FK checks.
func ResetMySQLTables(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), "SET FOREIGN_KEY_CHECKS=1")
	}()

	rows, err := conn.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'")
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()

			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()

		return err
	}
	_ = rows.Close()

	for _, name := range tables {
		if _, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS "+QuoteMySQLIdent(name)); err != nil {
			return err
		}
	}

	return nil
}

func QuoteMySQLIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
