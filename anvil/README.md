# Anvil

**A crash-safe embedded key/value storage engine, built from the Go standard
library — proven by destruction.**

Anvil is a small, transactional, single-writer/many-reader embedded database. It
stores data in a copy-on-write B+tree over a single file, and it is designed so
that *even if you kill the process at the most dangerous moment of a commit*,
reopening the file always yields a valid database with every acknowledged
transaction intact.

The distinguishing feature is not the engine — copy-on-write B+trees are
well-trodden ground (LMDB, bbolt). It is the **proof**: a fault-injecting harness
that repeatedly crashes the engine at every seam of its commit protocol and
mechanically verifies recovery against an independent model — plus a **negative
control** that removes a durability barrier and confirms the harness *catches*
the resulting failure.

> We defined a failure model in which only synchronized bytes survive, then
> exhaustively exercised the defined persistence seams and verified that every
> acknowledged transaction recovered to a valid committed state.

## Zero dependencies

```
$ cat go.mod
module anvil
go 1.27

$ go list -m all
anvil                     # only this module — no third-party runtime deps

$ CGO_ENABLED=0 go build -o anvil ./cmd/anvil   # one command, static binary
```

No cgo, no external database, no serialization/checksum/test libraries. See
[STDLIB.md](STDLIB.md) for the full capability → standard-library mapping.

## Quickstart (CLI)

```
go build -o anvil ./cmd/anvil

./anvil create data.anvil
./anvil put   data.anvil user:1 lakshya
./anvil put   data.anvil user:2 harsh
./anvil get   data.anvil user:1          # -> lakshya
./anvil scan  data.anvil user:           # ordered range scan by prefix
./anvil info  data.anvil                 # generation, root page, page count
./anvil check data.anvil                 # deep structural verification
```

### The proof, on your machine

```
./anvil torture --cycles 100000          # kill at every commit seam, verify recovery
./anvil torture --negative-control       # remove a barrier -> MUST report FAIL (teeth)
```

`torture` reports lost acknowledged writes, invariant violations, and per-seam
coverage. `--negative-control` deliberately removes the pre-meta durability
barrier and confirms the verifier reports `FAIL` — a passing negative control
would mean the test proves nothing.

## Library API

```go
db, err := anvil.Open("data.anvil")
defer db.Close()

// Single operations (each its own transaction).
db.Put([]byte("name"), []byte("lakshya"))
v, _ := db.Get([]byte("name"))          // "lakshya"
db.Delete([]byte("name"))

// Atomic multi-key transaction: readers see all of it or none of it.
tx, _ := db.BeginWrite()
tx.Put([]byte("account:A"), []byte("900"))
tx.Put([]byte("account:B"), []byte("100"))
tx.Commit()

// Snapshot read: a stable, consistent view unaffected by later commits.
rtx, _ := db.BeginRead()
it := rtx.Scan([]byte("account:"), []byte("account:~")) // ordered range
for it.Next() {
    fmt.Printf("%s = %s\n", it.Key(), it.Value())
}
rtx.Close()
```

## How it works

- **Copy-on-write B+tree.** A write never modifies a committed page. It copies
  the path from root to leaf into fresh pages and publishes a new root. Old pages
  are untouched, so readers holding an old root keep a consistent snapshot.
- **Double-buffered metadata.** Two meta pages (page 0 and page 1) alternate by
  transaction parity. Each carries a generation number and a checksum. Recovery
  selects the newest generation that validates; if it is torn, it falls back to
  the other.
- **The commit protocol** is the whole ballgame:
  ```
  write new pages → fdatasync → write meta (alternate slot) → fdatasync → ACK
  ```
  Data is durable before the meta that references it; the second sync is the
  durable commit point; the caller is acknowledged only after it returns. Before
  that point, the new tree is just orphaned, unreferenced pages.
- **WAL-free shadow paging.** The meta page *is* the commit record. There is no
  redo log and no replay — recovery is O(1) generation selection.
- **Grow-only (v1).** Freed pages are not reused; the file grows. This is what
  makes snapshot readers safe by construction (a committed page is never
  overwritten). Space reclamation is deliberately deferred.

## What it guarantees (and what it doesn't)

**Honest durability model.** Anvil is *crash-consistent under a documented
storage and failure model*:

- **Simulated power loss** (the fault-injecting backend, where only `fsync`'d
  bytes survive): every acknowledged transaction recovers to a valid committed
  state, across the defined commit seams. This is the primary proof.
- **Real process kill** (`kill -9`): recovers cleanly — validated end-to-end
  against a real file. (Process kill does not discard page-cache writes, so this
  validates *consistency*; durability is proven by the fault backend.)
- **Real hardware power loss / lying device firmware:** not experimentally
  claimed.

We do **not** claim: "crash-safe" unqualified, "can never lose data",
"mathematically proven", "corruption-proof", or "faster than production
databases".

**v1 limits:** keys ≤ 1 KB and key+value ≤ ~2 KB (no overflow pages);
single-writer; grow-only (no space reclamation); no WAL, SQL, secondary indexes,
networking, or replication.

## Positioning / prior art

Anvil's architecture is a faithful, from-scratch, dependency-free implementation
in the lineage of **LMDB** and **bbolt** (shadow-paging COW B+tree, double meta
pages, sync-then-publish); **voidDB** is a close Go relative. It does **not**
claim a novel architecture. Its contribution is the dependency-free build plus a
verification methodology in the lineage of **FoundationDB**, **TigerBeetle's
VOPR**, and **LazyFS** — deterministic fault injection with an independent model
and a negative control — applied to a small engine you can read in an afternoon.

## Non-negotiable invariants

The laws Anvil is built to keep (enforced by tests and the structural checker):

1. Committed pages are never modified in place.
2. Metadata never references a page that has not satisfied the durability barrier.
3. The committed meta slot is never overwritten before the alternate is durable.
4. A commit is acknowledged only after the durable commit point.
5. Every page reachable from the committed root is checksum-valid.
6. Every leaf is at the same depth (underfull/empty nodes are legal).
7. An active snapshot never observes an overwritten or reclaimed page.
8. A failed/uncommitted transaction is never partially visible.
9. Recovery selects only a checksum-valid committed generation, or fails loudly.
10. The reference model shares no implementation logic with the B+tree.

## Tests

```
go test ./...            # unit, property, model-based, crash torture, real kill
go test -race ./...      # concurrency (CI; needs a cgo-capable toolchain)
```
