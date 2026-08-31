# Anvil — Architecture

How the engine works, in the order you should read the code.

```
                    application / CLI
                            │
                      Anvil API  (engine/db.go, engine/tx.go)
                            │
              ┌─────────────┴─────────────┐
        read transaction            write transaction
        (snapshot root)              (single writer)
              └─────────────┬─────────────┘
                            │
                   copy-on-write B+tree  (engine/btree.go)
                            │
                    page encode/decode  (engine/page.go)
                            │
                      Storage interface  (engine/storage.go)
                            │
              ┌─────────────┴─────────────┐
         FileStorage                 FaultStorage
      (real file, fdatasync)   (simulated stable storage)
              │                             │
           disk                    only synced bytes survive
```

Reading order: `engine/page.go` → `engine/btree.go` → `engine/tx.go` → `engine/db.go` → `verification/torture.go` →
`engine/checker.go`.

---

## 1. File and page layout

One file, fixed 4 KiB pages. Page size is hard-coded in v1: storing it in the
metadata but *locating* the metadata by it would be circular, and a corrupt
first page would make the file unreadable. It is recorded for validation only.

```
page 0    META A ┐ alternating metadata slots
page 1    META B ┘
page 2+   tree pages (internal / leaf)
```

Every page carries a 32-byte header:

| Offset | Field | Purpose |
|---|---|---|
| 0–4 | magic | identifies an Anvil page |
| 4 | page type | meta / internal / leaf |
| 6–8 | key count | records or separators |
| 8–16 | page id | self-identification |
| 16–24 | txn id | generation that wrote it |
| 24–28 | payload length | bounds-checked on decode |
| 28–32 | checksum | CRC-32C over the page, excluding these four bytes |

All integers are little-endian, explicitly encoded — the format does not depend
on host byte order. Every decode path bounds-checks lengths and offsets before
slicing, so a malformed file produces `ErrCorrupt`, never a panic.

**Record sizing.** A single record is capped at half the payload area. That cap
is what makes splitting always make progress: any overflowing node divides into
two halves that each fit in a page.

---

## 2. Copy-on-write B+tree

Keys are sorted bytewise. Leaves hold records; internal nodes hold separators
and child pointers, where `keys[i]` is the first key of the subtree at
`children[i+1]`.

A write never touches a committed page. It clones the path from root to leaf,
applies the change to the clones, and allocates fresh page ids for them:

```
    before                    after

     ROOT                    ROOT'      ROOT
    /    \                  /    \     /    \
   A      B      ───►      A'     B   A      B
                            ↑         ↑
                        new pages   still intact, still valid
```

If a node overflows it splits and promotes a separator to its parent; if the
root splits, the tree grows a level.

**Deletion does not rebalance.** Underfull and empty nodes are allowed. This is
a deliberate trade: merge/rebalance logic is where B-tree implementations
historically go wrong, and every page it would save is a page the crash
protocol would have to reason about. Search correctness only requires ordering
and separator validity, both of which hold without merging. The structural
checker therefore treats fill factor as *not* an invariant.

**Iteration** uses an explicit stack of `(node, index)` frames rather than
sibling pointers, so ordered scans need no extra on-disk links to maintain.

---

## 3. Transactions

**Readers** capture the committed root at `BeginRead` and read only from it.
Because committed pages are immutable and v1 never reuses pages, that snapshot
stays valid however many commits follow — no reader locks, no copying.

**One writer at a time**, serialized by a mutex held for the transaction's life.
Single-writer is a design choice, not a limitation to apologize for: it removes
write-write conflicts entirely (which is why there is no `ErrConflict`), makes
the commit protocol a straight line, and makes crash reasoning tractable.

Staged pages live in memory until commit. Rollback simply discards them — they
were never referenced by anything.

---

## 4. The commit protocol

This is the part that must be correct; everything else is an optimization
around it.

```
    1. write new pages
    2. fdatasync                 ← data durable BEFORE anything points at it
    3. write meta to the ALTERNATE slot   (generation N+1, checksummed)
    4. fdatasync                 ← the durable commit point
    5. ACK the caller
```

Two properties do the work:

- **Data before metadata.** The metadata is the only thing that makes new pages
  reachable. Publishing it after the data sync means a surviving metadata
  generation can never point at pages that did not reach stable storage. (This
  is exactly the ordering the negative control removes to prove the test works.)
- **The committed slot is never touched.** Writing generation N+1 into the
  *other* slot means the last good generation stays intact throughout. At most
  one slot is mid-write at any instant.

The acknowledgement rule follows: the caller is told "committed" only after step
4 returns. Before that, the new tree is unreferenced garbage and recovery
selects the previous generation.

**Page-id compaction.** During a transaction, superseded copy-on-write pages are
allocated and then abandoned (a key rewritten twice clones its path twice). At
commit, only pages reachable from the final root are written, renumbered into a
contiguous range. Without this the file grows by every intermediate copy — in
practice a 40× difference in file size.

### Crash cases

| Crash point | Durable state | Recovery |
|---|---|---|
| During page writes | Old metadata, partial new pages | Old generation; new pages unreferenced |
| After data sync, before metadata | Old metadata, all new pages durable | Old generation; new pages unreferenced |
| During metadata write | Torn metadata in the alternate slot | Checksum fails → fall back to the committed slot |
| After metadata sync | New metadata durable | New generation — the transaction is committed |

There is no case that yields a partially applied transaction, which is what the
torture harness asserts on every cycle.

---

## 5. Recovery

No write-ahead log, so no replay. Recovery is selection:

```
read META A and META B
  → validate magic, format, page size, checksum on each
  → choose the highest transaction id among those that validate
  → sanity-check the root pointer, decode the root page
  → open
```

If neither slot validates, `Open` fails with `ErrCorrupt`. Anvil does not invent
a state it cannot justify. It auto-handles exactly the residue its own commit
protocol can produce — a torn metadata write, unreferenced pages — and refuses
anything else loudly. Corruption in a *reachable committed* page is detected by
checksum, not silently accepted.

---

## 6. Verification architecture

The `Storage` interface is what makes the durability claim testable:

```go
type Storage interface {
    ReadPage(id uint64, buf []byte) error
    WritePage(id uint64, p []byte) error
    Sync() error
    NumPages() (uint64, error)
    Close() error
}
```

`FaultStorage` implements stable storage honestly: writes land in a volatile
staging map and move to a durable map only on `Sync`. A simulated crash drops
the staging map. A live process still reads its own un-synced writes, exactly as
the page cache behaves.

This gives the harness two crash styles:

- **Clean crash** — discard everything un-synced. Used for the main torture run.
- **Partial flush** (`CrashPersistingOnly`) — persist only chosen pages. This
  models the write-ordering hazard where metadata reaches the platter and its
  data does not. With the correct protocol this is harmless, because the data
  was already synced. With the barrier removed, recovery breaks — which is the
  negative control.

`Commit` calls a `checkpoint(seam)` hook at the nine boundaries listed in
`engine.CommitSeams`; in production the hook is nil. The harness arms it via
`engine.WithCheckpoint` to abort at one seam per cycle.

### Package boundaries

The engine and its adversary are separate packages, which is what keeps the
evidence honest:

```
cli/            orchestration and presentation only — implements nothing
  ↓
verification/   FaultStorage, torture, model oracle, benchmark, suite
  ↓
engine/         the database itself
  ↓
standard library / OS
```

`engine` exposes exactly three verification affordances — `OpenStorage`,
`WithCheckpoint`, and `WithoutDataSync` — so the harness drives the real commit
path rather than a test-only replica of it. `WithoutDataSync` deliberately
breaks the protocol and exists solely for the negative control; it is documented
as such at its definition.

Verification logic lives in ordinary files (`verification/model.go`,
`verification/suite.go`, …) that both `go test` and `anvil verify` call. Neither
entry point has its own copy, so they cannot drift apart.

The one exception to the split is `engine/page_test.go`, which tests unexported
page-format internals and therefore stays in `engine`. Moving it would mean
exporting the entire on-disk format to satisfy a directory layout.

See [VERIFICATION.md](../VERIFICATION.md) for results.

---

## 7. Deliberate omissions

| Not built | Why |
|---|---|
| Free-list / space reclamation | Reusing pages reintroduces the hardest bug class in COW engines: freeing a page still visible to a reader or to a recoverable generation. Grow-only makes snapshot safety true by construction. Deferred behind the frozen commit core. |
| Overflow pages | Values are capped at ~2 KiB instead. Chained overflow pages add a second reachability structure the checker and crash protocol must handle. |
| Write-ahead log | Shadow paging makes it unnecessary — the metadata page *is* the commit record. |
| Node merging on delete | See §2. |
| Compression, encryption, SQL, networking | Out of scope for a storage engine. |
