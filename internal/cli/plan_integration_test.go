//go:build integration

package cli_test

import (
	"bytes"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mickamy/subset/internal/cli"
	"github.com/mickamy/subset/internal/dsn"
	"github.com/mickamy/subset/internal/test/tsql"
)

//nolint:paralleltest // mutates the public schema; cannot run in parallel
func TestPlan_E2E_Postgres(t *testing.T) {
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

	t.Run("clone --plan emits a summary, not SQL", func(t *testing.T) {
		out := runPlan(t, rawDSN, "clone", "orders")
		assertContains(t, out, "subset clone: 3 tables, 3 rows in ")
		assertContains(t, out, "orders")
		assertContains(t, out, "via orders → users → tenants")
		assertContains(t, out, "seed")
		assertNotContains(t, out, "INSERT")
	})

	t.Run("delete --plan emits a summary, not SQL", func(t *testing.T) {
		out := runPlan(t, rawDSN, "delete", "tenants")
		assertContains(t, out, "subset delete:")
		assertContains(t, out, "via tenants → users → orders")
		assertNotContains(t, out, "DELETE")
	})
}

//nolint:paralleltest // mutates schema; cannot run in parallel
func TestPlan_E2E_MySQL(t *testing.T) {
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

	t.Run("clone --plan emits a summary, not SQL", func(t *testing.T) {
		out := runPlan(t, rawDSN, "clone", "orders")
		assertContains(t, out, "subset clone: 3 tables, 3 rows in ")
		assertContains(t, out, "via orders → users → tenants")
		assertNotContains(t, out, "INSERT")
	})

	t.Run("delete --plan emits a summary, not SQL", func(t *testing.T) {
		out := runPlan(t, rawDSN, "delete", "tenants")
		assertContains(t, out, "subset delete:")
		assertNotContains(t, out, "DELETE")
	})
}

func runPlan(t *testing.T, rawDSN, cmd, table string) string {
	t.Helper()
	var out, errBuf bytes.Buffer
	exitCode := cli.Run([]string{cmd, rawDSN, table, "--id", "1", "--plan"}, &out, &errBuf)
	if exitCode != 0 {
		t.Fatalf("%s --plan exit %d; stderr=%s", cmd, exitCode, errBuf.String())
	}

	return out.String()
}

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("output missing %q; got:\n%s", sub, s)
	}
}

func assertNotContains(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("output unexpectedly contains %q; got:\n%s", sub, s)
	}
}
