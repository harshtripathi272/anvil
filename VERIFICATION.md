# Anvil — Verification Report

**The single source of truth for what Anvil claims, what evidence supports each
claim, and what is explicitly *not* claimed.**

Two commands reproduce everything:

```bash
anvil verify              # the whole evidence set, in process
./scripts/verify.sh       # the above, plus the toolchain checks a shell must do
```

The script owns only what needs the Go toolchain (dependency proof, static
build, cross-compile, gofmt, vet, race tests) and delegates the rest to
`anvil verify`, so the two can never test different things. Raw evidence from
the committed run is in [`docs/benchmarks/`](docs/benchmarks/).

Last full run on **two platforms**, Go 1.27.0 — **all nine checks passed on both**:

| Platform | Race detector | Durability barrier | Evidence |
|---|---|---|---|
| linux/amd64 | ✅ `go test -race`, 22 tests | real `syscall.Fdatasync` | [`docs/benchmarks/linux/`](docs/benchmarks/linux/) |
| windows/amd64 | unavailable (no cgo toolchain) | `FlushFileBuffers` via `File.Sync` | [`docs/benchmarks/`](docs/benchmarks/) |

---

## 1. Claims and evidence

| # | Claim | Evidence | Result |
|---|---|---|---|
| 1 | Zero third-party runtime dependencies | `go.mod` has no `require` block; `go list -m all` prints one module | ✅ [`deps-proof.txt`](deps-proof.txt) |
| 2 | Builds with one command, statically, without cgo | `CGO_ENABLED=0 go build ./...` | ✅ |
| 3 | Portable across platforms | Cross-compiles linux/amd64, linux/arm64, darwin/arm64, windows/amd64 | ✅ |
| 4 | Idiomatic, clean code | `gofmt -l .` empty; `go vet ./...` clean | ✅ |
| 5 | Correct under normal operation | 22 tests: format, storage semantics, property, model, concurrency | ✅ [`tests.txt`](docs/benchmarks/tests.txt) |
| 6 | **Every acknowledged transaction survives a crash at any commit seam** | 100,000 injected crashes across all 9 seams | ✅ **0 lost, 0 violations** — [`torture.txt`](docs/benchmarks/torture.txt) |
| 7 | **The durability test actually tests durability** | Negative control removes the pre-metadata barrier | ✅ **correctly FAILS** — [`negative-control.txt`](docs/benchmarks/negative-control.txt) |
| 8 | Recovers from real process death | A child process is killed mid-commit; the file is reopened and checked | ✅ [`verify.txt`](docs/benchmarks/verify.txt) |
| 9 | Semantics match an independent oracle | 40,000 random ops vs a plain ordered map (no shared code) | ✅ [`verify.txt`](docs/benchmarks/verify.txt) |
| 10 | Readers never observe a partial transaction | 8 concurrent readers, 1,934 snapshots across 3,000 commits | ✅ [`verify.txt`](docs/benchmarks/verify.txt) |
| 11 | Corruption is detected, not silently accepted | Metadata fallback and checksum tests | ✅ `TestMetadataCorruptionFallback` |
| 12 | Performance is adequate and honestly reported | Benchmark against a real file | ✅ [`bench.txt`](docs/benchmarks/bench.txt) |

---

## 2. The durability model (read this before judging claim 6)

Anvil is **crash-consistent under a documented storage and failure model**. The
model matters, because different crash types prove different things.

| Failure type | How it is exercised | What it proves |
|---|---|---|
| **Simulated power loss** | `FaultStorage`: writes stage in memory and become durable only at `Sync`; a crash discards everything not yet synced | **Durability.** The primary instrument — the only mode where a mis-placed `fsync` is detectable |
| **Real process kill** (`SIGKILL`) | A child process commits in a loop and is killed; the parent reopens the real file | **Consistency** on the real syscall path. A process kill does *not* discard page-cache writes, so this alone cannot prove durability |
| **Media corruption** | Byte-flips injected into durable pages | Checksum detection and metadata generation fallback |
| **Real hardware power loss** | — | **Not claimed.** Storage firmware that misreports flushes is outside our evidence |

### Why the negative control is the important result

A crash-torture harness that only kills processes would pass even if every
`fsync` were deleted from the engine — the page cache would serve the data back.
Such a test proves nothing about durability.

So the harness is calibrated: with a required durability barrier **removed**, it
must fail. It does.

```console
$ anvil torture --negative-control

  RESULT: FAIL
    cycle 0, seam before_meta_sync, crash style persist-meta-only
    recovery open failed: anvil: database corrupt

  EXPECTED FAILURE: with the pre-metadata durability barrier removed,
  the verifier caught the resulting data loss. The test has teeth.
```

With the correct protocol, the identical injection passes — data is already
durable before the metadata that references it. Both halves of this experiment
are asserted in `TestNegativeControlHasTeeth`, and the suite treats "the
negative control failed" as the passing outcome.

---

## 3. The verification suite

`anvil verify` runs five checks in process. `go test` runs the same functions,
so neither can drift from the other.

```
ANVIL VERIFY  seed 430916313

  PASS  model equivalence    40000 operations, 350 keys, 0 mismatches
                             expected: matches an independent reference model (738ms)
  PASS  concurrency          3000 commits, 8 readers, 1934 snapshots, 0 partial reads
                             expected: no reader observes a partial transaction (296ms)
  PASS  crash torture        100000 crashes across 9 seams, 0 lost, 0 violations
                             expected: 0 acknowledged writes lost, 0 invariant violations (1m48.752s)
  PASS  negative control     durability violation correctly detected: recovery open failed: anvil: database corrupt
                             expected: harness detects a removed durability barrier (0s)
  PASS  process kill         killed at generation 1308; 1307 acknowledged keys intact, 8 pages valid
                             expected: recovers cleanly after SIGKILL on a real file (286ms)

  5 checks in 1m50.072s

  RESULT: PASS
```

| Check | Implementation | What it establishes |
|---|---|---|
| model equivalence | `verification/model.go` | Observable behaviour matches an independent oracle over 40,000 randomized operations |
| concurrency | `verification/concurrency.go` | No snapshot reader ever sees a torn transaction |
| crash torture | `verification/torture.go` | Durability across every commit seam |
| negative control | `verification/torture.go` | The durability test has teeth |
| process kill | `verification/suite.go` | Real `SIGKILL` on the real file recovers cleanly |

---

## 4. Crash torture — full result

```
ANVIL TORTURE  seed 430916313

  cycles completed       100000
  crashes injected       100000
  recovery failures      0
  acked writes lost      0
  partial txns observed  0
  invariant violations   0

  commit seams exercised
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
equally. The seams are defined by the engine (`engine.CommitSeams`) and consumed
by the harness, so a new seam cannot be silently skipped. Crash points are
enumerated at I/O-operation granularity — not every possible machine
instruction. Every run is reproducible from its seed.

---

## 5. Test inventory (22 tests, all passing)

```console
$ go test -count=1 -v ./...
ok  github.com/harshtripathi272/anvil/cli           0.027s
ok  github.com/harshtripathi272/anvil/engine        0.026s
ok  github.com/harshtripathi272/anvil/verification  3.855s
```

### Format and encoding — `engine/page_test.go` (white-box)
| Test | What it proves |
|---|---|
| `TestLeafRoundTrip` | Leaf pages encode/decode losslessly, including binary and empty values |
| `TestInternalRoundTrip` | Internal pages preserve separators and child pointers |
| `TestChecksumDetectsCorruption` | A flipped payload byte is caught by CRC-32C |
| `TestMetaRoundTripAndCorruption` | Metadata round-trips; corrupted metadata is rejected |
| `TestKeyValueBounds` | Oversized keys/values and empty keys are rejected cleanly |

These stay in `engine` because they exercise unexported format internals — page
offsets, node encoding, metadata layout. Relocating them would mean exporting
the entire on-disk format.

### Storage semantics — `verification/db_test.go`
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

### Crash safety — `verification/crash_test.go`
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

### Command line — `cli/cli_test.go`
| Test | What it proves |
|---|---|
| `TestPrefixEnd` | Prefix-to-range conversion is correct, including carry past trailing `0xff` |
| `TestPrefixEndBoundsScan` | Every key carrying a prefix sorts inside the derived range, and neighbours sort outside |
| `TestCommandTable` | Every visible command has a group, a summary and an implementation; no duplicate names |

**Reference-model independence:** the oracle is a plain Go `map` plus `sort`,
sharing no code with the B+tree implementation. The verifier never asks Anvil to
verify Anvil.

---

## 6. Benchmark

Real file backend, 100,000 keys × 16-byte values. **Performance is not a
competition claim** — this exists to detect catastrophic regressions and to be
honest about the fsync-bound path.

| Operation | linux/amd64 (`fdatasync`) | windows/amd64 (`FlushFileBuffers`) |
|---|---|---|
| Batched write (1,000 ops/txn) | 705 ms · **141.8 K ops/s** | 443 ms · 225.8 K ops/s |
| Single-commit write | **2.35 ms** · 426 ops/s | 282 µs · 3.5 K ops/s |
| Random point read | 16 µs · 61.5 K ops/s | 20 µs · 49.2 K ops/s |
| Full scan | 6 ms · **16.98 M keys/s** | 7 ms · 15.42 M keys/s |
| Recovery (reopen) | 30 µs | < 1 ms |
| Deep structural check | 7 ms | 7 ms |
| File size | 27.7 MB (7,084 pages, incl. copy-on-write history) | same |

**Read the single-commit row carefully.** Linux is ~8× slower there, and the
Linux number is the trustworthy one: `syscall.Fdatasync` on ext4 is a real
durability barrier, whereas the Windows figure reflects `FlushFileBuffers`
against a developer machine's cache. Quoting only the faster platform for a
durability-focused project would be misleading, so both are published.

### Latency percentiles, curves and a head-to-head

Averages hide tails, so the full profile lives in
[docs/BENCHMARKS.md](docs/BENCHMARKS.md): p50/p95/p99 for commit and read, the
batching curve (647 → 213,000 ops/s across batch sizes), dataset scaling, and a
measured head-to-head against **bbolt** — Anvil's closest architectural relative.

Headline from that comparison (linux/amd64, 100k keys, same machine):

| | Anvil | bbolt | |
|---|---|---|---|
| Batched load | 168.3 K ops/s | 450.6 K ops/s | bbolt 2.7× |
| Commit p50 | 1,722 µs | 750 µs | bbolt 2.3× |
| Random read p50 | 10.5 µs | 0.8 µs | **bbolt 13.1×** |
| Reopen | **33.7 µs** | 63.4 µs | **Anvil 1.9×** |
| File size | **8.7 MB** | 16.0 MB | **Anvil 1.8×** |

Reads are the one large gap and the reason is structural: bbolt memory-maps the
database, Anvil issues a `pread` per page and decodes it with no page cache.
That was a deliberate trade for a simpler, portable crash model, and it is the
clearest candidate for future work.

Single-commit throughput is deliberately fsync-bound: each commit pays two
durability barriers. Batching amortizes them, which is why the batched figure is
two orders of magnitude higher. Shadow paging also means each commit rewrites
the root-to-leaf path — the same write-amplification trade LMDB and bbolt make
in exchange for a simpler crash-consistency argument and lock-free snapshot
reads.

---

## 7. Invariants and where they are enforced

| # | Invariant | Enforced by |
|---|---|---|
| I1 | Committed pages are never modified in place | Copy-on-write in `engine/btree.go`; grow-only allocation |
| I2 | Metadata never references a page that has not satisfied the durability barrier | `Commit` ordering, `engine/tx.go`; proven by the negative control |
| I3 | The committed meta slot is never overwritten before the alternate is durable | Slot alternation by transaction parity, `engine/tx.go` |
| I4 | A commit is acknowledged only after the durable commit point | `Commit` returns after the second `Sync`, `engine/tx.go` |
| I5 | Every page reachable from the committed root is checksum-valid | `engine/checker.go`, run every torture cycle |
| I6 | Every leaf is at the same depth (underfull nodes are legal) | `engine/checker.go` |
| I7 | An active snapshot never observes an overwritten or reclaimed page | Grow-only allocation, by construction in v1 |
| I8 | A failed or uncommitted transaction is never partially visible | Torture all-or-nothing assertion; `RunConcurrencyCheck` |
| I9 | Recovery selects only a checksum-valid committed generation, or fails loudly | `recover()` in `engine/db.go`; `TestMetadataCorruptionFallback` |
| I10 | The reference model shares no implementation logic with the B+tree | `verification/model.go` uses a plain map |

---

## 8. What is NOT claimed, and what is NOT verified

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
- Crash injection covers I/O-operation seams, not arbitrary instruction
  boundaries.
- The benchmark host is a developer machine, not isolated hardware; timings vary
  between runs and between the two platforms (see §6).
- Torture has been run to 100,000 cycles per seed. Longer campaigns
  (`anvil torture --cycles 1000000`) are supported but are not part of the
  committed evidence.

**Version 1 limitations (by design, per the specification):**
- Keys ≤ 1 KiB; key + value ≤ ~2 KiB (no overflow pages).
- Single writer; many concurrent readers.
- Grow-only allocation: freed pages are not reused, so the file grows with
  history. Space reclamation is deliberately deferred to keep the commit
  protocol — the part that must be correct — small and verifiable.
- No write-ahead log (by design), SQL, secondary indexes, networking, or
  replication.
