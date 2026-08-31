package verification

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"

	"github.com/harshtripathi272/anvil/engine"
)

// ModelConfig configures a model-equivalence run.
type ModelConfig struct {
	Seed     int64
	Steps    int // total operations
	Keyspace int // distinct keys operations range over
}

// ModelResult is the verdict of a model-equivalence run.
type ModelResult struct {
	Seed        int64
	Steps       int
	Keys        int // keys present at the end
	Mismatches  int
	OK          bool
	FailStep    int
	FailDetail  string
	CheckReport engine.CheckReport
}

// RunModelCheck drives Anvil and an independent reference model through a long
// randomized operation sequence and asserts their observable states agree at
// every step and on a final ordered scan.
//
// The reference model is a plain Go map plus sort — deliberately sharing no
// implementation logic with the B+tree, so a bug in the engine cannot reproduce
// itself in the oracle (SRS v1.1 §I10).
func RunModelCheck(cfg ModelConfig) ModelResult {
	if cfg.Steps <= 0 {
		cfg.Steps = 40000
	}
	if cfg.Keyspace <= 0 {
		cfg.Keyspace = 400
	}
	res := ModelResult{Seed: cfg.Seed, OK: true}

	db, err := engine.OpenStorage(NewFaultStorage())
	if err != nil {
		return modelFail(res, 0, "open: "+err.Error())
	}
	defer db.Close()

	ref := map[string]string{}
	rng := rand.New(rand.NewSource(cfg.Seed))

	for step := 0; step < cfg.Steps; step++ {
		res.Steps++
		key := fmt.Sprintf("k%03d", rng.Intn(cfg.Keyspace))

		switch r := rng.Intn(100); {
		case r < 50: // put
			val := fmt.Sprintf("v%d", rng.Intn(1_000_000))
			if err := db.Put([]byte(key), []byte(val)); err != nil {
				return modelFail(res, step, "put: "+err.Error())
			}
			ref[key] = val

		case r < 65: // multi-key atomic transaction
			tx, err := db.BeginWrite()
			if err != nil {
				return modelFail(res, step, "begin write: "+err.Error())
			}
			batch := map[string]string{}
			for i := 0; i < 1+rng.Intn(5); i++ {
				k := fmt.Sprintf("k%03d", rng.Intn(cfg.Keyspace))
				v := fmt.Sprintf("t%d", rng.Intn(1_000_000))
				if err := tx.Put([]byte(k), []byte(v)); err != nil {
					tx.Rollback()
					return modelFail(res, step, "txn put: "+err.Error())
				}
				batch[k] = v
			}
			if err := tx.Commit(); err != nil {
				return modelFail(res, step, "commit: "+err.Error())
			}
			for k, v := range batch {
				ref[k] = v
			}

		case r < 80: // delete
			if err := db.Delete([]byte(key)); err != nil {
				return modelFail(res, step, "delete: "+err.Error())
			}
			delete(ref, key)

		default: // get, compared against the model
			got, err := db.Get([]byte(key))
			want, exists := ref[key]
			if exists {
				if err != nil || string(got) != want {
					res.Mismatches++
					return modelFail(res, step,
						fmt.Sprintf("get %q = %q (%v), model says %q", key, got, err, want))
				}
			} else if err != engine.ErrNotFound {
				res.Mismatches++
				return modelFail(res, step,
					fmt.Sprintf("get %q returned %q (%v), model says absent", key, got, err))
			}
		}
	}

	// Final ordered scan must match the model exactly.
	want := make([]string, 0, len(ref))
	for k := range ref {
		want = append(want, k)
	}
	sort.Strings(want)
	res.Keys = len(want)

	tx, err := db.BeginRead()
	if err != nil {
		return modelFail(res, cfg.Steps, "begin read: "+err.Error())
	}
	it := tx.Scan(nil, nil)
	i := 0
	for it.Next() {
		if i >= len(want) {
			tx.Close()
			return modelFail(res, cfg.Steps, fmt.Sprintf("scan returned extra key %q", it.Key()))
		}
		if string(it.Key()) != want[i] {
			tx.Close()
			return modelFail(res, cfg.Steps,
				fmt.Sprintf("scan[%d] = %q, model says %q", i, it.Key(), want[i]))
		}
		if !bytes.Equal(it.Value(), []byte(ref[want[i]])) {
			tx.Close()
			return modelFail(res, cfg.Steps, fmt.Sprintf("value mismatch at %q", want[i]))
		}
		i++
	}
	scanErr := it.Err()
	tx.Close()
	if scanErr != nil {
		return modelFail(res, cfg.Steps, "scan: "+scanErr.Error())
	}
	if i != len(want) {
		return modelFail(res, cfg.Steps, fmt.Sprintf("scan returned %d keys, model says %d", i, len(want)))
	}

	rep, err := db.Check()
	if err != nil {
		return modelFail(res, cfg.Steps, "structural check: "+err.Error())
	}
	res.CheckReport = rep
	return res
}

func modelFail(res ModelResult, step int, detail string) ModelResult {
	res.OK = false
	res.FailStep = step
	res.FailDetail = detail
	return res
}
