package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/emit"
	"github.com/mickamy/subset/internal/exit"
	"github.com/mickamy/subset/internal/extract"
	"github.com/mickamy/subset/internal/introspect"
	"github.com/mickamy/subset/internal/plan"
)

func runDelete(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printDeleteUsage(stderr) }

	whereFlag := fs.String("where", "", "WHERE clause selecting seed rows")
	idFlag := fs.String("id", "", `shortcut for --where "id=<value>"`)
	planFlag := fs.Bool("plan", false, "print row counts and FK paths; emit no rows")

	flagArgs, posArgs := splitArgs(args, isBoolFlag(fs))
	if err := fs.Parse(flagArgs); err != nil {
		return exit.Usage
	}

	if len(posArgs) != 2 {
		fmt.Fprintln(stderr, "subset delete: expected <dsn> <table>")
		printDeleteUsage(stderr)

		return exit.Usage
	}
	dsnStr := posArgs[0]
	tableName := posArgs[1]

	whereClause, err := buildWhereClause(*whereFlag, *idFlag)
	if err != nil {
		fmt.Fprintf(stderr, "subset delete: %v\n", err)

		return exit.Usage
	}

	if *planFlag {
		fmt.Fprintln(stderr, "subset delete: --plan is not yet implemented")

		return exit.NotImplemented
	}

	ctx := context.Background()
	if err := deleteRows(ctx, stdout, dsnStr, tableName, whereClause); err != nil {
		fmt.Fprintf(stderr, "subset delete: %v\n", err)

		return exit.Error
	}

	return exit.OK
}

func deleteRows(ctx context.Context, stdout io.Writer, dsnStr, tableName, whereClause string) error {
	db, err := openDB(dsnStr)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	schema, err := introspect.Do(ctx, dsnStr)
	if err != nil {
		return fmt.Errorf("introspect: %w", err)
	}

	d, err := dialect.New(dsnStr)
	if err != nil {
		return fmt.Errorf("dialect: %w", err)
	}

	collected, err := extract.Walk(ctx, db, d, schema, tableName, whereClause, extract.Backward)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	order, err := plan.Build(schema.Tables)
	if err != nil {
		return fmt.Errorf("topo sort: %w", err)
	}
	tableByName := make(map[string]introspect.Table, len(schema.Tables))
	for _, t := range schema.Tables {
		tableByName[t.Name] = t
	}

	// Delete most-dependent rows first: walk the parents-first topo order in
	// reverse across tables, and reverse SortByPK within each self-referencing
	// table so that referencing rows are removed before the rows they point at.
	for _, name := range slices.Backward(order) {
		rows := collected.Rows[name]
		if len(rows) == 0 {
			continue
		}
		sortedRows, err := extract.SortByPK(rows, tableByName[name])
		if err != nil {
			return fmt.Errorf("sort %q: %w", name, err)
		}
		for _, row := range slices.Backward(sortedRows) {
			fmt.Fprintln(stdout, emit.BuildDelete(d, tableByName[name], row))
		}
	}

	return nil
}
