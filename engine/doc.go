/*
Package engine implements Anvil: a crash-consistent embedded transactional
key/value storage engine built entirely from the Go standard library.

Anvil stores data in a copy-on-write B+tree over a single file. A write never
modifies a committed page: it copies the path from root to leaf into fresh
pages and publishes a new root, so readers holding an older root keep a stable
snapshot. Two alternating metadata pages carry a generation number and a
checksum; recovery selects the newest generation that validates.

# Commit protocol

The ordering is the correctness mechanism:

	write new pages -> fdatasync -> write meta (alternate slot) -> fdatasync -> ACK

Data is durable before the metadata that references it, and the second sync is
the durable commit point: a caller is acknowledged only after it returns.
Before that point the new tree is unreferenced, and recovery simply selects the
previous generation. There is no write-ahead log — the metadata page is the
commit record, so recovery is generation selection rather than replay.

# Usage

	db, err := engine.Open("data.anvil")
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Put([]byte("name"), []byte("value")); err != nil {
		return err
	}

	tx, err := db.BeginWrite()
	if err != nil {
		return err
	}
	tx.Put([]byte("account:A"), []byte("900"))
	tx.Put([]byte("account:B"), []byte("100"))
	if err := tx.Commit(); err != nil { // atomic: all of it, or none
		return err
	}

# Guarantees

Anvil is crash-consistent under a documented storage and failure model. Every
acknowledged transaction recovers to a valid committed state, verified by
injecting crashes at each seam of the commit protocol against a backend where
only synchronized bytes survive. Real hardware power loss and storage firmware
that misreports flushes are outside what this evidence covers. See
VERIFICATION.md for the full claim/evidence table.

Version 1 is single-writer with many concurrent readers, allocates pages
grow-only (no space reclamation), and bounds keys to 1 KiB and key+value to
roughly 2 KiB.
*/
package engine
