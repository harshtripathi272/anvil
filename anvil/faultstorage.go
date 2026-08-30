package anvil

import "sync"

// FaultStorage is an in-memory Storage that models stable storage precisely:
// writes land in a volatile staging area and become durable only on Sync. A
// simulated crash discards everything not yet synced — exactly the failure
// model in which "only synchronized bytes survive" (SRS v1.1 §C1).
//
// It is the primary scientific instrument for Anvil's durability proof: it is
// deterministic, fast (no real disk), and lets a harness reopen the durable
// image after a crash at any point in the commit protocol.
type FaultStorage struct {
	mu      sync.Mutex
	durable map[uint64][]byte // survives a crash (has been Sync'd)
	pending map[uint64][]byte // lost on a crash (written, not yet Sync'd)

	writes int64
	syncs  int64
	reads  int64
}

// NewFaultStorage returns an empty fault-injecting backend.
func NewFaultStorage() *FaultStorage {
	return &FaultStorage{
		durable: make(map[uint64][]byte),
		pending: make(map[uint64][]byte),
	}
}

func (s *FaultStorage) ReadPage(id uint64, buf []byte) error {
	if len(buf) != PageSize {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	// A live process sees its own un-synced writes (page cache), so pending
	// shadows durable until a crash drops it.
	if p, ok := s.pending[id]; ok {
		copy(buf, p)
		return nil
	}
	if p, ok := s.durable[id]; ok {
		copy(buf, p)
		return nil
	}
	// Reading a page that was never written: return zeros. Decode will reject
	// it (magic mismatch), which is the correct signal for a dangling pointer.
	for i := range buf {
		buf[i] = 0
	}
	return nil
}

func (s *FaultStorage) WritePage(id uint64, p []byte) error {
	if len(p) != PageSize {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	s.pending[id] = clonebytes(p)
	return nil
}

func (s *FaultStorage) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncs++
	for id, p := range s.pending {
		s.durable[id] = p
		delete(s.pending, id)
	}
	return nil
}

func (s *FaultStorage) NumPages() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var max uint64
	for id := range s.durable {
		if id+1 > max {
			max = id + 1
		}
	}
	for id := range s.pending {
		if id+1 > max {
			max = id + 1
		}
	}
	return max, nil
}

func (s *FaultStorage) Close() error { return nil }

// --- crash / corruption controls (test + torture only) ---

// Crash simulates abrupt power loss: all un-synced writes vanish, the durable
// image remains. A fresh Open over this storage then performs real recovery.
func (s *FaultStorage) Crash() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = make(map[uint64][]byte)
}

// CrashPersistingOnly models power loss *during* a flush where only some pages
// reached the platter: the given un-synced pages become durable and every other
// un-synced write is lost. This exposes write-ordering hazards — a meta page
// can survive while the data pages it references do not (SRS v1.1 §C1 negative
// control). With the correct protocol the data pages are already durable from
// the pre-meta sync, so persisting only the meta is still consistent; with that
// barrier removed, it is not.
func (s *FaultStorage) CrashPersistingOnly(ids ...uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		keep[id] = true
	}
	for id, p := range s.pending {
		if keep[id] {
			s.durable[id] = p
		}
	}
	s.pending = make(map[uint64][]byte)
}

// CorruptDurablePage flips a byte in a durably-stored page, modeling on-media
// corruption or a torn write that reached the platter. Used to exercise
// checksum detection and metadata fallback (SRS v1.1 §52, §C6).
func (s *FaultStorage) CorruptDurablePage(id uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.durable[id]
	if !ok || len(p) == 0 {
		return false
	}
	p[headerSize+1] ^= 0xff // flip a payload byte -> checksum mismatch
	return true
}

// Stats reports cumulative I/O counts (for observability/coverage reporting).
func (s *FaultStorage) Stats() (reads, writes, syncs int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads, s.writes, s.syncs
}
