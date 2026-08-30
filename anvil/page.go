package anvil

import (
	"encoding/binary"
	"hash/crc32"
)

// On-disk format constants. Page size is fixed at 4096 in v1 (SRS v1.1 §C4):
// META A lives at page 0, META B at page 1, tree/data pages at 2+.
const (
	// PageSize is the fixed physical page size in bytes.
	PageSize = 4096
	// FormatVersion is the on-disk format version this build reads and writes.
	FormatVersion = formatVersion
	// headerSize is the fixed per-page header length.
	headerSize = 32
	// payloadMax is the maximum bytes of payload a page can hold.
	payloadMax = PageSize - headerSize // 4064
	// maxRecord bounds a single record so a split always makes progress: with
	// every record <= payloadMax/2, any overflowing node splits into two halves
	// that each fit (SRS reasoning locked in v1.1).
	maxRecord = payloadMax / 2 // 2032

	// MaxKeySize is the largest permitted key.
	MaxKeySize = 1024

	magic         = 0x414E5650 // "ANVP", little-endian
	formatVersion = 1

	metaSlotA = 0
	metaSlotB = 1
	firstPage = 2 // first id available for tree/data pages
)

// page types
const (
	pageMeta     = 1
	pageInternal = 2
	pageLeaf     = 3
)

// Header layout (little-endian), 32 bytes:
//
//	[0:4]   magic       uint32
//	[4]     pageType    uint8
//	[5]     flags       uint8 (reserved, 0)
//	[6:8]   count       uint16   number of keys
//	[8:16]  pageID      uint64
//	[16:24] txnID       uint64   generation that wrote this page (informational)
//	[24:28] payloadLen  uint32
//	[28:32] checksum    uint32   crc32c over the page excluding these 4 bytes
const (
	offMagic      = 0
	offType       = 4
	offFlags      = 5
	offCount      = 6
	offPageID     = 8
	offTxnID      = 16
	offPayloadLen = 24
	offChecksum   = 28
)

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// computeChecksum returns the crc32c of a full page image with the checksum
// field (bytes [28:32]) excluded from the calculation.
func computeChecksum(p []byte) uint32 {
	h := crc32.New(crcTable)
	h.Write(p[:offChecksum])
	h.Write(p[offChecksum+4:])
	return h.Sum32()
}

// MaxValueSize is the largest value that fits given a key of length keyLen.
// v1 has no overflow pages, so key+value must fit within maxRecord.
func MaxValueSize(keyLen int) int {
	return maxRecord - 6 - keyLen
}

// checkKeyValue validates key/value bounds before they enter the tree.
func checkKeyValue(key, val []byte) error {
	if len(key) == 0 {
		return ErrInvalidArgument // empty keys are not allowed
	}
	if len(key) > MaxKeySize {
		return ErrInvalidArgument
	}
	if 6+len(key)+len(val) > maxRecord {
		return ErrInvalidArgument
	}
	return nil
}

// node is the decoded, in-memory form of an internal or leaf page. Anvil works
// on decoded nodes for clarity and encodes them to fixed pages at write time.
//
// For a leaf: len(keys) == len(vals).
// For an internal node: len(children) == len(keys)+1, and keys[i] separates
// children[i] and children[i+1] (keys[i] equals the first key of the subtree
// rooted at children[i+1], as in a classic B+tree).
type node struct {
	id       uint64
	txnID    uint64
	leaf     bool
	keys     [][]byte
	vals     [][]byte // leaf only
	children []uint64 // internal only
}

// payloadSize returns the encoded payload byte count for the node.
func (n *node) payloadSize() int {
	s := 0
	if n.leaf {
		for i := range n.keys {
			s += 6 + len(n.keys[i]) + len(n.vals[i])
		}
		return s
	}
	s += 8 * len(n.children)
	for i := range n.keys {
		s += 2 + len(n.keys[i])
	}
	return s
}

// encode serializes the node into a fresh PageSize buffer.
func (n *node) encode() []byte {
	p := make([]byte, PageSize)
	binary.LittleEndian.PutUint32(p[offMagic:], magic)
	if n.leaf {
		p[offType] = pageLeaf
	} else {
		p[offType] = pageInternal
	}
	binary.LittleEndian.PutUint16(p[offCount:], uint16(len(n.keys)))
	binary.LittleEndian.PutUint64(p[offPageID:], n.id)
	binary.LittleEndian.PutUint64(p[offTxnID:], n.txnID)

	off := headerSize
	if n.leaf {
		for i := range n.keys {
			binary.LittleEndian.PutUint16(p[off:], uint16(len(n.keys[i])))
			off += 2
			binary.LittleEndian.PutUint32(p[off:], uint32(len(n.vals[i])))
			off += 4
			off += copy(p[off:], n.keys[i])
			off += copy(p[off:], n.vals[i])
		}
	} else {
		for _, c := range n.children {
			binary.LittleEndian.PutUint64(p[off:], c)
			off += 8
		}
		for i := range n.keys {
			binary.LittleEndian.PutUint16(p[off:], uint16(len(n.keys[i])))
			off += 2
			off += copy(p[off:], n.keys[i])
		}
	}
	binary.LittleEndian.PutUint32(p[offPayloadLen:], uint32(off-headerSize))
	binary.LittleEndian.PutUint32(p[offChecksum:], computeChecksum(p))
	return p
}

// decodeNode parses a page image into a node, validating magic and checksum.
func decodeNode(id uint64, p []byte) (*node, error) {
	if len(p) != PageSize {
		return nil, ErrCorrupt
	}
	if binary.LittleEndian.Uint32(p[offMagic:]) != magic {
		return nil, ErrFormat
	}
	if computeChecksum(p) != binary.LittleEndian.Uint32(p[offChecksum:]) {
		return nil, ErrChecksum
	}
	count := int(binary.LittleEndian.Uint16(p[offCount:]))
	payloadLen := int(binary.LittleEndian.Uint32(p[offPayloadLen:]))
	if payloadLen > payloadMax {
		return nil, ErrCorrupt
	}
	end := headerSize + payloadLen
	n := &node{id: id, txnID: binary.LittleEndian.Uint64(p[offTxnID:])}
	off := headerSize

	switch p[offType] {
	case pageLeaf:
		n.leaf = true
		n.keys = make([][]byte, 0, count)
		n.vals = make([][]byte, 0, count)
		for i := 0; i < count; i++ {
			if off+6 > end {
				return nil, ErrCorrupt
			}
			kl := int(binary.LittleEndian.Uint16(p[off:]))
			off += 2
			vl := int(binary.LittleEndian.Uint32(p[off:]))
			off += 4
			if off+kl+vl > end {
				return nil, ErrCorrupt
			}
			n.keys = append(n.keys, clonebytes(p[off:off+kl]))
			off += kl
			n.vals = append(n.vals, clonebytes(p[off:off+vl]))
			off += vl
		}
	case pageInternal:
		n.children = make([]uint64, 0, count+1)
		for i := 0; i < count+1; i++ {
			if off+8 > end {
				return nil, ErrCorrupt
			}
			n.children = append(n.children, binary.LittleEndian.Uint64(p[off:]))
			off += 8
		}
		n.keys = make([][]byte, 0, count)
		for i := 0; i < count; i++ {
			if off+2 > end {
				return nil, ErrCorrupt
			}
			kl := int(binary.LittleEndian.Uint16(p[off:]))
			off += 2
			if off+kl > end {
				return nil, ErrCorrupt
			}
			n.keys = append(n.keys, clonebytes(p[off:off+kl]))
			off += kl
		}
	default:
		return nil, ErrCorrupt
	}
	return n, nil
}

// meta is the decoded form of a metadata page. The two meta slots (page 0 and
// page 1) alternate by transaction parity; recovery selects the newest that
// validates (SRS v1.1 §C6, §I9).
type meta struct {
	txnID     uint64
	rootID    uint64
	pageCount uint64 // high-water mark: next page id to allocate
	freelist  uint64 // reserved in v1, always 0
}

// Meta payload layout (little-endian), after the 32-byte header:
//
//	[32:36] formatVersion uint32
//	[36:40] pageSize      uint32
//	[40:48] txnID         uint64
//	[48:56] rootID        uint64
//	[56:64] pageCount     uint64
//	[64:72] freelist      uint64
func (m *meta) encode(slot uint64) []byte {
	p := make([]byte, PageSize)
	binary.LittleEndian.PutUint32(p[offMagic:], magic)
	p[offType] = pageMeta
	binary.LittleEndian.PutUint64(p[offPageID:], slot)
	binary.LittleEndian.PutUint64(p[offTxnID:], m.txnID)

	off := headerSize
	binary.LittleEndian.PutUint32(p[off:], formatVersion)
	off += 4
	binary.LittleEndian.PutUint32(p[off:], PageSize)
	off += 4
	binary.LittleEndian.PutUint64(p[off:], m.txnID)
	off += 8
	binary.LittleEndian.PutUint64(p[off:], m.rootID)
	off += 8
	binary.LittleEndian.PutUint64(p[off:], m.pageCount)
	off += 8
	binary.LittleEndian.PutUint64(p[off:], m.freelist)
	off += 8

	binary.LittleEndian.PutUint32(p[offPayloadLen:], uint32(off-headerSize))
	binary.LittleEndian.PutUint32(p[offChecksum:], computeChecksum(p))
	return p
}

// decodeMeta parses and validates a metadata page.
func decodeMeta(p []byte) (*meta, error) {
	if len(p) != PageSize {
		return nil, ErrCorrupt
	}
	if binary.LittleEndian.Uint32(p[offMagic:]) != magic {
		return nil, ErrFormat
	}
	if p[offType] != pageMeta {
		return nil, ErrFormat
	}
	if computeChecksum(p) != binary.LittleEndian.Uint32(p[offChecksum:]) {
		return nil, ErrChecksum
	}
	off := headerSize
	if binary.LittleEndian.Uint32(p[off:]) != formatVersion {
		return nil, ErrFormat
	}
	off += 4
	if binary.LittleEndian.Uint32(p[off:]) != PageSize {
		return nil, ErrFormat
	}
	off += 4
	m := &meta{}
	m.txnID = binary.LittleEndian.Uint64(p[off:])
	off += 8
	m.rootID = binary.LittleEndian.Uint64(p[off:])
	off += 8
	m.pageCount = binary.LittleEndian.Uint64(p[off:])
	off += 8
	m.freelist = binary.LittleEndian.Uint64(p[off:])
	return m, nil
}

// clonebytes returns an independent copy of b (nil-safe, empty-safe).
func clonebytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
