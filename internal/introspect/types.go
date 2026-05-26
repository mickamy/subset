package introspect

type Schema struct {
	Tables []Table
}

type Table struct {
	Name        string
	Columns     []Column
	PrimaryKey  []string
	ForeignKeys []ForeignKey
	// CompositeUniques lists multi-column UNIQUE constraints (single-column
	// UNIQUE constraints are surfaced via Column.IsUnique). Each entry is the
	// ordered list of columns participating in one constraint.
	CompositeUniques [][]string
}

type Column struct {
	Name string
	// DataType is the raw type name as returned by the driver
	// (e.g., "int", "enum" for MySQL; "integer", "USER-DEFINED" for Postgres).
	DataType string
	// UDTName names the underlying user-defined type when relevant.
	// On MySQL it is empty (enum labels live in EnumValues).
	// On Postgres it matches the pg_type name when DataType == "USER-DEFINED".
	UDTName string
	// EnumValues holds the labels for an enum column; nil for non-enum columns.
	EnumValues []string
	Kind       Kind
	Nullable   bool
	// Default is the raw column_default expression; nil when the column has
	// no default. A non-nil empty string is a valid value (e.g., MySQL
	// `DEFAULT ''`).
	Default    *string
	IsIdentity bool
	// IsGenerated is true for columns the database computes itself
	// (Postgres GENERATED ALWAYS AS, MySQL VIRTUAL / STORED GENERATED).
	// Writes that target generated columns are rejected by the database
	// (Postgres 428C9, MySQL Error 3105).
	IsGenerated bool
	// IsUnique is set only for single-column UNIQUE constraints.
	IsUnique bool
	// MaxLength is character_maximum_length for length-limited character
	// types (e.g., char, varchar). Zero means unspecified or unlimited;
	// for example, Postgres text reports NULL here and decodes to 0.
	MaxLength int
}

type ForeignKey struct {
	Name              string
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
}

// Kind is a driver-agnostic classification of a column's value space,
// abstracting over the raw type names reported by MySQL or Postgres.
type Kind int

const (
	KindUnknown Kind = iota
	KindBool
	KindInt
	KindFloat
	KindString
	KindUUID
	KindDate
	KindTime
	KindTimestamp
	KindJSON
	KindEnum
	KindBytes
)

func (k Kind) String() string {
	switch k {
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindUUID:
		return "uuid"
	case KindDate:
		return "date"
	case KindTime:
		return "time"
	case KindTimestamp:
		return "timestamp"
	case KindJSON:
		return "json"
	case KindEnum:
		return "enum"
	case KindBytes:
		return "bytes"
	case KindUnknown:
		return "unknown"
	}

	return "unknown"
}
