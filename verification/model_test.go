package verification

import "testing"

// TestModelBased drives Anvil and an independent reference model through 40,000
// random operations and asserts their observable states always agree.
//
// The logic lives in RunModelCheck so that `go test` and `anvil verify` execute
// the same code path and cannot drift apart.
func TestModelBased(t *testing.T) {
	res := RunModelCheck(ModelConfig{Seed: 0xC0FFEE, Steps: 40000, Keyspace: 400})
	if !res.OK {
		t.Fatalf("model mismatch at step %d: %s", res.FailStep, res.FailDetail)
	}
	t.Logf("%d operations, %d keys, %d pages checked",
		res.Steps, res.Keys, res.CheckReport.PagesChecked)
}
