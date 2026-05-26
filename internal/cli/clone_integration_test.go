//go:build integration

package cli_test

import (
	"bytes"
	"context"
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

const (
	cloneSchemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'cancelled');

CREATE TABLE tenants (
    id   serial PRIMARY KEY,
    name text   NOT NULL
);

CREATE TABLE users (
    id        serial PRIMARY KEY,
    tenant_id int    NOT NULL REFERENCES tenants(id),
    email     text   NOT NULL,
    name      text,
    UNIQUE (tenant_id, email)
);

CREATE TABLE products (
    id        serial PRIMARY KEY,
    tenant_id int    NOT NULL REFERENCES tenants(id),
    sku       text   NOT NULL,
    name      text   NOT NULL,
    price_yen int    NOT NULL,
    UNIQUE (tenant_id, sku)
);

CREATE TABLE orders (
    id        serial       PRIMARY KEY,
    user_id   int          NOT NULL REFERENCES users(id),
    status    order_status NOT NULL,
    total_yen int          NOT NULL
);

CREATE TABLE order_items (
    id         serial PRIMARY KEY,
    order_id   int    NOT NULL REFERENCES orders(id),
    product_id int    NOT NULL REFERENCES products(id),
    qty        int    NOT NULL,
    price_yen  int    NOT NULL
);

CREATE TABLE employees (
    id         serial PRIMARY KEY,
    name       text NOT NULL,
    manager_id int  REFERENCES employees(id)
);
`
	cloneSeedSQL = `
INSERT INTO tenants (id, name) VALUES (1, 'Acme'), (2, 'Beta');
SELECT setval('tenants_id_seq', 2);
INSERT INTO users (id, tenant_id, email, name) VALUES
    (1, 1, 'alice@acme.com', 'Alice'),
    (2, 1, 'bob@acme.com',   'Bob');
SELECT setval('users_id_seq', 2);
INSERT INTO products (id, tenant_id, sku, name, price_yen) VALUES
    (1, 1, 'ACME-001', 'Widget', 1000);
SELECT setval('products_id_seq', 1);
INSERT INTO orders (id, user_id, status, total_yen) VALUES
    (1, 1, 'paid', 1000);
SELECT setval('orders_id_seq', 1);
INSERT INTO order_items (id, order_id, product_id, qty, price_yen) VALUES
    (1, 1, 1, 1, 1000);
SELECT setval('order_items_id_seq', 1);

INSERT INTO employees (id, name, manager_id) VALUES
    (1, 'Alice', NULL),
    (2, 'Bob',   1),
    (3, 'Carol', 1),
    (4, 'Dave',  2);
SELECT setval('employees_id_seq', 4);
`
)

//nolint:paralleltest // mutates the public schema; cannot run in parallel
func TestClone_E2E_Postgres(t *testing.T) {
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

	// 1. Source DB: full fixture (schema + seed).
	if _, err := db.ExecContext(ctx, cloneSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, cloneSeedSQL); err != nil {
		t.Fatalf("apply seed: %v", err)
	}

	// 2. Run `subset clone orders --id 1` via the public CLI entry point.
	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"clone", rawDSN, "orders", "--id", "1"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("clone exit %d; stderr=%s", exitCode, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatal("clone produced no output")
	}

	// 3. Wipe the source and re-apply schema only.
	if _, err := db.ExecContext(ctx, cloneSchemaSQL); err != nil {
		t.Fatalf("re-apply schema: %v", err)
	}

	// 4. Apply the captured clone SQL.
	if _, err := db.ExecContext(ctx, out.String()); err != nil {
		t.Fatalf("apply clone output: %v\nSQL:\n%s", err, out.String())
	}

	// 5. Verify the closure: orders/1 -> users/1 -> tenants/1.
	checkCount(t, ctx, db, "tenants", 1)
	checkCount(t, ctx, db, "users", 1)
	checkCount(t, ctx, db, "orders", 1)

	// Children (order_items) and siblings (products) are not in the forward
	// closure; they must remain empty after restore.
	checkCount(t, ctx, db, "order_items", 0)
	checkCount(t, ctx, db, "products", 0)

	// Spot-check field values to confirm row data was preserved verbatim.
	var tenantName, userEmail string
	var totalYen int
	if err := db.QueryRowContext(ctx, "SELECT name FROM tenants WHERE id = 1").Scan(&tenantName); err != nil {
		t.Fatalf("read tenant: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT email FROM users WHERE id = 1").Scan(&userEmail); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT total_yen FROM orders WHERE id = 1").Scan(&totalYen); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if tenantName != "Acme" {
		t.Errorf("tenant name = %q; want Acme", tenantName)
	}
	if userEmail != "alice@acme.com" {
		t.Errorf("user email = %q; want alice@acme.com", userEmail)
	}
	if totalYen != 1000 {
		t.Errorf("order total = %d; want 1000", totalYen)
	}
}

//nolint:paralleltest // mutates the public schema; cannot run in parallel
func TestClone_E2E_Postgres_SelfRef(t *testing.T) {
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

	// 1. Source DB: full fixture (includes employees with manager_id self-ref).
	if _, err := db.ExecContext(ctx, cloneSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, cloneSeedSQL); err != nil {
		t.Fatalf("apply seed: %v", err)
	}

	// 2. Clone Dave (id=4). Forward closure walks the manager chain:
	//    Dave -> Bob -> Alice (Alice has NULL manager_id, terminates).
	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"clone", rawDSN, "employees", "--id", "4"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("clone exit %d; stderr=%s", exitCode, errBuf.String())
	}

	// 3. Wipe and re-apply schema only.
	if _, err := db.ExecContext(ctx, cloneSchemaSQL); err != nil {
		t.Fatalf("re-apply schema: %v", err)
	}

	// 4. Apply clone output. Must succeed: intra-table topological sort
	//    inside SortByPK ensures Alice's INSERT precedes Bob's, which
	//    precedes Dave's. Without sort, Dave's INSERT would reference
	//    manager_id=2 before Bob exists, triggering FK violation here.
	if _, err := db.ExecContext(ctx, out.String()); err != nil {
		t.Fatalf("apply clone output: %v\nSQL:\n%s", err, out.String())
	}

	// 5. Verify the closure: 3 employees, Carol (id=3, sibling) excluded.
	checkCount(t, ctx, db, "employees", 3)

	var aliceMgr sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT manager_id FROM employees WHERE id = 1").Scan(&aliceMgr); err != nil {
		t.Fatalf("read alice: %v", err)
	}
	if aliceMgr.Valid {
		t.Errorf("Alice manager_id = %d; want NULL", aliceMgr.Int64)
	}

	var bobMgr, daveMgr int
	if err := db.QueryRowContext(ctx, "SELECT manager_id FROM employees WHERE id = 2").Scan(&bobMgr); err != nil {
		t.Fatalf("read bob: %v", err)
	}
	if bobMgr != 1 {
		t.Errorf("Bob manager_id = %d; want 1", bobMgr)
	}
	if err := db.QueryRowContext(ctx, "SELECT manager_id FROM employees WHERE id = 4").Scan(&daveMgr); err != nil {
		t.Fatalf("read dave: %v", err)
	}
	if daveMgr != 2 {
		t.Errorf("Dave manager_id = %d; want 2", daveMgr)
	}

	// Carol (id=3, sibling of Bob under Alice) is not in Dave's forward closure.
	var carolCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM employees WHERE id = 3").Scan(&carolCount); err != nil {
		t.Fatalf("read carol: %v", err)
	}
	if carolCount != 0 {
		t.Errorf("Carol (id=3) present; want absent (not in closure)")
	}
}

const (
	mysqlCloneSchemaSQL = `
CREATE TABLE tenants (
    id   INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);
CREATE TABLE users (
    id        INT AUTO_INCREMENT PRIMARY KEY,
    tenant_id INT NOT NULL,
    email     VARCHAR(255) NOT NULL,
    name      VARCHAR(255),
    UNIQUE (tenant_id, email),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE TABLE products (
    id        INT AUTO_INCREMENT PRIMARY KEY,
    tenant_id INT NOT NULL,
    sku       VARCHAR(255) NOT NULL,
    name      VARCHAR(255) NOT NULL,
    price_yen INT NOT NULL,
    UNIQUE (tenant_id, sku),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE TABLE orders (
    id        INT AUTO_INCREMENT PRIMARY KEY,
    user_id   INT NOT NULL,
    status    ENUM('pending', 'paid', 'shipped', 'cancelled') NOT NULL,
    total_yen INT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE TABLE order_items (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    order_id   INT NOT NULL,
    product_id INT NOT NULL,
    qty        INT NOT NULL,
    price_yen  INT NOT NULL,
    FOREIGN KEY (order_id)   REFERENCES orders(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);
CREATE TABLE employees (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    manager_id INT,
    FOREIGN KEY (manager_id) REFERENCES employees(id)
);
`
	mysqlCloneSeedSQL = `
INSERT INTO tenants (id, name) VALUES (1, 'Acme'), (2, 'Beta');
INSERT INTO users (id, tenant_id, email, name) VALUES
    (1, 1, 'alice@acme.com', 'Alice'),
    (2, 1, 'bob@acme.com',   'Bob');
INSERT INTO products (id, tenant_id, sku, name, price_yen) VALUES
    (1, 1, 'ACME-001', 'Widget', 1000);
INSERT INTO orders (id, user_id, status, total_yen) VALUES
    (1, 1, 'paid', 1000);
INSERT INTO order_items (id, order_id, product_id, qty, price_yen) VALUES
    (1, 1, 1, 1, 1000);
INSERT INTO employees (id, name, manager_id) VALUES
    (1, 'Alice', NULL),
    (2, 'Bob',   1),
    (3, 'Carol', 1),
    (4, 'Dave',  2);
`
)

//nolint:paralleltest // mutates schema; cannot run in parallel
func TestClone_E2E_MySQL(t *testing.T) {
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

	// 1. Source DB: full fixture.
	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		t.Fatalf("reset: %v", err)
	}
	applyMySQLStatements(t, ctx, db, mysqlCloneSchemaSQL)
	applyMySQLStatements(t, ctx, db, mysqlCloneSeedSQL)

	// 2. Run subset clone via cli.Run.
	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"clone", rawDSN, "orders", "--id", "1"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("clone exit %d; stderr=%s", exitCode, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatal("clone produced no output")
	}

	// 3. Wipe and re-apply schema only.
	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		t.Fatalf("re-reset: %v", err)
	}
	applyMySQLStatements(t, ctx, db, mysqlCloneSchemaSQL)

	// 4. Apply the captured clone SQL (statement-by-statement; the driver
	//    doesn't accept multi-statement Exec without multiStatements=true).
	applyMySQLStatements(t, ctx, db, out.String())

	// 5. Verify the closure: orders/1 -> users/1 -> tenants/1.
	checkCount(t, ctx, db, "tenants", 1)
	checkCount(t, ctx, db, "users", 1)
	checkCount(t, ctx, db, "orders", 1)
	checkCount(t, ctx, db, "order_items", 0)
	checkCount(t, ctx, db, "products", 0)

	var tenantName, userEmail string
	var totalYen int
	if err := db.QueryRowContext(ctx, "SELECT name FROM tenants WHERE id = 1").Scan(&tenantName); err != nil {
		t.Fatalf("read tenant: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT email FROM users WHERE id = 1").Scan(&userEmail); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT total_yen FROM orders WHERE id = 1").Scan(&totalYen); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if tenantName != "Acme" {
		t.Errorf("tenant name = %q; want Acme", tenantName)
	}
	if userEmail != "alice@acme.com" {
		t.Errorf("user email = %q; want alice@acme.com", userEmail)
	}
	if totalYen != 1000 {
		t.Errorf("order total = %d; want 1000", totalYen)
	}
}

//nolint:paralleltest // mutates schema; cannot run in parallel
func TestClone_E2E_MySQL_SelfRef(t *testing.T) {
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

	// 1. Source DB: full fixture (employees with manager_id self-ref).
	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		t.Fatalf("reset: %v", err)
	}
	applyMySQLStatements(t, ctx, db, mysqlCloneSchemaSQL)
	applyMySQLStatements(t, ctx, db, mysqlCloneSeedSQL)

	// 2. Clone Dave (id=4). Walks the manager chain: Dave -> Bob -> Alice.
	var out, errBuf bytes.Buffer
	exitCode := cli.Run(
		[]string{"clone", rawDSN, "employees", "--id", "4"},
		&out, &errBuf,
	)
	if exitCode != 0 {
		t.Fatalf("clone exit %d; stderr=%s", exitCode, errBuf.String())
	}

	// 3. Wipe and re-apply schema only.
	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		t.Fatalf("re-reset: %v", err)
	}
	applyMySQLStatements(t, ctx, db, mysqlCloneSchemaSQL)

	// 4. Apply clone output. Must succeed thanks to intra-table topo sort.
	applyMySQLStatements(t, ctx, db, out.String())

	// 5. Verify the closure: 3 employees (Carol excluded).
	checkCount(t, ctx, db, "employees", 3)

	var aliceMgr sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT manager_id FROM employees WHERE id = 1").Scan(&aliceMgr); err != nil {
		t.Fatalf("read alice: %v", err)
	}
	if aliceMgr.Valid {
		t.Errorf("Alice manager_id = %d; want NULL", aliceMgr.Int64)
	}

	var bobMgr, daveMgr int
	if err := db.QueryRowContext(ctx, "SELECT manager_id FROM employees WHERE id = 2").Scan(&bobMgr); err != nil {
		t.Fatalf("read bob: %v", err)
	}
	if bobMgr != 1 {
		t.Errorf("Bob manager_id = %d; want 1", bobMgr)
	}
	if err := db.QueryRowContext(ctx, "SELECT manager_id FROM employees WHERE id = 4").Scan(&daveMgr); err != nil {
		t.Fatalf("read dave: %v", err)
	}
	if daveMgr != 2 {
		t.Errorf("Dave manager_id = %d; want 2", daveMgr)
	}
}

func applyMySQLStatements(t *testing.T, ctx context.Context, db *sql.DB, sqlText string) {
	t.Helper()
	for stmt := range strings.SplitSeq(sqlText, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("exec: %v\nstmt: %s", err, stmt)
		}
	}
}

func checkCount(t *testing.T, ctx context.Context, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Errorf("%s rows = %d; want %d", table, got, want)
	}
}
