# Zero Dependency 2026 — Product Research & Winner Recommendation

Research sprint for the Zero Dependency hackathon (Hackathon Raptors, Aug 28–31 2026).
Deliverable: candidate funnel → ranked ideas → deep-dives → feasibility audit → one winner.

---

## What the research changed about our strategy

- **The judge panel decides the product.** Lead-tier judges: Sergi Vladykin (Apple staff eng, committer on H2 Database and Apache Ignite — literally storage/SQL engine internals), Surabhi Boob (Azure hyperscaler infra), Dmitrii Kostriukov (SpaceX), Sergei Illarionov (senior data engineer), Oleksandr Moskalenko (builds low-dependency analytics and vector-search tools). This panel rewards storage engines, protocol correctness, and data tooling. It punishes CRUD.
- **Predecessor Raptors events** (Code Resurrection, Vanilla Web, Local-First) were won by app-level projects — facial-recog resurrection, snippet manager, chrome extension. Nobody in this event series' history has shipped a protocol-correct server or storage engine. The bar to stand out is lower than the tech-Twitter bar.
- **Judging is video + written forms**, multiple judges, independently. So the winning property is not "deep if you study it" — it's **objectively verifiable in 10 seconds of video**. That kills half the candidate list.
- **Rule nuances that matter:** you may not shell out to installed tools at runtime (so from-scratch `.git`/pcap parsing is a *feature*); crypto = compose stdlib primitives (DH + real KDF + encrypt-then-MAC is explicitly blessed); Rust/C's tiny stdlib earns extra respect; `node:sqlite` RC is explicitly legal.
- **Language matrix verdict:** Go 1.27 is the strongest overall (net/http, goroutines, crypto, testing/synctest, static binary, easy reproducible build). Node 24/26 second (fs.watch, node:sqlite, TUI via setRawMode). Python third (tomllib, sqlite3 — great for one class of tools). **Rust and Java are traps for networked systems at 72h** — no TLS/HTTP/JSON/async in Rust std, no JSON in JDK.

---

## 1. Top 20 (ranked)

| # | Name | What | Track | Lang |
|---|------|------|-------|------|
| 1 | **Cinder** | Redis-compatible server speaking real RESP, durable, replicating | D | Go |
| 2 | **Quern** | SQL query engine executing over CSV/JSON files, streaming | B/D | Go |
| 3 | **Lumber** | Local log search engine: parse, index, query, live tail | D | Go |
| 4 | **Chaperone** | Lockfile/supply-chain auditor (typosquat, scripts, advisories) | E | Python |
| 5 | **Crank** | Local CI: DAG task runner + content-addressed cache | A | Node 26 |
| 6 | **Fossil** | Git archaeology parsing packfiles w/o git binary | A | Go |
| 7 | **Quorum** | Raft-replicated KV store | C/D | Go |
| 8 | **Relay** | HTTP traffic lab: proxy + record/replay + fault injection | C | Go |
| 9 | **Resolv** | DNS server/resolver from the wire format up | C | Go |
| 10 | **Maelstrom** | BitTorrent client from scratch (bencode → peer-wire) | F | Go |
| 11 | **Foreman** | Process supervisor with health checks + log aggregation | C | Go |
| 12 | **Meridian** | Prometheus-compatible TSDB + query subset | D | Go |
| 13 | **Carrier** | NATS-like message broker with WAL | D | Go |
| 14 | **Ledger** | Event store with projections | D | Node |
| 15 | **Sentinel** | Policy engine (Rego-lite) over JSON inputs | A | Go |
| 16 | **Tongue** | jq-subset interpreter, streaming | B | Go |
| 17 | **Harpoon** | Linear-time regex engine (NFA, no backtracking) | B | Rust |
| 18 | **Cipherdrop** | age-style file encryption + TOTP vault | E | Node |
| 19 | **Rosetta** | Pcap analyzer (tshark-lite for capture files) | F | Go |
| 20 | **Marginalia** | BM25 local search over markdown notes | D/A | Node |

**DELETE bucket** (30+ killed): JSON/CSV/markdown parsers alone (toy), KV store without protocol interop (genre-saturated), single-pass secret scanner, SSG, paste bin, chat server, URL shortener, todo app, cron alone, template engine alone, CHIP-8, "AI-powered X", standalone load tester, standalone uptime monitor, structural diff alone, TOML/YAML parser alone (useless as a product), anything needing a running third-party service (banned).

---

## 2. Top 10 — architecture lines

1. **Cinder** — `TCP → RESP framer/parser → command dispatcher → sharded typed store (string/list/set/hash) → expiry wheel → AOF+RDB persistence → replication stream → synctest/bench harness`. The only idea where a judge can *prove* it works with a tool they already trust (real `redis-cli`).
2. **Quern** — `CLI → SQL lexer → recursive-descent parser → logical plan (predicate pushdown, projection pruning) → vectorized executor over streaming CSV/JSON (hash join/agg, spill sort) → formatters → honest bench`.
3. **Lumber** — `tail/ingest → line parser + format inference → segmented inverted index (time-partitioned, compressed) → query parser → search/aggregate/tail → CLI → retention`.
4. **Chaperone** — `lockfile parsers (npm/pnpm/requirements/poetry) → dep graph → checks (typosquat edit-distance, lifecycle scripts, license rollup, advisory DB) → sqlite cache → SARIF + terminal report`.
5. **Crank** — `crank.json → input hasher (node:crypto) → DAG topo sort → worker pool (child_process) → content-addressed cache (node:sqlite) → fs.watch invalidation → timing report`.
6. **Fossil** — `.git walker → zlib loose objects + packfile parser (ofs/ref delta resolution) → commit DAG + trees → churn × complexity hotspots, coupling, ownership → report`.
7. **Quorum** — `TCP mesh → framed RPC → Raft (election + log replication) → KV FSM → snapshots → fault-injection CLI → kill-the-leader demo`.
8. **Relay** — `proxy → recorder → rate-controlled replayer → fault injector (latency/loss/corruption rules) → diff report → net/http dashboard`.
9. **Resolv** — `UDP/TCP listeners → DNS wire codec (RFC 1035) → TTL cache → upstream forward (+DoH) → blocklist → verified against real dig`.
10. **Maelstrom** — `bencode decoder → tracker HTTP client → peer-wire protocol → piece picker → SHA-1 verification → file assembler`.

---

## 3. Top 5 — deep research + comparables

**Cinder** (1st): closest OSS = [tokio-rs/mini-redis](https://github.com/tokio-rs/mini-redis) (educational, "incomplete" by design, no persistence), the CodeCrafters redis-genre repos ([example](https://github.com/Intro0/redis-from-scratch-go)), microsoft/garnet and dragonflydb (proof that "protocol-compatible store" is a real product category). **Steal:** RESP2 grammar, client handshake quirks (INFO/CONFIG responses real clients probe), AOF-then-RDB persistence model. **Avoid:** full command surface, cluster mode. The genre is saturated at tutorial depth — our moat is product-grade interop + durability.

**Quern** (2nd): [duckdb/duckdb](https://github.com/duckdb/duckdb) (the giant), [dinedal/textql](https://github.com/dinedal/textql) (SQL-over-CSV precedent — cheats by loading into SQLite), csvkit, harelba/q, fselect. **Steal:** DuckDB's vectorized execution vocabulary, textql's zero-config UX. **Avoid:** full SQL grammar. **Kill list:** csvkit (millions of downloads/mo), textql, duckdb-CLI-for-quick-CSV.

**Lumber** (3rd): [tstack/lnav](https://github.com/tstack/lnav) (format detection, merge-by-time, TUI — the gold standard, huge C++), goaccess, angle-grinder, kibana (what we are not). **Steal:** lnav's auto-detection + merged timeline. **Avoid:** TUI depth in Go (no stdlib raw terminal — real gap).

**Chaperone** (4th): [google/osv-scanner](https://github.com/google/osv-scanner) (11+ ecosystems, 19+ lockfile types), Socket.dev, gitleaks. **Steal:** OSV batch API as optional refresh, SARIF output. **Thematic jackpot** — the event's own narrative is supply-chain fear — but craft-wow is lower than the top 3.

**Crank** (5th): Turborepo ([caching docs](https://turborepo.dev/docs/crafting-your-repository/caching): hash task inputs → restore outputs), nx, bazel. **Steal:** input-hash fingerprint → content-addressed restore. **Weakness:** no "impossible without deps" moment; the wow is behavioral.

---

## 4. Final 3 — complete cards

### Card 1 — Cinder (winner)

- **Pitch:** a Redis server your existing redis clients already work with — built entirely from the Go standard library.
- **User:** every backend dev who boots Docker just to give unit/integration tests a Redis; educators; air-gapped environments.
- **Problem:** Redis-in-Docker is the most-repeated dev-environment tax in the industry; ioredis-mock exists because running real Redis for tests hurts.
- **Why deps normally:** a real server means protocol libs, persistence libs, a container image.
- **Zero-dep angle:** `net` for sockets, `bufio` for the wire, hand-written RESP parser, `sync` sharded map store, `time`+heap expiry wheel, AOF append + custom RDB binary + `hash/crc32`, `testing` + `testing/synctest` for timing tests. Nothing missing — audited below.
- **Architecture:** TCP → RESP framer/parser → command dispatcher → sharded typed store (string/list/set/hash) → expiry wheel → AOF+RDB persistence → replication stream → synctest/bench harness. Five interacting subsystems: protocol, store, expiry, persistence, replication.
- **Killer feature:** `redis-benchmark` (the real tool) running against our server and printing numbers — plus `kill -9` mid-write, restart, AOF replay, zero data loss, on camera.
- **Demo (5 min):** `make` → `deps-proof` (go.mod empty) → real `redis-cli`: SET/GET/INCR/LPUSH/HSET/TTL → `redis-benchmark -q` → crash-recovery loop → pub/sub across two panes → (if shipped) replication: kill master, promote replica → STDLIB.md scroll: "12 packages we didn't install."
- **Technical depth:** wire-format correctness against three independent client implementations; crash-safe append logging with fsync policy trade-offs; O(1) expiry via wheel; lock-sharding to avoid a global mutex; RESP inline commands and error taxonomy so real clients don't choke.
- **Concurrency:** goroutine per connection, channel-per-subscriber pub/sub, sharded RWMutex store, `go test -race` in CI.
- **Persistence:** AOF (append + configurable fsync: always/everysec) + periodic binary snapshot with CRC; recovery = load snapshot, replay tail. Durability policy documented honestly (Track D explicitly rewards "where it cuts corners").
- **Failure handling:** malformed RESP → protocol error + close (spec-compliant); partial writes → AOF checksum + truncation on load with warning; port busy → clean exit code.
- **Language:** Go. Idiomatic goroutine-per-conn, single ~8MB static binary, `synctest` makes TTL tests deterministic, reproducible build is one flag.
- **Scope:**
  - MUST — RESP2 wire correctness with real clients; GET/SET/DEL/INCR/EXPIRE/TTL/EXISTS/TYPE/KEYS/LPUSH/LRANGE/LLEN/SADD/SMEMBERS/HSET/HGET/HGETALL/PING/FLUSHALL; concurrent clients; AOF recovery.
  - SHOULD — pub/sub, SCAN, RDB snapshots, INFO/CONFIG handshake answers, benchmark evidence.
  - IF TIME — replication-lite (AOF streaming to follower + promote), MULTI/EXEC, RESP3 HELLO.
- **72h feasibility:** yes, with margin — the most decomposable of the three (protocol → store → persistence are independent workstreams for 2–3 people).
- **Judge reaction:** a storage-engine judge (Vladykin, H2/Ignite) will check fsync policy and expiry correctness, and find them real. Interop is unfakeable.
- **Objections → answers:** *"Redis is free"* → so is our binary; the product is no-Docker test infra + a codebase you can read in an afternoon, which is the event's entire thesis. *"Tutorial genre"* → tutorials are in-memory, 8 commands, no clients; ours interoperates, survives kill -9, and replicates. *"Why Go?"* → idiomatic answer, not a fight.
- **Inspiration:** redis/redis (spec), tokio-rs/mini-redis (the floor we're exceeding), microsoft/garnet (category proof).

### Card 2 — Quern (runner-up)

- **Pitch:** SQL for your CSV and JSON files — a real query engine, streaming, zero installs.
- **User:** data engineers, analysts, devs with a folder of exports.
- **Architecture:** CLI → SQL lexer → recursive-descent parser → logical plan (predicate pushdown, projection pruning) → vectorized executor over streaming CSV/JSON (hash join/agg, spill sort) → formatters → honest bench.
- **Killer feature:** join + aggregate over a 2 GB CSV with constant memory, then show the same file make duckdb/csvkit choke on install friction.
- **Demo:** aggregations → hash join across CSV+JSON → column projection → error message with file:line:col on bad SQL → test suite proving the ugly cases (quoted newlines, BOMs, type inference).
- **Depth:** recursive-descent SQL parser, logical plan + pushdown, vectorized evaluation, external merge sort, spill-to-disk hash join.
- **Concurrency:** parallel scan/join fragments over worker pool. **Persistence:** none needed (document that honestly).
- **Failure handling:** malformed rows → error context, not a stack trace.
- **Language:** Go.
- **Scope:**
  - MUST — SELECT/WHERE/GROUP BY/aggregate/ORDER BY/LIMIT + equi-joins + type inference + CSV/JSON in, CSV/JSON/TSV out.
  - SHOULD — planner optimizations, glob multi-file FROM, bench vs csvkit/duckdb.
  - IF TIME — CTEs, HAVING edge cases, window-lite.
- **72h feasibility:** yes, but the parser/type-system is the scope swamp; the grammar must be frozen by hour 12.
- **Objections → answers:** *"duckdb exists"* → you can't ship it, can't install it everywhere, and it isn't auditable — plus ours proves the craft; *"types on CSV are a swamp"* → published coercion rules + casting functions, tested.
- **Inspiration:** duckdb/duckdb, dinedal/textql, csvkit.

### Card 3 — Lumber

- **Pitch:** a local log search engine: point it at files, query fields, tail live — mini-Splunk with an empty manifest.
- **User:** backend/SRE-ish devs drowning in app logs.
- **Architecture:** tail/ingest → line parser + format inference → segmented inverted index (time-partitioned, compressed) → query parser → search/aggregate/tail → CLI → retention.
- **Killer feature:** `status=5xx latency>100ms since 1h ago` answering instantly over 1 GB while `tail -f` streams new matches live.
- **Depth:** segmented inverted index, time partitioning, zstd segments, query planner over segments.
- **Objection to respect:** *"why not grep?"* — grep wins cold raw regex; we win field queries, time ranges, and live structured tail. Must be said in the README (honesty scores) and shown in the video.
- **Language:** Go, CLI-first — **TUI is a stdlib gap in Go** (no raw terminal; x/term is external). If TUI is core, switch to Node (setRawMode) or cut the TUI.
- **72h feasibility:** riskiest of the three — index design can eat the weekend.
- **Inspiration:** tstack/lnav, goaccess.

---

## 5. Zero-dependency feasibility audit (final 3)

| subsystem | normal package | stdlib replacement | avail? | difficulty |
|---|---|---|---|---|
| **Cinder** TCP server | — | `net` | ✅ | low |
| RESP wire parser | go-redis/resp libs | `bufio` + hand parser | ✅ | med |
| typed KV store | redis | sharded `sync` maps | ✅ | low-med |
| TTL expiry | — | `time` + min-heap wheel | ✅ | med |
| persistence | goleveldb/ledis | AOF append+fsync, custom RDB + `hash/crc32` | ✅ | med-high |
| pub/sub | — | per-conn channels | ✅ | low-med |
| replication | — | AOF stream over TCP | ✅ | high (if-time) |
| tests | testify | `testing` + `testing/synctest` | ✅ | low |
| **Quern** CSV reader | csvkit | `encoding/csv` streaming | ✅ | low |
| SQL parser | participle/yacc | hand recursive descent | ✅ | med-high |
| planner/executor | (duckdb) | hand vectorized + hash join/agg + spill sort | ✅ | high |
| type coercion | cast | `strconv`/`time` hand | ✅ | med |
| JSON in | — | `encoding/json` decoder | ✅ | med |
| table output | tablewriter | ANSI by hand | ✅ | low |
| **Lumber** tail/ingest | hpcloud/tail | polling + `bufio` | ✅ | med |
| inverted index | bleve | hand segmented index | ✅ | high |
| query language | — | hand parser | ✅ | med |
| compression | — | `compress/gzip` + zstd | ✅ | low |
| TUI | tview/bubbletea | **no Go stdlib equivalent — cut it or switch to Node** | ⚠️ | high |

No blocked subsystems in the top two. Lumber's TUI is the only "no stdlib equivalent" flag — it must be cut (CLI-first) or the language switched.

---

## 6. Package-killer opportunities (feed STDLIB.md + bonuses)

- **Cinder kills:** Docker-for-Redis in dev/test, ioredis-mock — and its STDLIB.md documents killing commander→`flag`, testify→`testing`+`synctest`, zap→`log/slog`, uuid→`crypto/rand`, leveldb→hand AOF/RDB. 10+ real substitutions, trivially.
- **Quern kills:** csvkit, textql, duckdb-CLI-for-CSV — plus chalk→ANSI, glob→`filepath.Glob`, axios→`net/http`.
- **Both take +5 reproducible build** (Go: `CGO_ENABLED=0 -trimpath -buildvcs=false`, publish two hashes) and **+3 STDLIB log** without damaging the core. Skip single-file (+5): a 5k-line one-file query engine is worse software, and judges read what you ship.

---

## 7. Winner

**If our only objective is maximizing the probability of winning this hackathon, build Cinder.**

Five reasons, no hedging:

1. It's the only candidate whose core claim is *objectively verifiable in seconds of video* by judges who weren't in the room — real `redis-cli`, real `redis-benchmark`, real client libraries, interop with tools they already trust.
2. The demo has a physical wow that survives compression to 5 minutes: `kill -9`, replay, benchmark numbers, replication.
3. The craft story is the event's own thesis — the infra you rent from a container image, readable in an afternoon, zero supply chain.
4. It decomposes cleanly across 2–4 people with a comfort margin, so we also bank +5 reproducible build and +3 STDLIB log instead of bleeding hours into a swamp.
5. It's aimed straight at the panel — a database-internals judge will audit the fsync policy and expiry wheel, and will find them real.

Quern is the standing switch if the final team is data-engineering-heavy; Lumber is third because of the Go TUI gap and index-design risk.

---

## Sources

- [Zero Dependency — official site, rules, judges, scoring](https://zerodepshack.com/)
- [Hackathon Raptors](https://www.raptors.dev/) · [Code Resurrection results (UK Tech News)](https://uktechnews.co.uk/2025/10/06/hackathon-raptors-72-hour-challenge-validates-measurable-approaches-to-reviving-abandoned-codebases/)
- [tokio-rs/mini-redis](https://github.com/tokio-rs/mini-redis) · [Intro0/redis-from-scratch-go (CodeCrafters genre)](https://github.com/Intro0/redis-from-scratch-go)
- [dinedal/textql](https://github.com/dinedal/textql) · [tstack/lnav](https://github.com/tstack/lnav) · [google/osv-scanner](https://github.com/google/osv-scanner) · [Turborepo caching docs](https://turborepo.dev/docs/crafting-your-repository/caching)
