package anvil

import (
	"errors"
	"sync"
)

// DB is an open Anvil database: a single-writer, many-reader embedded
// transactional key/value store backed by a copy-on-write B+tree.
//
// Anvil is WAL-free shadow paging (SRS v1.1 §C3): the metadata page is the
// commit record, and recovery is O(1) selection of the newest valid generation
// — there is no redo log to replay.
type DB struct {
	mu  sync.RWMutex // guards committed + closed + failed
	wmu sync.Mutex   // single-writer lock, held for a write transaction's life

	storage   Storage
	committed meta
	closed    bool
	failed    error // set on a fatal durability failure; makes the DB unusable
	readOnly  bool

	// checkpoint is a test/torture-only hook invoked at each commit seam. A
	// non-nil error aborts the commit there, modeling a crash at that seam
	// (SRS v1.1 §C1, §43). nil in production.
	checkpoint func(seam string) error

	// skipDataSync removes the pre-meta data durability barrier. It exists
	// solely for the negative control that proves the harness has teeth
	// (SRS v1.1 §C1). Never set in production.
	skipDataSync bool
}

type config struct {
	readOnly bool
}

// Option configures Open.
type Option func(*config)

// WithReadOnly opens the database without the ability to write.
func WithReadOnly() Option {
	return func(c *config) { c.readOnly = true }
}

// Open opens (creating if absent) the database at path.
func Open(path string, opts ...Option) (*DB, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	fs, created, err := openFileStorage(path)
	if err != nil {
		return nil, err
	}
	db, err := openOn(fs, created, cfg)
	if err != nil {
		fs.Close()
		return nil, err
	}
	return db, nil
}

// openOn builds a DB over any Storage (used by Open and by the test/torture
// harnesses with a FaultStorage).
func openOn(s Storage, created bool, cfg config) (*DB, error) {
	db := &DB{storage: s, readOnly: cfg.readOnly}
	if created {
		if cfg.readOnly {
			return nil, ErrReadOnly
		}
		if err := db.initialize(); err != nil {
			return nil, err
		}
		return db, nil
	}
	if err := db.recover(); err != nil {
		return nil, err
	}
	return db, nil
}

// initialize lays down an empty database: an empty root leaf plus the first
// committed metadata generation.
func (db *DB) initialize() error {
	root := &node{id: firstPage, leaf: true, txnID: 1}
	if err := db.storage.WritePage(root.id, root.encode()); err != nil {
		return err
	}
	if err := db.storage.Sync(); err != nil {
		return db.fail(err)
	}
	m := meta{txnID: 1, rootID: firstPage, pageCount: firstPage + 1}
	slot := m.txnID % 2
	if err := db.storage.WritePage(slot, m.encode(slot)); err != nil {
		return err
	}
	if err := db.storage.Sync(); err != nil {
		return db.fail(err)
	}
	db.committed = m
	return nil
}

// recover selects the newest valid committed metadata generation and validates
// its root. If neither slot is trustworthy it fails loudly (SRS v1.1 §I9, §32).
func (db *DB) recover() error {
	a, errA := db.readMetaSlot(metaSlotA)
	b, errB := db.readMetaSlot(metaSlotB)

	var best *meta
	if errA == nil {
		best = a
	}
	if errB == nil && (best == nil || b.txnID > best.txnID) {
		best = b
	}
	if best == nil {
		return ErrCorrupt
	}
	if best.rootID < firstPage || best.rootID >= best.pageCount {
		return ErrCorrupt
	}
	if _, err := db.readNodeFromStorage(best.rootID); err != nil {
		return ErrCorrupt
	}
	db.committed = *best
	return nil
}

func (db *DB) readMetaSlot(slot uint64) (*meta, error) {
	buf := make([]byte, PageSize)
	if err := db.storage.ReadPage(slot, buf); err != nil {
		return nil, err
	}
	return decodeMeta(buf)
}

// readNodeFromStorage reads and decodes a tree page. Because v1 is grow-only,
// committed pages are immutable, so a snapshot root yields a stable view (I7).
func (db *DB) readNodeFromStorage(id uint64) (*node, error) {
	buf := make([]byte, PageSize)
	if err := db.storage.ReadPage(id, buf); err != nil {
		return nil, err
	}
	return decodeNode(id, buf)
}

// fail records a fatal durability failure. Once failed, the DB refuses further
// work rather than pretend an operation succeeded (SRS v1.1 §29, §94).
func (db *DB) fail(err error) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.failed == nil {
		db.failed = errors.Join(ErrIO, err)
	}
	return db.failed
}

func (db *DB) hook(seam string) error {
	if db.checkpoint == nil {
		return nil
	}
	return db.checkpoint(seam)
}

// Close releases the database. A clean close is not required for crash
// correctness; the database is recoverable after abrupt process death.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil
	}
	db.closed = true
	return db.storage.Close()
}

// --- convenience auto-commit helpers ---

// Get returns a copy of the value for key, or ErrNotFound.
func (db *DB) Get(key []byte) ([]byte, error) {
	tx, err := db.BeginRead()
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	return tx.Get(key)
}

// Put sets key=val in its own transaction.
func (db *DB) Put(key, val []byte) error {
	tx, err := db.BeginWrite()
	if err != nil {
		return err
	}
	if err := tx.Put(key, val); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Delete removes key in its own transaction (removing a missing key is a no-op).
func (db *DB) Delete(key []byte) error {
	tx, err := db.BeginWrite()
	if err != nil {
		return err
	}
	if err := tx.Delete(key); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Info reports the current committed generation, root, and page count.
func (db *DB) Info() (txnID, rootID, pageCount uint64) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.committed.txnID, db.committed.rootID, db.committed.pageCount
}
