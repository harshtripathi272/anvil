package verification

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/harshtripathi272/anvil/engine"
)

// BenchConfig configures a benchmark run against a real file.
type BenchConfig struct {
	Keys  int    // dataset size, loaded via batched commits
	Seq   int    // single-commit writes, to measure per-commit latency
	Reads int    // random point reads
	Batch int    // operations per batched commit
	Path  string // database file; empty means a temp file, removed afterwards
}

// BenchResult holds measured timings. Performance is not a competition claim:
// these numbers exist to detect catastrophic regressions and to be honest about
// the fsync-bound commit path.
type BenchResult struct {
	Platform string
	Keys     int
	Batch    int

	BatchedWrite time.Duration // total, for Keys writes
	SingleCommit time.Duration // per commit
	RandomRead   time.Duration // per read
	FullScan     time.Duration // total
	ScanKeys     int
	Recovery     time.Duration
	Check        time.Duration

	Generation uint64
	Pages      uint64
	FileBytes  int64
	Report     engine.CheckReport
}

// RunBenchmark measures write, read, scan, recovery and verification cost
// against a real on-disk file.
func RunBenchmark(cfg BenchConfig) (BenchResult, error) {
	if cfg.Keys <= 0 {
		cfg.Keys = 200000
	}
	if cfg.Seq <= 0 {
		cfg.Seq = 5000
	}
	if cfg.Reads <= 0 {
		cfg.Reads = 200000
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 1000
	}

	path := cfg.Path
	if path == "" {
		path = filepath.Join(os.TempDir(), fmt.Sprintf("anvil-bench-%d.anvil", time.Now().UnixNano()))
		defer os.Remove(path)
	}
	os.Remove(path)

	res := BenchResult{
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Keys:     cfg.Keys,
		Batch:    cfg.Batch,
	}
	value := []byte("0123456789abcdef") // 16-byte value

	db, err := engine.Open(path)
	if err != nil {
		return res, err
	}

	// Batched writes: one pair of durability barriers amortized per transaction.
	t0 := time.Now()
	for i := 0; i < cfg.Keys; {
		tx, err := db.BeginWrite()
		if err != nil {
			return res, err
		}
		for j := 0; j < cfg.Batch && i < cfg.Keys; j, i = j+1, i+1 {
			if err := tx.Put([]byte(fmt.Sprintf("k%09d", i)), value); err != nil {
				tx.Rollback()
				return res, err
			}
		}
		if err := tx.Commit(); err != nil {
			return res, err
		}
	}
	res.BatchedWrite = time.Since(t0)

	// Single-commit writes: each pays two durability barriers.
	t0 = time.Now()
	for k := 0; k < cfg.Seq; k++ {
		if err := db.Put([]byte(fmt.Sprintf("s%09d", k)), value); err != nil {
			return res, err
		}
	}
	res.SingleCommit = time.Since(t0) / time.Duration(cfg.Seq)

	// Random point reads.
	rng := rand.New(rand.NewSource(1))
	t0 = time.Now()
	for k := 0; k < cfg.Reads; k++ {
		if _, err := db.Get([]byte(fmt.Sprintf("k%09d", rng.Intn(cfg.Keys)))); err != nil {
			return res, err
		}
	}
	res.RandomRead = time.Since(t0) / time.Duration(cfg.Reads)

	// Ordered full scan.
	tx, err := db.BeginRead()
	if err != nil {
		return res, err
	}
	t0 = time.Now()
	n := 0
	for it := tx.Scan(nil, nil); it.Next(); {
		n++
	}
	res.FullScan = time.Since(t0)
	res.ScanKeys = n
	tx.Close()

	res.Generation, _, res.Pages = db.Info()
	db.Close()

	// Recovery: reopen from the real file.
	t0 = time.Now()
	db2, err := engine.Open(path)
	if err != nil {
		return res, err
	}
	res.Recovery = time.Since(t0)

	// Deep structural verification.
	t0 = time.Now()
	rep, err := db2.Check()
	if err != nil {
		db2.Close()
		return res, err
	}
	res.Check = time.Since(t0)
	res.Report = rep
	db2.Close()

	if info, statErr := os.Stat(path); statErr == nil {
		res.FileBytes = info.Size()
	}
	return res, nil
}

// Rate formats an operations-per-second figure.
func Rate(n int, d time.Duration) string {
	if d <= 0 {
		return "n/a"
	}
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
