# STDLIB.md — how Anvil replaces a database dependency with the standard library

Anvil is a transactional, crash-consistent embedded key/value storage engine.
Reaching for one normally means adding a dependency: **bbolt/BoltDB**, **LMDB**
(cgo), **BadgerDB**, or **SQLite** (cgo). Anvil builds the same class of engine —
a copy-on-write B+tree with a durable commit protocol — from the Go standard
library alone. `go.mod` has no `require` block; `go list -m all` prints only
`anvil`; the binary links no C.

Package killer, precisely: **it removes an embedded-database dependency and its
(often cgo) toolchain, leaving one static binary.**

## Capability → standard library package

| Capability in Anvil | Standard library | Where |
|---|---|---|
| Single database file; page reads/writes at offsets; growth | `os` (`OpenFile`, `File.ReadAt`, `File.WriteAt`, `File.Stat`) | `storage.go` |
| Durability barrier (the commit point) | `syscall.Fdatasync` on Linux; `os.File.Sync` elsewhere, selected by build tag | `sync_linux.go`, `sync_other.go` |
| Durable directory entry on file creation | `os.Open(dir)` + `File.Sync` (best-effort) | `storage.go` |
| Fixed 4 KB page encode/decode, little-endian, explicit byte order | `encoding/binary` | `page.go` |
| Torn-write / corruption detection (per-page checksum) | `hash/crc32` (Castagnoli, i.e. CRC-32C) | `page.go` |
| Ordered keys, separator search, binary-safe comparison | `bytes` (`Compare`, `Equal`), `sort.Search` | `btree.go` |
| Single-writer / many-reader concurrency | `sync` (`Mutex`, `RWMutex`) | `db.go`, `tx.go` |
| Typed, classifiable errors | `errors` (`New`, `Is`, `Join`) | `errors.go` |
| CLI (create/put/get/del/scan/info/check/torture) | `flag`, `fmt`, `os` | `cmd/anvil` |
| Static, dependency-free build | `go build` with `CGO_ENABLED=0` | — |

## Verification, also standard-library-only

The proof harness is part of the product, and it too is pure stdlib:

| Verification layer | Standard library | Where |
|---|---|---|
| Deterministic, seeded random workloads (property + torture) | `math/rand` | `torture.go`, `*_test.go` |
| Independent reference model (an ordered `map`, materially different code) | builtin `map` + `sort` | `model_test.go` |
| Crash injection at named commit seams | a plain `func(seam string) error` hook | `db.go`, `tx.go`, `torture.go` |
| Simulated stable storage (only fsync'd bytes survive) | in-memory `map`s | `faultstorage.go` |
| Real process-kill recovery (secondary check) | `os/exec`, self re-exec | `real_kill_test.go` |
| Race checking (CI) | `go test -race` | CI |

## Notable "we didn't need a library for this" moments

- **No cgo.** LMDB and SQLite bindings drag in a C compiler; Anvil is
  `CGO_ENABLED=0` and cross-compiles trivially.
- **No serialization library.** Pages are hand-encoded with `encoding/binary`
  into a fixed 32-byte header + payload; the format is fully specified in
  `page.go`, endianness included.
- **No checksum/hash library.** `hash/crc32` with the Castagnoli table gives
  torn-page detection.
- **No test framework.** `testing` + `math/rand` drive 40k-op model comparisons,
  millions of crash cycles, and an independent reference oracle.
- **No fault-injection framework** (LazyFS, ALICE). The `Storage` interface plus
  an in-memory backend that discards un-synced writes reproduces the exact
  failure model those tools provide — including the write-ordering hazard the
  negative control exploits.
