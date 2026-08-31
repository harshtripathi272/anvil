# Evidence archive

Raw output from the last full verification run. Regenerate with:

```bash
OUT_DIR=docs/benchmarks ./scripts/verify.sh
```

| File | What it is |
|---|---|
| `deps-proof.txt` | `go.mod` and `go list -m all` — proof of an empty dependency manifest |
| `tests.txt` | Full `go test -count=1 -v ./...` output (22 tests) |
| `verify.txt` | `anvil verify`: model equivalence, concurrency, torture, negative control, process kill |
| `torture.txt` | 100,000-cycle crash torture with per-seam coverage |
| `negative-control.txt` | The calibration run: a required durability barrier removed, harness correctly FAILS |
| `bench.txt` | Write, read, scan, recovery, and structural-check timings |

`linux/` holds the same evidence set from a linux/amd64 run, where the race
detector is available (`go test -race`, 22 tests) and the durability barrier is
the real `syscall.Fdatasync`. The files at this level are from windows/amd64,
where the barrier is `FlushFileBuffers`. Both runs pass all nine checks; the
Linux benchmark is the trustworthy one for commit latency.

Run environment: Go 1.27.0.

Interpretation of every number lives in [VERIFICATION.md](../../VERIFICATION.md).
