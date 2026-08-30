# ANVIL — SRS v1.1 Amendment

This document amends the Anvil v1.0 SRS. It (1) locks in the technical deltas from
the adversarial review, (2) realigns scope to the **actual** Zero Dependency 2026
judging rubric discovered by research, and (3) records prior art so we never make a
false novelty claim. Where a delta replaces or constrains a v1.0 section, the v1.0
section number is cited. **On any conflict, v1.1 wins.**

---

## PART A — RUBRIC REALITY (read first; it reprioritizes everything)

Official Zero Dependency 2026 scoring (source: organizer announcement):

| Weight | Criterion | What it actually measures |
|---|---|---|
| 35% | Functionality & Usefulness | Builds with ONE command, runs, does something a real person wants |
| 30% | Zero-Dependency Craft | How well stdlib replaced typical imports; **`STDLIB.md` is graded here** |
| 25% | Code Quality & Idiom | Senior reviewers judge whether code reads naturally / doesn't fight stdlib |
| 10% | Innovation | Creative, surprising stdlib-only implementations |

Consequences for Anvil, in priority order:

1. **Usefulness (35%) is the top axis, and it is about the product, not the proof.**
   The scored artifact is a genuinely pleasant embedded KV a developer would use:
   `Open → Put/Get/Delete → Scan → atomic multi-key Txn → Close/reopen`, plus a clean
   CLI. Build must be **one command** (`go build ./...`). The crash-torture harness is
   NOT what this axis rewards — it is Innovation (10%) and the demo hook.
2. **Zero-Dependency Craft (30%) is where Anvil is strongest.** We delete an entire
   embedded-database dependency (bbolt/LMDB/BadgerDB) and replace it with `os` file I/O
   + `encoding/binary` + `hash/crc32` + `sync`. `STDLIB.md` is a graded deliverable and
   a first-class work product, not an afterthought. This is also our **Package Killer**
   award ($100) case.
3. **Code Quality & Idiom (25%) makes over-scoping a LIABILITY.** Abundant code
   generation must produce a *small, coherent, idiomatic* engine, not a sprawling one.
   Every subsystem an agent adds is more surface for "this fights readability." Prefer
   less code that reads naturally. The freelist, page compaction, and a sophisticated
   buffer manager are cut in v1 for correctness AND for readability score.
4. **Innovation (10%) is where the crash harness scores.** It is the memorability
   engine and the Write-Up Side Quest ($300 pool) case — but it is 10% of the grade.
   Do not let it consume the effort that belongs to usefulness and code quality.

**Net reprioritization:** optimize, in order — (a) a useful, one-command, idiomatic KV
library + CLI; (b) an outstanding `STDLIB.md`; (c) clean readable code; (d) the crash
harness + demo as the innovation/memorability layer. Depth is not a scored axis; it
buys us Innovation and the write-up, nothing more. Build accordingly.

---

## PART B — MANDATORY SUBMISSION ARTIFACTS (hard gate; not optional)

A submission that lacks any of these is disqualified or heavily penalized regardless
of engine quality:

- Public GitHub repo with a working implementation.
- **Single-command build** documented and true (`go build ./...`), `CGO_ENABLED=0`.
- **Empty dependency manifest with proof:** `go.mod` has NO `require` block; include CI
  log or `go list -m all` output in the repo showing stdlib-only. Vendoring third-party
  source without disclosure is a rules violation — do not do it.
- `README.md` — what it is, how to build/run, the API, the honest claims (Part D).
- `STDLIB.md` — graded artifact. Map every capability to the exact stdlib package that
  provides it (`os`, `encoding/binary`, `hash/crc32`, `sync`, `errors`, `bufio`,
  `testing`, `testing/synctest`), and narrate "what dependency this replaces."
- 5-minute demo video (see v1.0 §104–§106; keep the sound-off requirement).

---

## PART C — TECHNICAL DELTAS (locked)

### C1. The proof model — PRIMARY vs SECONDARY, and the negative control
Replaces the emphasis of v1.0 §43–§53, §45, §51.

- **PRIMARY PROOF = a fault-injecting storage backend, not process kill.** The engine
  writes through a `Storage` interface (v1.0 §92, now MANDATORY). A fault-injecting
  in-memory backend models stable storage precisely: **on a simulated crash, all bytes
  that were written but not yet `fsync`/`fdatasync`'d are discarded; only synced bytes
  survive.** Recovery then runs against exactly what the durability contract promised.
  This is the only mode that actually proves `fsync` placement is correct. It is
  deterministic, fast (no real disk), and lets us enumerate crash points at every
  write/sync boundary with a *defined* seam set (I/O-operation granularity, not
  instruction level — state this; do not claim "all possible crash points").
- **SECONDARY CHECK = real process kill** on the real file backend (v1.0 §45). This
  validates the true syscall path end-to-end, but its claim is **crash-CONSISTENCY
  only**: a `kill -9` does not discard un-`fsync`'d writes (the OS page cache serves
  them to the reopening process), so process-kill alone cannot prove durability. Label
  it as such everywhere.
- **NEGATIVE CONTROL (gating acceptance criterion, new).** With a required durability
  barrier removed (e.g., skip the pre-meta data `fdatasync`, or skip the meta `fdatasync`),
  the PRIMARY durability harness **MUST fail** (must report lost acknowledged writes or
  a partial/torn recovery). A build where the durability harness still passes with a
  barrier disabled is REJECTED — it proves the harness has no teeth. Keep a checked-in
  "sabotage" build tag/config that runs this control in CI and asserts RED.

### C2. Terminology — never "crash-safe" unqualified
Replaces v1.0 §79, §80 wording globally.

- Do not write "crash-safe" as an unqualified property. Write **"crash-consistent under
  the documented storage and failure model,"** and define the model precisely wherever
  claimed:
  - **Process crash (SIGKILL):** proven directly (consistency); acked data survives via
    page cache regardless of sync.
  - **Simulated power loss (fault-injecting backend):** proven for the defined seam set;
    only `fsync`'d bytes survive; this is what validates durability + atomic recovery.
  - **Real hardware power loss / device firmware lying about flushes:** NOT
    experimentally claimed. Acknowledged as out of scope of our evidence.
- Approved claims (v1.0 §79) stand, prefixed by the model. Forbidden claims (v1.0 §79)
  stand: never "can never lose data," "mathematically proven," "corruption-proof,"
  "faster than production databases."

### C3. Durability primitives — name them
Replaces v1.0 §25, §26, §30 "synchronize" language.

- Commit ordering, exact: `write new pages → fdatasync(file) → write new meta to
  alternate slot → fdatasync(file) → ACK`. Second `fdatasync` is the durable commit
  point; ACK only after it returns.
- Use **`fdatasync`** for ongoing appends (persists data + the file-size metadata needed
  to read it back; skips mtime). Use directory **`fsync` once at file creation** to make
  the file's directory entry durable (v1.0 §30). Do not directory-fsync per commit.
- **Anvil is WAL-free shadow paging.** State this in README/STDLIB and the pitch: the
  meta page IS the commit record; there is no redo log and no replay. Recovery is O(1)
  generation selection. (Pre-empts "where is your WAL?")
- **Write amplification is documented, not hidden.** Every write copies the root→leaf
  path (~tree-height pages per commit). This is the standard shadow-paging trade for
  crash-consistency simplicity and lock-free snapshot reads (same as LMDB/bbolt).

### C4. Fixed page size in v1
Replaces v1.0 §12.

- Page size is **hard-coded 4096 in v1.** META A at offset 0, META B at **fixed offset
  4096**. Store `page_size` in meta for validation only; reject any file whose stored
  page size ≠ 4096 with `ErrFormat`. (Variable page size creates a bootstrap problem:
  you cannot locate META B until you know the size, and a corrupt META A then makes the
  file unlocatable. Defer to v2.)

### C5. Grow-only allocation; NO freelist reuse in v1
Replaces/【constrains v1.0 §23, §24, §96, §97.

- v1 allocation is **strictly grow-only.** Freed pages are NOT reused. The file grows;
  reclamation is a v2 concern behind the frozen core.
- **Snapshot-reader safety in v1 derives from grow-only:** because no committed page is
  ever overwritten, a reader holding an old root can never have a page mutated under it.
  State this dependency explicitly (v1.0 §60, §83). Adding a freelist (v2) reintroduces
  the reader-pinning / reachable-page reclamation problem — the hardest bug class in COW
  engines — and MUST NOT touch the frozen commit protocol.

### C6. Meta-slot write invariant
Adds to v1.0 §14, §16.

- A meta page is published with a **single `pwrite` to the alternate slot** (alternating
  by txn parity). The slot holding the last durably-committed generation is **never
  touched** until the new generation is fully durable. Therefore at most one meta slot is
  mid-write at any instant, and recovery can always fall back to the other.
- Torn meta is handled by **checksum + generation selection**, NOT by assuming a 4 KB
  write is hardware-atomic. Never claim atomic-pointer-swap semantics; the commit is the
  durable, checksummed, higher-generation meta.

### C7. Invariant checker must not fight the deferred-merge policy
Constrains v1.0 §19, §39, §49.

- Deletion may leave **underfull or empty-but-valid** nodes (no merge/rebalance in v1).
  The structural checker MUST treat underfull nodes, empty leaves, and a shrunken tree as
  PASS. **Fill-factor is excluded from the invariant set.** The leaf-depth invariant is
  "all leaves at equal depth," which COW split-without-merge preserves. Do not let the
  checker flag legitimate underfull pages as corruption (it would produce false failures
  under delete-heavy property tests, v1.0 §75).

### C8. Remove `ErrConflict` from v1
Replaces v1.0 §11.

- Single-writer model has no write-write conflict to report. Remove `ErrConflict` from
  the v1 error set (or document it as explicitly reserved/unused). Keep `ErrNotFound`,
  `ErrClosed`, `ErrReadOnly`, `ErrTxnClosed`, `ErrCorrupt`, `ErrInvalidArgument`,
  `ErrIO`, `ErrChecksum`, `ErrFormat`, `ErrUnsupported`.

---

## PART D — PRIOR ART & POSITIONING (never claim we invented this)

Direct and near-direct prior art exists. We must position honestly:

- **voidDB (Go):** transactional KV, copy-on-write MVCC B+tree, resilient to torn writes,
  auto-restores to last stable state on mid-write crash, single-writer/many-reader with
  snapshot recycling gated by the oldest reader. This is architecturally almost identical
  to Anvil. **Anvil's distinguishing contribution over voidDB is the fault-injection
  proof harness + negative control + reference-model checking — voidDB documents no such
  harness.** Do not claim engine novelty; claim rigorous, verified, from-scratch,
  stdlib-only implementation.
- **bbolt / LMDB:** the acknowledged architectural ancestors (shadow-paging COW B+tree,
  double meta pages, two-phase sync-then-publish). Cite them; borrowing their design is
  legitimate and expected.
- **HaloDB (Go), and "Build Your Own Database From Scratch in Go":** from-scratch KV
  engines with B-tree + crash recovery exist as public projects/tutorials. Our edge is
  the verification methodology and the zero-dependency framing, not the engine idea.
- **Verification lineage (cite as respected precedent, not our invention):**
  FoundationDB's simulator, TigerBeetle's VOPR, Antithesis, WarpStream DST, and **LazyFS**
  (the standard fault-injection tool that does exactly "discard non-fsync'd writes"). Our
  harness applies this well-established methodology to our own engine. Framing:
  *"we applied deterministic fault-injection testing (in the lineage of FoundationDB /
  TigerBeetle / LazyFS) to a from-scratch, dependency-free storage engine."*

**Approved positioning sentence:** "Anvil is a small, from-scratch, standard-library-only
embedded transactional KV engine (copy-on-write B+tree, checksummed pages, double-buffered
generational metadata). Its contribution is not a new architecture but a dependency-free
implementation whose crash-consistency is actively attacked by a fault-injecting harness
and verified against an independent model — with a negative control proving the harness
has teeth."

---

## PART E — ACCEPTANCE CRITERIA ADDENDA
Adds to v1.0 §114, §115.

- [ ] `go.mod` has no `require` block; proof artifact checked in.
- [ ] `go build ./...` builds everything in one command; `CGO_ENABLED=0` static.
- [ ] `README.md` and graded `STDLIB.md` present and accurate.
- [ ] PRIMARY durability harness (fault-injecting backend) passes across the defined seam
      set with 0 lost acknowledged writes and 0 invariant violations.
- [ ] **NEGATIVE CONTROL passes: with a required sync barrier removed, the durability
      harness reports RED.** (Gate — without this, the proof is not credible.)
- [ ] SECONDARY real-process-kill test passes, labeled as consistency-only.
- [ ] All public claims use "crash-consistent under the documented model," with the model
      defined; no forbidden claims present anywhere in repo/video.
- [ ] Structural checker tolerates underfull/empty nodes (no false positives).

---

## PART F — NON-NEGOTIABLE INVARIANTS (I1–I10)

These are not tests. They are the laws of physics of Anvil. Any change — refactor,
rewrite, "optimization," new encoder, whatever — that violates any of I1–I10 is
**automatically rejected**, no discussion. Every PR and every subsystem reviewer
(v1.0 §118, Agent Group K) checks against this list first.

The whole system reduces to one sentence: **uncommitted state may be garbage;
committed state may never be.** Before the durable meta commit point, a new tree is
merely orphaned pages. After it, it is authoritative. I1–I10 enforce that line.

```
I1.  Committed pages are NEVER modified in place.
I2.  Metadata NEVER references a page that has not satisfied the required
     persistence barrier (data fdatasync must precede the meta that points at it).
I3.  The currently committed metadata slot is NEVER overwritten before the
     alternate generation is fully published and durable.
I4.  ACK is impossible before the durable commit point (second fdatasync returns).
I5.  Every page reachable from the committed root is checksum-valid.
I6.  Every leaf is at the same depth. (Fill-factor is NOT an invariant; underfull
     and empty-but-valid nodes are legal — see C7.)
I7.  An active snapshot can NEVER observe a page that has been overwritten or
     reclaimed. (In v1 this holds by construction: grow-only, no reuse — C5.)
I8.  A failed or uncommitted transaction can NEVER become partially visible.
I9.  Recovery selects ONLY a checksum-valid committed generation; if none exists,
     it fails loudly (never fabricates a recovered state).
I10. Reference-model code shares NO implementation logic with the production
     B+tree. The verifier must never ask Anvil to verify Anvil.
```

---

## PART G — CANONICAL PROOF CLAIM (use this wording, verbatim)

The only sanctioned way to state the result. Do not paraphrase it into anything
stronger.

- **Claim (README / video / write-up):** *"We defined a failure model in which only
  synchronized bytes survive, then exhaustively exercised the defined persistence
  seams and verified that every acknowledged transaction recovered to a valid
  committed state."*
- **The real brag (put this line in the demo):** *"When we deliberately remove a
  required persistence barrier, the verifier catches the resulting durability
  failure."* — i.e. show **TEST HAS TEETH**, not **TEST BIG NUMBER**. The negative
  control (C1) is the headline moment, not the crash counter.
- Forbidden phrasings remain forbidden (C2, v1.0 §79): "crash-safe" unqualified,
  "proved crash safety," "can never lose data," "mathematically proven,"
  "corruption-proof," "faster than production databases."

---

## PART H — SANCTIONED SCOPE (the ONLY work; everything else is bonked)

Implementation proceeds in these phases and no others. This is the complete list of
what Anvil v1 is:

```
phase 1  file / page format
phase 2  B+tree
phase 3  transactions + COW
phase 4  metadata commit / recovery
phase 5  reference model
phase 6  fault-injecting storage backend
phase 7  crash torture + NEGATIVE CONTROL
phase 8  real-file SIGKILL validation
phase 9  CLI + STDLIB.md + demo
```

**Forbidden additions (auto-reject on sight, regardless of which agent "improved"
things).** An embedded KV engine does NOT get, in v1:

- bloom filters, LSM/compaction, secondary indexes
- a write-ahead log (Anvil is WAL-free shadow paging — C3; a WAL is an architecture
  change, not an enhancement)
- adaptive/ARC caches or a sophisticated buffer manager (a trivial read cache is the
  ceiling — v1.0 §56, §57)
- freelist reuse / page compaction (v2 only — C5)
- an MVCC scheduler, isolation levels beyond single-writer snapshot
- networking, an HTTP/Prometheus/metrics endpoint, auth, TLS, users/roles
- SQL, query planning, stored procedures, scripting

Rule for the swarm: **more code is not more score.** Code Quality & Idiom is 25% of
the grade (Part A); every unrequested subsystem is surface that reads as fighting the
stdlib and dilutes the readable core. If an agent proposes anything outside phases
1–9, the answer is the SRS. Bonk accordingly.
