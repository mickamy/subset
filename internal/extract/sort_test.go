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

func TestSortByPK_MultipleSelfRefs(t *testing.T) {
	t.Parallel()

	// employees with two self-ref FKs: manager_id and mentor_id.
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
			{
				Name:              "fk_mentor",
				Columns:           []string{"mentor_id"},
				ReferencedTable:   "employees",
				ReferencedColumns: []string{"id"},
			},
		},
	}
	// Dave: manager=Bob(2), mentor=Alice(1). Bob: manager=Alice(1).
	rows := []extract.Row{
		{"id": 4, "manager_id": 2, "mentor_id": 1},
		{"id": 2, "manager_id": 1, "mentor_id": nil},
		{"id": 1, "manager_id": nil, "mentor_id": nil},
	}
	got, err := extract.SortByPK(rows, table)
	if err != nil {
		t.Fatalf("SortByPK: %v", err)
	}
	posOf := map[any]int{}
	for i, r := range got {
		posOf[r["id"]] = i
	}
	// Alice before Bob and Dave (manager + mentor); Bob before Dave (manager).
	if posOf[1] >= posOf[2] || posOf[1] >= posOf[4] {
		t.Errorf("id=1 must precede id=2 and id=4; got %v", got)
	}
	if posOf[2] >= posOf[4] {
		t.Errorf("id=2 must precede id=4; got %v", got)
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
	rows := []extract.Row{{"id": 1, "a": 1, "b": 2}}
	_, err := extract.SortByPK(rows, table)
	if err == nil {
		t.Fatal("expected error for composite self-ref")
	}
}

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   any
		want any
	}{
		{"int", 1, int64(1)},
		{"int8", int8(1), int64(1)},
		{"int16", int16(1), int64(1)},
		{"int32", int32(1), int64(1)},
		{"int64", int64(1), int64(1)},
		{"uint", uint(1), uint64(1)},
		{"uint8", uint8(1), uint64(1)},
		{"uint16", uint16(1), uint64(1)},
		{"uint32", uint32(1), uint64(1)},
		{"uint64", uint64(1), uint64(1)},
		{"bytes to string", []byte("abc"), "abc"},
		{"string passthrough", "x", "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := extract.NormalizeKey(tc.in); got != tc.want {
				t.Errorf("NormalizeKey(%v (%T)) = %v (%T); want %v (%T)",
					tc.in, tc.in, got, got, tc.want, tc.want)
			}
		})
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
	rows := []extract.Row{{"a": 1, "b": 2, "parent": 1}}
	_, err := extract.SortByPK(rows, table)
	if err == nil {
		t.Fatal("expected error for composite PK with self-ref")
	}
}

func TestSortByPK_NoPKWithSelfRefErrors(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name:       "nodes",
		PrimaryKey: nil,
		ForeignKeys: []introspect.ForeignKey{
			{
				Name:              "fk_self",
				Columns:           []string{"parent"},
				ReferencedTable:   "nodes",
				ReferencedColumns: []string{"id"},
			},
		},
	}
	rows := []extract.Row{{"parent": 1}}
	_, err := extract.SortByPK(rows, table)
	if err == nil {
		t.Fatal("expected error for self-ref table without a primary key")
	}
	if !strings.Contains(err.Error(), "single-column primary key") {
		t.Errorf("err = %v; want single-column primary key message", err)
	}
}

func TestSortByPK_NilPKErrors(t *testing.T) {
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
	rows := []extract.Row{{"id": nil, "manager_id": nil}}
	_, err := extract.SortByPK(rows, table)
	if err == nil {
		t.Fatal("expected error for nil primary key")
	}
	if !strings.Contains(err.Error(), "nil primary key") {
		t.Errorf("err = %v; want nil primary key message", err)
	}
}

func TestSortByPK_EmptyRows_NoValidation(t *testing.T) {
	t.Parallel()

	// Even a table with an unsupported composite PK returns no error when
	// there are no rows to sort — nothing to order, nothing to validate.
	table := introspect.Table{
		Name:       "nodes",
		PrimaryKey: []string{"a", "b"},
		ForeignKeys: []introspect.ForeignKey{
			{Name: "fk", Columns: []string{"parent"}, ReferencedTable: "nodes", ReferencedColumns: []string{"a"}},
		},
	}
	got, err := extract.SortByPK([]extract.Row{}, table)
	if err != nil {
		t.Fatalf("SortByPK on empty rows: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d; want 0", len(got))
	}
}
