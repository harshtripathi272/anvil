# STDLIB.md — building a database with no dependencies

Anvil is a transactional, crash-consistent embedded key/value storage engine.
Building one normally means taking a dependency: **bbolt**, **LMDB** (cgo),
**BadgerDB**, or **SQLite** (cgo). Anvil builds the same class of engine — a
copy-on-write B+tree with a durable commit protocol — from the Go standard
library alone.

```console
$ cat go.mod
module anvil
go 1.27          # no require block

$ go list -m all
anvil            # no third-party modules
```

**Package killed: the embedded database itself** — along with, in the cgo cases,
an entire C toolchain. What remains is one static binary.

---

## The 14 substitutions

Each row is a thing you would normally import, and what replaced it.

| # | Would normally use | Standard library instead | Where |
|---|---|---|---|
| 1 | An embedded KV store (`bbolt`, `badger`, `go.etcd.io/bbolt`) | The whole engine: `os` file I/O + a hand-built B+tree | `btree.go`, `tx.go` |
| 2 | A cgo SQLite/LMDB binding | `os.File.WriteAt` / `ReadAt` at page offsets — no cgo, `CGO_ENABLED=0` | `storage.go` |
| 3 | A durability/fsync helper library | `syscall.Fdatasync` on Linux; `os.File.Sync` elsewhere, selected by build tag | `sync_linux.go`, `sync_other.go` |
| 4 | A binary serialization library (`protobuf`, `gob`, `msgpack`) | `encoding/binary` — explicit little-endian, fixed 32-byte page header | `page.go` |
| 5 | A checksum/hash library (`xxhash`, `murmur3`) | `hash/crc32` with the Castagnoli table (CRC-32C) for torn-page detection | `page.go` |
| 6 | A sorted-container / btree package (`google/btree`, `tidwall/btree`) | `bytes.Compare` + `sort.Search` over hand-encoded pages | `btree.go` |
| 7 | A concurrency toolkit | `sync.Mutex` / `sync.RWMutex`: single writer, many snapshot readers | `db.go`, `tx.go` |
| 8 | An error-wrapping library (`pkg/errors`) | `errors.New` / `errors.Is` / `errors.Join` — 10 classifiable error types | `errors.go` |
| 9 | A CLI framework (`cobra`, `urfave/cli`) | `flag.NewFlagSet` per subcommand + a `switch` on `os.Args[1]` | `cmd/anvil` |
| 10 | A test framework (`testify`, `ginkgo`) | `testing` — 19 tests, table-driven, plain assertions | `*_test.go` |
| 11 | A property/fuzz library (`gopter`, `quick`) | `math/rand` with explicit seeds; 40,000-op randomized model comparison | `model_test.go` |
| 12 | A fault-injection tool (**LazyFS**, ALICE, `charybdefs`) | A `Storage` interface + an in-memory backend where only synced bytes survive | `faultstorage.go` |
| 13 | A reference/oracle database to diff against | A plain Go `map` + `sort` — deliberately different code from the B+tree | `model_test.go` |
| 14 | A process-supervision library | `os/exec` self re-exec, then kill the child mid-commit | `real_kill_test.go` |

**Total: 14 substitutions**, of which 3 (rows 1, 2, 12) replace things that
normally require either cgo or a separate installed tool.

---

## Complete import list

The entire engine, CLI, and test suite import only these:

```
bytes          encoding/binary   errors     flag       fmt
hash/crc32     io                math/rand  os         os/exec
path/filepath  runtime           sort       sync       sync/atomic
testing        time              syscall (linux only, for Fdatasync)
```

Verify with `go list -f '{{join .Imports "\n"}}' ./...`. No `internal/`
vendoring, no copied third-party source, no build-time codegen.

---

## Where the standard library was genuinely enough

**Durability.** The hardest requirement in the project is ordering `write`
against `fsync` correctly. `syscall.Fdatasync` (Linux) and `os.File.Sync`
(elsewhere) are the whole toolkit; `os.File.Sync` on a directory handle makes a
newly created file's directory entry durable. A build tag picks the right one —
no abstraction library needed.

**Fault injection.** This is the part that would most obviously "need" a
dependency: LazyFS and similar tools exist precisely to model "only fsync'd
bytes survive a crash." Expressing storage as a five-method interface makes that
model 100 lines of `map[uint64][]byte` — staged writes move to a durable map on
`Sync`, and a simulated crash drops the staging map. It also makes possible a
partial-flush mode that no off-the-shelf tool gives for free, which is what the
negative control needs.

**Verification.** `testing` plus `math/rand` with recorded seeds covers unit,
property, model-based, and crash-torture testing, with every failure
reproducible from its seed. `go test -race` covers the concurrency claims.

## Where the standard library imposed a real cost

Honest accounting:

- **No `fdatasync` on Windows/macOS**, so those platforms fall back to full
  `fsync` (`File.Sync`) — correct, since fsync is a strict superset, but slower.
- **No race detector without cgo.** `go test -race` needs a cgo-capable
  toolchain, so it runs in CI rather than on a cgo-less host.
- **No memory-mapped I/O without `syscall` platform code.** LMDB and bbolt mmap
  the file; Anvil uses ordinary `ReadAt`/`WriteAt`, which costs a copy per page
  read but keeps the code portable and the crash model simple.
- **Hand-rolled CLI parsing.** `flag` has no subcommand support, so dispatch is
  an explicit `switch` with a `FlagSet` per command. Slightly more code, and no
  dependency.
