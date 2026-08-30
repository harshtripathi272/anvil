package anvil

// ReadTx is a read-only snapshot transaction. It observes the committed root
// captured at BeginRead and never sees a later or partial commit (SRS §22, §I8).
type ReadTx struct {
	db   *DB
	root uint64
	txID uint64
	done bool
}

// BeginRead starts a snapshot read transaction.
func (db *DB) BeginRead() (*ReadTx, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, ErrClosed
	}
	if db.failed != nil {
		return nil, db.failed
	}
	return &ReadTx{db: db, root: db.committed.rootID, txID: db.committed.txnID}, nil
}

func (tx *ReadTx) node(id uint64) (*node, error) { return tx.db.readNodeFromStorage(id) }

// Get returns a copy of the value for key, or ErrNotFound.
func (tx *ReadTx) Get(key []byte) ([]byte, error) {
	if tx.done {
		return nil, ErrTxnClosed
	}
	return btreeGet(tx, tx.root, key)
}

// Scan returns an iterator over [start, end); nil start means from the first
// key, nil end means through the last key.
func (tx *ReadTx) Scan(start, end []byte) *Iterator {
	return &Iterator{c: newCursor(tx, tx.root, start), end: cloneOrNil(end)}
}

// TxID returns the committed generation this snapshot observes.
func (tx *ReadTx) TxID() uint64 { return tx.txID }

// Close finishes the read transaction.
func (tx *ReadTx) Close() error { tx.done = true; return nil }

// WriteTx is a single-writer transaction. It buffers new copy-on-write pages in
// memory and publishes them atomically at Commit. It also implements pager.
type WriteTx struct {
	db            *DB
	root          uint64
	baseTxID      uint64
	basePageCount uint64           // committed high-water at BeginWrite
	nextID        uint64           // provisional id counter during the transaction
	dirty         map[uint64]*node // staged new pages, by provisional id
	done          bool
	err           error
}

// BeginWrite starts the (single) write transaction, blocking until any prior
// writer finishes.
func (db *DB) BeginWrite() (*WriteTx, error) {
	if db.readOnly {
		return nil, ErrReadOnly
	}
	db.wmu.Lock()
	db.mu.RLock()
	closed, failed := db.closed, db.failed
	m := db.committed
	db.mu.RUnlock()
	if closed {
		db.wmu.Unlock()
		return nil, ErrClosed
	}
	if failed != nil {
		db.wmu.Unlock()
		return nil, failed
	}
	return &WriteTx{
		db:            db,
		root:          m.rootID,
		baseTxID:      m.txnID,
		basePageCount: m.pageCount,
		nextID:        m.pageCount,
		dirty:         make(map[uint64]*node),
	}, nil
}

// pager implementation.
func (tx *WriteTx) node(id uint64) (*node, error) {
	if n, ok := tx.dirty[id]; ok {
		return n, nil
	}
	return tx.db.readNodeFromStorage(id)
}

func (tx *WriteTx) alloc() uint64 {
	id := tx.nextID
	tx.nextID++
	return id
}

func (tx *WriteTx) stage(n *node) {
	n.txnID = tx.baseTxID + 1
	tx.dirty[n.id] = n
}

// reachableDirty returns the staged pages reachable from the final root, in a
// parent-before-child order. Superseded intermediate copies (staged earlier in
// the transaction but no longer referenced) are skipped, so a commit writes only
// the new pages of the final tree — never orphaned COW garbage.
func (tx *WriteTx) reachableDirty() []uint64 {
	var out []uint64
	seen := make(map[uint64]bool)
	var visit func(id uint64)
	visit = func(id uint64) {
		n, ok := tx.dirty[id]
		if !ok || seen[id] { // committed (already durable) or already collected
			return
		}
		seen[id] = true
		out = append(out, id)
		if !n.leaf {
			for _, c := range n.children {
				visit(c)
			}
		}
	}
	visit(tx.root)
	return out
}

// encodeRemapped encodes a staged page under its compact id, rewriting internal
// child pointers from provisional ids to their compact ids (committed children,
// absent from remap, keep their existing ids).
func (tx *WriteTx) encodeRemapped(oldID uint64, remap map[uint64]uint64) []byte {
	n := tx.dirty[oldID]
	out := &node{id: remap[oldID], txnID: n.txnID, leaf: n.leaf, keys: n.keys, vals: n.vals}
	if !n.leaf {
		out.children = make([]uint64, len(n.children))
		for i, c := range n.children {
			if nc, ok := remap[c]; ok {
				out.children[i] = nc
			} else {
				out.children[i] = c
			}
		}
	}
	return out.encode()
}

// Get reads within the transaction's working tree (sees its own pending writes).
func (tx *WriteTx) Get(key []byte) ([]byte, error) {
	if tx.done {
		return nil, ErrTxnClosed
	}
	return btreeGet(tx, tx.root, key)
}

// Put inserts or replaces key=val. Inputs are copied, so callers may reuse them.
func (tx *WriteTx) Put(key, val []byte) error {
	if tx.done {
		return ErrTxnClosed
	}
	if err := checkKeyValue(key, val); err != nil {
		return err
	}
	newRoot, err := btreePut(tx, tx.root, clonebytes(key), clonebytes(val))
	if err != nil {
		tx.err = err
		return err
	}
	tx.root = newRoot
	return nil
}

// Delete removes key if present (no-op if absent).
func (tx *WriteTx) Delete(key []byte) error {
	if tx.done {
		return ErrTxnClosed
	}
	newRoot, _, err := btreeDelete(tx, tx.root, key)
	if err != nil {
		tx.err = err
		return err
	}
	tx.root = newRoot
	return nil
}

// Scan returns an iterator over the transaction's working tree.
func (tx *WriteTx) Scan(start, end []byte) *Iterator {
	return &Iterator{c: newCursor(tx, tx.root, start), end: cloneOrNil(end)}
}

// Commit publishes all staged pages atomically. The ordering is the core
// correctness mechanism (SRS v1.1 §25, §26, §I2–§I4):
//
//	write data pages -> fdatasync -> write meta (alternate slot) -> fdatasync -> ACK
//
// The second sync is the durable commit point; the caller is acknowledged only
// after it returns. Before it, the new tree is just orphaned pages.
func (tx *WriteTx) Commit() error {
	if tx.done {
		return ErrTxnClosed
	}
	defer tx.finish()
	if tx.err != nil {
		return tx.err
	}
	db := tx.db
	if db.failed != nil {
		return db.failed
	}

	newTxID := tx.baseTxID + 1

	// Renumber the reachable new pages into a compact, contiguous range starting
	// at the committed high-water. Provisional allocation leaves gaps where
	// superseded COW copies were allocated; compaction makes the file grow by
	// exactly the number of pages this commit actually adds.
	reach := tx.reachableDirty()
	remap := make(map[uint64]uint64, len(reach))
	for i, oldID := range reach {
		remap[oldID] = tx.basePageCount + uint64(i)
	}
	newRoot := tx.root
	if r, ok := remap[tx.root]; ok {
		newRoot = r
	}
	newMeta := meta{txnID: newTxID, rootID: newRoot, pageCount: tx.basePageCount + uint64(len(reach))}

	// 1. write the reachable new pages at their compact ids (no orphans).
	if err := db.hook("before_data_write"); err != nil {
		return err
	}
	for _, oldID := range reach {
		if err := db.storage.WritePage(remap[oldID], tx.encodeRemapped(oldID, remap)); err != nil {
			return db.fail(err)
		}
	}
	if err := db.hook("after_data_write"); err != nil {
		return err
	}

	// 2. make data durable BEFORE the meta that references it (I2).
	if err := db.hook("before_data_sync"); err != nil {
		return err
	}
	if !db.skipDataSync {
		if err := db.storage.Sync(); err != nil {
			return db.fail(err)
		}
	}
	if err := db.hook("after_data_sync"); err != nil {
		return err
	}

	// 3. publish the new meta to the ALTERNATE slot; the committed slot is
	//    never touched until this generation is durable (I3).
	if err := db.hook("before_meta_write"); err != nil {
		return err
	}
	slot := newTxID % 2
	if err := db.storage.WritePage(slot, newMeta.encode(slot)); err != nil {
		return db.fail(err)
	}
	if err := db.hook("after_meta_write"); err != nil {
		return err
	}

	// 4. durable commit point.
	if err := db.hook("before_meta_sync"); err != nil {
		return err
	}
	if err := db.storage.Sync(); err != nil {
		return db.fail(err)
	}
	if err := db.hook("after_meta_sync"); err != nil {
		return err
	}

	// 5. only now is the transaction acknowledged (I4).
	if err := db.hook("before_ack"); err != nil {
		return err
	}
	db.mu.Lock()
	db.committed = newMeta
	db.mu.Unlock()
	return nil
}

// Rollback discards the transaction; its staged pages become unreachable and
// are simply never referenced (SRS §59).
func (tx *WriteTx) Rollback() error {
	if tx.done {
		return ErrTxnClosed
	}
	tx.finish()
	return nil
}

func (tx *WriteTx) finish() {
	if tx.done {
		return
	}
	tx.done = true
	tx.dirty = nil
	tx.db.wmu.Unlock()
}

func cloneOrNil(b []byte) []byte {
	if b == nil {
		return nil
	}
	return clonebytes(b)
}
