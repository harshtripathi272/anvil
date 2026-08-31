package verification

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/harshtripathi272/anvil/engine"
)

// ConcurrencyConfig configures a concurrent reader/writer run.
type ConcurrencyConfig struct {
	Commits int // write transactions the single writer performs
	Readers int // concurrent snapshot readers
}

// ConcurrencyResult is the verdict of a concurrency run.
type ConcurrencyResult struct {
	Commits        int
	Readers        int
	SnapshotsTaken int64
	OK             bool
	FailDetail     string
}

// RunConcurrencyCheck runs many snapshot readers against a single writer and
// asserts that no reader ever observes a partial transaction.
//
// Each commit sets two keys ("counter" and "mirror") to the same value in one
// transaction. A reader that ever sees them disagree has observed a torn
// transaction, which would violate snapshot isolation (SRS v1.1 §I8). Readers
// also assert their scans stay in key order within a snapshot.
func RunConcurrencyCheck(cfg ConcurrencyConfig) ConcurrencyResult {
	if cfg.Commits <= 0 {
		cfg.Commits = 3000
	}
	if cfg.Readers <= 0 {
		cfg.Readers = 8
	}
	res := ConcurrencyResult{Commits: cfg.Commits, Readers: cfg.Readers, OK: true}

	db, err := engine.OpenStorage(NewFaultStorage())
	if err != nil {
		res.OK, res.FailDetail = false, "open: "+err.Error()
		return res
	}
	defer db.Close()

	for _, k := range []string{"counter", "mirror"} {
		if err := db.Put([]byte(k), []byte("0")); err != nil {
			res.OK, res.FailDetail = false, "seed: "+err.Error()
			return res
		}
	}
	for i := 0; i < 200; i++ {
		db.Put([]byte(fmt.Sprintf("seed%04d", i)), []byte("x"))
	}

	var (
		mu        sync.Mutex
		failure   string
		snapshots int64
		wg        sync.WaitGroup
	)
	stop := make(chan struct{})
	report := func(msg string) {
		mu.Lock()
		if failure == "" {
			failure = msg
		}
		mu.Unlock()
	}

	// Writer: bump counter and mirror together, so they agree in every
	// committed state and disagree in any torn one.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		for i := 1; i <= cfg.Commits; i++ {
			tx, err := db.BeginWrite()
			if err != nil {
				report("begin write: " + err.Error())
				return
			}
			val := []byte(fmt.Sprintf("%d", i))
			tx.Put([]byte("counter"), val)
			tx.Put([]byte("mirror"), val)
			tx.Put([]byte(fmt.Sprintf("churn%04d", i%500)), val)
			if err := tx.Commit(); err != nil {
				report("commit: " + err.Error())
				return
			}
		}
	}()

	wg.Add(cfg.Readers)
	for r := 0; r < cfg.Readers; r++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				tx, err := db.BeginRead()
				if err != nil {
					report("begin read: " + err.Error())
					return
				}
				atomic.AddInt64(&snapshots, 1)

				counter, errC := tx.Get([]byte("counter"))
				mirror, errM := tx.Get([]byte("mirror"))
				if errC != nil || errM != nil {
					tx.Close()
					report(fmt.Sprintf("read counter/mirror: %v %v", errC, errM))
					return
				}
				if !bytes.Equal(counter, mirror) {
					tx.Close()
					report(fmt.Sprintf("partial transaction observed: counter=%s mirror=%s", counter, mirror))
					return
				}

				var prev []byte
				it := tx.Scan(nil, nil)
				for it.Next() {
					k := it.Key()
					if prev != nil && bytes.Compare(prev, k) >= 0 {
						tx.Close()
						report(fmt.Sprintf("scan out of order: %q then %q", prev, k))
						return
					}
					prev = k
				}
				tx.Close()
			}
		}()
	}

	wg.Wait()
	res.SnapshotsTaken = atomic.LoadInt64(&snapshots)

	if failure != "" {
		res.OK, res.FailDetail = false, failure
		return res
	}
	final, err := db.Get([]byte("counter"))
	if err != nil || string(final) != fmt.Sprintf("%d", cfg.Commits) {
		res.OK = false
		res.FailDetail = fmt.Sprintf("final counter = %q (%v), expected %d", final, err, cfg.Commits)
	}
	return res
}
