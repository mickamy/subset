# subset

> Carve a referentially-consistent slice out of your database — one command,
> plain SQL, read-only source.

[![CI](https://github.com/mickamy/subset/actions/workflows/ci.yaml/badge.svg)](https://github.com/mickamy/subset/actions/workflows/ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mickamy/subset)](https://goreportcard.com/report/github.com/mickamy/subset)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![GitHub Sponsors](https://img.shields.io/github/sponsors/mickamy?label=sponsor&logo=github)](https://github.com/sponsors/mickamy)

`subset` extracts a foreign-key-consistent subset of your MySQL or Postgres
database as plain SQL. Point it at a DSN and a seed (`<table> --where ...`) and
it introspects the schema, walks the foreign-key graph, and emits ordered
`INSERT` or `DELETE` statements to stdout. It never modifies the source DB — it
only runs read-only `SELECT`s. You decide where the SQL goes: pipe it straight
into another database, or save it to a file.

```bash
# Clone an order and everything it depends on into a fresh dev database.
$ subset clone postgres://user:pass@prod:5432/app orders --id 42 | psql "$DEV_URL"

# Preview the closure without emitting SQL.
$ subset clone postgres://user:pass@prod:5432/app orders --id 42 --plan
subset clone: 3 tables, 3 rows in 14ms
  tenants  1 row  via orders → users → tenants
  users    1 row  via orders → users
  orders   1 row  seed

# Delete a tenant and every row that depends on it, in FK-safe order.
$ subset delete postgres://user:pass@localhost:5432/app tenants --id 7 > cleanup.sql
```

## Why

You often need *part* of a database, kept consistent: one customer's data for a
repro, a single order graph for a test fixture, or a clean teardown of a tenant
that won't trip a foreign-key constraint. Doing this by hand means hand-writing
`SELECT`s and ordering `INSERT`s/`DELETE`s yourself, getting the FK direction
right every time.

`subset` does the graph walk for you:

1. **Referentially consistent.** `clone` follows FKs to parents so every
   `INSERT` lands after the rows it references. `delete` follows them to
   children so every `DELETE` runs before the row it depends on.
2. **Read-only source, plain SQL out.** The source DB only sees `SELECT`s. The
   output is ordinary SQL you can read, diff, and pipe to `psql` / `mysql`.
3. **Both engines.** PostgreSQL and MySQL, through the standard DSN form.

## Install

### Homebrew (macOS / Linux)

```bash
brew install mickamy/tap/subset
```

### Windows

Grab the latest `subset_<version>_windows_<arch>.zip` from the
[Releases](https://github.com/mickamy/subset/releases) page, unzip, and move
`subset.exe` into a directory already on your `PATH` (e.g.,
`%USERPROFILE%\bin`). The PowerShell snippet below picks the right archive for
the current host arch and unpacks it next to the working directory; you still
need the final move step yourself.

```powershell
$ver  = (Invoke-RestMethod https://api.github.com/repos/mickamy/subset/releases/latest).tag_name.TrimStart('v')
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
Invoke-WebRequest -OutFile subset.zip "https://github.com/mickamy/subset/releases/latest/download/subset_${ver}_windows_${arch}.zip"
Expand-Archive subset.zip -DestinationPath .\subset -Force
# Move .\subset\subset.exe into a directory on $env:PATH (e.g., $HOME\bin).
```

### From source

```bash
go install github.com/mickamy/subset@latest
```

Requires Go 1.26+ to build from source. Pre-built binaries (macOS / Linux /
Windows × amd64 / arm64) are published on
[GitHub Releases](https://github.com/mickamy/subset/releases) on every tag.

## Supported databases

- **PostgreSQL** — connect with a `postgres://` or `postgresql://` DSN.
- **MySQL** — connect with a `mysql://` DSN.

Both drivers read the schema from `information_schema` / `pg_catalog` and the
seed rows with ordinary `SELECT`s; nothing else touches the source. CI
exercises PostgreSQL and MySQL on every push.

## Usage

```
subset <command> <dsn> <table> [flags]

COMMANDS:
  clone   Pull rows plus their FK parents as INSERT statements
  delete  Remove rows plus their FK-dependent rows in safe order

FLAGS:
  --where <expr>   WHERE clause selecting the seed rows (e.g., "id=42")
  --id <value>     Shortcut for --where "id=<value>"
  --plan           Print row counts and FK paths; emit no rows
  --version, -v    Print subset version
  --help, -h       Show this help
```

`subset` writes SQL to stdout and progress/errors to stderr, so redirects and
pipes carry only the SQL.

### Clone: pull a row and its parents

`clone` starts at the seed rows and walks **forward** along foreign keys,
collecting every parent row the seed transitively references. It emits
`INSERT`s parents-first, so the output applies cleanly to a database that has
the schema but not the data.

```bash
# One order and its FK parents (user, tenant, ...).
subset clone postgres://user:pass@host:5432/app orders --id 42 | psql "$DEV_URL"

# Several rows via a WHERE clause.
subset clone mysql://root:pass@host:3306/app users --where "status = 'active'" > users.sql
```

### Delete: remove a row and its dependents

`delete` starts at the seed rows and walks **backward**, collecting every row
that transitively references the seed. It emits `DELETE`s most-dependent-first,
so applying them never violates a foreign key.

```bash
# A tenant and everything under it.
subset delete postgres://user:pass@host:5432/app tenants --id 7 | psql "$APP_URL"

# Save first, review, then apply.
subset delete postgres://... users --where "email LIKE '%@example.com'" > cleanup.sql
```

### `--plan`: see the closure before committing to SQL

`--plan` prints a summary to stdout instead of SQL: the tables involved, their
row counts, and a representative shortest FK path from the seed. A table
reachable by more than one path shows `(+N more path)`.

```bash
$ subset delete postgres://... tenants --id 7 --plan
subset delete: 5 tables, 7 rows in 18ms
  order_items  1 row   via tenants → products → order_items (+1 more path)
  products     1 row   via tenants → products
  orders       2 rows  via tenants → users → orders
  users        2 rows  via tenants → users
  tenants      1 row   seed
```

## How it works

1. **Introspect.** Read tables, columns, primary keys, and foreign keys from
   the source schema.
2. **Walk the FK graph.** From the seed rows, BFS in the chosen direction —
   forward to FK parents for `clone`, backward to referencing children for
   `delete` — fetching each adjacent row with a `SELECT`. Rows are deduplicated
   by primary key, so shared parents (or diamond-shaped graphs) are visited
   once.
3. **Order.** Tables are topologically sorted by their FK dependencies:
   parents-first for `clone`, the reverse for `delete`. Within a
   self-referencing table (e.g., `employees.manager_id`), rows are ordered so a
   referenced row is emitted before the row that points at it (reversed for
   `delete`).
4. **Emit.** One `INSERT` or `DELETE` per row, with a leading summary comment.
   Generated columns are skipped on `INSERT`; identifiers and literals are
   quoted per dialect (`"ident"` for Postgres, `` `ident` `` for MySQL).

Composite primary keys, composite foreign keys, and single-column
self-references are all supported.

## Current scope (v0.1)

`subset` v0.1 is deliberately small. Not yet included (planned for later):

- `clone --include-children` (opt-in backward closure during clone).
- Output formats other than SQL (`--format yaml`, etc.).
- ID remapping / sequence bumping (`--remap-ids`).
- Column masking / skipping (`--mask`, `--skip-columns`).
- Selecting schema/database via a CLI flag (use DSN parameters instead, e.g.,
  `?search_path=` on Postgres).
- `ON DELETE CASCADE / SET NULL` awareness — `delete` always emits explicit
  `DELETE`s regardless of the constraint's action.
- Composite self-referencing foreign keys and self-referencing data cycles
  (`A → B → A`) — both report an error.
- Foreign-key cycles between tables — detected and reported as an error.

## Develop

```bash
# Bring up local Postgres + MySQL via docker compose.
make compose-up

# Unit tests.
make test

# Integration tests (set either or both; unset drivers skip their tests).
SUBSET_TEST_DSN_POSTGRES="postgres://postgres:pass@127.0.0.1:5432/dev?sslmode=disable" \
SUBSET_TEST_DSN_MYSQL="mysql://root:pass@127.0.0.1:3306/dev?parseTime=true" \
  make test-integration

make compose-down
```

## License

[MIT](LICENSE)
