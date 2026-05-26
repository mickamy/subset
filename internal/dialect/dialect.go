package dialect

import (
	"fmt"

	"github.com/mickamy/subset/internal/dsn"
)

type Dialect interface {
	QuoteIdent(name string) string
	QuoteLiteral(v any) string
	Placeholder(n int) string
}

func New(dataSourceName string) (Dialect, error) {
	switch dsn.Scheme(dataSourceName) {
	case "mysql":
		return MySQL{}, nil
	case "postgres", "postgresql":
		return Postgres{}, nil
	case "":
		return nil, dsn.ErrMissingScheme
	default:
		scheme := dsn.Scheme(dataSourceName)

		return nil, fmt.Errorf("unsupported DSN scheme %q (supported: mysql, postgres, postgresql)", scheme)
	}
}
