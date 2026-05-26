package plan_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mickamy/subset/internal/introspect"
	"github.com/mickamy/subset/internal/plan"
)

func TestBuild_LinearChain(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "users"},
		{Name: "orders", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "users"}}},
		{Name: "items", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "orders"}}},
	}
	got, err := plan.Build(tables)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"users", "orders", "items"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v; want %v", got, want)
	}
}

func TestBuild_NoFKs(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}
	got, err := plan.Build(tables)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v; want %v (input order preserved)", got, want)
	}
}

func TestBuild_SelfReferenceIgnored(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "employees", ForeignKeys: []introspect.ForeignKey{
			{Name: "manager_fk", ReferencedTable: "employees"},
		}},
	}
	got, err := plan.Build(tables)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"employees"}) {
		t.Errorf("order = %v; want [employees]", got)
	}
}

func TestBuild_Cycle(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "a", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "b"}}},
		{Name: "b", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "a"}}},
	}
	_, err := plan.Build(tables)
	if err == nil {
		t.Fatal("Build returned nil error; want cycle error")
	}
	if !strings.Contains(err.Error(), "cyclic") {
		t.Errorf("err = %v; want error containing 'cyclic'", err)
	}
}

func TestBuild_Diamond(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "a"},
		{Name: "b", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "a"}}},
		{Name: "c", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "a"}}},
		{Name: "d", ForeignKeys: []introspect.ForeignKey{
			{ReferencedTable: "b"},
			{ReferencedTable: "c"},
		}},
	}
	got, err := plan.Build(tables)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	posOf := make(map[string]int, len(got))
	for i, n := range got {
		posOf[n] = i
	}
	if posOf["a"] >= posOf["b"] || posOf["a"] >= posOf["c"] {
		t.Errorf("a must come before b and c; got %v", got)
	}
	if posOf["b"] >= posOf["d"] || posOf["c"] >= posOf["d"] {
		t.Errorf("b and c must come before d; got %v", got)
	}
}

func TestBuild_IgnoresOutOfScopeFKs(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "a", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "external"}}},
	}
	got, err := plan.Build(tables)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("order = %v; want [a]", got)
	}
}

func TestBuild_AllTablesIncluded(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "users"},
		{Name: "orders", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "users"}}},
		{Name: "comments", ForeignKeys: []introspect.ForeignKey{
			{ReferencedTable: "users"},
			{ReferencedTable: "orders"},
		}},
	}
	got, err := plan.Build(tables)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, name := range []string{"users", "orders", "comments"} {
		if !slices.Contains(got, name) {
			t.Errorf("missing %q in order %v", name, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("len(order) = %d; want 3", len(got))
	}
}

func TestBuild_LongerCycle(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{
		{Name: "a", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "b"}}},
		{Name: "b", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "c"}}},
		{Name: "c", ForeignKeys: []introspect.ForeignKey{{ReferencedTable: "a"}}},
	}
	_, err := plan.Build(tables)
	if err == nil {
		t.Fatal("Build returned nil error; want cycle error")
	}
	if !strings.Contains(err.Error(), "cyclic") {
		t.Errorf("err = %v; want cyclic error", err)
	}
}
