package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/mickamy/subset/internal/dialect"
	"github.com/mickamy/subset/internal/emit"
	"github.com/mickamy/subset/internal/exit"
	"github.com/mickamy/subset/internal/extract"
	"github.com/mickamy/subset/internal/introspect"
	"github.com/mickamy/subset/internal/plan"
)

func runClone(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clone", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printCloneUsage(stderr) }

	whereFlag := fs.String("where", "", "WHERE clause selecting seed rows")
	idFlag := fs.String("id", "", `shortcut for --where "id=<value>"`)
	planFlag := fs.Bool("plan", false, "print row counts and FK paths; emit no rows")

	flagArgs, posArgs := splitArgs(args, isBoolFlag(fs))
	if err := fs.Parse(flagArgs); err != nil {
		return exit.Usage
	}

	if len(posArgs) != 2 {
		fmt.Fprintln(stderr, "subset clone: expected <dsn> <table>")
		printCloneUsage(stderr)

		return exit.Usage
	}
	dsnStr := posArgs[0]
	tableName := posArgs[1]

	whereClause, err := buildWhereClause(*whereFlag, *idFlag)
	if err != nil {
		fmt.Fprintf(stderr, "subset clone: %v\n", err)

		return exit.Usage
	}

	if *planFlag {
		fmt.Fprintln(stderr, "subset clone: --plan is not yet implemented")

		return exit.NotImplemented
	}

	ctx := context.Background()
	if err := clone(ctx, stdout, dsnStr, tableName, whereClause); err != nil {
		fmt.Fprintf(stderr, "subset clone: %v\n", err)

		return exit.Error
	}

	return exit.OK
}

func buildWhereClause(whereFlag, idFlag string) (string, error) {
	switch {
	case whereFlag != "" && idFlag != "":
		return "", errors.New("--where and --id are mutually exclusive")
	case whereFlag != "":
		return whereFlag, nil
	case idFlag != "":
		return "id = " + idFlag, nil
	default:
		return "", errors.New("--where or --id is required")
	}
}

func clone(ctx context.Context, stdout io.Writer, dsnStr, tableName, whereClause string) error {
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

	collected, err := extract.Walk(ctx, db, d, schema, tableName, whereClause)
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
	for _, name := range order {
		rows := collected.Rows[name]
		if len(rows) == 0 {
			continue
		}
		for _, row := range rows {
			fmt.Fprintln(stdout, emit.BuildInsert(d, tableByName[name], row))
		}
	}

	return nil
}
