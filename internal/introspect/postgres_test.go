package introspect_test

import (
	"testing"

	"github.com/mickamy/subset/internal/introspect"
)

func TestPgKind(t *testing.T) {
	t.Parallel()

	enums := map[string][]string{
		"order_status": {"pending", "paid", "shipped"},
	}

	cases := []struct {
		name     string
		dataType string
		udtName  string
		want     introspect.Kind
	}{
		{"boolean", "boolean", "", introspect.KindBool},
		{"smallint", "smallint", "", introspect.KindInt},
		{"integer", "integer", "", introspect.KindInt},
		{"bigint", "bigint", "", introspect.KindInt},
		{"real", "real", "", introspect.KindFloat},
		{"double precision", "double precision", "", introspect.KindFloat},
		{"numeric", "numeric", "", introspect.KindFloat},
		{"decimal", "decimal", "", introspect.KindFloat},
		{"text", "text", "", introspect.KindString},
		{"character varying", "character varying", "", introspect.KindString},
		{"character", "character", "", introspect.KindString},
		{"name", "name", "", introspect.KindString},
		{"uuid", "uuid", "", introspect.KindUUID},
		{"date", "date", "", introspect.KindDate},
		{"timestamp", "timestamp", "", introspect.KindTimestamp},
		{"timestamp without time zone", "timestamp without time zone", "", introspect.KindTimestamp},
		{"timestamp with time zone", "timestamp with time zone", "", introspect.KindTimestamp},
		{"timestamptz", "timestamptz", "", introspect.KindTimestamp},
		{"time", "time", "", introspect.KindTime},
		{"time without time zone", "time without time zone", "", introspect.KindTime},
		{"time with time zone", "time with time zone", "", introspect.KindTime},
		{"timetz", "timetz", "", introspect.KindTime},
		{"json", "json", "", introspect.KindJSON},
		{"jsonb", "jsonb", "", introspect.KindJSON},
		{"bytea", "bytea", "", introspect.KindBytes},
		{"USER-DEFINED enum", "USER-DEFINED", "order_status", introspect.KindEnum},
		{"USER-DEFINED non-enum", "USER-DEFINED", "unregistered_type", introspect.KindUnknown},
		{"unknown raw type", "geometry", "", introspect.KindUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := introspect.PgKind(tc.dataType, tc.udtName, enums); got != tc.want {
				t.Errorf("PgKind(%q, %q) = %v; want %v", tc.dataType, tc.udtName, got, tc.want)
			}
		})
	}
}
