# Anvil — 5-minute demo script

Every claim below is something the viewer watches happen. No narration required
to follow it; the on-screen output tells the story.

## 0. Provenance first (20s) — prove there is no hidden database

```
cat go.mod                 # module anvil / go 1.27 — no require block
go list -m all             # prints only: anvil
CGO_ENABLED=0 go build -o anvil ./cmd/anvil
```

Say: *"One static binary, standard library only, no C, no external database."*

## 1. It's a real, useful KV store (50s)

```
./anvil create data.anvil
./anvil put  data.anvil user:1 lakshya
./anvil put  data.anvil user:2 harsh
./anvil get  data.anvil user:1            # lakshya
./anvil scan data.anvil user:             # ordered range scan
./anvil info data.anvil                   # generation / root page / pages
```

## 2. Atomic multi-key + snapshot (40s)

Show (in a tiny Go snippet or the library example in the README) a transaction
moving a balance from A to B committing atomically, and a snapshot reader that
keeps seeing the old world while a writer commits a new one.

## 3. The headline: proof by destruction (60s)

```
./anvil torture --cycles 100000
```

On screen:

```
crashes injected:        100000
recovery failures:       0
acked writes lost:       0
invariant violations:    0
fault seams exercised:   (all 9 commit seams)
RESULT: PASS
```

Say: *"We killed the engine 100,000 times — at every seam of the commit protocol
— and verified after each one that recovery is valid and no acknowledged write
was lost."*

## 4. The moment that makes it credible — the negative control (60s)

```
./anvil torture --negative-control
```

On screen:

```
RESULT: FAIL
  seam=before_meta_sync  style=persist-meta-only
  detail: recovery open failed: anvil: database corrupt

EXPECTED FAILURE: with the pre-meta data barrier removed, the
verifier caught the durability violation. The test has teeth.
```

Say: *"When we delete a required durability barrier, the verifier immediately
catches it. So the passing result above isn't a big number — it's a test with
teeth."*

## 5. Real process kill (40s)

```
# terminal A: commit in a loop to a real file
# terminal B: kill -9 it, then:
./anvil check data.anvil        # structural check: PASS
./anvil info  data.anvil        # a valid committed generation survived
```

Say: *"Not a simulation — a real process, really killed, reopened clean, every
acknowledged key intact."*

## Close (10s)

*"A crash-safe storage engine, from nothing but the standard library, and a test
that proves — not asserts — that it survives its own destruction."*
