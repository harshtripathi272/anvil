# Anvil — Benchmarks

All figures from `linux/amd64`, Go 1.27.0, real `syscall.Fdatasync`. Reproduce
with:

```bash
anvil bench --profile --json profile.json    # latency, batching, scaling
```

**Performance is not a claim Anvil makes.** These numbers exist to detect
regressions, to be honest about an fsync-bound commit path, and to show where a
from-scratch engine sits next to a mature one. Where Anvil loses, the reason is
stated.

---

## 1. Latency distribution

Averages hide tails, so percentiles are reported. 1,500 commit samples,
15,000 read samples, against a 100,000-key database.

| Operation | mean | p50 | p95 | p99 | max |
|---|---|---|---|---|---|
| Single-key commit | 1.53 ms | 1.458 ms | 2.181 ms | 2.62 ms | 5.97 ms |
| Random point read | 15 µs | 10.5 µs | 24.2 µs | 134.6 µs | 1.505 ms |

Commit latency is dominated by two `fdatasync` barriers — that is the design,
not an accident. The read tail (p99 134 µs against a p50 of 10.5 µs) comes from
page-cache misses: Anvil has **no page cache**, so every node access is a
`pread` plus a full page decode.

## 2. Batching curve

How many operations share one pair of durability barriers:

| Batch size | Throughput | Per operation |
|---|---|---|
| 1 | 647 ops/s | 1.546 ms |
| 10 | 6.8 K ops/s | 147 µs |
| 100 | 51.7 K ops/s | 19 µs |
| 1,000 | 183.9 K ops/s | 5 µs |
| 10,000 | 213.0 K ops/s | 5 µs |

**329× between a batch of 1 and a batch of 10,000**, flattening after ~1,000
once fsync stops being the bottleneck. This is the single most useful thing to
know when using Anvil: batch your writes.

## 3. Scaling

| Keys | Load | Read p50 | Scan | Recovery | Leaf depth | File |
|---|---|---|---|---|---|---|
| 10,000 | 182.9 K ops/s | 5.9 µs | 21.3 M keys/s | **34 µs** | 1 | 0.4 MB |
| 100,000 | 186.3 K ops/s | 9.1 µs | 20.0 M keys/s | **34 µs** | 2 | 4.2 MB |

Two results worth naming:

- **Read cost grows with tree depth, not dataset size** — 5.9 µs → 9.1 µs as the
  tree goes from depth 1 to depth 2. That is the B+tree behaving as advertised.
- **Recovery is flat at 34 µs across a 10× dataset increase.** This is the
  empirical evidence for "recovery is generation selection, not log replay" —
  there is no write-ahead log to scan, so reopening reads two metadata pages and
  validates a root regardless of how much data is in the file.

---

## 4. Head to head with bbolt

**Why bbolt:** it is Anvil's closest architectural relative — a copy-on-write
B+tree over a single file, two alternating meta pages, sync-then-publish commit.
Comparing against an LSM engine or a SQL database would be comparing different
things. bbolt is also mature, `mmap`-backed and has a real freelist, none of
which Anvil claims.

Identical workload: 100,000 keys × 16-byte values, batch 1,000, both at default
durability, same machine, same run.

| Metric | Anvil | bbolt | Anvil relative |
|---|---|---|---|
| Batched load | 168.3 K ops/s | 450.6 K ops/s | 0.37× |
| Commit p50 | 1,722 µs | 750 µs | 0.44× |
| Commit p99 | 2,903 µs | 1,611 µs | 0.55× |
| Random read p50 | 10.5 µs | 0.8 µs | **0.08×** |
| Random read p99 | 141.2 µs | 3.1 µs | **0.02×** |
| Full scan | 16.2 M keys/s | 138.5 M keys/s | 0.12× |
| **Reopen** | **33.7 µs** | 63.4 µs | **1.88× faster** |
| **File size** | **8.7 MB** | 16.0 MB | **1.84× smaller** |

### Reading this honestly

**Writes are in the same order of magnitude.** 2–3× slower than an engine that
has been tuned for a decade is a reasonable place for a from-scratch
implementation to land.

**Reads are 13× slower at p50 and ~50× at p99, and the reason is structural:**
bbolt memory-maps the database, so a read is a pointer dereference into
already-mapped memory. Anvil issues an explicit `ReadAt` per page and decodes
the page into Go structures on every access, with no cache. That was a
deliberate choice — `mmap` requires platform-specific `syscall` code and
complicates the crash model, and Anvil optimizes for a crash story that is
simple enough to verify exhaustively. It is documented as a cost in
[STDLIB.md](../STDLIB.md), and it is the clearest candidate for future work
(a page cache would close most of this gap without touching the commit
protocol).

**Anvil is faster to open and smaller on disk**, which follows from the same
design: there is no log to inspect and no freelist structure to load, and this
workload never exercises bbolt's page reuse.

**What this comparison does not show:** concurrent throughput, mixed
read/write workloads, large values, or long-running-reader behaviour. It is one
workload on one machine.

---

## Raw evidence

| File | Contents |
|---|---|
| [`linux/profile.json`](benchmarks/linux/profile.json) | Latency percentiles, batching curve, scaling curve |
| [`linux/compare-bbolt.json`](benchmarks/linux/compare-bbolt.json) | Head-to-head results |
| [`linux/bench.txt`](benchmarks/linux/bench.txt) | Standard benchmark, Linux |
| [`bench.txt`](benchmarks/bench.txt) | Standard benchmark, Windows |

The comparison harness lives outside this repository in its own module, so
bbolt is never a dependency of Anvil. `go.mod` stays empty.
