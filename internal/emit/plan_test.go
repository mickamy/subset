package emit_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/mickamy/subset/internal/emit"
	"github.com/mickamy/subset/internal/introspect"
)

// fixture mirrors the integration schema: orders -> users -> tenants, plus
// products and order_items so a table (tenants) is reachable by two paths.
func planFixture() introspect.Schema {
	return introspect.Schema{Tables: []introspect.Table{
		{Name: "tenants", PrimaryKey: []string{"id"}},
		{
			Name:        "users",
			PrimaryKey:  []string{"id"},
			ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "tenants"}},
		},
		{
			Name:        "products",
			PrimaryKey:  []string{"id"},
			ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "tenants"}},
		},
		{
			Name:        "orders",
			PrimaryKey:  []string{"id"},
			ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "users"}},
		},
		{
			Name:       "order_items",
			PrimaryKey: []string{"id"},
			ForeignKeys: []introspect.ForeignKey{
				{ReferencedTable: "orders"},
				{ReferencedTable: "products"},
			},
		},
	}}
}

func TestWritePlan_CloneForward(t *testing.T) {
	t.Parallel()

	counts := map[string]int{"tenants": 1, "users": 1, "orders": 1}
	order := []string{"tenants", "users", "orders"} // parents-first

	var buf bytes.Buffer
	emit.WritePlan(&buf, "clone", true, planFixture(), counts, order, "orders", 12*time.Millisecond)

	want := "subset clone: 3 tables, 3 rows in 12ms\n" +
		"  tenants  1 row  via orders → users → tenants\n" +
		"  users    1 row  via orders → users\n" +
		"  orders   1 row  seed\n"
	if got := buf.String(); got != want {
		t.Errorf("WritePlan =\n%q\nwant\n%q", got, want)
	}
}

func TestWritePlan_CloneForward_MultiPath(t *testing.T) {
	t.Parallel()

	// Clone seeded at order_items reaches tenants via products (shortest) and
	// via orders -> users, so tenants shows "(+1 more path)".
	counts := map[string]int{"tenants": 1, "users": 1, "products": 1, "orders": 1, "order_items": 1}
	order := []string{"tenants", "users", "products", "orders", "order_items"}

	var buf bytes.Buffer
	emit.WritePlan(&buf, "clone", true, planFixture(), counts, order, "order_items", 20*time.Millisecond)

	want := "subset clone: 5 tables, 5 rows in 20ms\n" +
		"  tenants      1 row  via order_items → products → tenants (+1 more path)\n" +
		"  users        1 row  via order_items → orders → users\n" +
		"  products     1 row  via order_items → products\n" +
		"  orders       1 row  via order_items → orders\n" +
		"  order_items  1 row  seed\n"
	if got := buf.String(); got != want {
		t.Errorf("WritePlan =\n%q\nwant\n%q", got, want)
	}
}

func TestWritePlan_DeleteBackward(t *testing.T) {
	t.Parallel()

	// Delete seeded at tenants walks to referencing children, children-first.
	counts := map[string]int{"tenants": 1, "users": 2, "products": 1, "orders": 1, "order_items": 1}
	order := []string{"order_items", "orders", "products", "users", "tenants"}

	var buf bytes.Buffer
	emit.WritePlan(&buf, "delete", false, planFixture(), counts, order, "tenants", 18*time.Millisecond)

	want := "subset delete: 5 tables, 6 rows in 18ms\n" +
		"  order_items  1 row   via tenants → products → order_items (+1 more path)\n" +
		"  orders       1 row   via tenants → users → orders\n" +
		"  products     1 row   via tenants → products\n" +
		"  users        2 rows  via tenants → users\n" +
		"  tenants      1 row   seed\n"
	if got := buf.String(); got != want {
		t.Errorf("WritePlan =\n%q\nwant\n%q", got, want)
	}
}

func TestSummaryComment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cmd     string
		rows    int
		tables  int
		forward bool
		want    string
	}{
		{"clone", 3, 3, true, "-- subset clone: 3 rows from 3 tables, parents-first"},
		{"delete", 6, 5, false, "-- subset delete: 6 rows from 5 tables, children-first"},
		{"clone", 1, 1, true, "-- subset clone: 1 row from 1 table, parents-first"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			t.Parallel()
			if got := emit.SummaryComment(tc.cmd, tc.rows, tc.tables, tc.forward); got != tc.want {
				t.Errorf("SummaryComment = %q; want %q", got, tc.want)
			}
		})
	}
}
