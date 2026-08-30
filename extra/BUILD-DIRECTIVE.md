# ANVIL — FINAL AUTONOMOUS BUILD + VERIFICATION + SELF-CORRECTION DIRECTIVE

you are the primary autonomous engineering system responsible for implementing **ANVIL**.

the attached/documented artifact:

> **SRS-v1.1-amendment.md**

is the **sole product contract**.

treat it as authoritative.

do not reinterpret it casually.

do not reopen project selection.

do not propose another architecture.

do not substitute a different product.

do not optimize for novelty at the expense of the contract.

your job is:

> **implement the exact Anvil system defined by the SRS, then attack your own implementation relentlessly until independent verification converges on the required invariants.**

you have access to extensive coding agents, review agents, test agents, fuzzing infrastructure, fault-injection infrastructure, static analysis, race detection, benchmarking, mutation/sabotage testing, and repeated automated execution.

use them aggressively.

---

# 1. PRIMARY OBJECTIVE

produce a submission-ready Anvil implementation satisfying the SRS in full.

the final repository must contain:

- working Anvil engine
- public API
- persistent on-disk format
- copy-on-write B+tree
- transactions
- snapshot readers
- double-buffered metadata
- checksummed pages
- crash recovery
- fault-injecting storage backend
- independent reference model
- structural verifier
- crash-torture harness
- negative-control durability test
- real process-kill consistency test
- CLI
- README
- STDLIB.md
- tests
- reproducible verification reports

the implementation is not complete merely because it compiles.

it is complete only when the correctness evidence satisfies the SRS acceptance gates.

---

# 2. ABSOLUTE SOURCE OF TRUTH

the SRS defines:

- scope
- architecture
- semantics
- terminology
- persistence model
- correctness requirements
- acceptance criteria
- deliverables
- project claims
- non-goals
- required proof model

whenever a proposed optimization, abstraction, feature, refactor, or "improvement" conflicts with the SRS:

> **the SRS wins.**

do not silently modify the contract.

if an implementation detail is unspecified, choose the simplest correct implementation consistent with the SRS.

---

# 3. ARCHITECTURE FREEZE

the architecture is already selected.

DO NOT reopen:

- COW vs WAL
- B+tree vs another tree
- grow-only allocation vs freelist
- single writer vs multi-writer
- metadata generation design
- 4096-byte page size
- two metadata slots
- reference-model independence
- primary fault-injecting storage backend
- secondary real process-kill backend

the architecture is:

```text
application
    ↓
transaction API
    ↓
copy-on-write B+tree
    ↓
page manager
    ↓
storage backend abstraction
    ├── real file backend
    └── fault-injecting backend
```

with:

```text
META A
META B
DATA PAGES
```

and:

```text
write new pages
↓
fdatasync(file)
↓
write alternate meta
↓
fdatasync(file)
↓
ACK
```

the implementation may improve internal code structure while preserving this architecture.

---

# 4. SCOPE FREEZE

DO NOT add features outside the SRS merely because implementation capacity exists.

especially prohibited unless the SRS explicitly requires them:

- WAL
- replication
- Raft
- clustering
- networking
- SQL
- HTTP API
- authentication
- TLS
- Prometheus
- bloom filters
- sophisticated adaptive cache
- MVCC scheduler
- page compaction
- freelist reuse
- arbitrary background maintenance
- distributed consensus
- secondary indexes
- scripting
- query language
- cloud integrations

do not let agents invent:

> "small helper"

that quietly becomes a new subsystem.

---

# 5. ENGINEERING PHILOSOPHY

the objective is not:

> maximum code

the objective is:

> **minimum coherent implementation that satisfies every requirement and survives adversarial verification.**

prefer:

- explicit code
- boring code
- idiomatic Go
- small interfaces
- deterministic behavior
- understandable invariants
- testable components
- independent verification

avoid:

- speculative abstraction
- framework-building
- excessive generic programming
- clever metaprogramming
- unnecessary interfaces
- premature optimization
- feature creep

a line of code is not automatically progress.

a verified invariant is progress.

---

# 6. MANDATORY PHASE ORDER

implement in this order unless a dependency forces a small deviation.

## phase 1 — repository and build skeleton

establish:

- module
- package layout
- CLI skeleton
- test structure
- CI
- documentation skeleton

required build:

```text
go build ./...
```

required dependency property:

```text
go.mod
```

must contain no third-party requirements.

verify:

```text
go list -m all
```

and repository inspection.

also verify:

```text
CGO_ENABLED=0 go build ./...
```

where platform/toolchain allows.

---

# 7. PHASE 2 — PAGE FORMAT

implement the immutable page-format layer first.

must define explicitly:

- page size = 4096
- byte ordering
- page header
- page types
- checksum field
- payload boundaries
- page identifiers
- transaction/generation metadata
- metadata format
- format version

create:

```text
page.go
format.go
checksum.go
meta.go
```

or equivalent coherent organization.

requirements:

- encode/decode round-trip
- malformed input rejection
- checksum validation
- bounds checking
- overflow checks
- deterministic encoding

do NOT proceed while the format layer is ambiguous.

---

# 8. IMMEDIATE SELF-REVIEW AFTER PAGE FORMAT

before continuing, spawn independent reviewers.

review questions:

1. can a malformed page trigger a panic?
2. can an integer overflow corrupt offsets?
3. can page IDs point outside the file?
4. is checksum coverage unambiguous?
5. can decoding depend on host endianness?
6. can two implementations interpret a page differently?
7. is metadata discoverable with fixed 4096-byte layout?
8. can META A/B be distinguished unambiguously?

fix every discovered issue.

repeat until reviewers produce no unresolved correctness defects.

---

# 9. PHASE 3 — STORAGE ABSTRACTION

create a storage abstraction sufficient to support BOTH:

```text
real persistent file
```

and:

```text
fault-injecting simulated stable storage
```

the abstraction must expose the persistence semantics needed by the proof model.

conceptually:

```text
read
write/pwrite-equivalent
sync/fdatasync
size
close
```

plus deterministic fault injection capabilities for tests.

DO NOT allow test logic to bypass the production storage interface.

the same engine must run against:

```text
real backend
```

and:

```text
simulation backend
```

---

# 10. PRIMARY SIMULATED DURABILITY MODEL

this is the central proof mechanism.

the fault-injecting backend must maintain a distinction between:

```text
written but unsynced bytes
```

and:

```text
synced bytes
```

on a simulated crash:

> all unsynced bytes disappear.

only bytes that crossed the simulated synchronization barrier survive.

this is the storage model the durability proof is based on.

DO NOT weaken this semantics.

DO NOT let the recovery process see unsynced bytes after a simulated crash.

DO NOT secretly implement the simulator as a normal byte array that preserves everything.

that would invalidate the proof.

---

# 11. SIMULATED STORAGE MUST BE INDEPENDENTLY TESTED

before trusting Anvil:

test the storage backend itself.

examples:

```text
write
crash
read
```

must show that unsynced writes disappear.

and:

```text
write
sync
crash
read
```

must show that synced writes survive.

test:

- short writes
- partial writes
- sync failures
- truncation
- file growth
- overlapping writes
- repeated syncs
- empty writes
- page-sized writes
- partial-page writes

the storage simulator is part of the scientific apparatus.

if it is wrong, the entire durability proof is meaningless.

---

# 12. PHASE 4 — B+ TREE

implement:

- point lookup
- insertion
- replacement
- deletion
- ordered scan
- leaf pages
- internal pages
- root
- splitting
- COW path copying

mandatory property:

> **committed pages are never modified in place.**

the implementation must be able to explain every page mutation.

for writes:

```text
old page
↓
new page
```

never:

```text
committed page
↓
destructive modification
```

---

# 13. B+ TREE INVARIANTS

the structural verifier must independently verify:

### ordering

keys in each node are correctly ordered.

### separators

internal-node separator semantics are correct.

### child references

all referenced pages exist.

### leaf depth

all leaves have equal depth.

### reachability

all pages reachable from root are valid.

### cycles

tree traversal contains no invalid cycles.

### page type correctness

a referenced child has a legal type.

### binary correctness

keys/values are treated as bytes.

### deferred deletion semantics

underfull and empty-but-valid pages are allowed.

DO NOT enforce a fill factor in v1.

---

# 14. B+ TREE TESTING

construct an independent model.

do NOT use:

- the same tree code
- the same split logic
- the same iterator logic
- the same serialization logic

for the oracle.

generate sequences involving:

```text
put
get
delete
replace
scan
```

compare semantic state.

include:

- empty keys if supported by the API
- repeated keys
- repeated deletes
- ascending insertions
- descending insertions
- random insertions
- deletes causing empty leaves
- root splits
- deep trees
- large keys
- large values
- binary bytes

do not proceed until randomized model testing is stable.

---

# 15. PHASE 5 — TRANSACTIONS

implement:

- read transaction
- write transaction
- commit
- rollback
- snapshot root
- snapshot generation

single writer.

multiple readers.

a read transaction must retain a stable committed root.

a writer must construct its new state through COW.

---

# 16. TRANSACTION INVARIANTS

I1:

> committed pages are never modified in place.

I2:

> metadata references only pages satisfying the required persistence barrier.

I3:

> the metadata slot holding the last committed generation is never overwritten until the alternate generation is fully published.

I4:

> ACK is impossible before the durable commit point.

I5:

> every reachable page from the committed root is valid.

I6:

> every leaf is at the same depth; fill factor is NOT part of the invariant set.

I7:

> active snapshots can never observe overwritten/reclaimed pages; in v1 this follows by construction because allocation is grow-only.

I8:

> an uncommitted transaction cannot become partially visible.

I9:

> recovery selects only a checksum-valid committed generation.

I10:

> reference-model code never shares implementation logic with the production tree.

these are not suggestions.

these are laws.

any violation is a blocking defect.

---

# 17. PHASE 6 — COMMIT PROTOCOL

implement exactly:

```text
build new pages
↓
write new pages
↓
fdatasync(file)
↓
write alternate metadata slot
↓
fdatasync(file)
↓
ACK
```

the implementation must make the dangerous seams explicit.

use a single metadata write to the alternate slot.

never overwrite the currently committed metadata slot before the new generation has satisfied the required durability barrier.

---

# 18. METADATA GENERATION RULE

each committed state has monotonically increasing transaction/generation ID.

recovery:

```text
read meta A
read meta B
validate checksum
validate format
validate root
choose newest valid generation
```

must NOT infer validity merely from:

- generation number
- successful read
- physical position
- write completion

checksum and structural validation matter.

---

# 19. PHASE 7 — RECOVERY

recovery must handle:

### crash before data sync

old generation remains authoritative.

### crash after data sync before metadata publication

new pages are unreachable garbage; old generation remains authoritative.

### crash during metadata write

torn/invalid metadata is rejected; previous valid generation survives.

### crash after metadata sync

new generation becomes authoritative.

### invalid metadata

fallback if another valid generation exists.

### both metadata invalid

return explicit corruption error.

---

# 20. RECOVERY MUST BE BORING

do not make recovery "repair" arbitrary corruption.

do not guess.

do not silently reconstruct unknown state.

if the committed structure cannot be justified:

> fail clearly.

the safest recovery implementation is often the least clever.

---

# 21. PHASE 8 — PAGE CHECKER

build an independent deep checker.

minimum:

```text
metadata
root
reachable pages
checksums
page references
tree ordering
separator validity
uniform leaf depth
cycles
```

the checker must distinguish:

```text
valid but underfull
```

from:

```text
corrupt
```

empty-but-valid pages may exist under v1 deletion semantics.

---

# 22. PHASE 9 — REFERENCE MODEL

implement a trivially correct independent model.

recommended:

```text
ordered in-memory map
```

the model should have:

- different internal data structures
- separate code
- separate tests
- separate assumptions

DO NOT reduce duplication by sharing implementation helpers with Anvil's production tree.

independence is part of the proof.

---

# 23. PHASE 10 — MODEL-BASED TESTING

create stateful workload generators.

example:

```text
put
put
get
scan
delete
put
txn
scan
delete
get
```

record:

- seed
- operation sequence
- expected state
- actual state

when a mismatch occurs, automatically minimize the sequence if practical.

required failure output:

```text
seed:
operation:
transaction:
expected:
actual:
```

every failure must be reproducible.

---

# 24. PHASE 11 — PRIMARY DURABILITY HARNESS

this is the flagship verification system.

the harness must:

1. generate a workload
2. execute transactions
3. inject deterministic crashes at persistence seams
4. simulate loss of all unsynced bytes
5. restart/recover
6. compare recovered state with the expected committed model
7. run the structural checker
8. record the result
9. repeat

critical fault seams must be explicitly enumerated.

minimum conceptual seam classes:

```text
after writing a data page
before data sync
after data sync
before metadata write
during metadata write
after metadata write
before metadata sync
after metadata sync
before acknowledgement
after acknowledgement
```

the simulator operates at I/O-operation granularity.

DO NOT claim instruction-level exhaustive coverage.

---

# 25. PRIMARY DURABILITY PROPERTY

the central property is:

> **every transaction acknowledged as committed must remain recoverable after a simulated crash under the defined storage model.**

for every crash:

```text
acked transaction set
```

must be reconciled with:

```text
recovered committed transaction set
```

there must be:

```text
zero lost acknowledged transactions
```

and:

```text
zero partial transactions
```

---

# 26. NEGATIVE CONTROL — MANDATORY

this is a gating requirement.

create an explicit sabotage configuration.

examples:

```text
disable pre-meta fdatasync
```

or:

```text
disable meta fdatasync
```

then run the PRIMARY durability harness.

the harness MUST detect the defect.

expected result:

```text
DURABILITY TEST: FAIL
```

if the sabotaged implementation passes:

> **STOP.**

the proof apparatus is insufficient.

do not rationalize.

do not proceed.

improve the harness until the removed barrier creates an observable failure in the defined model.

---

# 27. MUTATION/SABOTAGE TESTING

do not only test the named fsync sabotage.

where practical, automatically create controlled mutations such as:

- remove sync
- reorder sync
- publish metadata early
- swap metadata order
- skip checksum validation
- accept invalid page references
- reuse an old generation incorrectly
- acknowledge before metadata sync

for each mutation ask:

> does the verification system catch it?

this is one of the most important uses of the agent fleet.

a verification system that only catches the exact bug you thought of is weak.

---

# 28. PROOF-STRENGTH LOOP

run this loop continuously:

```text
IMPLEMENT
    ↓
TEST
    ↓
ATTACK
    ↓
FIND FAILURE
    ↓
MINIMIZE
    ↓
FIX
    ↓
ADD REGRESSION TEST
    ↓
ATTACK AGAIN
```

do not stop at the first green build.

---

# 29. ADVERSARIAL REVIEWER AGENTS

maintain separate reviewers whose only job is to try to break Anvil.

assign different specialties:

### storage reviewer

attacks page format, COW, commit ordering, recovery.

### filesystem reviewer

attacks sync semantics, short writes, file growth, directory persistence assumptions.

### B+tree reviewer

attacks splits, deletion, ordering, scans.

### transaction reviewer

attacks snapshots, atomicity, reader/writer interactions.

### corruption reviewer

mutates bytes and metadata.

### concurrency reviewer

attacks race conditions and snapshot lifetime.

### verification reviewer

tries to create false positives/false negatives in the proof system.

### API reviewer

searches for semantic inconsistencies and lifecycle bugs.

### SRS compliance reviewer

checks implementation against every requirement in the contract.

### presentation reviewer

looks for claims in README/demo that are stronger than the actual evidence.

these agents must be adversarial, not collaborative.

---

# 30. CROSS-REVIEW RULE

no critical subsystem should be approved by only the agent that implemented it.

for each major component:

```text
implementer
↓
independent reviewer A
↓
independent reviewer B
↓
automated tests
↓
adversarial tests
↓
merge
```

where practical, reviewers should not be shown the implementer's reasoning first.

make them inspect the artifact independently.

---

# 31. REAL PROCESS-KILL TEST

after the primary simulated durability proof is stable, test the real file backend.

workflow:

```text
start database
↓
perform real transactions
↓
SIGKILL process
↓
restart
↓
recover
↓
check tree
↓
compare state
```

label this correctly:

> **real process-crash consistency test**

do NOT claim that this independently proves power-loss durability.

the purpose is end-to-end validation of:

- real syscalls
- actual file I/O
- actual reopen
- actual recovery

---

# 32. REAL DISK CORRUPTION TESTS

where safely possible, create controlled corruption experiments.

examples:

- damage newest metadata
- damage older metadata
- corrupt reachable page
- truncate file
- modify checksum
- corrupt page header

expected behavior must match the documented recovery model.

the test must distinguish:

```text
recoverable stale generation
```

from:

```text
committed corruption
```

and must fail loudly when appropriate.

---

# 33. SHORT-WRITE TESTING

do not assume:

> one Write call = all bytes written.

the storage layer must correctly handle short writes.

test:

```text
write 4096
→ only 17 bytes accepted
```

and analogous partial results.

verify that no persistence boundary is falsely treated as complete.

---

# 34. SYNC-FAILURE TESTING

simulate:

```text
write succeeds
sync fails
```

verify:

- transaction is not acknowledged
- state is not incorrectly published
- engine enters the documented failure state
- recovery remains valid

do not silently ignore sync errors.

do not continue as though durability succeeded.

---

# 35. FILE-GROWTH TESTING

test page allocation around:

```text
page boundary
file growth boundary
metadata boundary
large file offsets
```

test:

- empty file
- first page
- first data page
- multiple metadata generations
- thousands of pages

---

# 36. INTEGER / OFFSET SAFETY

all calculations involving:

- page IDs
- byte offsets
- file sizes
- page lengths
- child references

must protect against overflow and underflow.

malformed disk data must never create an unsafe allocation or invalid seek.

---

# 37. CONCURRENCY VERIFICATION

run:

```text
go test -race
```

regularly.

add deterministic concurrency tests where useful.

test:

```text
reader + writer
reader + reader
many readers
repeated commits
long-lived snapshots
rapid reopen
```

the core invariant:

> a reader sees one coherent committed generation.

---

# 38. DO NOT ADD FREELIST REUSE

v1 is grow-only.

do not optimize away the storage growth.

do not reclaim pages.

do not reuse old pages.

the snapshot safety of v1 depends on:

> old committed pages never being overwritten.

protect this invariant.

---

# 39. DO NOT ADD PAGE COMPACTION

even if the database grows rapidly.

even if an agent says:

> "this is easy."

do not.

space efficiency is secondary to proof clarity.

---

# 40. PERFORMANCE

benchmark for regression awareness only.

measure:

- get
- put
- delete
- scan
- transaction commit
- recovery
- database growth

do not frame Anvil as beating established storage engines.

the engineering objective is:

> correctness and coherent implementation.

---

# 41. API QUALITY PASS

after engine correctness is stable, inspect the public API as if you were a library user.

verify:

- names
- ownership
- errors
- lifecycle
- transaction invalidation
- close behavior
- reader lifetime
- binary safety
- documentation
- predictable semantics

the final API should feel like idiomatic Go.

---

# 42. IDIOMATIC GO REVIEW

perform a dedicated Go review.

look for:

- unnecessary abstractions
- awkward interfaces
- error wrapping problems
- ignored errors
- lock misuse
- goroutine leaks
- context misuse if applicable
- unnecessary allocations
- unclear ownership
- poor naming
- package-level state
- surprising APIs
- non-idiomatic serialization
- giant functions
- giant files
- generated-code ugliness

the rubric explicitly rewards code quality.

agents generating huge amounts of code is NOT a reason to keep huge amounts of code.

delete unnecessary code.

---

# 43. STDLIB.md — GRADED DELIVERABLE

produce an excellent `STDLIB.md`.

it must explain, for every significant capability:

```text
problem
↓
typical external dependency
↓
Anvil requirement
↓
stdlib package / primitive used
↓
why the replacement works
↓
tradeoffs
```

examples:

```text
persistent file I/O
→ database/storage dependency
→ os.File + file operations
```

```text
binary page encoding
→ serialization dependency
→ encoding/binary
```

```text
checksums
→ checksum library
→ hash/crc32
```

```text
concurrency
→ synchronization framework
→ sync
```

```text
testing
→ external testing framework
→ testing
```

```text
deterministic synchronization testing
→ external concurrency-test tooling
→ testing/synctest where appropriate
```

be precise.

do not claim that Go's standard library "replaces a database" by itself.

explain the engineering work we implemented around these primitives.

also explain:

- why no third-party runtime dependencies exist
- why test-only external tools do not become runtime dependencies
- how the standard library maps to the storage architecture

`STDLIB.md` is part of the product.

review it like a graded artifact.

---

# 44. README.md

README must contain:

## what Anvil is

## who it is for

## build

```text
go build ./...
```

## usage

CLI examples.

## architecture

concise diagram.

## transaction model

single writer, snapshot readers.

## persistence model

shadow paging / COW.

## crash model

precise wording.

## verification

primary simulated durability proof.

secondary process-kill consistency test.

## negative control

explain that disabling a required persistence barrier makes the durability verifier fail.

## limitations

be explicit.

## prior art

cite architectural ancestry honestly.

do not claim invention of COW B+trees.

---

# 45. SANCTIONED LANGUAGE

the following language must be used consistently.

preferred:

> **crash-consistent under the documented storage and failure model**

preferred:

> **deterministically tested against a defined simulated-crash model**

preferred:

> **every acknowledged transaction recovered correctly across the defined fault-injection seam set**

preferred:

> **independent reference-model verification**

preferred:

> **WAL-free shadow paging**

forbidden:

> "can never lose data"

> "mathematically proven"

> "corruption-proof"

> "perfectly crash-safe"

> "guaranteed against arbitrary hardware failure"

> "faster than production databases"

---

# 46. PRIOR ART HONESTY

do not claim:

> invented COW databases

do not claim:

> invented shadow paging

do not claim:

> invented crash testing

do not claim:

> invented deterministic fault injection

acknowledge relevant architectural and verification precedents.

the contribution is:

> coherent from-scratch standard-library implementation + rigorous fault-injection methodology + independent model checking + negative control.

---

# 47. VERIFICATION REPORT

automatically generate machine-readable and human-readable verification reports.

include:

```text
build
dependency check
unit tests
property tests
model tests
race tests
structural checks
fault seams
crash cycles
recovery failures
acknowledged writes lost
invariant violations
negative control result
process-kill result
```

example:

```text
ANVIL VERIFICATION

build: PASS
stdlib-only: PASS

unit tests: PASS
property tests: PASS
model tests: PASS
race detector: PASS

primary durability:
  crash cycles:              ...
  acknowledged writes lost: 0
  invariant violations:     0
  recovery failures:        0

fault-seam coverage:
  data-write:               PASS
  data-sync:                PASS
  meta-write:               PASS
  meta-sync:                 PASS
  acknowledgment boundary:  PASS

negative control:
  required sync removed:    FAIL DETECTED ✅

real process crash:
  recovery:                  PASS

RESULT: PASS
```

numbers must be real.

never fabricate them.

---

# 48. NEGATIVE-CONTROL REPORTING

the negative control should explicitly show:

```text
GOOD BUILD
→ durability verifier PASS

SABOTAGED BUILD
→ durability verifier FAIL
```

this should be visible in CI artifacts.

the negative-control implementation may use:

- build tag
- test-only configuration
- dependency injection
- mutation hook

but it must be checked in and reproducible.

the control itself must not alter production behavior.

---

# 49. FAILURE REPRODUCTION

any verification failure must output:

```text
seed
test name
operation sequence
transaction sequence
crash seam
storage state
expected state
actual state
```

where possible, automatically save a compact reproducer.

a bug that cannot be reproduced is not considered solved.

---

# 50. FAILURE MINIMIZATION

when randomized testing finds a failure:

attempt to minimize:

```text
number of operations
number of keys
transaction size
number of pages
crash point complexity
```

until a small deterministic reproducer exists.

add that reproducer to regression tests.

---

# 51. REGRESSION RULE

every discovered bug must become:

```text
bug
↓
minimal reproduction
↓
fixed implementation
↓
permanent regression test
```

no "we fixed it manually" without a test.

---

# 52. CONVERGENCE CRITERION

do not declare victory because:

```text
tests pass
```

declare victory only when:

1. SRS compliance passes.
2. independent code review passes.
3. reference-model testing passes.
4. structural checker passes.
5. primary durability harness passes.
6. negative control fails as intended.
7. real process-kill consistency passes.
8. race detector passes.
9. corruption tests pass.
10. repeated adversarial review produces no unresolved blocking issue.
11. README claims exactly match evidence.
12. STDLIB.md accurately reflects the implementation.
13. build/dependency requirements pass from a clean environment.

---

# 53. CLEAN-ROOM VALIDATION

perform at least one final test from a clean environment.

verify:

```text
clone repository
↓
go build ./...
↓
run tests
↓
create database
↓
put/get/scan/txn
↓
reopen
↓
run checker
```

the project must not depend on:

- undeclared files
- hidden local tools
- developer machine state
- untracked fixtures
- environment variables not documented
- external runtime services

---

# 54. REPOSITORY HYGIENE

before finalization inspect:

- tracked files
- generated artifacts
- temporary logs
- secrets
- binaries
- stale experiments
- dead code
- abandoned packages
- commented-out implementations
- benchmark debris
- debug flags
- accidental external dependencies

the repository should look intentional.

---

# 55. AGENT ARTIFACT HYGIENE

do not let generated agent output create:

- redundant implementations
- duplicate packages
- multiple competing APIs
- contradictory test helpers
- shadow specifications
- stale architecture diagrams
- conflicting terminology

consolidate.

prefer one canonical implementation.

---

# 56. STATIC ANALYSIS

run all appropriate standard Go tooling available in the environment.

at minimum consider:

```text
go vet ./...
go test ./...
go test -race ./...
```

plus relevant static analysis and formatting.

the repository should be formatted and clean.

---

# 57. FUZZING

fuzz at multiple levels.

## page fuzzing

random bytes into decoders.

## transaction fuzzing

random stateful operations.

## tree fuzzing

random insertion/deletion sequences.

## recovery fuzzing

random valid/invalid metadata/page combinations.

## crash fuzzing

random persistence seams.

## workload fuzzing

random keys, values, transaction sizes, and operation distributions.

all fuzzers must preserve reproducible seeds.

---

# 58. CORRUPTION FUZZING

mutate persisted databases:

- flip bits
- truncate bytes
- alter page IDs
- alter lengths
- alter generation IDs
- alter checksums
- alter separators
- alter child pointers
- alter metadata

verify:

> valid corruption is detected rather than silently accepted.

do not expect arbitrary corruption to be repaired.

---

# 59. LONG-RUN TORTURE

run very large workloads.

the exact number is not important until the implementation is stable.

the goal is:

> enough repeated execution that rare ordering defects have a chance to surface.

parallelize workers if useful.

each worker gets an independent seed.

collect all failures.

never hide failures just because another worker passed.

---

# 60. DIFFERENTIAL / MODEL TESTING MUST REMAIN INDEPENDENT

do not accidentally make the reference model depend on:

- Anvil serialization
- Anvil page layout
- Anvil iterator
- Anvil comparison functions
- Anvil transaction logic

independent means independent enough to catch the same bug.

when in doubt, rewrite the oracle more simply.

---

# 61. OBSERVABILITY

the engine should expose useful internal diagnostics for testing:

- generation
- root
- page count
- active readers
- transaction state
- commit state
- fault checkpoint

but diagnostics must not become a production dependency.

---

# 62. CLI QUALITY

the CLI must feel deliberate.

required:

```text
create
put
get
del
scan
txn
check
info
torture
```

use clear exit status codes.

errors must be understandable.

output should be useful in a five-minute demo.

---

# 63. DEMO PREPARATION

the final demo must demonstrate:

### 1. useful product

put/get/delete/scan.

### 2. atomic transaction

multiple mutations become one committed state.

### 3. snapshot behavior

old reader sees stable state.

### 4. simulated crash proof

fault injector destroys dangerous commit transitions.

### 5. real process crash

SIGKILL then reopen.

### 6. negative control

show that removing a required sync causes the verifier to fail.

### 7. invariant checking

show structural validation.

---

# 64. FIVE-MINUTE DEMO STORY

recommended flow:

```text
0:00
"this is Anvil, an embedded transactional storage engine."

0:20
basic Put/Get/Scan

0:50
multi-key transaction

1:20
snapshot reader while writer commits

2:00
explain:
"we deliberately kill the storage engine during its commit protocol."

2:20
live torture system

crashes survived: ...
acked writes lost: 0
invariant violations: 0

3:10
show exact commit seams

3:30
negative control:
remove required sync
→ verifier turns RED

4:00
real SIGKILL
→ restart
→ recover

4:30
show STDLIB.md / zero-dependency build
→ final limitations
```

do not oversell.

---

# 65. SOUND-OFF TEST

watch the recorded demo with narration muted.

the viewer must still understand:

```text
database
transaction
crash
recovery
verification
zero lost acknowledged writes
negative control
```

improve on-screen labels until this works.

---

# 66. JUDGE ATTACK SIMULATION

before submission, conduct hostile Q&A.

examples:

> where is your WAL?

answer:

> Anvil uses WAL-free shadow paging; the metadata page is the durable commit record.

> isn't this just bbolt?

answer honestly:

> the architecture is closely related; we do not claim to invent it. Our emphasis is a stdlib-only implementation plus an independently verified fault-injection methodology.

> how do you know your durability test has teeth?

answer:

> we remove a required persistence barrier in a sabotage build, and the same harness detects the resulting durability failure.

> why does SIGKILL prove anything?

answer:

> it validates real process-crash recovery; the primary durability evidence comes from the simulated stable-storage model where unsynced bytes disappear.

> how do you know the B+tree is not corrupt?

answer:

> the structural checker traverses the entire reachable tree and verifies checksums, references, ordering, separators, and leaf-depth invariants independently.

> why don't you reclaim pages?

answer:

> v1 intentionally uses grow-only allocation so old snapshots can never observe reused pages; reclamation is outside the frozen proof core.

> isnt write amplification bad?

answer:

> yes. shadow paging intentionally trades write amplification for simple crash consistency and stable snapshots. performance is not our headline claim.

the agents must generate additional questions.

---

# 67. CLAIM AUDIT

before release, crawl:

- README
- STDLIB
- docs
- CLI help
- comments
- demo captions
- source comments
- test names
- verification reports

look for forbidden or inaccurate claims.

examples:

```text
crash-safe
durable
guaranteed
atomic
ACID
proof
never loses data
production-ready
faster
```

every occurrence must be checked against the SRS.

---

# 68. FINAL SRS TRACEABILITY MATRIX

generate a machine-readable matrix:

```text
SRS requirement
→ implementation file(s)
→ test(s)
→ verification evidence
→ status
```

example:

```text
C1 PRIMARY PROOF
→ storage/fault.go
→ verifier/durability_test.go
→ report.json
→ PASS
```

every requirement must map to evidence.

zero unmapped mandatory requirements.

---

# 69. AUTO-REJECT CONDITIONS

the build is NOT complete if ANY of these occur:

- third-party runtime dependency exists
- `go build ./...` fails
- page size is not fixed at 4096
- freelist reuse exists in v1
- committed pages are mutated in place
- metadata publication violates ordering
- ACK occurs before required sync
- reference model shares critical implementation logic
- structural checker has false positives on allowed underfull nodes
- primary durability harness passes when required sync is disabled
- real process-kill recovery fails
- race detector reports an unresolved race
- corruption can silently produce invalid committed state
- README overclaims guarantees
- STDLIB.md is inaccurate
- architecture changes from the SRS
- out-of-scope features are introduced at the expense of core correctness

---

# 70. AUTO-REJECT FOR AGENT CREEP

reject any PR whose justification is primarily:

- "future-proofing"
- "performance"
- "cleaner architecture"
- "standard database practice"
- "users may want this"
- "we have enough agents"
- "it only adds a few hundred lines"

unless the change is already required by the SRS.

---

# 71. WHEN TO STOP CODING

stop adding features when:

```text
all requirements satisfied
+
all mandatory tests pass
+
verification converges
+
negative control works
+
claims are audited
+
documentation is complete
```

do not keep coding because agent capacity remains.

unused capacity is preferable to unnecessary complexity.

---

# 72. FINAL DESTRUCTIVE REVIEW

before declaring completion, intentionally ask independent agents to answer:

> "how could Anvil be wrong even though every current test passes?"

collect answers.

for every plausible failure mode:

- create a targeted test
- create a fault injection
- create a structural check
- or formally document why it lies outside the defined model

repeat until no plausible untested blocking issue remains.

---

# 73. FINAL CONVERGENCE RUN

run the entire pipeline from scratch:

```text
clean checkout
↓
dependency verification
↓
build
↓
unit tests
↓
property tests
↓
model tests
↓
fuzz tests
↓
race tests
↓
structural checker
↓
primary durability torture
↓
negative control
↓
real process-kill test
↓
corruption tests
↓
documentation validation
↓
claim audit
↓
final report
```

do not rely on results from a dirty workspace.

---

# 74. FINAL OUTPUTS

the finished repository must contain at minimum:

```text
README.md
STDLIB.md
go.mod
source/
tests/
CLI/
verification/
```

exact package structure may differ, but the repository must be coherent.

also produce:

```text
verification/latest-report.json
verification/latest-report.txt
```

or equivalent machine-readable/human-readable artifacts.

---

# 75. FINAL REPORT MUST STATE

```text
ANVIL FINAL STATUS

SRS compliance: PASS/FAIL
build: PASS/FAIL
stdlib-only: PASS/FAIL
API tests: PASS/FAIL
B+tree tests: PASS/FAIL
transaction tests: PASS/FAIL
snapshot tests: PASS/FAIL
structural checker: PASS/FAIL
primary durability: PASS/FAIL
negative control: PASS/FAIL
real process crash: PASS/FAIL
corruption tests: PASS/FAIL
race tests: PASS/FAIL
documentation audit: PASS/FAIL

blocking defects: 0
```

never invent a PASS.

---

# 76. FINAL RULE ABOUT "PROOF"

you are not being asked to mathematically prove arbitrary correctness.

you are being asked to build:

> **a well-defined failure model + an implementation + an independent verification system that aggressively tests the implementation against that model.**

the credibility comes from:

```text
defined model
+
independent oracle
+
structural invariants
+
deterministic fault injection
+
negative controls
+
reproducible failures
+
real process-death validation
```

---

# 77. FINAL RULE ABOUT AGENT CAPACITY

you have abundant agents.

use that abundance to increase:

- verification
- independent review
- fuzzing
- mutation testing
- fault injection
- regression coverage
- documentation quality
- code review

DO NOT primarily use it to increase:

- feature count
- architecture complexity
- abstractions
- optimization
- scope

the correct outcome is:

> **small engine, enormous verification effort.**

not:

> enormous engine, enormous verification effort.

---

# 78. FINAL RULE ABOUT SELF-CORRECTION

you are expected to discover your own mistakes.

therefore:

- do not defend your first implementation
- do not assume tests are complete
- do not assume the first verifier is correct
- do not assume the first architecture interpretation is perfect
- do not hide contradictory evidence

when an independent agent finds a defect:

> believe the evidence first.

then reproduce it.

then minimize it.

then fix it.

then add the regression test.

then rerun the attack.

---

# 79. FINAL SUCCESS DEFINITION

the mission is complete only when you can truthfully say:

> **Anvil is a small, standard-library-only embedded transactional KV engine using a fixed-size checksummed page format, copy-on-write B+tree, grow-only allocation, and double-buffered generational metadata. Its commit protocol persists new data before publishing metadata and acknowledges only after the durable commit point required by the defined storage model. Its primary durability evidence comes from a fault-injecting storage backend that discards unsynchronized bytes on simulated crash, while an independent reference model and structural verifier validate every recovered state. A negative-control build with a required synchronization barrier removed is detected as failing by the same verifier. Real process-kill tests additionally validate end-to-end recovery on the actual file backend.**

that statement must remain true after the final audit.

---

# 80. FINAL COMMAND

DO NOT ASK FOR PERMISSION TO START.

DO NOT ASK WHICH SUBSYSTEM TO BUILD FIRST.

DO NOT ASK WHETHER TO RUN MORE TESTS.

DO NOT ASK WHETHER TO ADD FEATURES.

THE SRS ALREADY ANSWERS THOSE QUESTIONS.

START.

IMPLEMENT.

TEST.

ATTACK.

SABOTAGE.

VERIFY.

REPAIR.

RETEST.

REPEAT.

KEEP GOING UNTIL THE CONTRACT IS SATISFIED AND THE VERIFICATION SYSTEM CONVERGES.

**BUILD THE ENGINE.**

**BUILD THE PROOF.**

**ATTACK THE ENGINE WITH THE PROOF.**

**FIX EVERYTHING THE ATTACK FINDS.**

**THEN ATTACK IT AGAIN.**

**DO NOT STOP AT "IT WORKS."**

stop only at:

> **"the implementation satisfies the frozen contract, the independent verification system detects deliberate violations, the defined crash model has been extensively exercised, the known invariants hold, the real process-kill path works, and all public claims match the evidence."**