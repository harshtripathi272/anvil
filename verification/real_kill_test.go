package verification

import (
	"os"
	"os/exec"
	"testing"
)

// TestRealProcessKillRecovery is the secondary, real-syscall validation
// (SRS v1.1 §C1, §45): a child process commits to a real file and is killed
// abruptly; the parent reopens the file and requires a clean recovery.
//
// It exercises the true file and fdatasync path end to end. Its claim is
// crash-CONSISTENCY only: a process kill does not discard page-cache writes, so
// durability itself is proven by the fault-injecting backend, not here.
//
// The test binary re-executes itself as the crash writer, selecting that mode
// through an environment variable rather than the CLI's subcommand.
func TestRealProcessKillRecovery(t *testing.T) {
	// Child mode: commit until killed.
	if path := os.Getenv("ANVIL_CRASHWRITER"); path != "" {
		if err := RunCrashWriter(path); err != nil {
			os.Exit(3)
		}
		return
	}
	if testing.Short() {
		t.Skip("skipping real-process-kill test in -short mode")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate test binary: %v", err)
	}

	ok, detail := RunProcessKillCheck(func(dbPath string) *exec.Cmd {
		cmd := exec.Command(exe, "-test.run=TestRealProcessKillRecovery")
		cmd.Env = append(os.Environ(), "ANVIL_CRASHWRITER="+dbPath)
		return cmd
	})
	if !ok {
		t.Fatal(detail)
	}
	t.Log(detail)
}
