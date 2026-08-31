package verification

import "testing"

// TestConcurrentReadersAndWriter runs eight snapshot readers against a single
// writer and asserts no reader ever observes a partial transaction.
//
// Run with -race to also cover data races. The logic lives in
// RunConcurrencyCheck so `go test` and `anvil verify` share one implementation.
func TestConcurrentReadersAndWriter(t *testing.T) {
	res := RunConcurrencyCheck(ConcurrencyConfig{Commits: 3000, Readers: 8})
	if !res.OK {
		t.Fatalf("concurrency check failed: %s", res.FailDetail)
	}
	t.Logf("%d commits, %d readers, %d snapshots taken, 0 partial reads",
		res.Commits, res.Readers, res.SnapshotsTaken)
}
