package verification

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/harshtripathi272/anvil/engine"
)

// Latency summarises a distribution of per-operation timings. Averages hide
// tail behaviour, which is usually the number that matters for a storage
// engine, so percentiles are reported alongside the mean.
type Latency struct {
	Op      string        `json:"op"`
	Samples int           `json:"samples"`
	Mean    time.Duration `json:"mean"`
	P50     time.Duration `json:"p50"`
	P95     time.Duration `json:"p95"`
	P99     time.Duration `json:"p99"`
	Max     time.Duration `json:"max"`
}

func summarize(op string, d []time.Duration) Latency {
	if len(d) == 0 {
		return Latency{Op: op}
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	var total time.Duration
	for _, v := range d {
		total += v
	}
	at := func(q float64) time.Duration {
		i := int(q * float64(len(d)-1))
		return d[i]
	}
	return Latency{
		Op:      op,
		Samples: len(d),
		Mean:    total / time.Duration(len(d)),
		P50:     at(0.50),
		P95:     at(0.95),
		P99:     at(0.99),
		Max:     d[len(d)-1],
	}
}

// BatchPoint is one sample of the fsync-amortization curve.
type BatchPoint struct {
	Batch  int     `json:"batch"`
	OpsSec float64 `json:"ops_sec"`
	PerOp  string  `json:"per_op"`
}

// ScalePoint is one sample of the dataset-size scaling curve.
type ScalePoint struct {
	Keys      int     `json:"keys"`
	LoadOpsS  float64 `json:"load_ops_sec"`
	ReadP50   string  `json:"read_p50"`
	ScanKeysS float64 `json:"scan_keys_sec"`
	Recovery  string  `json:"recovery"`
	Check     string  `json:"check"`
	FileMB    float64 `json:"file_mb"`
	Depth     int     `json:"leaf_depth"`
	Pages     int     `json:"pages"`
}

// Profile is the full performance picture, suitable for charting.
type Profile struct {
	Platform  string       `json:"platform"`
	Latencies []Latency    `json:"latencies"`
	BatchM    []BatchPoint `json:"batch_curve"`
	Scaling   []ScalePoint `json:"scaling"`
}

func tempDB() (string, func()) {
	p := filepath.Join(os.TempDir(), fmt.Sprintf("anvil-prof-%d.anvil", time.Now().UnixNano()))
	os.Remove(p)
	return p, func() { os.Remove(p) }
}

var value16 = []byte("0123456789abcdef")

// RunLatencyProfile measures per-operation latency distributions for a
// single-key commit (two durability barriers) and for a random point read.
func RunLatencyProfile(samples int) ([]Latency, error) {
	if samples <= 0 {
		samples = 2000
	}
	path, cleanup := tempDB()
	defer cleanup()

	db, err := engine.Open(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Preload so reads traverse a real multi-level tree.
	const preload = 100_000
	for i := 0; i < preload; {
		tx, err := db.BeginWrite()
		if err != nil {
			return nil, err
		}
		for j := 0; j < 1000 && i < preload; j, i = j+1, i+1 {
			if err := tx.Put([]byte(fmt.Sprintf("k%09d", i)), value16); err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	commits := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		key := []byte(fmt.Sprintf("c%09d", i))
		t0 := time.Now()
		if err := db.Put(key, value16); err != nil {
			return nil, err
		}
		commits = append(commits, time.Since(t0))
	}

	rng := rand.New(rand.NewSource(7))
	reads := make([]time.Duration, 0, samples*10)
	for i := 0; i < samples*10; i++ {
		key := []byte(fmt.Sprintf("k%09d", rng.Intn(preload)))
		t0 := time.Now()
		if _, err := db.Get(key); err != nil {
			return nil, err
		}
		reads = append(reads, time.Since(t0))
	}

	return []Latency{
		summarize("single-key commit", commits),
		summarize("random point read", reads),
	}, nil
}

// RunBatchCurve measures how batching amortizes the two durability barriers a
// commit must pay. This is the curve behind "batching is 300x faster".
func RunBatchCurve(total int, batches []int) ([]BatchPoint, error) {
	if total <= 0 {
		total = 20_000
	}
	if len(batches) == 0 {
		batches = []int{1, 10, 100, 1000, 10000}
	}
	out := make([]BatchPoint, 0, len(batches))

	for _, b := range batches {
		path, cleanup := tempDB()
		db, err := engine.Open(path)
		if err != nil {
			cleanup()
			return nil, err
		}
		// Smaller batches are far slower; cap the work so a batch of 1 does not
		// dominate the whole run.
		n := total
		if b == 1 {
			n = min(total, 2000)
		} else if b == 10 {
			n = min(total, 10000)
		}

		t0 := time.Now()
		for i := 0; i < n; {
			tx, err := db.BeginWrite()
			if err != nil {
				db.Close()
				cleanup()
				return nil, err
			}
			for j := 0; j < b && i < n; j, i = j+1, i+1 {
				if err := tx.Put([]byte(fmt.Sprintf("k%09d", i)), value16); err != nil {
					tx.Rollback()
					db.Close()
					cleanup()
					return nil, err
				}
			}
			if err := tx.Commit(); err != nil {
				db.Close()
				cleanup()
				return nil, err
			}
		}
		d := time.Since(t0)
		db.Close()
		cleanup()

		ops := float64(n) / d.Seconds()
		out = append(out, BatchPoint{
			Batch:  b,
			OpsSec: ops,
			PerOp:  (d / time.Duration(n)).Round(time.Microsecond).String(),
		})
	}
	return out, nil
}

// RunScalingCurve measures how the engine behaves as the dataset grows, which
// is where a B+tree should show logarithmic read cost and near-constant
// recovery.
func RunScalingCurve(sizes []int) ([]ScalePoint, error) {
	if len(sizes) == 0 {
		sizes = []int{10_000, 100_000, 1_000_000}
	}
	out := make([]ScalePoint, 0, len(sizes))

	for _, n := range sizes {
		path, cleanup := tempDB()
		db, err := engine.Open(path)
		if err != nil {
			cleanup()
			return nil, err
		}

		t0 := time.Now()
		for i := 0; i < n; {
			tx, err := db.BeginWrite()
			if err != nil {
				db.Close()
				cleanup()
				return nil, err
			}
			for j := 0; j < 1000 && i < n; j, i = j+1, i+1 {
				if err := tx.Put([]byte(fmt.Sprintf("k%09d", i)), value16); err != nil {
					tx.Rollback()
					db.Close()
					cleanup()
					return nil, err
				}
			}
			if err := tx.Commit(); err != nil {
				db.Close()
				cleanup()
				return nil, err
			}
		}
		load := time.Since(t0)

		rng := rand.New(rand.NewSource(11))
		reads := make([]time.Duration, 0, 20000)
		for i := 0; i < 20000; i++ {
			key := []byte(fmt.Sprintf("k%09d", rng.Intn(n)))
			t := time.Now()
			if _, err := db.Get(key); err != nil {
				db.Close()
				cleanup()
				return nil, err
			}
			reads = append(reads, time.Since(t))
		}
		rl := summarize("read", reads)

		tx, err := db.BeginRead()
		if err != nil {
			db.Close()
			cleanup()
			return nil, err
		}
		t0 = time.Now()
		scanned := 0
		for it := tx.Scan(nil, nil); it.Next(); {
			scanned++
		}
		scanDur := time.Since(t0)
		tx.Close()
		db.Close()

		t0 = time.Now()
		db2, err := engine.Open(path)
		if err != nil {
			cleanup()
			return nil, err
		}
		rec := time.Since(t0)

		t0 = time.Now()
		rep, err := db2.Check()
		if err != nil {
			db2.Close()
			cleanup()
			return nil, err
		}
		chk := time.Since(t0)
		db2.Close()

		var mb float64
		if info, e := os.Stat(path); e == nil {
			mb = float64(info.Size()) / (1 << 20)
		}
		cleanup()

		out = append(out, ScalePoint{
			Keys:      n,
			LoadOpsS:  float64(n) / load.Seconds(),
			ReadP50:   rl.P50.String(),
			ScanKeysS: float64(scanned) / scanDur.Seconds(),
			Recovery:  rec.Round(time.Microsecond).String(),
			Check:     chk.Round(time.Millisecond).String(),
			FileMB:    mb,
			Depth:     rep.LeafDepth,
			Pages:     rep.PagesChecked,
		})
	}
	return out, nil
}

// RunProfile runs every performance measurement and returns one chartable
// result.
func RunProfile(samples int, sizes []int) (Profile, error) {
	p := Profile{Platform: runtime.GOOS + "/" + runtime.GOARCH}

	lat, err := RunLatencyProfile(samples)
	if err != nil {
		return p, err
	}
	p.Latencies = lat

	bc, err := RunBatchCurve(20_000, nil)
	if err != nil {
		return p, err
	}
	p.BatchM = bc

	sc, err := RunScalingCurve(sizes)
	if err != nil {
		return p, err
	}
	p.Scaling = sc
	return p, nil
}
