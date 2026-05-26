package introspect

import (
	"context"
	"fmt"

	"github.com/mickamy/subset/internal/dsn"
)

type Driver interface {
	Close(ctx context.Context) error
	Introspect(ctx context.Context) (Schema, error)
}

// uniquesInfo carries the per-table outcome of scanning UNIQUE constraints:
// single-column constraints flip Column.IsUnique, composite constraints feed
// Table.CompositeUniques. Both driver implementations populate this shape
// from one information_schema query.
type uniquesInfo struct {
	single    map[string]map[string]bool
	composite map[string][][]string
}

func Do(ctx context.Context, dataSourceName string) (Schema, error) {
	drv, err := openDriver(ctx, dataSourceName)
	if err != nil {
		return Schema{}, err
	}
	defer func() { _ = drv.Close(ctx) }()

	schema, err := drv.Introspect(ctx)
	if err != nil {
		return Schema{}, fmt.Errorf("introspect: %w", err)
	}

	return schema, nil
}

func openDriver(ctx context.Context, dataSourceName string) (Driver, error) {
	switch dsn.Scheme(dataSourceName) {
	case "mysql":
		return newMySQLDriver(ctx, dataSourceName)
	case "postgres", "postgresql":
		return newPostgresDriver(ctx, dataSourceName)
	case "":
		return nil, dsn.ErrMissingScheme
	default:
		scheme := dsn.Scheme(dataSourceName)

		return nil, fmt.Errorf("unsupported DSN scheme %q (supported: mysql, postgres, postgresql)", scheme)
	}
}
