// Command anvil is the single entry point to the Anvil storage engine.
//
// It exposes the database itself (create, put, get, delete, scan, info, check)
// and the verification tooling that makes Anvil's durability claim checkable by
// hand (verify, torture, bench).
//
// The CLI orchestrates; it implements nothing. Database behaviour lives in
// package engine, verification logic in package verification, so the command
// line and the test suite exercise exactly the same code.
package main

import (
	"flag"
	"fmt"
	"os"
)

// command is one CLI verb.
type command struct {
	name    string
	args    string // argument summary shown in help
	summary string
	group   string
	run     func(args []string) error
	hidden  bool
}

const (
	groupData   = "database"
	groupVerify = "verification"
	groupBench  = "benchmark"
)

func commands() []command {
	return []command{
		{name: "create", args: "<db>", group: groupData, summary: "create an empty database", run: cmdCreate},
		{name: "put", args: "<db> <key> <value>", group: groupData, summary: "write a key/value pair", run: cmdPut},
		{name: "get", args: "<db> <key>", group: groupData, summary: "read a value", run: cmdGet},
		{name: "delete", args: "<db> <key>", group: groupData, summary: "delete a key", run: cmdDelete},
		{name: "scan", args: "<db> [prefix]", group: groupData, summary: "list keys in order", run: cmdScan},
		{name: "info", args: "<db>", group: groupData, summary: "show generation, root page, size", run: cmdInfo},
		{name: "check", args: "<db>", group: groupData, summary: "verify structural invariants", run: cmdCheck},

		{name: "verify", args: "[flags]", group: groupVerify, summary: "run the complete verification suite", run: cmdVerify},
		{name: "torture", args: "[flags]", group: groupVerify, summary: "crash the engine at every commit seam", run: cmdTorture},

		{name: "bench", args: "[flags]", group: groupBench, summary: "measure write, read, scan and recovery", run: cmdBench},

		// Internal: re-executed by the process-kill check as the victim process.
		{name: crashWriterCmd, args: "<db>", hidden: true, summary: "internal crash-writer mode", run: cmdCrashWriter},
	}
}

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	name, args := os.Args[1], os.Args[2:]

	switch name {
	case "help", "-h", "--help":
		usage(os.Stdout)
		return
	}

	for _, c := range commands() {
		if c.name != name && !(name == "del" && c.name == "delete") {
			continue
		}
		if err := c.run(args); err != nil {
			if err == flag.ErrHelp {
				return
			}
			// Engine errors already carry an "anvil:" prefix and usage errors
			// name the command, so printing plainly avoids doubling it up.
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "anvil: unknown command %q\n\n", name)
	usage(os.Stderr)
	os.Exit(2)
}

func usage(w *os.File) {
	fmt.Fprint(w, `anvil — a dependency-free embedded transactional key/value store

usage:
  anvil <command> [options]

`)
	for _, g := range []string{groupData, groupVerify, groupBench} {
		fmt.Fprintf(w, "%s:\n", g)
		for _, c := range commands() {
			if c.group != g || c.hidden {
				continue
			}
			fmt.Fprintf(w, "  %-9s %-20s %s\n", c.name, c.args, c.summary)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, `Every acknowledged transaction survives an abrupt crash. Prove it here:

  anvil verify                       run every check and print a verdict
  anvil torture --cycles 100000      crash at each commit seam, verify recovery
  anvil torture --negative-control   remove a durability barrier; MUST fail

Run 'anvil <command> --help' for the options of a single command.
`)
}

// newFlagSet builds a FlagSet whose usage line matches the command it serves.
func newFlagSet(name, args, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\nusage: anvil %s %s\n\noptions:\n", summary, name, args)
		fs.PrintDefaults()
	}
	return fs
}
