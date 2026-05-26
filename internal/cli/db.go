package cli

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mickamy/subset/internal/dsn"
)

// openDB opens a *sql.DB for the DSN's scheme, registering the appropriate
// driver via blank import. Postgres uses the pgx stdlib bridge; MySQL uses
// go-sql-driver and the DSN is converted via dsn.ToMySQLDSN.
func openDB(dsnStr string) (*sql.DB, error) {
	scheme := dsn.Scheme(dsnStr)
	var driverName, driverDSN string
	switch scheme {
	case "postgres", "postgresql":
		driverName = "pgx"
		driverDSN = dsnStr
	case "mysql":
		driverName = "mysql"
		converted, err := dsn.ToMySQLDSN(dsnStr)
		if err != nil {
			return nil, fmt.Errorf("convert mysql dsn: %w", err)
		}
		driverDSN = converted
	case "":
		return nil, dsn.ErrMissingScheme
	default:
		return nil, fmt.Errorf("unsupported DSN scheme %q", scheme)
	}

	db, err := sql.Open(driverName, driverDSN)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	return db, nil
}
