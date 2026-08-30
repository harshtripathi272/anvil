package anvil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRealProcessKillRecovery is the SECONDARY, real-syscall validation
// (SRS v1.1 §C1, §45): a child process commits to a real file and is killed
// abruptly; the parent reopens the file and requires a clean, valid recovery.
//
// This exercises the true file/fdatasync path end to end. Its claim is
// crash-CONSISTENCY only: a process kill does not discard page-cache writes, so
// durability itself is proven by the deterministic fault backend, not here.
func TestRealProcessKillRecovery(t *testing.T) {
	// Child mode: write forever until killed.
	if path := os.Getenv("ANVIL_CRASHWRITER"); path != "" {
		runCrashWriter(path)
		return
	}
	if testing.Short() {
		t.Skip("skipping real-process-kill test in -short mode")
	}

	path := filepath.Join(t.TempDir(), "kill.anvil")
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate test binary: %v", err)
	}

	cmd := exec.Command(exe, "-test.run=TestRealProcessKillRecovery")
	cmd.Env = append(os.Environ(), "ANVIL_CRASHWRITER="+path)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start child: %v", err)
	}

	// Let it commit for a while, then kill it mid-stream.
	time.Sleep(200 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	// Reopen from the real file and require a valid, uncorrupted recovery.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("recovery after kill failed: %v", err)
	}
	defer db.Close()
	rep, err := db.Check()
	if err != nil {
		t.Fatalf("structural corruption after kill: %v", err)
	}
	gen, _, _ := db.Info()
	if gen < 2 {
		t.Fatalf("expected committed writes to survive, generation=%d", gen)
	}

	// Whatever generation survived, its keys must be internally consistent:
	// counter N implies keys 0..N-1 are all present (each was committed before
	// counter advanced).
	v, err := db.Get([]byte("counter"))
	if err != nil {
		t.Fatalf("counter missing after recovery: %v", err)
	}
	var n int
	fmt.Sscanf(string(v), "%d", &n)
	for i := 0; i < n; i++ {
		if _, err := db.Get([]byte(fmt.Sprintf("k%08d", i))); err != nil {
			t.Fatalf("acknowledged key k%08d lost after kill: %v", i, err)
		}
	}
	t.Logf("survived kill: generation=%d, %d acknowledged keys intact, %d pages checked",
		gen, n, rep.PagesChecked)
}

// runCrashWriter commits keys k00000000, k00000001, ... to a real file, bumping
// "counter" in the same transaction, until the process is killed.
func runCrashWriter(path string) {
	db, err := Open(path)
	if err != nil {
		os.Exit(3)
	}
	for i := 0; ; i++ {
		tx, err := db.BeginWrite()
		if err != nil {
			os.Exit(4)
		}
		tx.Put([]byte(fmt.Sprintf("k%08d", i)), []byte("v"))
		tx.Put([]byte("counter"), []byte(fmt.Sprintf("%d", i+1)))
		if err := tx.Commit(); err != nil {
			os.Exit(5)
		}
	}
}
