package verification

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/harshtripathi272/anvil/engine"
)

// SuiteConfig configures the full verification suite.
type SuiteConfig struct {
	Seed          int64
	TortureCycles int
	ModelSteps    int
	Commits       int
	// SelfExe is the path to a binary that supports the crash-writer mode (see
	// RunCrashWriter). Empty skips the real-process-kill check.
	SelfExe string
}

// Check is one named result within the suite.
type Check struct {
	Name    string
	OK      bool
	Detail  string
	Elapsed time.Duration
	// Expect records what a passing result means, so a reader understands why
	// the negative control "failing" is the correct outcome.
	Expect string
}

// SuiteResult aggregates every verification check.
type SuiteResult struct {
	Checks []Check
	OK     bool
}

// RunSuite runs the complete in-process verification suite: model equivalence,
// concurrency, crash torture, the negative control, and — when SelfExe is set —
// real process-kill recovery.
//
// This is the single implementation. Both `go test` and the CLI call it, so the
// two can never drift apart.
func RunSuite(cfg SuiteConfig) SuiteResult {
	if cfg.TortureCycles <= 0 {
		cfg.TortureCycles = 20000
	}
	if cfg.ModelSteps <= 0 {
		cfg.ModelSteps = 40000
	}
	if cfg.Commits <= 0 {
		cfg.Commits = 3000
	}
	res := SuiteResult{OK: true}

	add := func(name, expect string, start time.Time, ok bool, detail string) {
		res.Checks = append(res.Checks, Check{
			Name: name, OK: ok, Detail: detail,
			Elapsed: time.Since(start), Expect: expect,
		})
		if !ok {
			res.OK = false
		}
	}

	// 1. Model equivalence against an independent oracle.
	t0 := time.Now()
	m := RunModelCheck(ModelConfig{Seed: cfg.Seed, Steps: cfg.ModelSteps})
	add("model equivalence", "matches an independent reference model", t0, m.OK,
		pick(m.OK, fmt.Sprintf("%d operations, %d keys, 0 mismatches", m.Steps, m.Keys),
			fmt.Sprintf("step %d: %s", m.FailStep, m.FailDetail)))

	// 2. Snapshot isolation under concurrent readers.
	t0 = time.Now()
	c := RunConcurrencyCheck(ConcurrencyConfig{Commits: cfg.Commits})
	add("concurrency", "no reader observes a partial transaction", t0, c.OK,
		pick(c.OK, fmt.Sprintf("%d commits, %d readers, %d snapshots, 0 partial reads",
			c.Commits, c.Readers, c.SnapshotsTaken), c.FailDetail))

	// 3. Crash torture across every commit seam.
	t0 = time.Now()
	tr := RunTorture(TortureConfig{Seed: cfg.Seed, Cycles: cfg.TortureCycles})
	add("crash torture", "0 acknowledged writes lost, 0 invariant violations", t0, tr.OK,
		pick(tr.OK, fmt.Sprintf("%d crashes across %d seams, 0 lost, 0 violations",
			tr.CrashesInjected, len(tr.SeamCoverage)),
			fmt.Sprintf("cycle %d seam %s: %s", tr.FailCycle, tr.FailSeam, tr.FailDetail)))

	// 4. Negative control. This one must FAIL to pass: with a required
	//    durability barrier removed, the harness has to notice.
	t0 = time.Now()
	nc := RunNegativeControl(TortureConfig{Seed: cfg.Seed, Cycles: 500})
	add("negative control", "harness detects a removed durability barrier", t0, !nc.OK,
		pick(!nc.OK, "durability violation correctly detected: "+nc.FailDetail,
			"harness did NOT detect the removed barrier — the test has no teeth"))

	// 5. Real process kill, when a self-executable is available.
	if cfg.SelfExe != "" {
		t0 = time.Now()
		exe := cfg.SelfExe
		ok, detail := RunProcessKillCheck(func(dbPath string) *exec.Cmd {
			return exec.Command(exe, crashWriterCommand, dbPath)
		})
		add("process kill", "recovers cleanly after SIGKILL on a real file", t0, ok, detail)
	}

	return res
}

func pick(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

// RunProcessKillCheck spawns a crash-writer child, kills it mid-commit, and
// verifies the real file recovers to a valid, internally consistent state.
//
// spawn builds the child command for a given database path. Callers differ in
// how they select crash-writer mode — the CLI uses a subcommand, the test binary
// an environment variable — so the strategy is supplied rather than assumed.
//
// This exercises the true syscall path. Its claim is crash-consistency: a
// process kill does not discard page-cache writes, so durability itself is
// established by the fault-injecting backend, not here.
func RunProcessKillCheck(spawn func(dbPath string) *exec.Cmd) (bool, string) {
	dir, err := os.MkdirTemp("", "anvil-kill")
	if err != nil {
		return false, "temp dir: " + err.Error()
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "kill.anvil")

	cmd := spawn(path)
	if err := cmd.Start(); err != nil {
		return false, "start child: " + err.Error()
	}
	time.Sleep(250 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	db, err := engine.Open(path)
	if err != nil {
		return false, "recovery after kill failed: " + err.Error()
	}
	defer db.Close()

	rep, err := db.Check()
	if err != nil {
		return false, "structural corruption after kill: " + err.Error()
	}
	gen, _, _ := db.Info()
	if gen < 2 {
		return false, fmt.Sprintf("no committed writes survived (generation %d)", gen)
	}

	// The surviving generation must be internally consistent: counter N implies
	// keys 0..N-1 are all present, since each was committed before it advanced.
	v, err := db.Get([]byte("counter"))
	if err != nil {
		return false, "counter missing after recovery: " + err.Error()
	}
	n, err := strconv.Atoi(string(v))
	if err != nil {
		return false, "counter unreadable: " + string(v)
	}
	for i := 0; i < n; i++ {
		if _, err := db.Get([]byte(fmt.Sprintf("k%08d", i))); err != nil {
			return false, fmt.Sprintf("acknowledged key k%08d lost after kill: %v", i, err)
		}
	}
	return true, fmt.Sprintf("killed at generation %d; %d acknowledged keys intact, %d pages valid",
		gen, n, rep.PagesChecked)
}

// crashWriterCommand is the hidden subcommand a binary must implement to be
// usable as SelfExe: it commits forever until killed.
const crashWriterCommand = "_crashwriter"

// CrashWriterCommand reports the subcommand name RunProcessKillCheck invokes.
func CrashWriterCommand() string { return crashWriterCommand }

// RunCrashWriter commits keys k00000000, k00000001, ... to path, bumping
// "counter" in the same transaction, until the process is killed. It never
// returns normally.
func RunCrashWriter(path string) error {
	db, err := engine.Open(path)
	if err != nil {
		return err
	}
	for i := 0; ; i++ {
		tx, err := db.BeginWrite()
		if err != nil {
			return err
		}
		tx.Put([]byte(fmt.Sprintf("k%08d", i)), []byte("v"))
		tx.Put([]byte("counter"), []byte(strconv.Itoa(i+1)))
		if err := tx.Commit(); err != nil {
			return err
		}
	}
}
