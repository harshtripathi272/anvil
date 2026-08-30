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

Run environment: Go 1.27.0, windows/amd64. The race detector needs a
cgo-capable toolchain and runs in CI on ubuntu-latest, which also exercises the
Linux `fdatasync` path.

Interpretation of every number lives in [VERIFICATION.md](../../VERIFICATION.md).
