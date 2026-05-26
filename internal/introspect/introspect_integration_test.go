//go:build integration

package introspect_test

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mickamy/subset/internal/introspect"
)

const schemaSQL = `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;

CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped');

CREATE TABLE users (
    id         serial      PRIMARY KEY,
    email      text        NOT NULL,
    name       text,
    username   varchar(32),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id      serial       PRIMARY KEY,
    user_id int          NOT NULL REFERENCES users(id),
    status  order_status NOT NULL,
    amount  int
);

CREATE TABLE comments (
    id       serial PRIMARY KEY,
    user_id  int    NOT NULL REFERENCES users(id),
    order_id int    REFERENCES orders(id),
    body     text
);
`

// Set SUBSET_TEST_DSN_POSTGRES=postgres://... to run; otherwise the test is skipped.
//
//nolint:paralleltest,tparallel // mutates the public schema; cannot run in parallel
func TestIntrospect(t *testing.T) {
	dsn := os.Getenv("SUBSET_TEST_DSN_POSTGRES")
	if dsn == "" {
		t.Skip("SUBSET_TEST_DSN_POSTGRES not set")
	}

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	schema, err := introspect.Do(ctx, dsn)
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}

	gotTables := make([]string, 0, len(schema.Tables))
	for _, table := range schema.Tables {
		gotTables = append(gotTables, table.Name)
	}
	wantTables := []string{"comments", "orders", "users"}
	if !reflect.DeepEqual(gotTables, wantTables) {
		t.Errorf("tables = %v; want %v", gotTables, wantTables)
	}

	users := findTable(t, schema, "users")
	gotCols := make([]string, 0, len(users.Columns))
	for _, c := range users.Columns {
		gotCols = append(gotCols, c.Name)
	}
	wantCols := []string{"id", "email", "name", "username", "created_at"}
	if !reflect.DeepEqual(gotCols, wantCols) {
		t.Errorf("users columns = %v; want %v", gotCols, wantCols)
	}
	if !reflect.DeepEqual(users.PrimaryKey, []string{"id"}) {
		t.Errorf("users PK = %v; want [id]", users.PrimaryKey)
	}

	idCol := findColumn(t, users, "id")
	if idCol.Default == nil || !strings.HasPrefix(*idCol.Default, "nextval(") {
		t.Errorf("users.id Default = %v; want non-nil with prefix nextval(", idCol.Default)
	}
	if !idCol.IsUnique {
		t.Errorf("users.id IsUnique = false; want true (single-column PK)")
	}
	nameCol := findColumn(t, users, "name")
	if !nameCol.Nullable {
		t.Errorf("users.name Nullable = false; want true")
	}
	emailCol := findColumn(t, users, "email")
	if emailCol.Nullable {
		t.Errorf("users.email Nullable = true; want false")
	}
	if emailCol.MaxLength != 0 {
		t.Errorf("users.email MaxLength = %d; want 0 (text is unlimited on Postgres)", emailCol.MaxLength)
	}
	usernameCol := findColumn(t, users, "username")
	if usernameCol.MaxLength != 32 {
		t.Errorf("users.username MaxLength = %d; want 32 (varchar(32))", usernameCol.MaxLength)
	}

	orders := findTable(t, schema, "orders")
	statusCol := findColumn(t, orders, "status")
	if statusCol.DataType != "USER-DEFINED" {
		t.Errorf("orders.status DataType = %q; want USER-DEFINED", statusCol.DataType)
	}
	if statusCol.UDTName != "order_status" {
		t.Errorf("orders.status UDTName = %q; want order_status", statusCol.UDTName)
	}
	if statusCol.Kind != introspect.KindEnum {
		t.Errorf("orders.status Kind = %d; want KindEnum", statusCol.Kind)
	}

	if idCol.Kind != introspect.KindInt {
		t.Errorf("users.id Kind = %d; want KindInt", idCol.Kind)
	}
	if emailCol.Kind != introspect.KindString {
		t.Errorf("users.email Kind = %d; want KindString", emailCol.Kind)
	}
	createdAtCol := findColumn(t, users, "created_at")
	if createdAtCol.Kind != introspect.KindTimestamp {
		t.Errorf("users.created_at Kind = %d; want KindTimestamp", createdAtCol.Kind)
	}

	if len(orders.ForeignKeys) != 1 {
		t.Fatalf("orders FKs = %d; want 1", len(orders.ForeignKeys))
	}
	fk := orders.ForeignKeys[0]
	if fk.ReferencedTable != "users" {
		t.Errorf("orders FK ref table = %q; want users", fk.ReferencedTable)
	}
	if !reflect.DeepEqual(fk.Columns, []string{"user_id"}) {
		t.Errorf("orders FK cols = %v; want [user_id]", fk.Columns)
	}
	if !reflect.DeepEqual(fk.ReferencedColumns, []string{"id"}) {
		t.Errorf("orders FK ref cols = %v; want [id]", fk.ReferencedColumns)
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

	wantVals := []string{"pending", "paid", "shipped"}
	if !reflect.DeepEqual(statusCol.EnumValues, wantVals) {
		t.Errorf("orders.status EnumValues = %v; want %v", statusCol.EnumValues, wantVals)
	}
}

func findTable(t *testing.T, schema introspect.Schema, name string) introspect.Table {
	t.Helper()
	for _, tbl := range schema.Tables {
		if tbl.Name == name {
			return tbl
		}
	}
	t.Fatalf("table not found: %s", name)

	return introspect.Table{}
}

func findColumn(t *testing.T, table introspect.Table, name string) introspect.Column {
	t.Helper()
	for _, c := range table.Columns {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("column %s.%s not found", table.Name, name)

	return introspect.Column{}
}
