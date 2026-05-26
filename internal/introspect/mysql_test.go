package introspect_test

import (
	"reflect"
	"testing"

	"github.com/mickamy/subset/internal/introspect"
)

func TestMySQLKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		dataType   string
		columnType string
		wantKind   introspect.Kind
		wantEnum   []string
	}{
		{"tinyint(1) is bool", "tinyint", "tinyint(1)", introspect.KindBool, nil},
		{"tinyint(1) unsigned is bool", "tinyint", "tinyint(1) unsigned", introspect.KindBool, nil},
		{"tinyint(4) is int", "tinyint", "tinyint(4)", introspect.KindInt, nil},
		{"tinyint without width is int", "tinyint", "tinyint", introspect.KindInt, nil},
		{"int", "int", "int(11)", introspect.KindInt, nil},
		{"bigint", "bigint", "bigint(20)", introspect.KindInt, nil},
		{"varchar is string", "varchar", "varchar(255)", introspect.KindString, nil},
		{"text is string", "text", "text", introspect.KindString, nil},
		{"date", "date", "date", introspect.KindDate, nil},
		{"timestamp", "timestamp", "timestamp", introspect.KindTimestamp, nil},
		{"datetime", "datetime", "datetime", introspect.KindTimestamp, nil},
		{"json", "json", "json", introspect.KindJSON, nil},
		{"varbinary is bytes", "varbinary", "varbinary(64)", introspect.KindBytes, nil},
		{"bit(1) is bool", "bit", "bit(1)", introspect.KindBool, nil},
		{"bit(8) is bytes", "bit", "bit(8)", introspect.KindBytes, nil},
		{"enum", "enum", "enum('a','b','c')", introspect.KindEnum, []string{"a", "b", "c"}},
		{"unknown", "geometry", "geometry", introspect.KindUnknown, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotKind, gotEnum := introspect.MySQLKind(tc.dataType, tc.columnType)
			if gotKind != tc.wantKind {
				t.Errorf("kind = %v; want %v", gotKind, tc.wantKind)
			}
			if !reflect.DeepEqual(gotEnum, tc.wantEnum) {
				t.Errorf("enum = %v; want %v", gotEnum, tc.wantEnum)
			}
		})
	}
}

func TestParseMySQLEnumLabels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "enum('a','b','c')", []string{"a", "b", "c"}},
		{"set", "set('x','y')", []string{"x", "y"}},
		{"escaped quote", "enum('it''s','ok')", []string{"it's", "ok"}},
		{"single label", "enum('only')", []string{"only"}},
		{"no labels", "enum()", nil},
		{"non-enum", "varchar(10)", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := introspect.ParseMySQLEnumLabels(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseMySQLEnumLabels(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}
