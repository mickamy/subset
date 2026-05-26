package extract_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mickamy/subset/internal/extract"
	"github.com/mickamy/subset/internal/introspect"
)

func TestSortByPK_NoSelfRef_ReturnsInput(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    []introspect.Column{{Name: "id"}, {Name: "name"}},
	}
	rows := []extract.Row{
		{"id": 3, "name": "C"},
		{"id": 1, "name": "A"},
	}
	got, err := extract.SortByPK(rows, table)
	if err != nil {
		t.Fatalf("SortByPK: %v", err)
	}
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("rows changed; want input unchanged when no self-ref")
	}
}

func TestSortByPK_SelfRef_ParentsBeforeChildren(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "employees",
		PrimaryKey: []string{"id"},
		Columns:    []introspect.Column{{Name: "id"}, {Name: "name"}, {Name: "manager_id"}},
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_manager",
				Columns:           []string{"manager_id"},
				ReferencedTable:   "employees",
				ReferencedColumns: []string{"id"},
			},
		},
	}
	// Input order: leaves first, root last.
	rows := []extract.Row{
		{"id": 4, "manager_id": 2}, // Dave -> Bob
		{"id": 2, "manager_id": 1}, // Bob  -> Alice
		{"id": 1, "manager_id": nil},
	}
	got, err := extract.SortByPK(rows, table)
	if err != nil {
		t.Fatalf("SortByPK: %v", err)
	}
	gotIDs := make([]any, 0, len(got))
	for _, r := range got {
		gotIDs = append(gotIDs, r["id"])
	}
	want := []any{1, 2, 4}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("order = %v; want %v", gotIDs, want)
	}
}

func TestSortByPK_SelfRef_BranchingTree(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "employees",
		PrimaryKey: []string{"id"},
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_manager",
				Columns:           []string{"manager_id"},
				ReferencedTable:   "employees",
				ReferencedColumns: []string{"id"},
			},
		},
	}
	rows := []extract.Row{
		{"id": 2, "manager_id": 1}, // Bob   -> Alice
		{"id": 3, "manager_id": 1}, // Carol -> Alice
		{"id": 4, "manager_id": 2}, // Dave  -> Bob
		{"id": 1, "manager_id": nil},
	}
	got, err := extract.SortByPK(rows, table)
	if err != nil {
		t.Fatalf("SortByPK: %v", err)
	}
	posOf := map[any]int{}
	for i, r := range got {
		posOf[r["id"]] = i
	}
	if posOf[1] >= posOf[2] || posOf[1] >= posOf[3] {
		t.Errorf("id=1 must come before id=2 and id=3; got %v", got)
	}
	if posOf[2] >= posOf[4] {
		t.Errorf("id=2 must come before id=4; got %v", got)
	}
}

func TestSortByPK_SelfRef_DataCycleErrors(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "employees",
		PrimaryKey: []string{"id"},
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_manager",
				Columns:           []string{"manager_id"},
				ReferencedTable:   "employees",
				ReferencedColumns: []string{"id"},
			},
		},
	}
	// A -> B -> A cycle in data.
	rows := []extract.Row{
		{"id": 1, "manager_id": 2},
		{"id": 2, "manager_id": 1},
	}
	_, err := extract.SortByPK(rows, table)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cyclic") {
		t.Errorf("err = %v; want cyclic error", err)
	}
}

func TestSortByPK_SelfRef_ExternalParentIsFine(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "employees",
		PrimaryKey: []string{"id"},
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_manager",
				Columns:           []string{"manager_id"},
				ReferencedTable:   "employees",
				ReferencedColumns: []string{"id"},
			},
		},
	}
	// manager_id=99 is not in the collected slice (parent already exists in target DB).
	rows := []extract.Row{
		{"id": 5, "manager_id": 99},
	}
	got, err := extract.SortByPK(rows, table)
	if err != nil {
		t.Fatalf("SortByPK: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d; want 1", len(got))
	}
}

func TestSortByPK_CompositeSelfRefErrors(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "nodes",
		PrimaryKey: []string{"id"},
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_parent",
				Columns:           []string{"a", "b"},
				ReferencedTable:   "nodes",
				ReferencedColumns: []string{"x", "y"},
			},
		},
	}
	_, err := extract.SortByPK(nil, table)
	if err == nil {
		t.Fatal("expected error for composite self-ref")
	}
}

func TestSortByPK_CompositePKErrors(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "nodes",
		PrimaryKey: []string{"a", "b"},
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_self",
				Columns:           []string{"parent"},
				ReferencedTable:   "nodes",
				ReferencedColumns: []string{"a"},
			},
		},
	}
	_, err := extract.SortByPK(nil, table)
	if err == nil {
		t.Fatal("expected error for composite PK with self-ref")
	}
}
