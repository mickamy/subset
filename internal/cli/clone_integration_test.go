//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mickamy/subset/internal/cli"
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
