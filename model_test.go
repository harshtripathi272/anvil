package anvil

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// TestModelBased drives Anvil and an independent ordered reference model (a
// plain map — materially different code, per SRS v1.1 §I10) through a long
// random operation sequence and asserts their observable states always agree
// (Layer 3, §41).
func TestModelBased(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	ref := map[string]string{}
	rng := rand.New(rand.NewSource(0xC0FFEE))

	const steps = 40000
	for step := 0; step < steps; step++ {
		key := fmt.Sprintf("k%03d", rng.Intn(400))
		switch r := rng.Intn(100); {
		case r < 50: // put
			val := fmt.Sprintf("v%d", rng.Intn(1_000_000))
			if err := db.Put([]byte(key), []byte(val)); err != nil {
				t.Fatalf("step %d put: %v", step, err)
			}
			ref[key] = val

		case r < 65: // multi-key atomic transaction
			tx, _ := db.BeginWrite()
			n := 1 + rng.Intn(5)
			batch := map[string]string{}
			for i := 0; i < n; i++ {
				k := fmt.Sprintf("k%03d", rng.Intn(400))
				v := fmt.Sprintf("t%d", rng.Intn(1_000_000))
				tx.Put([]byte(k), []byte(v))
				batch[k] = v
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("step %d txn: %v", step, err)
			}
			for k, v := range batch {
				ref[k] = v
			}

		case r < 80: // delete
			if err := db.Delete([]byte(key)); err != nil {
				t.Fatalf("step %d delete: %v", step, err)
			}
			delete(ref, key)

		default: // get
			got, err := db.Get([]byte(key))
			want, ok := ref[key]
			if ok {
				if err != nil || string(got) != want {
					t.Fatalf("step %d get %q = %q,%v want %q", step, key, got, err, want)
				}
			} else if err != ErrNotFound {
				t.Fatalf("step %d get %q expected ErrNotFound, got %q,%v", step, key, got, err)
			}
		}
	}

	// Final full-scan equality against the reference model.
	want := make([]string, 0, len(ref))
	for k := range ref {
		want = append(want, k)
	}
	sort.Strings(want)

	tx, _ := db.BeginRead()
	it := tx.Scan(nil, nil)
	i := 0
	for it.Next() {
		if i >= len(want) {
			t.Fatalf("scan returned extra key %q", it.Key())
		}
		if string(it.Key()) != want[i] {
			t.Fatalf("scan[%d] = %q want %q", i, it.Key(), want[i])
		}
		if !bytes.Equal(it.Value(), []byte(ref[want[i]])) {
			t.Fatalf("scan value mismatch at %q", want[i])
		}
		i++
	}
	tx.Close()
	if i != len(want) {
		t.Fatalf("scan returned %d keys, want %d", i, len(want))
	}

	if _, err := db.Check(); err != nil {
		t.Fatalf("final structural check: %v", err)
	}
}
