//go:build integration

package extract_test

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/dsn"
	"github.com/mickamy/subset/internal/extract"
	"github.com/mickamy/subset/internal/introspect"
	"github.com/mickamy/subset/internal/test/tsql"
)

const postgresFixtureSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TABLE tenants (
    id   serial PRIMARY KEY,
    name text   NOT NULL
);

CREATE TABLE users (
    id        serial PRIMARY KEY,
    tenant_id int    NOT NULL REFERENCES tenants(id),
    email     text   NOT NULL
);

CREATE TABLE products (
    id        serial PRIMARY KEY,
    tenant_id int    NOT NULL REFERENCES tenants(id),
    sku       text   NOT NULL
);

CREATE TABLE orders (
    id      serial PRIMARY KEY,
    user_id int    NOT NULL REFERENCES users(id),
    amount  int
);

CREATE TABLE order_items (
    id         serial PRIMARY KEY,
    order_id   int    NOT NULL REFERENCES orders(id),
    product_id int    NOT NULL REFERENCES products(id),
    qty        int    NOT NULL
);

-- Composite PK / FK fixture.
CREATE TABLE sites (
    id   serial PRIMARY KEY,
    name text   NOT NULL
);

CREATE TABLE pages (
    site_id int  NOT NULL REFERENCES sites(id),
    slug    text NOT NULL,
    title   text,
    PRIMARY KEY (site_id, slug)
);

CREATE TABLE page_comments (
    id      serial PRIMARY KEY,
    site_id int    NOT NULL,
    slug    text   NOT NULL,
    body    text,
    FOREIGN KEY (site_id, slug) REFERENCES pages(site_id, slug)
);

INSERT INTO tenants (id, name) VALUES (1, 'Acme'), (2, 'Beta');
INSERT INTO users (id, tenant_id, email) VALUES
    (1, 1, 'alice@acme.com'),
    (2, 1, 'bob@acme.com'),
    (3, 2, 'dave@beta.com');
INSERT INTO products (id, tenant_id, sku) VALUES
    (1, 1, 'ACME-001'),
    (2, 2, 'BETA-001');
INSERT INTO orders (id, user_id, amount) VALUES
    (1, 1, 100),
    (2, 2, 200);
INSERT INTO order_items (id, order_id, product_id, qty) VALUES
    (1, 1, 1, 2);

INSERT INTO sites (id, name) VALUES (1, 'Main');
INSERT INTO pages (site_id, slug, title) VALUES
    (1, 'home',  'Home'),
    (1, 'about', 'About');
INSERT INTO page_comments (id, site_id, slug, body) VALUES
    (1, 1, 'home', 'Hello');
`

//nolint:paralleltest // mutates the public schema; cannot run in parallel
func TestWalk_ForwardClosure_Postgres(t *testing.T) {
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

	if _, err := db.ExecContext(ctx, postgresFixtureSQL); err != nil {
		t.Fatalf("apply fixture: %v", err)
	}

	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	t.Run("orders seed pulls users and tenants but not children", func(t *testing.T) {
		result, err := extract.Walk(ctx, db, dialect.Postgres{}, schema, "orders", "id = 1")
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		// orders/1 -> users/1 -> tenants/1 (forward closure).
		// order_items and products must NOT appear: they are children of orders, not parents.
		assertRowCount(t, result, "orders", 1)
		assertRowCount(t, result, "users", 1)
		assertRowCount(t, result, "tenants", 1)
		assertRowCount(t, result, "order_items", 0)
		assertRowCount(t, result, "products", 0)
	})

	t.Run("dedup across overlapping parents", func(t *testing.T) {
		// Both orders belong to Acme users, so we should see 1 tenant (deduped),
		// 2 users (alice, bob), and 2 orders.
		result, err := extract.Walk(ctx, db, dialect.Postgres{}, schema, "orders", "id IN (1, 2)")
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		assertRowCount(t, result, "orders", 2)
		assertRowCount(t, result, "users", 2)
		assertRowCount(t, result, "tenants", 1)
	})

	t.Run("empty whereClause selects all rows from seed", func(t *testing.T) {
		// "tenants" sits at the top of the FK graph (no outgoing FKs),
		// so passing empty whereClause should yield exactly all tenants.
		result, err := extract.Walk(ctx, db, dialect.Postgres{}, schema, "tenants", "")
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		assertRowCount(t, result, "tenants", 2)
	})

	t.Run("composite FK walks through composite PK", func(t *testing.T) {
		// page_comments(1) -> pages(site_id=1, slug='home') via composite FK,
		// then pages -> sites(1) via single-column FK.
		result, err := extract.Walk(ctx, db, dialect.Postgres{}, schema, "page_comments", "id = 1")
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		assertRowCount(t, result, "page_comments", 1)
		assertRowCount(t, result, "pages", 1)
		assertRowCount(t, result, "sites", 1)
	})

	t.Run("unknown table errors", func(t *testing.T) {
		_, err := extract.Walk(ctx, db, dialect.Postgres{}, schema, "nope", "id = 1")
		if err == nil {
			t.Fatal("expected error for unknown table")
		}
	})
}

const mysqlFixtureSQL = `
CREATE TABLE tenants (
    id   INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);
CREATE TABLE users (
    id        INT AUTO_INCREMENT PRIMARY KEY,
    tenant_id INT NOT NULL,
    email     VARCHAR(255) NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE TABLE products (
    id        INT AUTO_INCREMENT PRIMARY KEY,
    tenant_id INT NOT NULL,
    sku       VARCHAR(255) NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE TABLE orders (
    id      INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    amount  INT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE TABLE order_items (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    order_id   INT NOT NULL,
    product_id INT NOT NULL,
    qty        INT NOT NULL,
    FOREIGN KEY (order_id)   REFERENCES orders(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);
CREATE TABLE sites (
    id   INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);
CREATE TABLE pages (
    site_id INT NOT NULL,
    slug    VARCHAR(255) NOT NULL,
    title   VARCHAR(255),
    PRIMARY KEY (site_id, slug),
    FOREIGN KEY (site_id) REFERENCES sites(id)
);
CREATE TABLE page_comments (
    id      INT AUTO_INCREMENT PRIMARY KEY,
    site_id INT NOT NULL,
    slug    VARCHAR(255) NOT NULL,
    body    TEXT,
    FOREIGN KEY (site_id, slug) REFERENCES pages(site_id, slug)
);
INSERT INTO tenants (id, name) VALUES (1, 'Acme'), (2, 'Beta');
INSERT INTO users (id, tenant_id, email) VALUES
    (1, 1, 'alice@acme.com'),
    (2, 1, 'bob@acme.com'),
    (3, 2, 'dave@beta.com');
INSERT INTO products (id, tenant_id, sku) VALUES
    (1, 1, 'ACME-001'),
    (2, 2, 'BETA-001');
INSERT INTO orders (id, user_id, amount) VALUES
    (1, 1, 100),
    (2, 2, 200);
INSERT INTO order_items (id, order_id, product_id, qty) VALUES
    (1, 1, 1, 2);
INSERT INTO sites (id, name) VALUES (1, 'Main');
INSERT INTO pages (site_id, slug, title) VALUES
    (1, 'home',  'Home'),
    (1, 'about', 'About');
INSERT INTO page_comments (id, site_id, slug, body) VALUES
    (1, 1, 'home', 'Hello');
`

//nolint:paralleltest // mutates schema; cannot run in parallel
func TestWalk_ForwardClosure_MySQL(t *testing.T) {
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
	for stmt := range strings.SplitSeq(mysqlFixtureSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("apply fixture: %v\nstmt: %s", err, stmt)
		}
	}

	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	t.Run("orders seed pulls users and tenants but not children", func(t *testing.T) {
		result, err := extract.Walk(ctx, db, dialect.MySQL{}, schema, "orders", "id = 1")
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		assertRowCount(t, result, "orders", 1)
		assertRowCount(t, result, "users", 1)
		assertRowCount(t, result, "tenants", 1)
		assertRowCount(t, result, "order_items", 0)
		assertRowCount(t, result, "products", 0)
	})

	t.Run("composite FK walks through composite PK", func(t *testing.T) {
		result, err := extract.Walk(ctx, db, dialect.MySQL{}, schema, "page_comments", "id = 1")
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		assertRowCount(t, result, "page_comments", 1)
		assertRowCount(t, result, "pages", 1)
		assertRowCount(t, result, "sites", 1)
	})
}

func assertRowCount(t *testing.T, c *extract.Collected, table string, want int) {
	t.Helper()
	if got := len(c.Rows[table]); got != want {
		t.Errorf("rows[%q] = %d; want %d", table, got, want)
	}
}
