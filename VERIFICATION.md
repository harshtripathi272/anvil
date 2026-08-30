# Anvil — Verification Report

**The single source of truth for what Anvil claims, what evidence supports each
claim, and what is explicitly *not* claimed.**

Everything below is reproducible with one command:

```bash
./scripts/verify.sh
```

That script runs all eight checks and writes raw evidence to `results/`. The
committed run of that evidence lives in [`docs/benchmarks/`](docs/benchmarks/).

Last full run: Go 1.27.0, windows/amd64. **All checks passed.**

---

## 1. Claims and evidence

| # | Claim | Evidence | Result |
|---|---|---|---|
| 1 | Zero third-party runtime dependencies | `go.mod` has no `require` block; `go list -m all` prints only `anvil` | ✅ [`deps-proof.txt`](deps-proof.txt) |
| 2 | Builds with one command, statically, without cgo | `CGO_ENABLED=0 go build ./...` | ✅ |
| 3 | Portable across platforms | Cross-compiles linux/amd64, linux/arm64, darwin/arm64, windows/amd64 | ✅ |
| 4 | Idiomatic, clean code | `gofmt -l .` empty; `go vet ./...` clean | ✅ |
| 5 | Correct under normal operation | 19 tests: unit, property, model-based, concurrency | ✅ [`tests.txt`](docs/benchmarks/tests.txt) |
| 6 | **Every acknowledged transaction survives a crash at any commit seam** | 100,000 injected crashes across all 9 seams | ✅ **0 lost, 0 violations** — [`torture.txt`](docs/benchmarks/torture.txt) |
| 7 | **The durability test actually tests durability** | Negative control removes the pre-meta barrier | ✅ **correctly FAILS** — [`negative-control.txt`](docs/benchmarks/negative-control.txt) |
| 8 | Recovers from real process death | Child process `kill`ed mid-commit, file reopened and checked | ✅ `TestRealProcessKillRecovery` |
| 9 | Semantics match an independent oracle | 40,000 random ops vs a plain ordered map (no shared code) | ✅ `TestModelBased` |
| 10 | Readers never observe a partial transaction | 8 concurrent readers assert a two-key invariant across 3,000 commits | ✅ `TestConcurrentReadersAndWriter` |
| 11 | Corruption is detected, not silently accepted | Metadata fallback and checksum tests | ✅ `TestMetadataCorruptionFallback` |
| 12 | Performance is adequate and honestly reported | Benchmark against a real file | ✅ [`bench.txt`](docs/benchmarks/bench.txt) |

---

## 2. The durability model (read this before judging claim 6)

Anvil is **crash-consistent under a documented storage and failure model**. The
model matters, because different crash types prove different things.

| Failure type | How it is exercised | What it proves |
|---|---|---|
| **Simulated power loss** | `FaultStorage`: writes stage in memory and become durable only at `Sync`; a crash discards everything not yet synced | **Durability.** This is the primary instrument — it is the only mode where mis-placed `fsync` calls are detectable |
| **Real process kill** (`SIGKILL`) | A child process commits in a loop and is killed; the parent reopens the real file | **Consistency** on the real syscall path. A process kill does *not* discard page-cache writes, so this alone cannot prove durability |
| **Media corruption** | Byte-flips injected into durable pages | Checksum detection and metadata generation fallback |
| **Real hardware power loss** | — | **Not claimed.** Storage firmware that misreports flushes is outside our evidence |

### Why the negative control is the important result

A crash-torture harness that only kills processes would pass even if every
`fsync` were deleted from the engine — the page cache would serve the data back.
Such a test proves nothing about durability.

So the harness is calibrated: with a required durability barrier **removed**, it
must fail. It does.

```
$ go run ./cmd/anvil torture --negative-control

RESULT: FAIL
  cycle=0 seam=before_meta_sync style=persist-meta-only
  detail: recovery open failed: anvil: database corrupt

EXPECTED FAILURE: with the pre-meta data barrier removed, the
verifier caught the durability violation. The test has teeth.
```

With the correct protocol, the identical injection passes — data is already
durable before the metadata that references it. Both halves of this experiment
are asserted in `TestNegativeControlHasTeeth`.

---

## 3. Crash torture — full result

```
ANVIL CRASH TORTURE

seed:                    430916313
cycles completed:        100000
crashes injected:        100000
recovery failures:       0
acked writes lost:       0
partial txns observed:   0
invariant violations:    0

fault seams exercised:
  after_data_sync      11111      before_data_sync     11111
  after_data_write     11111      before_data_write    11112
  after_meta_sync      11111      before_meta_sync     11111
  after_meta_write     11111      before_meta_write    11111
  before_ack           11111

RESULT: PASS
```

**What each cycle does:** reopen from the durable image → build a random
transaction (1–10 put/delete ops) → inject a crash at the chosen seam → discard
un-synced writes → reopen → run the full structural checker → compare recovered
state against the reference model.

**What is asserted every cycle:**

1. Recovery opens successfully.
2. Every reachable page has a valid checksum.
3. B+tree invariants hold (sorted keys, correct separators, uniform leaf depth,
   no cycles or aliased pages).
4. Recovered state equals the pre-image **or** the post-image — never a partial
   mix. Any other outcome is a failure.
5. No acknowledged write is missing.

**Seam coverage** is round-robin, so all nine commit seams are exercised roughly
equally. Crash points are enumerated at I/O-operation granularity — not every
possible machine instruction. Every run is reproducible from its seed.

---

## 4. Test inventory (19 tests, all passing)

```
$ go test -v ./...      # 4.07s
```

### Format and encoding — `page_test.go`
| Test | What it proves |
|---|---|
| `TestLeafRoundTrip` | Leaf pages encode/decode losslessly, including binary and empty values |
| `TestInternalRoundTrip` | Internal pages preserve separators and child pointers |
| `TestChecksumDetectsCorruption` | A flipped payload byte is caught by CRC-32C |
| `TestMetaRoundTripAndCorruption` | Metadata round-trips; corrupted metadata is rejected |
| `TestKeyValueBounds` | Oversized keys/values and empty keys are rejected cleanly |

### Storage semantics — `db_test.go`
| Test | What it proves |
|---|---|
| `TestBasicPutGetDelete` | Put, get, overwrite, delete, and missing-key behaviour |
| `TestBinarySafeKeysAndValues` | Keys/values containing `0x00` and `0xff` survive intact; empty values are legal |
| `TestManyKeysSplitsAndScan` | 6,000 randomized keys force node and root splits; full and range scans stay ordered and complete |
| `TestDeleteHeavyStaysConsistent` | 1,500 deletes across 3,000 keys leave the tree correct (underfull nodes are legal) |
| `TestReopenFileBackend` | 500 keys survive close and reopen on a real file; structure verifies |
| `TestSnapshotIsolation` | A reader keeps its snapshot while a writer commits 1,001 changes |
| `TestAtomicMultiKeyTransaction` | A multi-key transaction becomes visible all at once |
| `TestRollbackDiscards` | Rollback leaves no trace of uncommitted work |

### Crash safety — `crash_test.go`
| Test | What it proves |
|---|---|
| `TestTortureStandard` | 5,000 crash cycles, all 9 seams covered, zero losses |
| `TestNegativeControlHasTeeth` | Correct protocol survives the ordering injection; **removing the barrier makes it fail** |
| `TestMetadataCorruptionFallback` | Damaged newest generation → falls back to previous; both damaged → fails loudly |

### Equivalence and concurrency
| Test | What it proves |
|---|---|
| `TestModelBased` | 40,000 random operations agree with an independent ordered map at every step and on a final full scan |
| `TestConcurrentReadersAndWriter` | 8 readers + 1 writer, 3,000 commits: no partial transaction, scans stay sorted |
| `TestRealProcessKillRecovery` | A real killed process leaves a recoverable database with all acknowledged keys intact |

**Reference-model independence:** the oracle is a plain Go `map` plus `sort`,
sharing no code with the B+tree implementation. The verifier never asks Anvil to
verify Anvil.

---

## 5. Benchmark

Real file backend, `fdatasync` per commit, 100,000 keys × 16-byte values,
windows/amd64. **Performance is not a competition claim** — this exists to
detect catastrophic regressions and to be honest about the fsync-bound path.

| Operation | Latency | Throughput |
|---|---|---|
| Batched write (1,000 ops/txn) | 355 ms total | **281.9 K ops/s** |
| Single-commit write | 217 µs | 4.6 K ops/s (2 `fdatasync` per commit) |
| Random point read | 23 µs | 43.9 K ops/s |
| Full scan | 9 ms | **11.53 M keys/s** |
| Recovery (reopen) | < 1 ms | generation 2101 |
| Deep structural check | 9 ms | 809 pages, 102,000 keys, leaf depth 2 |
| File size | 27.7 MB | 7,084 pages including copy-on-write history |

Single-commit throughput is deliberately fsync-bound: each commit pays two
durability barriers. Batching amortizes them, which is why the batched figure is
~60× higher. Shadow paging also means each commit rewrites the root-to-leaf path
(tree-height pages) — the same write-amplification trade LMDB and bbolt make in
exchange for crash-safety simplicity and lock-free snapshot reads.

---

## 6. Invariants and where they are enforced

| # | Invariant | Enforced by |
|---|---|---|
| I1 | Committed pages are never modified in place | Copy-on-write in `btree.go`; grow-only allocation |
| I2 | Metadata never references a page that has not satisfied the durability barrier | `Commit` ordering, `tx.go`; proven by the negative control |
| I3 | The committed meta slot is never overwritten before the alternate is durable | Slot alternation by transaction parity, `tx.go` |
| I4 | A commit is acknowledged only after the durable commit point | `Commit` returns after the second `Sync`, `tx.go` |
| I5 | Every page reachable from the committed root is checksum-valid | `checker.go`, run every torture cycle |
| I6 | Every leaf is at the same depth (underfull nodes are legal) | `checker.go` |
| I7 | An active snapshot never observes an overwritten or reclaimed page | Grow-only allocation, by construction in v1 |
| I8 | A failed or uncommitted transaction is never partially visible | Torture all-or-nothing assertion; `TestConcurrentReadersAndWriter` |
| I9 | Recovery selects only a checksum-valid committed generation, or fails loudly | `recover()` in `db.go`; `TestMetadataCorruptionFallback` |
| I10 | The reference model shares no implementation logic with the B+tree | `model_test.go` uses a plain map |

---

## 7. What is NOT claimed, and what is NOT verified

Stated plainly, because a verifier should not have to discover these.

**Not claimed:**
- "Crash-safe" without qualification. The model is documented above.
- Durability against real hardware power loss, or storage devices that lie about
  flushes.
- "Can never lose data", "mathematically proven", or "corruption-proof". This is
  extensive mechanical testing over a defined failure model, not a proof.
- Any performance superiority over LMDB, bbolt, SQLite or similar.
- Architectural novelty. Shadow-paging copy-on-write B+trees with double
  metadata pages are established design (LMDB, bbolt; voidDB is a close Go
  relative). The contribution is a dependency-free from-scratch implementation
  plus the verification methodology — deterministic fault injection in the
  lineage of FoundationDB, TigerBeetle's VOPR, and LazyFS.

**Not verified (honest gaps):**
- The race detector requires a cgo-capable toolchain. The development host has a
  32-bit gcc, so `go test -race` runs in CI (ubuntu-latest) rather than locally.
  `scripts/verify.sh` attempts `-race` and reports which mode ran.
- The Linux `syscall.Fdatasync` path compiles and vets cleanly for
  linux/amd64 and linux/arm64, but the committed benchmark run is from Windows,
  where the barrier is `FlushFileBuffers` via `File.Sync`. CI executes the
  Linux path.
- Crash injection covers I/O-operation seams, not arbitrary instruction
  boundaries.
- Torture has been run to 100,000 cycles per seed. Longer campaigns
  (`--cycles 1000000`) are supported but not part of the committed evidence.

**Version 1 limitations (by design, per the specification):**
- Keys ≤ 1 KiB; key + value ≤ ~2 KiB (no overflow pages).
- Single writer; many concurrent readers.
- Grow-only allocation: freed pages are not reused, so the file grows with
  history. Space reclamation is deliberately deferred to keep the commit
  protocol — the part that must be correct — small and verifiable.
- No write-ahead log (by design), SQL, secondary indexes, networking, or
  replication.
