# Evidence archive

Raw output from the last full verification run. Regenerate with:

```bash
OUT_DIR=docs/benchmarks ./scripts/verify.sh
```

| File | What it is |
|---|---|
| `deps-proof.txt` | `go.mod` and `go list -m all` — proof of an empty dependency manifest |
| `tests.txt` | Full `go test -v ./...` output (19 tests) |
| `torture.txt` | 100,000-cycle crash torture, with per-seam coverage |
| `negative-control.txt` | The calibration run: a required durability barrier removed, harness correctly FAILS |
| `bench.txt` | Write, read, scan, recovery, and structural-check timings |

Run environment: Go 1.27.0, windows/amd64. The race detector needs a
cgo-capable toolchain and runs in CI on ubuntu-latest.

Interpretation of every number lives in [VERIFICATION.md](../../VERIFICATION.md).
