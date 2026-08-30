package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/harshtripathi272/anvil/verification"
)

// crashWriterCmd is the hidden subcommand the process-kill check re-executes.
var crashWriterCmd = verification.CrashWriterCommand()

// cmdVerify runs the complete verification suite and prints one verdict.
func cmdVerify(args []string) error {
	fs := newFlagSet("verify", "[flags]", "Run the complete verification suite.")
	quick := fs.Bool("quick", false, "smaller run for a fast smoke check")
	seed := fs.Int64("seed", 0x19af42d9, "random seed (runs are reproducible)")
	cycles := fs.Int("cycles", 0, "crash-torture cycles (0 picks a default)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := verification.SuiteConfig{
		Seed:          *seed,
		TortureCycles: *cycles,
		ModelSteps:    40000,
		Commits:       3000,
	}
	if *quick {
		cfg.TortureCycles, cfg.ModelSteps, cfg.Commits = 2000, 5000, 500
	}
	cfg.SelfExe = selfExecutable() // enables the real process-kill check

	fmt.Printf("ANVIL VERIFY  seed %d\n\n", cfg.Seed)
	start := time.Now()
	res := verification.RunSuite(cfg)

	for _, c := range res.Checks {
		status := "PASS"
		if !c.OK {
			status = "FAIL"
		}
		fmt.Printf("  %-4s  %-20s %s\n", status, c.Name, c.Detail)
		fmt.Printf("        %-20s expected: %s (%s)\n", "", c.Expect, c.Elapsed.Round(time.Millisecond))
	}

	fmt.Printf("\n  %d checks in %s\n\n", len(res.Checks), time.Since(start).Round(time.Millisecond))
	if !res.OK {
		fmt.Println("  RESULT: FAIL")
		return fmt.Errorf("verification failed")
	}
	fmt.Println("  RESULT: PASS")
	return nil
}

// selfExecutable returns a path to this binary that exec can actually launch,
// or "" if it cannot be determined. On Windows os.Executable may report the
// path without the .exe suffix the file actually carries.
func selfExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if _, err := os.Stat(exe); err == nil {
		return exe
	}
	if _, err := os.Stat(exe + ".exe"); err == nil {
		return exe + ".exe"
	}
	return ""
}

// cmdTorture runs the crash-torture proof, or the negative control that shows
// the proof has teeth.
func cmdTorture(args []string) error {
	fs := newFlagSet("torture", "[flags]",
		"Crash the engine at every seam of the commit protocol and verify recovery.")
	cycles := fs.Int("cycles", 100000, "number of crash/recover cycles")
	seed := fs.Int64("seed", 0x19af42d9, "random seed (runs are reproducible)")
	keyspace := fs.Int("keyspace", 500, "distinct keys operations range over")
	maxops := fs.Int("maxops", 10, "maximum operations per transaction")
	negative := fs.Bool("negative-control", false,
		"remove a required durability barrier; the harness MUST then report FAIL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := verification.TortureConfig{
		Seed: *seed, Cycles: *cycles, Keyspace: *keyspace, MaxOps: *maxops,
	}

	if *negative {
		res := verification.RunNegativeControl(cfg)
		printTorture(res)
		if res.OK {
			return fmt.Errorf("negative control PASSED, which means the verifier has no teeth")
		}
		fmt.Println("\n  EXPECTED FAILURE: with the pre-metadata durability barrier removed,")
		fmt.Println("  the verifier caught the resulting data loss. The test has teeth.")
		return nil
	}

	res := verification.RunTorture(cfg)
	printTorture(res)
	if !res.OK {
		return fmt.Errorf("torture failed")
	}
	return nil
}

func printTorture(r verification.TortureResult) {
	fmt.Printf("ANVIL TORTURE  seed %d\n\n", r.Seed)
	fmt.Printf("  cycles completed       %d\n", r.Cycles)
	fmt.Printf("  crashes injected       %d\n", r.CrashesInjected)
	fmt.Printf("  recovery failures      %d\n", r.RecoveryFailures)
	fmt.Printf("  acked writes lost      %d\n", r.AckedWritesLost)
	fmt.Printf("  partial txns observed  %d\n", r.PartialTxObserved)
	fmt.Printf("  invariant violations   %d\n\n", r.InvariantViolations)

	seams := make([]string, 0, len(r.SeamCoverage))
	for s := range r.SeamCoverage {
		seams = append(seams, s)
	}
	sort.Strings(seams)
	fmt.Println("  commit seams exercised")
	for _, s := range seams {
		fmt.Printf("    %-20s %d\n", s, r.SeamCoverage[s])
	}
	fmt.Println()

	if r.OK {
		fmt.Println("  RESULT: PASS")
		return
	}
	fmt.Printf("  RESULT: FAIL\n    cycle %d, seam %s, crash style %s\n    %s\n",
		r.FailCycle, r.FailSeam, r.FailStyle, r.FailDetail)
}

// cmdBench measures the engine against a real file.
func cmdBench(args []string) error {
	fs := newFlagSet("bench", "[flags]",
		"Measure write, read, scan, recovery and verification cost against a real file.")
	keys := fs.Int("keys", 200000, "dataset size in keys, loaded via batched commits")
	seq := fs.Int("seq", 5000, "single-commit writes, to measure per-commit latency")
	reads := fs.Int("reads", 200000, "random point reads")
	batch := fs.Int("batch", 1000, "operations per batched commit")
	path := fs.String("path", "", "database file (default: a temp file, removed afterwards)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	res, err := verification.RunBenchmark(verification.BenchConfig{
		Keys: *keys, Seq: *seq, Reads: *reads, Batch: *batch, Path: *path,
	})
	if err != nil {
		return err
	}

	fmt.Printf("ANVIL BENCH  real file, fdatasync per commit, %s\n\n", res.Platform)
	fmt.Printf("  dataset            %d keys x 16-byte values, batch %d\n\n", res.Keys, res.Batch)
	fmt.Printf("  batched write      %-10s %s\n",
		res.BatchedWrite.Round(time.Millisecond), verification.Rate(res.Keys, res.BatchedWrite))
	fmt.Printf("  single commit      %-10s %s  (2 fdatasync per commit)\n",
		res.SingleCommit.Round(time.Microsecond), verification.Rate(1, res.SingleCommit))
	fmt.Printf("  random read        %-10s %s\n",
		res.RandomRead.Round(time.Microsecond), verification.Rate(1, res.RandomRead))
	fmt.Printf("  full scan          %-10s %s  (%d keys)\n",
		res.FullScan.Round(time.Millisecond), verification.Rate(res.ScanKeys, res.FullScan), res.ScanKeys)
	fmt.Printf("  recovery open      %-10s (generation %d)\n",
		res.Recovery.Round(time.Microsecond), res.Generation)
	fmt.Printf("  structural check   %-10s (%d pages, %d keys, leaf depth %d)\n",
		res.Check.Round(time.Millisecond), res.Report.PagesChecked, res.Report.Keys, res.Report.LeafDepth)
	if res.FileBytes > 0 {
		fmt.Printf("  file size          %.1f MB   (%d pages incl. copy-on-write history)\n",
			float64(res.FileBytes)/(1<<20), res.Pages)
	}
	fmt.Printf("\n  Performance is not a competition claim. Single-commit throughput is\n")
	fmt.Printf("  fsync-bound by design; correctness comes first.\n")
	return nil
}

// cmdCrashWriter is the hidden victim process: it commits until killed.
func cmdCrashWriter(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil %s <db>", crashWriterCmd)
	}
	return verification.RunCrashWriter(args[0])
}
