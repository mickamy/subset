//go:build integration

package cli_test

import (
	"bytes"
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mickamy/subset/internal/cli"
	"github.com/mickamy/subset/internal/dsn"
	"github.com/mickamy/subset/internal/test/tsql"
)

//nolint:paralleltest // mutates the public schema; cannot run in parallel
func TestDelete_E2E_Postgres(t *testing.T) {
	rawDSN := os.Getenv("SUBSET_TEST_DSN_POSTGRES")
	if rawDSN == "" {
		t.Skip("SUBSET_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	db, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// 1. Populate the source DB with the full fixture.
	if _, err := db.ExecContext(ctx, cloneSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, cloneSeedSQL); err != nil {
		t.Fatalf("apply seed: %v", err)
	}

	// 2. Run `subset delete tenants --id 1`. Backward closure pulls Acme's
	//    users, products, orders, and order_items.
	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"delete", rawDSN, "tenants", "--id", "1"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("delete exit %d; stderr=%s", exitCode, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatal("delete produced no output")
	}

	// 3. Apply the DELETE SQL to the same populated DB. Must succeed: rows are
	//    deleted most-dependent-first (order_items -> orders -> users/products
	//    -> tenants), so no FK constraint is ever violated mid-apply.
	if _, err := db.ExecContext(ctx, out.String()); err != nil {
		t.Fatalf("apply delete output: %v\nSQL:\n%s", err, out.String())
	}

	// 4. Acme (tenant 1) and all its dependents are gone; Beta (tenant 2)
	//    survives. The fixture gives Beta no users/products/orders, so every
	//    dependent table is emptied while the Beta tenant row remains.
	checkCount(t, ctx, db, "tenants", 1)
	checkCount(t, ctx, db, "users", 0)
	checkCount(t, ctx, db, "products", 0)
	checkCount(t, ctx, db, "orders", 0)
	checkCount(t, ctx, db, "order_items", 0)

	// employees has no FK to tenants, so it is untouched.
	checkCount(t, ctx, db, "employees", 4)

	var tenantName string
	if err := db.QueryRowContext(ctx, "SELECT name FROM tenants").Scan(&tenantName); err != nil {
		t.Fatalf("read surviving tenant: %v", err)
	}
	if tenantName != "Beta" {
		t.Errorf("surviving tenant = %q; want Beta", tenantName)
	}
}

//nolint:paralleltest // mutates the public schema; cannot run in parallel
func TestDelete_E2E_Postgres_SelfRef(t *testing.T) {
	rawDSN := os.Getenv("SUBSET_TEST_DSN_POSTGRES")
	if rawDSN == "" {
		t.Skip("SUBSET_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	db, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, cloneSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, cloneSeedSQL); err != nil {
		t.Fatalf("apply seed: %v", err)
	}

	// Delete Alice (id=1). Backward closure walks the manager chain downward:
	// Alice -> Bob, Carol (manager_id=1) -> Dave (manager_id=2), so every
	// employee is in the closure.
	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"delete", rawDSN, "employees", "--id", "1"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("delete exit %d; stderr=%s", exitCode, errBuf.String())
	}

	// Apply must succeed: intra-table reverse ordering deletes reports before
	// their managers (Dave before Bob, Bob/Carol before Alice). Without it,
	// deleting Alice while Bob still references her would raise an FK error.
	if _, err := db.ExecContext(ctx, out.String()); err != nil {
		t.Fatalf("apply delete output: %v\nSQL:\n%s", err, out.String())
	}

	checkCount(t, ctx, db, "employees", 0)
}

//nolint:paralleltest // mutates schema; cannot run in parallel
func TestDelete_E2E_MySQL(t *testing.T) {
	rawDSN := os.Getenv("SUBSET_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SUBSET_TEST_DSN_MYSQL not set")
	}

	ctx := t.Context()
	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("convert dsn: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		t.Fatalf("reset: %v", err)
	}
	applyMySQLStatements(t, ctx, db, mysqlCloneSchemaSQL)
	applyMySQLStatements(t, ctx, db, mysqlCloneSeedSQL)

	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"delete", rawDSN, "tenants", "--id", "1"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("delete exit %d; stderr=%s", exitCode, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatal("delete produced no output")
	}

	applyMySQLStatements(t, ctx, db, out.String())

	checkCount(t, ctx, db, "tenants", 1)
	checkCount(t, ctx, db, "users", 0)
	checkCount(t, ctx, db, "products", 0)
	checkCount(t, ctx, db, "orders", 0)
	checkCount(t, ctx, db, "order_items", 0)
	checkCount(t, ctx, db, "employees", 4)

	var tenantName string
	if err := db.QueryRowContext(ctx, "SELECT name FROM tenants").Scan(&tenantName); err != nil {
		t.Fatalf("read surviving tenant: %v", err)
	}
	if tenantName != "Beta" {
		t.Errorf("surviving tenant = %q; want Beta", tenantName)
	}
}

//nolint:paralleltest // mutates schema; cannot run in parallel
func TestDelete_E2E_MySQL_SelfRef(t *testing.T) {
	rawDSN := os.Getenv("SUBSET_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SUBSET_TEST_DSN_MYSQL not set")
	}

	ctx := t.Context()
	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("convert dsn: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		t.Fatalf("reset: %v", err)
	}
	applyMySQLStatements(t, ctx, db, mysqlCloneSchemaSQL)
	applyMySQLStatements(t, ctx, db, mysqlCloneSeedSQL)

	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"delete", rawDSN, "employees", "--id", "1"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("delete exit %d; stderr=%s", exitCode, errBuf.String())
	}

	applyMySQLStatements(t, ctx, db, out.String())

	checkCount(t, ctx, db, "employees", 0)
}
