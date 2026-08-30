package verification

import (
	"testing"

	"github.com/harshtripathi272/anvil/engine"
)

// TestTortureStandard is the primary durability proof: clean crashes injected
// round-robin across every commit seam must always recover to a valid committed
// generation with no lost acknowledged writes and no invariant violations.
func TestTortureStandard(t *testing.T) {
	res := RunTorture(TortureConfig{Seed: 0x19af42d9, Cycles: 5000, Keyspace: 400, MaxOps: 10})
	if !res.OK {
		t.Fatalf("torture failed:\n%s", res.Report())
	}
	if res.AckedWritesLost != 0 || res.InvariantViolations != 0 || res.RecoveryFailures != 0 {
		t.Fatalf("nonzero failures:\n%s", res.Report())
	}
	// Every seam must have been exercised.
	for _, seam := range commitSeams {
		if res.SeamCoverage[seam] == 0 {
			t.Fatalf("seam %q never exercised", seam)
		}
	}
	t.Logf("\n%s", res.Report())
}

// TestNegativeControlHasTeeth is the credibility experiment (SRS v1.1 §C1, §G).
// The ordering-hazard injection (persist only the meta page during its sync)
// must PASS with the correct protocol and FAIL when the required pre-meta data
// barrier is removed. If both passed, the harness would be proving nothing.
func TestNegativeControlHasTeeth(t *testing.T) {
	seam := "before_meta_sync"

	// Correct protocol: data is already durable before the meta, so persisting
	// only the meta is still consistent.
	good := runTorture(tortureOpts{
		TortureConfig: TortureConfig{Seed: 7, Cycles: 300, Keyspace: 200, MaxOps: 6},
		style:         stylePersistMeta,
		fixedSeam:     seam,
		skipDataSync:  false,
	})
	if !good.OK {
		t.Fatalf("correct protocol should survive the ordering injection:\n%s", good.Report())
	}

	// Negative control: remove the pre-meta data barrier. The meta can now reach
	// the platter while the data it references does not -> recovery must break.
	bad := runTorture(tortureOpts{
		TortureConfig: TortureConfig{Seed: 7, Cycles: 300, Keyspace: 200, MaxOps: 6},
		style:         stylePersistMeta,
		fixedSeam:     seam,
		skipDataSync:  true,
	})
	if bad.OK {
		t.Fatalf("NEGATIVE CONTROL FAILED: harness did not catch the removed barrier — it has no teeth")
	}
	t.Logf("negative control correctly detected the durability failure: %s", bad.FailDetail)
}

// TestMetadataCorruptionFallback exercises SRS v1.1 §52: if the newest metadata
// generation is damaged, recovery falls back to the previous valid generation;
// if both are damaged, recovery fails loudly.
func TestMetadataCorruptionFallback(t *testing.T) {
	s := NewFaultStorage()
	db := openFault(t, s)
	db.Put([]byte("a"), []byte("1")) // gen 2 -> slot 0
	db.Put([]byte("b"), []byte("2")) // gen 3 -> slot 1 (newest)
	newestTx, _, _ := db.Info()
	db.Close()

	// Damage the newest meta slot.
	if !s.CorruptDurablePage(newestTx % 2) {
		t.Fatal("failed to corrupt newest meta")
	}
	db2, err := engine.OpenStorage(s)
	if err != nil {
		t.Fatalf("should have fallen back to previous generation, got: %v", err)
	}
	// Previous generation had a=1 but not b=2.
	if v, err := db2.Get([]byte("a")); err != nil || string(v) != "1" {
		t.Fatalf("fallback lost a: %q %v", v, err)
	}
	if _, err := db2.Get([]byte("b")); err != engine.ErrNotFound {
		t.Fatalf("fallback should predate b: %v", err)
	}
	if _, err := db2.Check(); err != nil {
		t.Fatalf("fallback generation fails structural check: %v", err)
	}
	prevTx, _, _ := db2.Info()
	db2.Close()

	// Damage the surviving slot too -> both invalid -> loud failure.
	if !s.CorruptDurablePage(prevTx % 2) {
		t.Fatal("failed to corrupt previous meta")
	}
	if _, err := engine.OpenStorage(s); err == nil {
		t.Fatal("expected engine.ErrCorrupt when both meta slots are damaged")
	}
}
