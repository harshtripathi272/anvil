package anvil

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestLeafRoundTrip(t *testing.T) {
	n := &node{
		id:    7,
		txnID: 42,
		leaf:  true,
		keys:  [][]byte{[]byte("a"), []byte("bb"), {0x00, 0xff}},
		vals:  [][]byte{[]byte("1"), []byte(""), {0x00}},
	}
	p := n.encode()
	if len(p) != PageSize {
		t.Fatalf("page size = %d", len(p))
	}
	got, err := decodeNode(7, p)
	if err != nil {
		t.Fatal(err)
	}
	if !got.leaf || got.id != 7 || got.txnID != 42 {
		t.Fatalf("header mismatch: %+v", got)
	}
	for i := range n.keys {
		if !bytes.Equal(got.keys[i], n.keys[i]) || !bytes.Equal(got.vals[i], n.vals[i]) {
			t.Fatalf("record %d mismatch", i)
		}
	}
}

func TestInternalRoundTrip(t *testing.T) {
	n := &node{
		id:       3,
		leaf:     false,
		keys:     [][]byte{[]byte("m"), []byte("t")},
		children: []uint64{10, 20, 30},
	}
	p := n.encode()
	got, err := decodeNode(3, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.leaf || len(got.children) != 3 || len(got.keys) != 2 {
		t.Fatalf("bad decode: %+v", got)
	}
	if got.children[0] != 10 || got.children[2] != 30 {
		t.Fatalf("children mismatch: %v", got.children)
	}
}

func TestChecksumDetectsCorruption(t *testing.T) {
	n := &node{id: 1, leaf: true, keys: [][]byte{[]byte("k")}, vals: [][]byte{[]byte("v")}}
	p := n.encode()
	p[headerSize+2] ^= 0x40 // flip a payload byte
	if _, err := decodeNode(1, p); err != ErrChecksum {
		t.Fatalf("expected ErrChecksum, got %v", err)
	}
}

func TestMetaRoundTripAndCorruption(t *testing.T) {
	m := &meta{txnID: 99, rootID: 5, pageCount: 12}
	p := m.encode(metaSlotB)
	if binary.LittleEndian.Uint64(p[offPageID:]) != metaSlotB {
		t.Fatal("slot not recorded")
	}
	got, err := decodeMeta(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.txnID != 99 || got.rootID != 5 || got.pageCount != 12 {
		t.Fatalf("meta mismatch: %+v", got)
	}
	p[offChecksum+4] ^= 0x01 // corrupt first payload byte of meta
	if _, err := decodeMeta(p); err != ErrChecksum {
		t.Fatalf("expected ErrChecksum, got %v", err)
	}
}

func TestKeyValueBounds(t *testing.T) {
	if err := checkKeyValue(nil, []byte("x")); err != ErrInvalidArgument {
		t.Fatal("empty key should be rejected")
	}
	if err := checkKeyValue(bytes.Repeat([]byte("k"), MaxKeySize+1), nil); err != ErrInvalidArgument {
		t.Fatal("oversized key should be rejected")
	}
	if err := checkKeyValue([]byte("k"), bytes.Repeat([]byte("v"), maxRecord)); err != ErrInvalidArgument {
		t.Fatal("oversized value should be rejected")
	}
	if err := checkKeyValue([]byte("k"), []byte("v")); err != nil {
		t.Fatalf("valid kv rejected: %v", err)
	}
}
