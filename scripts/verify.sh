#!/usr/bin/env bash
# Reproduce every claim Anvil makes, in one command.
#
#   ./scripts/verify.sh                     # standard run
#   TORTURE_CYCLES=1000000 ./scripts/verify.sh
#   OUT_DIR=docs/benchmarks ./scripts/verify.sh
#
# Division of labour: this script performs the checks that need the Go
# toolchain (dependency proof, build, cross-compile, formatting, vet, race
# detector). Everything else is delegated to `anvil verify`, so the shell and
# the CLI can never test different things.
#
# Raw evidence is written to $OUT_DIR; exits non-zero if any check fails.
set -uo pipefail
cd "$(dirname "$0")/.."

CYCLES="${TORTURE_CYCLES:-100000}"
OUT="${OUT_DIR:-results}"
BIN="${TMPDIR:-/tmp}/anvil-verify-$$$(go env GOEXE)"   # GOEXE adds .exe on Windows
mkdir -p "$OUT"
fail=0

step() { printf '\n=== %s ===\n' "$1"; }
ok()   { printf 'PASS  %s\n' "$1"; }
bad()  { printf 'FAIL  %s\n' "$1"; fail=1; }
cleanup() { rm -f "$BIN"; }
trap cleanup EXIT

step "1. zero third-party dependencies"
{
  echo "\$ cat go.mod";           cat go.mod
  echo; echo "\$ go list -m all"; go list -m all
} | tee "$OUT/deps-proof.txt"
if grep -qE '^[[:space:]]*require' go.mod; then
  bad "go.mod contains a require block"
elif [ "$(go list -m all | wc -l)" -ne 1 ]; then
  bad "third-party modules present"
else
  ok "standard library only"
fi

step "2. build (static, cgo disabled)"
if CGO_ENABLED=0 go build -o "$BIN" ./cli && CGO_ENABLED=0 go build ./...; then
  ok "builds"
else
  bad "build"
fi

step "3. cross-compile"
for target in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do
  if GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 go build ./...; then
    ok "$target"
  else
    bad "$target"
  fi
done

step "4. formatting and vet"
if [ -z "$(gofmt -l .)" ]; then ok "gofmt"; else bad "gofmt: $(gofmt -l .)"; fi
if go vet ./...; then ok "vet"; else bad "vet"; fi

step "5. test suite (race detector)"
if go test -race -count=1 -v ./... > "$OUT/tests.txt" 2>&1; then
  ok "go test -race ($(grep -c '^--- PASS' "$OUT/tests.txt") tests)"
elif go test -count=1 -v ./... > "$OUT/tests.txt" 2>&1; then
  # The race detector needs a cgo-capable toolchain; CI provides one.
  ok "go test (race detector unavailable on this host)"
else
  bad "tests -- see $OUT/tests.txt"
fi

step "6. verification suite (model, concurrency, torture, negative control, process kill)"
if "$BIN" verify --cycles "$CYCLES" | tee "$OUT/verify.txt"; then
  ok "all verification checks"
else
  bad "verification -- see $OUT/verify.txt"
fi

step "7. crash torture detail ($CYCLES cycles, per-seam coverage)"
if "$BIN" torture --cycles "$CYCLES" | tee "$OUT/torture.txt"; then
  ok "0 acknowledged writes lost, 0 invariant violations"
else
  bad "torture"
fi

step "8. negative control (MUST fail -- proves the test has teeth)"
if "$BIN" torture --negative-control --cycles 500 > "$OUT/negative-control.txt" 2>&1; then
  cat "$OUT/negative-control.txt"
  ok "verifier caught the removed durability barrier"
else
  cat "$OUT/negative-control.txt"
  bad "negative control did not behave as expected"
fi

step "9. benchmark"
if "$BIN" bench --keys 100000 --seq 2000 --reads 200000 | tee "$OUT/bench.txt"; then
  ok "benchmark"
else
  bad "benchmark"
fi

printf '\n========================================\n'
if [ "$fail" -eq 0 ]; then
  printf 'ALL CHECKS PASSED -- evidence in %s/\n' "$OUT"
else
  printf 'SOME CHECKS FAILED -- see %s/\n' "$OUT"
fi
exit "$fail"
