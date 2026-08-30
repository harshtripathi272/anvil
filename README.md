# Anvil

**A crash-safe embedded key/value database, built entirely from the Go standard
library — and proven by destruction.**

Anvil is a transactional storage engine you link into your program: no server,
no daemon, no driver, no dependencies. It keeps data in a copy-on-write B+tree
over a single file, and it is designed so that killing the process at the most
dangerous moment of a commit still leaves a valid database with every
acknowledged transaction intact.

Copy-on-write B+trees are established design (LMDB, bbolt). What Anvil adds is
**calibrated proof**: a harness that crashes the engine at every seam of its
commit protocol and verifies recovery against an independent model — plus a
*negative control* that removes a durability barrier and confirms the harness
catches the failure.

> We defined a failure model in which only synchronized bytes survive, then
> exhaustively exercised the defined persistence seams and verified that every
> acknowledged transaction recovered to a valid committed state.

---

## Zero dependencies

```console
$ cat go.mod
module anvil
go 1.27                    # no require block

$ go list -m all
anvil                      # no third-party modules

$ CGO_ENABLED=0 go build ./...    # one command, static binary, no cgo
```

Anvil replaces an embedded-database dependency — bbolt, LMDB, BadgerDB, or
SQLite, several of which drag in cgo and a C toolchain. See
**[STDLIB.md](STDLIB.md)** for the capability-by-capability mapping.

## Quickstart

```console
$ go build -o anvil ./cmd/anvil

$ ./anvil create data.anvil
$ ./anvil put   data.anvil user:1 lakshya
$ ./anvil put   data.anvil user:2 harsh
$ ./anvil get   data.anvil user:1
lakshya
$ ./anvil scan  data.anvil user:
user:1	lakshya
user:2	harsh
(2 keys)
$ ./anvil check data.anvil
RESULT: PASS
```

### See the proof yourself

```console
$ ./anvil torture --cycles 100000     # kill at every commit seam, verify recovery
crashes injected:        100000
acked writes lost:       0
invariant violations:    0
RESULT: PASS

$ ./anvil torture --negative-control  # remove a barrier — MUST fail
RESULT: FAIL
  detail: recovery open failed: anvil: database corrupt
EXPECTED FAILURE: the verifier caught the durability violation. The test has teeth.
```

The second command is the important one. A crash test that only kills processes
would pass even with every `fsync` deleted, because the page cache serves the
data back. Anvil's harness is calibrated to fail when durability is actually
broken.

## Library API

```go
db, err := anvil.Open("data.anvil")
if err != nil {
	return err
}
defer db.Close()

// Single operations, each its own transaction.
db.Put([]byte("name"), []byte("lakshya"))
v, err := db.Get([]byte("name"))
db.Delete([]byte("name"))

// Atomic multi-key transaction: all of it, or none of it.
tx, err := db.BeginWrite()
tx.Put([]byte("account:A"), []byte("900"))
tx.Put([]byte("account:B"), []byte("100"))
err = tx.Commit()

// Snapshot read: a stable view, unaffected by concurrent commits.
rtx, err := db.BeginRead()
defer rtx.Close()
for it := rtx.Scan([]byte("account:"), []byte("account:~")); it.Next(); {
	fmt.Printf("%s = %s\n", it.Key(), it.Value())
}
```

Keys and values are arbitrary bytes — no UTF-8 assumptions anywhere in the
storage layer.

## How it works, in six lines

A write clones the root-to-leaf path into fresh pages instead of modifying
committed ones, then publishes a new root. Two alternating metadata pages carry
a generation number and a checksum. The commit protocol is:

```
write new pages → fdatasync → write meta (alternate slot) → fdatasync → ACK
```

Data is durable before the metadata that references it; the second sync is the
commit point. Recovery picks the newest metadata generation that validates —
there is no write-ahead log and nothing to replay.

Full detail: **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)**.

## What it guarantees

Anvil is **crash-consistent under a documented storage and failure model**:

- **Simulated power loss** (only `fsync`'d bytes survive): every acknowledged
  transaction recovers to a valid committed state, across all nine commit seams.
  100,000 cycles, zero losses, zero invariant violations.
- **Real process kill** (`SIGKILL`): recovers cleanly on a real file — verified
  end to end. This proves *consistency*; durability is proven by the fault
  backend above.
- **Real hardware power loss**, or storage firmware that lies about flushes:
  **not claimed.**

Anvil does not claim to be "crash-safe" unqualified, to never lose data, to be
mathematically proven, or to be faster than any production database.

**v1 limits:** keys ≤ 1 KiB and key+value ≤ ~2 KiB; single writer with many
readers; grow-only allocation (no space reclamation yet); no WAL, SQL, secondary
indexes, or networking.

Every claim, its evidence, and every known gap:
**[VERIFICATION.md](VERIFICATION.md)**.

## Verify everything yourself

```console
$ ./scripts/verify.sh
```

Runs all eight checks — dependency proof, build, cross-compile, gofmt/vet, the
test suite, crash torture, the negative control, and the benchmark — and writes
raw evidence to `results/`. A committed run lives in
[`docs/benchmarks/`](docs/benchmarks/).

## Repository layout

```
*.go                  the engine (one flat package, as bbolt and LMDB do)
  page.go             page format, encoding, checksums, metadata
  btree.go            copy-on-write B+tree, splits, cursor
  tx.go               transactions and the commit protocol
  db.go               open, recovery, generation selection
  storage.go          Storage interface + real file backend
  faultstorage.go     simulated stable storage (only synced bytes survive)
  torture.go          crash-torture harness + negative control
  checker.go          structural invariant verifier
cmd/anvil/            CLI: store commands, torture, bench
scripts/verify.sh     reproduce every claim in one command
docs/
  ARCHITECTURE.md     how the engine works
  DEMO.md             5-minute demo script
  benchmarks/         raw evidence from the last full verification run
  internal/           project spec and research notes (not part of the product)
VERIFICATION.md       source of truth: claims, tests, results, known gaps
STDLIB.md             capability → standard library mapping
deps-proof.txt        generated proof of an empty dependency manifest
```

## Prior art

Anvil's architecture follows **LMDB** and **bbolt** (shadow-paging copy-on-write
B+tree, double metadata pages, sync-then-publish); **voidDB** is a close Go
relative. No architectural novelty is claimed. The contribution is a
dependency-free from-scratch implementation, plus a verification methodology in
the lineage of **FoundationDB**, **TigerBeetle's VOPR**, and **LazyFS**, applied
to an engine small enough to read in an afternoon.

## License

MIT — see [LICENSE](LICENSE).
