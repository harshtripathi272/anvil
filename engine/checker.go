package engine

import (
	"bytes"
	"fmt"
)

// CheckReport summarizes a structural verification pass.
type CheckReport struct {
	PagesChecked int
	LeafDepth    int
	Keys         int
}

// Check performs deep structural validation of the committed tree (SRS v1.1
// §39, §49): every reachable page's checksum, valid page types and pointers,
// sorted keys, separator consistency, uniform leaf depth, and no cycles/aliased
// pages within the tree. Underfull and empty nodes are legal (§C7) and are not
// reported.
func (db *DB) Check() (CheckReport, error) {
	db.mu.RLock()
	m := db.committed
	db.mu.RUnlock()

	c := &checker{
		db:        db,
		pageCount: m.pageCount,
		visited:   make(map[uint64]bool),
		leafDepth: -1,
	}
	if err := c.walk(m.rootID, 0, nil, nil); err != nil {
		return CheckReport{}, err
	}
	return CheckReport{
		PagesChecked: len(c.visited),
		LeafDepth:    c.leafDepth,
		Keys:         c.keys,
	}, nil
}

type checker struct {
	db        *DB
	pageCount uint64
	visited   map[uint64]bool
	leafDepth int
	keys      int
}

// walk validates the subtree rooted at id, where every key must lie in [lo, hi)
// (nil bounds mean unbounded).
func (c *checker) walk(id uint64, depth int, lo, hi []byte) error {
	if id < firstPage || id >= c.pageCount {
		return fmt.Errorf("%w: page id %d out of range [%d,%d)", ErrCorrupt, id, firstPage, c.pageCount)
	}
	if c.visited[id] {
		return fmt.Errorf("%w: page %d referenced more than once (cycle or aliasing)", ErrCorrupt, id)
	}
	c.visited[id] = true

	n, err := c.db.readNodeFromStorage(id) // validates checksum + format
	if err != nil {
		return fmt.Errorf("page %d: %w", id, err)
	}

	for i := range n.keys {
		if i > 0 && bytes.Compare(n.keys[i-1], n.keys[i]) >= 0 {
			return fmt.Errorf("%w: page %d keys not strictly sorted at %d", ErrCorrupt, id, i)
		}
		if lo != nil && bytes.Compare(n.keys[i], lo) < 0 {
			return fmt.Errorf("%w: page %d key below lower separator bound", ErrCorrupt, id)
		}
		if hi != nil && bytes.Compare(n.keys[i], hi) >= 0 {
			return fmt.Errorf("%w: page %d key at/above upper separator bound", ErrCorrupt, id)
		}
	}

	if n.leaf {
		if c.leafDepth == -1 {
			c.leafDepth = depth
		} else if depth != c.leafDepth {
			return fmt.Errorf("%w: leaf at depth %d, expected %d", ErrCorrupt, depth, c.leafDepth)
		}
		c.keys += len(n.keys)
		return nil
	}

	if len(n.children) != len(n.keys)+1 {
		return fmt.Errorf("%w: page %d has %d children, expected %d", ErrCorrupt, id, len(n.children), len(n.keys)+1)
	}
	for i, ch := range n.children {
		childLo, childHi := lo, hi
		if i > 0 {
			childLo = n.keys[i-1]
		}
		if i < len(n.keys) {
			childHi = n.keys[i]
		}
		if err := c.walk(ch, depth+1, childLo, childHi); err != nil {
			return err
		}
	}
	return nil
}
