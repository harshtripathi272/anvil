package anvil

import (
	"bytes"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
)

// openFault opens a DB over a FaultStorage, initializing if empty.
func openFault(t *testing.T, s *FaultStorage) *DB {
	t.Helper()
	np, _ := s.NumPages()
	db, err := openOn(s, np == 0, config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestBasicPutGetDelete(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	if err := db.Put([]byte("name"), []byte("lakshya")); err != nil {
		t.Fatal(err)
	}
	if err := db.Put([]byte("branch"), []byte("cse")); err != nil {
		t.Fatal(err)
	}
	v, err := db.Get([]byte("name"))
	if err != nil || string(v) != "lakshya" {
		t.Fatalf("get name = %q, %v", v, err)
	}
	// overwrite
	if err := db.Put([]byte("name"), []byte("harsh")); err != nil {
		t.Fatal(err)
	}
	v, _ = db.Get([]byte("name"))
	if string(v) != "harsh" {
		t.Fatalf("overwrite failed: %q", v)
	}
	// delete
	if err := db.Delete([]byte("name")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get([]byte("name")); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	// missing
	if _, err := db.Get([]byte("nope")); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBinarySafeKeysAndValues(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	key := []byte{0x00, 0xff, 0x00, 0x10}
	val := []byte{0x00, 0x00, 0x00}
	if err := db.Put(key, val); err != nil {
		t.Fatal(err)
	}
	got, err := db.Get(key)
	if err != nil || !bytes.Equal(got, val) {
		t.Fatalf("binary kv mismatch: %v %v", got, err)
	}
	// empty value is legal
	if err := db.Put([]byte("empty"), []byte{}); err != nil {
		t.Fatal(err)
	}
	got, err = db.Get([]byte("empty"))
	if err != nil || len(got) != 0 {
		t.Fatalf("empty value mismatch: %v %v", got, err)
	}
}

func TestManyKeysSplitsAndScan(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	const N = 6000
	rng := rand.New(rand.NewSource(1))
	model := map[string]string{}

	// Insert N randomized keys with varied sizes to force many splits.
	for i := 0; i < N; i++ {
		k := fmt.Sprintf("key:%08d:%d", rng.Intn(N*2), i)
		v := bytes.Repeat([]byte{byte('a' + i%26)}, rng.Intn(200))
		if err := db.Put([]byte(k), v); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
		model[k] = string(v)
	}
	txid, _, pages := db.Info()
	t.Logf("after %d puts: gen=%d pages=%d", N, txid, pages)

	// Every key must read back.
	for k, v := range model {
		got, err := db.Get([]byte(k))
		if err != nil || string(got) != v {
			t.Fatalf("get %q = %q,%v want %q", k, got, err, v)
		}
	}

	// Full scan must be sorted and complete.
	want := make([]string, 0, len(model))
	for k := range model {
		want = append(want, k)
	}
	sort.Strings(want)

	tx, _ := db.BeginRead()
	it := tx.Scan(nil, nil)
	var got []string
	for it.Next() {
		got = append(got, string(it.Key()))
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	tx.Close()
	if len(got) != len(want) {
		t.Fatalf("scan count = %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scan order mismatch at %d: %q vs %q", i, got[i], want[i])
		}
	}

	// Range scan [lo, hi).
	lo, hi := "key:00030000", "key:00060000"
	var wantR []string
	for _, k := range want {
		if k >= lo && k < hi {
			wantR = append(wantR, k)
		}
	}
	tx, _ = db.BeginRead()
	it = tx.Scan([]byte(lo), []byte(hi))
	var gotR []string
	for it.Next() {
		gotR = append(gotR, string(it.Key()))
	}
	tx.Close()
	if len(gotR) != len(wantR) {
		t.Fatalf("range scan count = %d want %d", len(gotR), len(wantR))
	}
	for i := range wantR {
		if gotR[i] != wantR[i] {
			t.Fatalf("range mismatch at %d", i)
		}
	}
}

func TestDeleteHeavyStaysConsistent(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	const N = 3000
	for i := 0; i < N; i++ {
		db.Put([]byte(fmt.Sprintf("k%06d", i)), []byte("v"))
	}
	// delete every other key
	for i := 0; i < N; i += 2 {
		if err := db.Delete([]byte(fmt.Sprintf("k%06d", i))); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < N; i++ {
		_, err := db.Get([]byte(fmt.Sprintf("k%06d", i)))
		if i%2 == 0 && err != ErrNotFound {
			t.Fatalf("k%06d should be deleted, got %v", i, err)
		}
		if i%2 == 1 && err != nil {
			t.Fatalf("k%06d should exist, got %v", i, err)
		}
	}
}

func TestReopenFileBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.anvil")

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		if err := db.Put([]byte(fmt.Sprintf("k%04d", i)), []byte(fmt.Sprintf("v%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	// Reopen and verify.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	for i := 0; i < 500; i++ {
		v, err := db2.Get([]byte(fmt.Sprintf("k%04d", i)))
		if err != nil || string(v) != fmt.Sprintf("v%d", i) {
			t.Fatalf("after reopen k%04d = %q,%v", i, v, err)
		}
	}
	if err := verifyStructure(db2); err != nil {
		t.Fatalf("structure check: %v", err)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	db.Put([]byte("x"), []byte("1"))

	// Reader captures the snapshot at x=1.
	rtx, _ := db.BeginRead()

	// Writer advances to x=2 (and adds 1000 more keys).
	db.Put([]byte("x"), []byte("2"))
	for i := 0; i < 1000; i++ {
		db.Put([]byte(fmt.Sprintf("extra%04d", i)), []byte("y"))
	}

	// Old reader must still see x=1 and none of the new keys.
	v, err := rtx.Get([]byte("x"))
	if err != nil || string(v) != "1" {
		t.Fatalf("snapshot leaked: x=%q,%v (want 1)", v, err)
	}
	if _, err := rtx.Get([]byte("extra0001")); err != ErrNotFound {
		t.Fatalf("snapshot saw future key: %v", err)
	}
	rtx.Close()

	// A fresh reader sees the new world.
	v, _ = db.Get([]byte("x"))
	if string(v) != "2" {
		t.Fatalf("fresh read x=%q want 2", v)
	}
}

func TestAtomicMultiKeyTransaction(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	db.Put([]byte("account:A"), []byte("1000"))
	db.Put([]byte("account:B"), []byte("0"))

	tx, _ := db.BeginWrite()
	tx.Put([]byte("account:A"), []byte("900"))
	tx.Put([]byte("account:B"), []byte("100"))
	tx.Put([]byte("pending:7"), []byte("transfer"))
	tx.Delete([]byte("pending:7"))
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	a, _ := db.Get([]byte("account:A"))
	b, _ := db.Get([]byte("account:B"))
	if string(a) != "900" || string(b) != "100" {
		t.Fatalf("multi-key commit wrong: A=%q B=%q", a, b)
	}
	if _, err := db.Get([]byte("pending:7")); err != ErrNotFound {
		t.Fatalf("pending:7 should be gone: %v", err)
	}
}

func TestRollbackDiscards(t *testing.T) {
	db := openFault(t, NewFaultStorage())
	db.Put([]byte("k"), []byte("committed"))
	tx, _ := db.BeginWrite()
	tx.Put([]byte("k"), []byte("uncommitted"))
	tx.Put([]byte("k2"), []byte("uncommitted"))
	tx.Rollback()
	v, _ := db.Get([]byte("k"))
	if string(v) != "committed" {
		t.Fatalf("rollback failed: k=%q", v)
	}
	if _, err := db.Get([]byte("k2")); err != ErrNotFound {
		t.Fatalf("rollback leaked k2: %v", err)
	}
}
