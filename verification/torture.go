package verification

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"github.com/harshtripathi272/anvil/engine"
)

// errInjectedCrash aborts a commit at a chosen seam, standing in for abrupt
// process/power loss at that exact point in the protocol.
var errInjectedCrash = errors.New("anvil: injected crash")

// commitSeams are the persistence boundaries the harness crashes at. The engine
// defines them; the harness only enumerates them.
var commitSeams = engine.CommitSeams

// TortureConfig configures a crash-torture run.
type TortureConfig struct {
	Seed     int64
	Cycles   int
	Keyspace int // number of distinct keys operations range over
	MaxOps   int // max operations per transaction
}

// TortureResult is the machine-checkable verdict of a torture run
// (SRS v1.1 §70, §115).
type TortureResult struct {
	Seed                int64
	Cycles              int
	CrashesInjected     int
	RecoveryFailures    int
	AckedWritesLost     int
	InvariantViolations int
	PartialTxObserved   int
	SeamCoverage        map[string]int

	OK         bool
	FailCycle  int
	FailSeam   string
	FailStyle  string
	FailDetail string
}

type crashStyle int

const (
	styleClean       crashStyle = iota // discard all un-synced writes
	stylePersistMeta                   // persist only the meta page (ordering hazard)
)

func (c crashStyle) String() string {
	if c == stylePersistMeta {
		return "persist-meta-only"
	}
	return "clean"
}

// tortureOpts is the internal, fully-parameterized torture driver config.
type tortureOpts struct {
	TortureConfig
	skipDataSync bool       // negative control: remove the pre-meta data barrier
	style        crashStyle // how a crash persists un-synced writes
	fixedSeam    string     // if set, always inject here; else round-robin seams
}

// RunTorture runs the standard proof: clean crashes injected round-robin across
// every commit seam, with the correct protocol. It is expected to pass with
// zero lost acknowledged writes and zero invariant violations.
func RunTorture(cfg TortureConfig) TortureResult {
	return runTorture(tortureOpts{TortureConfig: cfg, style: styleClean})
}

// RunNegativeControl runs the ordering-hazard injection (a crash that persists
// only the meta page during its sync) with the required pre-meta data barrier
// REMOVED. A correct harness MUST report FAIL here: the meta reaches the platter
// while the data it references does not. This is the experiment that proves the
// durability test has teeth (SRS v1.1 §G) — if it ever returns OK, the harness
// is not actually testing durability.
func RunNegativeControl(cfg TortureConfig) TortureResult {
	if cfg.Cycles <= 0 {
		cfg.Cycles = 200
	}
	return runTorture(tortureOpts{
		TortureConfig: cfg,
		style:         stylePersistMeta,
		fixedSeam:     "before_meta_sync",
		skipDataSync:  true,
	})
}

// runTorture is the flexible driver used by RunTorture and by the negative
// control / ordering tests.
func runTorture(opts tortureOpts) TortureResult {
	if opts.Cycles <= 0 {
		opts.Cycles = 1000
	}
	if opts.Keyspace <= 0 {
		opts.Keyspace = 500
	}
	if opts.MaxOps <= 0 {
		opts.MaxOps = 8
	}
	rng := rand.New(rand.NewSource(opts.Seed))
	res := TortureResult{Seed: opts.Seed, OK: true, SeamCoverage: map[string]int{}}

	storage := NewFaultStorage()
	model := map[string]string{} // durable, acknowledged state

	// Bootstrap an empty database.
	db, err := engine.OpenStorage(storage)
	if err != nil {
		res.OK = false
		res.FailDetail = "initial open: " + err.Error()
		return res
	}
	db.Close()

	for cycle := 0; cycle < opts.Cycles; cycle++ {
		res.Cycles++

		seam := opts.fixedSeam
		if seam == "" {
			seam = commitSeams[cycle%len(commitSeams)]
		}

		// Arm the crash at this cycle's seam before opening, so the hook is in
		// place for the single commit this cycle performs.
		hit := false
		openOpts := []engine.Option{
			engine.WithCheckpoint(func(s string) error {
				if s == seam {
					hit = true
					return errInjectedCrash
				}
				return nil
			}),
		}
		if opts.skipDataSync {
			// Negative control: deliberately break the commit protocol.
			openOpts = append(openOpts, engine.WithoutDataSync())
		}

		// Reopen over the current durable image.
		db, err := engine.OpenStorage(storage, openOpts...)
		if err != nil {
			return fail(res, cycle, seam, opts.style, "reopen before txn: "+err.Error())
		}
		baseTx, _, _ := db.Info()
		metaSlot := (baseTx + 1) % 2

		// Build a random transaction; compute the would-be post state.
		after := cloneModel(model)
		ops := 1 + rng.Intn(opts.MaxOps)
		wtx, err := db.BeginWrite()
		if err != nil {
			return fail(res, cycle, seam, opts.style, "begin write: "+err.Error())
		}
		for i := 0; i < ops; i++ {
			key := fmt.Sprintf("k%04d", rng.Intn(opts.Keyspace))
			if rng.Intn(100) < 25 {
				wtx.Delete([]byte(key))
				delete(after, key)
			} else {
				val := fmt.Sprintf("v%d-%d", cycle, i)
				if err := wtx.Put([]byte(key), []byte(val)); err != nil {
					return fail(res, cycle, seam, opts.style, "put: "+err.Error())
				}
				after[key] = val
			}
		}

		// Attempt the commit; the armed checkpoint aborts it at the seam.
		commitErr := wtx.Commit()
		if !errors.Is(commitErr, errInjectedCrash) {
			// Seam not reached (shouldn't happen) — count as a clean commit.
			if commitErr == nil {
				model = after
			}
		}
		if hit {
			res.CrashesInjected++
			res.SeamCoverage[seam]++
		}

		// Simulate the crash.
		switch opts.style {
		case stylePersistMeta:
			storage.CrashPersistingOnly(metaSlot)
		default:
			storage.Crash()
		}

		// Recover from exactly the durable bytes and verify.
		rdb, err := engine.OpenStorage(storage)
		if err != nil {
			res.RecoveryFailures++
			return fail(res, cycle, seam, opts.style, "recovery open failed: "+err.Error())
		}
		if _, err := rdb.Check(); err != nil {
			res.InvariantViolations++
			return fail(res, cycle, seam, opts.style, "structural check: "+err.Error())
		}
		got, err := scanAll(rdb)
		if err != nil {
			res.InvariantViolations++
			return fail(res, cycle, seam, opts.style, "scan after recovery: "+err.Error())
		}

		// The unacknowledged transaction must be all-or-nothing: recovered state
		// equals the pre-image or the post-image, never a partial mix (I8, §50).
		switch {
		case mapsEqual(got, model):
			// transaction did not take; model unchanged
		case mapsEqual(got, after):
			model = after // transaction took durably
		default:
			// Determine whether an acknowledged (pre-image) write was lost.
			for k, v := range model {
				if got[k] != v {
					res.AckedWritesLost++
					break
				}
			}
			res.PartialTxObserved++
			res.InvariantViolations++
			return fail(res, cycle, seam, opts.style, partialDetail(model, after, got))
		}
		rdb.Close()
	}
	return res
}

func fail(res TortureResult, cycle int, seam string, style crashStyle, detail string) TortureResult {
	res.OK = false
	res.FailCycle = cycle
	res.FailSeam = seam
	res.FailStyle = style.String()
	res.FailDetail = detail
	return res
}

func partialDetail(before, after, got map[string]string) string {
	return fmt.Sprintf("recovered state matches neither pre-image (%d keys) nor post-image (%d keys): got %d keys",
		len(before), len(after), len(got))
}

func scanAll(db *engine.DB) (map[string]string, error) {
	tx, err := db.BeginRead()
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	m := map[string]string{}
	it := tx.Scan(nil, nil)
	for it.Next() {
		m[string(it.Key())] = string(it.Value())
	}
	return m, it.Err()
}

func cloneModel(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// Report renders a human-readable torture verdict (used by the CLI).
func (r TortureResult) Report() string {
	seams := make([]string, 0, len(r.SeamCoverage))
	for s := range r.SeamCoverage {
		seams = append(seams, s)
	}
	sort.Strings(seams)

	out := "ANVIL CRASH TORTURE\n\n"
	out += fmt.Sprintf("seed:                    %d\n", r.Seed)
	out += fmt.Sprintf("cycles completed:        %d\n", r.Cycles)
	out += fmt.Sprintf("crashes injected:        %d\n", r.CrashesInjected)
	out += fmt.Sprintf("recovery failures:       %d\n", r.RecoveryFailures)
	out += fmt.Sprintf("acked writes lost:       %d\n", r.AckedWritesLost)
	out += fmt.Sprintf("partial txns observed:   %d\n", r.PartialTxObserved)
	out += fmt.Sprintf("invariant violations:    %d\n\n", r.InvariantViolations)
	out += "fault seams exercised:\n"
	for _, s := range seams {
		out += fmt.Sprintf("  %-20s %d\n", s, r.SeamCoverage[s])
	}
	out += "\n"
	if r.OK {
		out += "RESULT: PASS\n"
	} else {
		out += fmt.Sprintf("RESULT: FAIL\n  cycle=%d seam=%s style=%s\n  detail: %s\n",
			r.FailCycle, r.FailSeam, r.FailStyle, r.FailDetail)
	}
	return out
}
