package anvil

import "errors"

// Error classes exposed by Anvil. Callers distinguish these with errors.Is.
//
// Note: there is deliberately no ErrConflict. Anvil is single-writer
// (SRS v1.1 §C8), so there is no write-write conflict to report.
var (
	// ErrNotFound is returned when a key does not exist.
	ErrNotFound = errors.New("anvil: key not found")
	// ErrClosed is returned when the database has been closed.
	ErrClosed = errors.New("anvil: database closed")
	// ErrReadOnly is returned when a mutation is attempted on a read transaction.
	ErrReadOnly = errors.New("anvil: read-only transaction")
	// ErrTxnClosed is returned when a finished transaction is reused.
	ErrTxnClosed = errors.New("anvil: transaction already finished")
	// ErrCorrupt is returned when the database is structurally invalid in a way
	// the commit protocol cannot explain (see SRS v1.1 §I9, §32).
	ErrCorrupt = errors.New("anvil: database corrupt")
	// ErrChecksum is returned when a page fails checksum validation.
	ErrChecksum = errors.New("anvil: page checksum mismatch")
	// ErrFormat is returned when a file is not a recognized Anvil database or
	// uses an incompatible format/page size.
	ErrFormat = errors.New("anvil: unrecognized file format")
	// ErrInvalidArgument is returned for out-of-range keys/values or bad API use.
	ErrInvalidArgument = errors.New("anvil: invalid argument")
	// ErrIO wraps an underlying storage I/O failure.
	ErrIO = errors.New("anvil: i/o error")
	// ErrUnsupported is returned for operations not implemented in this version.
	ErrUnsupported = errors.New("anvil: unsupported operation")
)
