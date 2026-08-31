# Demo video — script and recording notes

The submission video is 4:56. It is built as 17 scenes; the animated cut is
rendered from `anvil-video/src/v2`, and any scene can be swapped for a real
screen recording without touching the others.

**Tone:** explain, don't sell. The viewer may never have used an embedded
database. No superlatives, no "insane", no "we killed it". Say what the thing
does and then show it doing it.

---

## Narration

Read at a normal pace — roughly 140 words per minute. Timings are the scene
lengths, so there is deliberate silence where something is being shown.

### 01 · Title (0:00–0:11)

> Anvil is an embedded key/value database, written in Go, with no third-party
> dependencies. It's our entry for Track D — Data and Storage. Everything in
> this video can be reproduced from the repository.

### 02 · The problem (0:11–0:27)

> Almost every application has to save data to disk, and almost nobody writes
> that layer themselves. You install a library, and that library installs its
> own. A typical application ends up carrying more than a thousand of them.
> Anvil is that layer, written from scratch, with an empty dependency list.

### 03 · What it is (0:27–0:45)

> If you haven't used one before: an embedded database runs inside your program.
> There's no server to install or connect to. Your whole database is one file.
> Changes are transactional — a group of writes either all happen or none of
> them do. And if the process is killed halfway through a write, the file is
> still valid when you open it again.

### 04 · Zero dependencies (0:45–1:07)

> The rule of this hackathon is an empty dependency manifest. Here's ours.
> `go.mod` has no require block. `go list` shows one module — our own. And the
> binary is statically linked, so there's nothing loaded at runtime either.

### 05 · What the standard library replaced (1:07–1:22)

> Fourteen things we would normally have installed, and what we used instead.
> The database itself, a serialization library, a checksum library, a B-tree
> package, a CLI framework, a test framework, and a fault-injection tool. All
> of them are documented in STDLIB.md.

### 06 · Demo — the basics (1:22–1:48)

> One command builds it. Then: create a database, store a couple of records,
> read one back. This is a real file on disk — close the program and the data
> is still there.

### 07 · Demo — ordered scans (1:48–2:09)

> Keys come back in sorted order, and you can ask for a range of them. That's
> what a B-tree gives you over a hash map. `check` walks the entire file and
> verifies every page.

### 08 · Transactions (2:09–2:24)

> Moving money between two accounts is two writes. If the machine fails between
> them, one account has lost money that never arrived. Inside a transaction,
> that can't happen — a reader sees both balances or neither.

### 09 · Why databases break (2:24–2:38)

> Here's the hard part. The dangerous way to save is to write new data on top of
> old data. If the power fails halfway, the file holds half of each, and it
> won't open again.

### 10 · Copy-on-write (2:38–2:57)

> So Anvil never edits in place. A change writes brand-new pages and leaves the
> old ones untouched. Only when the new version is completely safe on disk does
> it switch over. A crash can only ever leave you the old version or the new one.

### 11 · The commit order (2:57–3:13)

> The order is the whole guarantee. Write the pages. Wait for the disk to
> confirm. Only then write the pointer to the new version, and wait again. The
> application is told the save succeeded only after that second confirmation.

### 12 · Why prove it (3:13–3:27)

> Every database claims to survive a crash. It's easy to say and hard to show.
> So Anvil ships a tool that attacks itself.

### 13 · 100,000 crashes (3:27–3:51)

> It kills the engine at nine different points inside a save, reopens the file
> from exactly the bytes that reached the disk, and compares the result against
> a separate, simpler model of the data. A hundred thousand times. Nothing that
> was acknowledged was ever lost.

### 14 · Testing the test (3:51–4:12)

> A big number isn't evidence on its own — a crash test can pass for the wrong
> reason. So we broke Anvil on purpose, removed one of the two waits for the
> disk, and ran the identical test. It fails, immediately. That's why the green
> result counts.

### 15 · Benchmarks (4:12–4:32)

> Same workload, same machine, against the two libraries you'd otherwise
> install. Anvil opens a file faster than both, and beats SQLite on saves and
> scans. bbolt reads faster because it maps the file into memory — a shortcut we
> chose not to take, and one we document.

### 16 · Reproduce it (4:32–4:49)

> None of this asks you to take our word. One script runs every check in this
> video.

### 17 · Close (4:49–4:56)

> Anvil. An embedded, crash-consistent key/value database. Written in Go, with
> no third-party dependencies.

---

## Recording the demo scenes yourself

The animated scenes reproduce real output, but if you want genuine screen
capture, these four are the ones worth recording. Everything else is
architecture, which a terminal cannot show.

**Setup, once:**

```bash
cd /path/to/anvil
go build -o anvil ./cli
mkdir -p /tmp/demo && cd /tmp/demo
```

- Terminal at **1920×1080**, font size **20–22pt**, a dark theme, and a short
  prompt (`PS1='$ '`). A long prompt with a path eats the frame.
- Clear scrollback before each take (`clear`).
- Do a dry run first: two of these commands take real time, and you want to
  know how long before you are recording.

### Take 1 — dependency proof (~20 s)

```bash
cat ../anvil/go.mod
go list -m all
ldd ./anvil        # macOS: otool -L ./anvil
```

Expect: no `require` block, one module, `not a dynamic executable`.

### Take 2 — basic usage (~40 s)

```bash
./anvil create library.anvil
./anvil put library.anvil book:1984 'George Orwell'
./anvil put library.anvil book:dune 'Frank Herbert'
./anvil put library.anvil book:neuromancer 'William Gibson'
./anvil get library.anvil book:1984
./anvil scan library.anvil book:
./anvil check library.anvil
```

Expect: `OK` on writes, the value on `get`, three sorted rows on `scan`,
`RESULT: PASS` on `check`.

### Take 3 — the crash test (~2 min of real time)

```bash
./anvil torture --cycles 100000
```

Expect: `acked writes lost 0`, `invariant violations 0`, all nine seams listed,
`RESULT: PASS`. **This takes around 110 seconds** — record it in full and speed
the middle up in the edit, or use `--cycles 20000` for a shorter take. Do not
cut away and back; a continuous shot is the point.

### Take 4 — the negative control (~10 s)

```bash
./anvil torture --negative-control
```

Expect: `RESULT: FAIL`, then the `EXPECTED FAILURE` explanation. It exits 0
because failing is the correct outcome here — say that out loud, because a
viewer seeing red needs to be told immediately that red is the pass condition.

### Optional — the full run (~4 min)

```bash
./scripts/verify.sh
```

Nine checks end to end. Too long to show whole; capture the final summary block.

---

## Editing notes

- If you intercut real captures, keep them in the same slot as the animated
  scene they replace so the narration still lines up.
- Do not speed up the negative control. It is the most important eight seconds
  in the video.
- No music under the crash test. The counter climbing in silence is stronger.
- Render individual scenes with
  `npx remotion render src/index.ts <scene-id> out/<id>.mp4` — scene ids are
  listed in `anvil-video/src/v2/Video.tsx`.
