package cli

import (
	"fmt"
	"io"

	"github.com/mickamy/subset/internal/exit"
)

type command struct {
	name    string
	summary string
	run     func(args []string, stdout, stderr io.Writer) int
	usage   func(w io.Writer)
}

var commands = []command{
	{
		name:    "clone",
		summary: "Pull rows plus their FK parents as INSERT statements",
		run:     runClone,
		usage:   printCloneUsage,
	},
	{
		name:    "delete",
		summary: "Remove rows plus their FK-dependent rows in safe order",
		run:     notImplemented("delete"),
		usage:   printDeleteUsage,
	},
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		PrintUsage(stderr)

		return exit.Usage
	}

	name := args[0]
	rest := args[1:]

	for _, c := range commands {
		if c.name == name {
			for _, a := range rest {
				if a == "--help" || a == "-h" {
					c.usage(stdout)

					return exit.OK
				}
			}

			return c.run(rest, stdout, stderr)
		}
	}

	fmt.Fprintf(stderr, "subset: unknown command %q\n", name)
	fmt.Fprintln(stderr, "Run 'subset --help' for a list of commands.")

	return exit.Usage
}

func PrintUsage(w io.Writer) {
	fmt.Fprintln(w, "subset — clone or delete a referentially-consistent subset of your database.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  subset <command> <dsn> <table> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  subset emits SQL to stdout; it never modifies the source DB.")
	fmt.Fprintln(w, "  Pipe to psql/mysql to apply.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")

	width := 0

	for _, c := range commands {
		width = max(width, len(c.name))
	}

	for _, c := range commands {
		fmt.Fprintf(w, "  %-*s  %s\n", width, c.name, c.summary)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  # Clone an order and its FK parents")
	fmt.Fprintln(w, "  subset clone postgres://user:pass@localhost:5432/mydb orders --where \"id=42\"")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  # Delete a tenant and all FK-dependent rows")
	fmt.Fprintln(w, "  subset delete postgres://... tenants --id 7 > cleanup.sql")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --version, -v    Print subset version")
	fmt.Fprintln(w, "  --help, -h       Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'subset <command> --help' for command-specific flags.")
	fmt.Fprintln(w, "More: https://github.com/mickamy/subset")
}

func printCloneUsage(w io.Writer) {
	fmt.Fprintln(w, "subset clone — pull rows plus their FK parents as INSERT statements.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  subset clone <dsn> <table> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  subset clone postgres://user:pass@localhost:5432/mydb orders --where \"id=42\"")
	fmt.Fprintln(w, "  subset clone postgres://... orders --id 42 > clone.sql")
	fmt.Fprintln(w, "  subset clone postgres://... orders --where \"id=42\" --plan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --where <expr>   WHERE clause selecting the seed rows (e.g., \"id=42\")")
	fmt.Fprintln(w, "  --id <value>     Shortcut for --where \"id=<value>\"")
	fmt.Fprintln(w, "  --plan           Print row counts and FK paths; emit no rows")
	fmt.Fprintln(w, "  --help, -h       Show this help")
}

func printDeleteUsage(w io.Writer) {
	fmt.Fprintln(w, "subset delete — remove rows plus their FK-dependent rows in safe order.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  subset delete <dsn> <table> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  subset delete postgres://user:pass@localhost:5432/mydb tenants --id 7")
	fmt.Fprintln(w, "  subset delete postgres://... users --where \"status='banned'\" > cleanup.sql")
	fmt.Fprintln(w, "  subset delete postgres://... tenants --id 7 --plan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --where <expr>   WHERE clause selecting the seed rows (e.g., \"id=42\")")
	fmt.Fprintln(w, "  --id <value>     Shortcut for --where \"id=<value>\"")
	fmt.Fprintln(w, "  --plan           Print row counts and FK paths; emit no rows")
	fmt.Fprintln(w, "  --help, -h       Show this help")
}

func notImplemented(name string) func([]string, io.Writer, io.Writer) int {
	return func(_ []string, _ io.Writer, stderr io.Writer) int {
		fmt.Fprintf(stderr, "subset: %s is not yet implemented\n", name)

		return exit.NotImplemented
	}
}
