# Anvil — 5-minute demo script

Every claim below is something the viewer watches happen. No narration is needed
to follow it; the on-screen output tells the story.

Build once before recording:

```bash
CGO_ENABLED=0 go build -o anvil ./cli
```

## 0. Provenance first (20s) — prove there is no hidden database

```bash
cat go.mod          # module line, no require block
go list -m all      # one module: no third-party dependencies
ldd ./anvil         # "not a dynamic executable"
```

Say: *"One static binary, standard library only, no C, no external database."*

## 1. It is a real, useful store (50s)

```bash
./anvil --help                            # one command, everything discoverable
./anvil create data.anvil
./anvil put  data.anvil user:1 lakshya
./anvil put  data.anvil user:2 harsh
./anvil get  data.anvil user:1            # lakshya
./anvil scan data.anvil user:             # ordered range scan
./anvil info data.anvil                   # generation, root page, pages
```

## 2. Atomic multi-key and snapshots (40s)

Show the library example from the README: a transfer that commits atomically,
and a snapshot reader that keeps seeing the old world while a writer commits a
new one. `anvil check data.anvil` afterwards to show the tree is sound.

## 3. The headline: proof by destruction (60s)

```bash
./anvil torture --cycles 100000
```

On screen:

```
  crashes injected       100000
  recovery failures      0
  acked writes lost      0
  invariant violations   0

  commit seams exercised     (all nine)

  RESULT: PASS
```

Say: *"We killed the engine 100,000 times — at every seam of the commit protocol
— and after each one verified that recovery is valid and no acknowledged write
was lost."*

## 4. The moment that makes it credible (60s)

```bash
./anvil torture --negative-control
```

```
  RESULT: FAIL
    cycle 0, seam before_meta_sync, crash style persist-meta-only
    recovery open failed: anvil: database corrupt

  EXPECTED FAILURE: with the pre-metadata durability barrier removed,
  the verifier caught the resulting data loss. The test has teeth.
```

Say: *"When we delete a required durability barrier, the verifier catches it
immediately. So the passing result above isn't a big number — it's a test with
teeth."*

## 5. Everything at once (40s)

```bash
./anvil verify
```

Five checks — model equivalence, concurrency, crash torture, the negative
control, and a real process kill — one verdict. Point out the process-kill line:
*"Not a simulation. A real process, really killed, reopened clean, every
acknowledged key intact."*

## Close (10s)

*"A crash-consistent storage engine from nothing but the standard library, and a test
that proves — rather than asserts — that it survives its own destruction."*

Full reproduction for anyone watching: `./scripts/verify.sh`.
