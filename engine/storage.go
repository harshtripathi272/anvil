package engine

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Storage is the physical persistence layer beneath the page manager. Anvil
// talks to storage only through this interface so the engine can run against a
// real file or against the deterministic fault-injecting backend without any
// change to the commit protocol (SRS v1.1 §C1, §92).
//
// Pages are addressed by id; byte offset is id*PageSize. Writes are staged and
// become durable only at Sync — this models real stable storage, where a crash
// discards everything not yet fsync'd.
type Storage interface {
	// ReadPage reads exactly one page into buf (len(buf) must be PageSize).
	ReadPage(id uint64, buf []byte) error
	// WritePage stages one full page for durability at the next Sync.
	WritePage(id uint64, p []byte) error
	// Sync makes all staged writes durable (fdatasync/fsync semantics).
	Sync() error
	// NumPages reports how many pages the durable image currently spans.
	NumPages() (uint64, error)
	// Close releases the storage.
	Close() error
}

// FileStorage is the real, on-disk backend using a single database file.
type FileStorage struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

// openFileStorage opens (creating if absent) the database file. created reports
// whether the file was freshly created (empty), so the caller can initialize it.
func openFileStorage(path string) (fs *FileStorage, created bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, errors.Join(ErrIO, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, false, errors.Join(ErrIO, err)
	}
	created = info.Size() == 0
	fs = &FileStorage{f: f, path: path}
	if created {
		// Make the directory entry durable before we rely on the file
		// (SRS v1.1 §C3). Best-effort: not all platforms permit dir fsync.
		syncDir(filepath.Dir(path))
	}
	return fs, created, nil
}

func (s *FileStorage) ReadPage(id uint64, buf []byte) error {
	if len(buf) != PageSize {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.f.ReadAt(buf, int64(id)*PageSize)
	if err != nil && !(err == io.EOF && n == PageSize) {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return errors.Join(ErrCorrupt, err)
		}
		return errors.Join(ErrIO, err)
	}
	return nil
}

func (s *FileStorage) WritePage(id uint64, p []byte) error {
	if len(p) != PageSize {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.f.WriteAt(p, int64(id)*PageSize); err != nil {
		return errors.Join(ErrIO, err)
	}
	return nil
}

func (s *FileStorage) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fdatasync(s.f); err != nil {
		return errors.Join(ErrIO, err)
	}
	return nil
}

func (s *FileStorage) NumPages() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.f.Stat()
	if err != nil {
		return 0, errors.Join(ErrIO, err)
	}
	return uint64(info.Size()) / PageSize, nil
}

func (s *FileStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

// syncDir best-effort fsyncs a directory so a newly created file's directory
// entry is durable. On platforms that disallow opening a directory for sync
// (e.g. Windows) this is a no-op.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}
