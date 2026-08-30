package anvil

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestConcurrentReadersAndWriter runs many readers against a single writer and
// checks two invariants under the race detector (SRS v1.1 §76, §I7, §I8):
//   - snapshot isolation: a reader never sees a partial transaction. Each commit
//     atomically sets "counter" and "mirror" to the same value, so every snapshot
//     must observe counter == mirror.
//   - ordered, self-consistent scans with no data races.
func TestConcurrentReadersAndWriter(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	db.Put([]byte("counter"), []byte("0"))
	db.Put([]byte("mirror"), []byte("0"))
	for i := 0; i < 200; i++ {
		db.Put([]byte(fmt.Sprintf("seed%04d", i)), []byte("x"))
	}

	const commits = 3000
	var writes int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer: atomically bump counter and mirror together, plus churn keys.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= commits; i++ {
			tx, err := db.BeginWrite()
			if err != nil {
				t.Errorf("begin write: %v", err)
				return
			}
			val := []byte(fmt.Sprintf("%d", i))
			tx.Put([]byte("counter"), val)
			tx.Put([]byte("mirror"), val)
			tx.Put([]byte(fmt.Sprintf("churn%04d", i%500)), val)
			if err := tx.Commit(); err != nil {
				t.Errorf("commit: %v", err)
				return
			}
			atomic.AddInt64(&writes, 1)
		}
		close(stop)
	}()

	// Readers: verify counter == mirror in every snapshot and scans stay sorted.
	readers := 8
	wg.Add(readers)
	for r := 0; r < readers; r++ {
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
					t.Errorf("begin read: %v", err)
					return
				}
				c, errC := tx.Get([]byte("counter"))
				m, errM := tx.Get([]byte("mirror"))
				if errC != nil || errM != nil {
					t.Errorf("read counter/mirror: %v %v", errC, errM)
					tx.Close()
					return
				}
				if !bytes.Equal(c, m) {
					t.Errorf("partial transaction observed: counter=%s mirror=%s", c, m)
					tx.Close()
					return
				}
				// Scan must stay sorted within the snapshot.
				it := tx.Scan(nil, nil)
				var prev []byte
				for it.Next() {
					k := it.Key()
					if prev != nil && bytes.Compare(prev, k) >= 0 {
						t.Errorf("scan not sorted: %q then %q", prev, k)
						break
					}
					prev = k
				}
				tx.Close()
			}
		}()
	}

	wg.Wait()
	if atomic.LoadInt64(&writes) != commits {
		t.Fatalf("writer completed %d/%d commits", writes, commits)
	}
	if v, _ := db.Get([]byte("counter")); string(v) != fmt.Sprintf("%d", commits) {
		t.Fatalf("final counter = %s want %d", v, commits)
	}
}
