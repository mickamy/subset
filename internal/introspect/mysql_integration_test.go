//go:build integration

package introspect_test

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/mickamy/subset/internal/dsn"
	"github.com/mickamy/subset/internal/introspect"
	"github.com/mickamy/subset/internal/test/tsql"
)

const mysqlSchemaSQL = `
CREATE TABLE users (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    email      VARCHAR(255) NOT NULL UNIQUE,
    name       VARCHAR(255),
    bio        TEXT,
    is_active  TINYINT(1)   NOT NULL DEFAULT 1,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE orders (
    id       INT AUTO_INCREMENT PRIMARY KEY,
    user_id  INT NOT NULL,
    status   ENUM('pending','paid','shipped') NOT NULL,
    amount   INT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE comments (
    id       INT AUTO_INCREMENT PRIMARY KEY,
    user_id  INT NOT NULL,
    order_id INT,
    body     TEXT,
    FOREIGN KEY (user_id)  REFERENCES users(id),
    FOREIGN KEY (order_id) REFERENCES orders(id)
)
`

// Set SUBSET_TEST_DSN_MYSQL=mysql://... to run; otherwise the test is skipped.
//
//nolint:paralleltest,tparallel // mutates schema; cannot run in parallel
func TestIntrospect_MySQL(t *testing.T) {
	rawDSN := os.Getenv("SUBSET_TEST_DSN_MYSQL")
	if rawDSN == "" {
		t.Skip("SUBSET_TEST_DSN_MYSQL not set")
	}

	driverDSN, err := dsn.ToMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("ToMySQLDSN: %v", err)
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := t.Context()
	if err := applyMySQLSchema(ctx, db, mysqlSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	schema, err := introspect.Do(ctx, rawDSN)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}

	gotTables := make([]string, 0, len(schema.Tables))
	for _, table := range schema.Tables {
		gotTables = append(gotTables, table.Name)
	}
	slices.Sort(gotTables)
	wantTables := []string{"comments", "orders", "users"}
	if !reflect.DeepEqual(gotTables, wantTables) {
		t.Errorf("tables = %v; want %v", gotTables, wantTables)
	}

	users := findTable(t, schema, "users")
	idCol := findColumn(t, users, "id")
	if idCol.Kind != introspect.KindInt {
		t.Errorf("users.id Kind = %v; want KindInt", idCol.Kind)
	}
	if !idCol.IsIdentity {
		t.Errorf("users.id IsIdentity = false; want true (AUTO_INCREMENT)")
	}
	if !idCol.IsUnique {
		t.Errorf("users.id IsUnique = false; want true (single-column PK)")
	}

	emailCol := findColumn(t, users, "email")
	if emailCol.Kind != introspect.KindString {
		t.Errorf("users.email Kind = %v; want KindString", emailCol.Kind)
	}
	if emailCol.Nullable {
		t.Errorf("users.email Nullable = true; want false")
	}
	if emailCol.MaxLength != 255 {
		t.Errorf("users.email MaxLength = %d; want 255 (VARCHAR(255))", emailCol.MaxLength)
	}

	isActiveCol := findColumn(t, users, "is_active")
	if isActiveCol.Kind != introspect.KindBool {
		t.Errorf("users.is_active Kind = %v; want KindBool (tinyint(1))", isActiveCol.Kind)
	}

	createdAtCol := findColumn(t, users, "created_at")
	if createdAtCol.Kind != introspect.KindTimestamp {
		t.Errorf("users.created_at Kind = %v; want KindTimestamp", createdAtCol.Kind)
	}
	if createdAtCol.IsGenerated {
		t.Errorf("users.created_at IsGenerated = true; want false (DEFAULT CURRENT_TIMESTAMP is not a generated column)")
	}

	orders := findTable(t, schema, "orders")
	statusCol := findColumn(t, orders, "status")
	if statusCol.Kind != introspect.KindEnum {
		t.Errorf("orders.status Kind = %v; want KindEnum", statusCol.Kind)
	}
	wantEnum := []string{"pending", "paid", "shipped"}
	if !reflect.DeepEqual(statusCol.EnumValues, wantEnum) {
		t.Errorf("orders.status EnumValues = %v; want %v", statusCol.EnumValues, wantEnum)
	}

	if len(orders.ForeignKeys) != 1 {
		t.Fatalf("orders FKs = %d; want 1", len(orders.ForeignKeys))
	}
	fk := orders.ForeignKeys[0]
	if fk.ReferencedTable != "users" {
		t.Errorf("orders FK ref table = %q; want users", fk.ReferencedTable)
	}

	comments := findTable(t, schema, "comments")
	if len(comments.ForeignKeys) != 2 {
		t.Fatalf("comments FKs = %d; want 2", len(comments.ForeignKeys))
	}
	commentFKTables := make([]string, 0, len(comments.ForeignKeys))
	for _, fk := range comments.ForeignKeys {
		commentFKTables = append(commentFKTables, fk.ReferencedTable)
	}
	if !slices.Contains(commentFKTables, "users") || !slices.Contains(commentFKTables, "orders") {
		t.Errorf("comments FK tables = %v; want to include both users and orders", commentFKTables)
	}
}

func applyMySQLSchema(ctx context.Context, db *sql.DB, schemaSQL string) error {
	if err := tsql.ResetMySQLTables(ctx, db); err != nil {
		return err
	}
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
