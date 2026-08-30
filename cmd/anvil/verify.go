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

// cmdTorture runs the crash-torture proof, or the negative control that shows
// the proof has teeth.
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

// cmdBench measures write, read, scan, recovery and verification cost against
// a real file. Performance is not a competition claim; this exists to detect
// catastrophic regressions and to be honest about the fsync-bound commit path.
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

	fmt.Printf("ANVIL BENCH   real file backend, fdatasync per commit, %s/%s\n\n",
		runtime.GOOS, runtime.GOARCH)
	fmt.Printf("dataset:         %d keys x 16-byte values, batch=%d\n\n", *keys, *batch)

	// 1. batched writes (one pair of fdatasync amortized across a transaction)
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

	// 2. single-commit writes (each pays two fdatasync -> latency-bound)
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

	// 5. recovery: reopen from the real file
	t0 = time.Now()
	db2, err := anvil.Open(dbpath)
	if err != nil {
		return err
	}
	drec := time.Since(t0)

	// 6. deep structural verification
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
		fmt.Printf("file size:       %.1f MB  (%d pages incl. copy-on-write history)\n",
			float64(info.Size())/(1<<20), pages)
	}
	fmt.Printf("\nnote: performance is not a competition claim. Single-commit throughput is\n")
	fmt.Printf("      fsync-bound by design; correctness comes first.\n")
	return nil
}

// rate formats an operations-per-second figure.
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

// per formats the average duration of a single operation.
func per(d time.Duration, n int) string {
	return (d / time.Duration(n)).Round(time.Microsecond).String()
}
