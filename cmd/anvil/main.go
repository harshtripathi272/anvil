// Command anvil is the command-line interface to the Anvil storage engine.
//
// It exposes the store itself (create, put, get, del, scan, info, check) and
// the verification tools that make Anvil's durability claim checkable by hand
// (torture, bench).
//
// Commands are grouped across files: store.go holds the data commands,
// verify.go holds torture and bench.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "create":
		err = cmdCreate(args)
	case "put":
		err = cmdPut(args)
	case "get":
		err = cmdGet(args)
	case "del", "delete":
		err = cmdDel(args)
	case "scan":
		err = cmdScan(args)
	case "info":
		err = cmdInfo(args)
	case "check":
		err = cmdCheck(args)
	case "torture":
		err = cmdTorture(args)
	case "bench":
		err = cmdBench(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "anvil: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "anvil:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `anvil - a crash-safe embedded key/value store (standard library only)

store:
  anvil create <db>
  anvil put    <db> <key> <value>
  anvil get    <db> <key>
  anvil del    <db> <key>
  anvil scan   <db> [prefix]
  anvil info   <db>
  anvil check  <db>

verify:
  anvil torture [--cycles N] [--seed S] [--keyspace K] [--maxops M]
                [--negative-control]
  anvil bench   [--keys N] [--seq N] [--reads N] [--batch N] [--path P]

Every acknowledged transaction survives an abrupt crash. 'torture' proves it by
killing the engine at each seam of the commit protocol and verifying recovery;
'torture --negative-control' removes a durability barrier and must then FAIL,
which is what shows the test has teeth.
`)
}
