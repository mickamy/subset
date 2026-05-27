package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

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

	ctx := context.Background()
	if err := clone(ctx, stdout, dsnStr, tableName, whereClause, *planFlag); err != nil {
		fmt.Fprintf(stderr, "subset clone: %v\n", err)

		return exit.Error
	}

	return exit.OK
}

// requireTable reports a clean error when the seed table is not in the schema,
// so the common typo case avoids the deeper "could not collect rows" wrapping.
func requireTable(schema introspect.Schema, name string) error {
	for _, t := range schema.Tables {
		if t.Name == name {
			return nil
		}
	}

	return fmt.Errorf("unknown table %q", name)
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

func clone(ctx context.Context, stdout io.Writer, dsnStr, tableName, whereClause string, planOnly bool) error {
	start := time.Now()

	db, err := openDB(dsnStr)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	schema, err := introspect.Do(ctx, dsnStr)
	if err != nil {
		return fmt.Errorf("could not read database schema: %w", err)
	}

	d, err := dialect.New(dsnStr)
	if err != nil {
		return fmt.Errorf("could not select SQL dialect: %w", err)
	}

	if err := requireTable(schema, tableName); err != nil {
		return err
	}

	collected, err := extract.Walk(ctx, db, d, schema, tableName, whereClause, extract.Forward)
	if err != nil {
		return fmt.Errorf("could not collect rows: %w", err)
	}

	order, err := plan.Build(schema.Tables)
	if err != nil {
		return fmt.Errorf("could not order tables: %w", err)
	}
	tableByName := make(map[string]introspect.Table, len(schema.Tables))
	for _, t := range schema.Tables {
		tableByName[t.Name] = t
	}

	// Collected tables in emit order (parents first), with their row counts.
	counts := make(map[string]int)
	var emitOrder []string
	for _, name := range order {
		if c := len(collected.Rows[name]); c > 0 {
			counts[name] = c
			emitOrder = append(emitOrder, name)
		}
	}

	if planOnly {
		emit.WritePlan(stdout, "clone", true, schema, counts, emitOrder, tableName, time.Since(start))

		return nil
	}

	total := 0
	for _, c := range counts {
		total += c
	}
	fmt.Fprintln(stdout, emit.SummaryComment("clone", total, len(emitOrder), true))
	for _, name := range emitOrder {
		sortedRows, err := extract.SortByPK(collected.Rows[name], tableByName[name])
		if err != nil {
			return fmt.Errorf("could not order rows: %w", err)
		}
		for _, row := range sortedRows {
			fmt.Fprintln(stdout, emit.BuildInsert(d, tableByName[name], row))
		}
	}

	return nil
}
