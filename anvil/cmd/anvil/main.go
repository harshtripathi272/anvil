// Command anvil is a small CLI over the Anvil embedded storage engine:
// create/put/get/del/scan/info/check, plus the crash-torture demonstration.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"anvil"
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

usage:
  anvil create <db>
  anvil put    <db> <key> <value>
  anvil get    <db> <key>
  anvil del    <db> <key>
  anvil scan   <db> [prefix]
  anvil info   <db>
  anvil check  <db>
  anvil torture [--cycles N] [--seed S] [--keyspace K] [--maxops M]
  anvil bench   [--keys N] [--seq N] [--reads N] [--batch N] [--path P]

Every acknowledged transaction survives an abrupt crash; 'torture' proves it by
repeatedly killing the engine at each commit seam and verifying recovery.
`)
}

func cmdCreate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil create <db>")
	}
	db, err := anvil.Open(args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	fmt.Printf("created %s\n", args[0])
	return nil
}

func cmdPut(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: anvil put <db> <key> <value>")
	}
	db, err := anvil.Open(args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Put([]byte(args[1]), []byte(args[2])); err != nil {
		return err
	}
	fmt.Println("OK")
	return nil
}

func cmdGet(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: anvil get <db> <key>")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()
	v, err := db.Get([]byte(args[1]))
	if err == anvil.ErrNotFound {
		fmt.Println("NOT FOUND")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", v)
	return nil
}

func cmdDel(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: anvil del <db> <key>")
	}
	db, err := anvil.Open(args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Delete([]byte(args[1])); err != nil {
		return err
	}
	fmt.Println("OK")
	return nil
}

func cmdScan(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: anvil scan <db> [prefix]")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginRead()
	if err != nil {
		return err
	}
	defer tx.Close()

	var start, end []byte
	if len(args) == 2 && args[1] != "" {
		start = []byte(args[1])
		end = prefixEnd(start)
	}
	it := tx.Scan(start, end)
	n := 0
	for it.Next() {
		fmt.Printf("%s\t%s\n", it.Key(), it.Value())
		n++
	}
	if err := it.Err(); err != nil {
		return err
	}
	fmt.Printf("(%d keys)\n", n)
	return nil
}

func cmdInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil info <db>")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()
	gen, root, pages := db.Info()
	fmt.Printf("ANVIL INFO\n\n")
	fmt.Printf("format version:   %d\n", anvil.FormatVersion)
	fmt.Printf("page size:        %d\n", anvil.PageSize)
	fmt.Printf("generation:       %d\n", gen)
	fmt.Printf("root page:        %d\n", root)
	fmt.Printf("pages allocated:  %d\n", pages)
	return nil
}

func cmdCheck(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil check <db>")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()
	rep, err := db.Check()
	fmt.Printf("ANVIL CHECK\n\n")
	if err != nil {
		fmt.Printf("RESULT: FAIL\n  %v\n", err)
		return err
	}
	fmt.Printf("metadata:              OK\n")
	fmt.Printf("root:                  OK\n")
	fmt.Printf("pages checked:         %d\n", rep.PagesChecked)
	fmt.Printf("leaf depth:            %d\n", rep.LeafDepth)
	fmt.Printf("keys:                  %d\n", rep.Keys)
	fmt.Printf("checksum failures:     0\n")
	fmt.Printf("ordering violations:   0\n")
	fmt.Printf("leaf-depth violations: 0\n\n")
	fmt.Printf("RESULT: PASS\n")
	return nil
}

func cmdTorture(args []string) error {
	fs := flag.NewFlagSet("torture", flag.ContinueOnError)
	cycles := fs.Int("cycles", 100000, "number of crash/recover cycles")
	seed := fs.Int64("seed", 0x19af42d9, "random seed (for reproducibility)")
	keyspace := fs.Int("keyspace", 500, "distinct keys operations range over")
	maxops := fs.Int("maxops", 10, "max operations per transaction")
	negative := fs.Bool("negative-control", false, "remove a required durability barrier; the harness MUST then report FAIL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := anvil.TortureConfig{Seed: *seed, Cycles: *cycles, Keyspace: *keyspace, MaxOps: *maxops}

	if *negative {
		res := anvil.RunNegativeControl(cfg)
		fmt.Print(res.Report())
		if res.OK {
			// The harness failed to notice the removed barrier — no teeth.
			return fmt.Errorf("negative control PASSED, which means the verifier has no teeth")
		}
		fmt.Println("\nEXPECTED FAILURE: with the pre-meta data barrier removed, the")
		fmt.Println("verifier caught the durability violation. The test has teeth.")
		return nil
	}

	res := anvil.RunTorture(cfg)
	fmt.Print(res.Report())
	if !res.OK {
		return fmt.Errorf("torture FAILED")
	}
	return nil
}

func cmdBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	keys := fs.Int("keys", 200000, "dataset size (keys) loaded via batched commits")
	seq := fs.Int("seq", 5000, "single-commit writes to measure per-commit latency")
	reads := fs.Int("reads", 200000, "random point reads to measure")
	batch := fs.Int("batch", 1000, "operations per batched commit")
	path := fs.String("path", "", "database file (default: a temp file, removed after)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dbpath := *path
	if dbpath == "" {
		dbpath = filepath.Join(os.TempDir(), fmt.Sprintf("anvil-bench-%d.anvil", time.Now().UnixNano()))
		defer os.Remove(dbpath)
	}
	os.Remove(dbpath)

	value := []byte("0123456789abcdef") // 16-byte value
	db, err := anvil.Open(dbpath)
	if err != nil {
		return err
	}

	fmt.Printf("ANVIL BENCH   real file backend, fdatasync per commit, %s\n\n", runtime.GOOS)
	fmt.Printf("dataset:         %d keys x 16-byte values, batch=%d\n\n", *keys, *batch)

	// 1. batched writes (fsync amortized across a transaction)
	t0 := time.Now()
	for i := 0; i < *keys; {
		tx, err := db.BeginWrite()
		if err != nil {
			return err
		}
		for j := 0; j < *batch && i < *keys; j, i = j+1, i+1 {
			if err := tx.Put([]byte(fmt.Sprintf("k%09d", i)), value); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	d := time.Since(t0)
	fmt.Printf("batched write:   %-10s  %s\n", d.Round(time.Millisecond), rate(*keys, d))

	// 2. single-commit writes (each pays 2 fdatasync -> latency-bound)
	t0 = time.Now()
	for k := 0; k < *seq; k++ {
		if err := db.Put([]byte(fmt.Sprintf("s%09d", k)), value); err != nil {
			return err
		}
	}
	d = time.Since(t0)
	fmt.Printf("single commit:   %-10s  %s  (2 fdatasync/commit)\n", per(d, *seq), rate(*seq, d))

	// 3. random point reads
	rng := rand.New(rand.NewSource(1))
	t0 = time.Now()
	for k := 0; k < *reads; k++ {
		if _, err := db.Get([]byte(fmt.Sprintf("k%09d", rng.Intn(*keys)))); err != nil {
			return err
		}
	}
	d = time.Since(t0)
	fmt.Printf("random read:     %-10s  %s\n", per(d, *reads), rate(*reads, d))

	// 4. full scan throughput
	tx, _ := db.BeginRead()
	t0 = time.Now()
	n := 0
	for it := tx.Scan(nil, nil); it.Next(); {
		n++
	}
	d = time.Since(t0)
	tx.Close()
	fmt.Printf("full scan:       %-10s  %s  (%d keys)\n", d.Round(time.Millisecond), rate(n, d), n)

	gen, _, pages := db.Info()
	db.Close()

	// 5. recovery (reopen from the real file)
	t0 = time.Now()
	db2, err := anvil.Open(dbpath)
	if err != nil {
		return err
	}
	drec := time.Since(t0)

	// 6. structural verification
	t0 = time.Now()
	rep, err := db2.Check()
	if err != nil {
		return err
	}
	dchk := time.Since(t0)
	db2.Close()

	fmt.Printf("recovery open:   %-10s  (generation %d)\n", drec.Round(time.Microsecond), gen)
	fmt.Printf("check:           %-10s  (%d pages, %d keys, leaf depth %d)\n",
		dchk.Round(time.Millisecond), rep.PagesChecked, rep.Keys, rep.LeafDepth)

	if info, err := os.Stat(dbpath); err == nil {
		fmt.Printf("file size:       %.1f MB  (%d pages allocated incl. COW history)\n",
			float64(info.Size())/(1<<20), pages)
	}
	fmt.Printf("\nnote: performance is not a competition claim; single-commit throughput is\n")
	fmt.Printf("      fsync-bound by design. Correctness comes first.\n")
	return nil
}

func rate(n int, d time.Duration) string {
	ops := float64(n) / d.Seconds()
	switch {
	case ops >= 1e6:
		return fmt.Sprintf("%.2f M ops/s", ops/1e6)
	case ops >= 1e3:
		return fmt.Sprintf("%.1f K ops/s", ops/1e3)
	default:
		return fmt.Sprintf("%.0f ops/s", ops)
	}
}

func per(d time.Duration, n int) string {
	return (d / time.Duration(n)).Round(time.Microsecond).String()
}

// prefixEnd returns the smallest key strictly greater than every key with the
// given prefix, or nil if the prefix is all 0xff (unbounded above).
func prefixEnd(prefix []byte) []byte {
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}
