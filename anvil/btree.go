package anvil

import (
	"bytes"
	"sort"
)

// reader provides read access to decoded pages by id.
type reader interface {
	node(id uint64) (*node, error)
}

// pager adds page allocation and staging for write transactions. Copy-on-write
// means every node touched on a path is cloned and re-staged under a fresh id;
// the old pages are never modified (SRS v1.1 §I1).
type pager interface {
	reader
	alloc() uint64
	stage(n *node)
}

// --- node helpers ---

func (n *node) clone() *node {
	c := &node{leaf: n.leaf}
	c.keys = append([][]byte(nil), n.keys...)
	if n.leaf {
		c.vals = append([][]byte(nil), n.vals...)
	} else {
		c.children = append([]uint64(nil), n.children...)
	}
	return c
}

// lowerBound returns the first index i with keys[i] >= key.
func (n *node) lowerBound(key []byte) int {
	return sort.Search(len(n.keys), func(i int) bool {
		return bytes.Compare(n.keys[i], key) >= 0
	})
}

// childIndex returns the child to descend into for key: the first index i with
// keys[i] > key (separators equal the first key of their right subtree).
func (n *node) childIndex(key []byte) int {
	return sort.Search(len(n.keys), func(i int) bool {
		return bytes.Compare(n.keys[i], key) > 0
	})
}

// leafPut inserts or replaces (key,val) in a leaf clone, keeping keys sorted.
func (c *node) leafPut(key, val []byte) {
	i := c.lowerBound(key)
	if i < len(c.keys) && bytes.Equal(c.keys[i], key) {
		c.vals[i] = val
		return
	}
	c.keys = append(c.keys, nil)
	copy(c.keys[i+1:], c.keys[i:])
	c.keys[i] = key
	c.vals = append(c.vals, nil)
	copy(c.vals[i+1:], c.vals[i:])
	c.vals[i] = val
}

// internalInsert splices a promoted separator and its right child into an
// internal clone at position idx.
func (c *node) internalInsert(idx int, sep []byte, right uint64) {
	c.keys = append(c.keys, nil)
	copy(c.keys[idx+1:], c.keys[idx:])
	c.keys[idx] = sep
	c.children = append(c.children, 0)
	copy(c.children[idx+2:], c.children[idx+1:])
	c.children[idx+1] = right
}

// split carries the result of a node overflow up to the parent.
type split struct {
	sep   []byte
	right uint64
}

// finishNode stages c if it fits, otherwise splits it and returns the split.
func finishNode(pg pager, c *node) (uint64, *split, error) {
	if c.payloadSize() <= payloadMax {
		c.id = pg.alloc()
		pg.stage(c)
		return c.id, nil, nil
	}
	var leftID, rightID uint64
	var sep []byte
	if c.leaf {
		leftID, rightID, sep = c.splitLeaf(pg)
	} else {
		leftID, rightID, sep = c.splitInternal(pg)
	}
	return leftID, &split{sep: sep, right: rightID}, nil
}

// splitPointLeaf returns the largest prefix count whose encoded size fits a
// page; because every record is <= payloadMax/2, both halves are guaranteed to
// fit (SRS split invariant).
func (c *node) splitPointLeaf() int {
	size, mid := 0, 0
	for i := 0; i < len(c.keys); i++ {
		rs := 6 + len(c.keys[i]) + len(c.vals[i])
		if size+rs > payloadMax {
			break
		}
		size += rs
		mid = i + 1
	}
	if mid < 1 {
		mid = 1
	}
	if mid > len(c.keys)-1 {
		mid = len(c.keys) - 1
	}
	return mid
}

func (c *node) splitLeaf(pg pager) (leftID, rightID uint64, sep []byte) {
	mid := c.splitPointLeaf()
	left := &node{leaf: true,
		keys: append([][]byte(nil), c.keys[:mid]...),
		vals: append([][]byte(nil), c.vals[:mid]...)}
	right := &node{leaf: true,
		keys: append([][]byte(nil), c.keys[mid:]...),
		vals: append([][]byte(nil), c.vals[mid:]...)}
	left.id = pg.alloc()
	right.id = pg.alloc()
	pg.stage(left)
	pg.stage(right)
	return left.id, right.id, clonebytes(right.keys[0])
}

// splitPointInternal returns the promote index p in [1, K-1]: left keeps
// keys[:p] and children[:p+1], keys[p] is promoted, right keeps the rest.
func (c *node) splitPointInternal() int {
	size := 8 // children[0]
	best := 1
	for p := 1; p <= len(c.keys)-1; p++ {
		size += (2 + len(c.keys[p-1])) + 8
		if size <= payloadMax {
			best = p
		} else {
			break
		}
	}
	return best
}

func (c *node) splitInternal(pg pager) (leftID, rightID uint64, sep []byte) {
	p := c.splitPointInternal()
	left := &node{leaf: false,
		keys:     append([][]byte(nil), c.keys[:p]...),
		children: append([]uint64(nil), c.children[:p+1]...)}
	right := &node{leaf: false,
		keys:     append([][]byte(nil), c.keys[p+1:]...),
		children: append([]uint64(nil), c.children[p+1:]...)}
	sep = clonebytes(c.keys[p])
	left.id = pg.alloc()
	right.id = pg.alloc()
	pg.stage(left)
	pg.stage(right)
	return left.id, right.id, sep
}

// --- operations ---

// btreeGet returns a copy of the value for key, or ErrNotFound.
func btreeGet(r reader, root uint64, key []byte) ([]byte, error) {
	id := root
	for {
		n, err := r.node(id)
		if err != nil {
			return nil, err
		}
		if n.leaf {
			i := n.lowerBound(key)
			if i < len(n.keys) && bytes.Equal(n.keys[i], key) {
				return clonebytes(n.vals[i]), nil
			}
			return nil, ErrNotFound
		}
		id = n.children[n.childIndex(key)]
	}
}

// btreeInsert inserts/replaces (key,val) below id, returning the new subtree id
// and a non-nil split if the node overflowed.
func btreeInsert(pg pager, id uint64, key, val []byte) (uint64, *split, error) {
	n, err := pg.node(id)
	if err != nil {
		return 0, nil, err
	}
	if n.leaf {
		c := n.clone()
		c.leafPut(key, val)
		return finishNode(pg, c)
	}
	idx := n.childIndex(key)
	childNew, sp, err := btreeInsert(pg, n.children[idx], key, val)
	if err != nil {
		return 0, nil, err
	}
	c := n.clone()
	c.children[idx] = childNew
	if sp != nil {
		c.internalInsert(idx, sp.sep, sp.right)
	}
	return finishNode(pg, c)
}

// btreePut inserts/replaces a key and returns the new root id, growing the tree
// by one level if the root splits.
func btreePut(pg pager, root uint64, key, val []byte) (uint64, error) {
	newRoot, sp, err := btreeInsert(pg, root, key, val)
	if err != nil {
		return 0, err
	}
	if sp == nil {
		return newRoot, nil
	}
	r := &node{leaf: false, keys: [][]byte{sp.sep}, children: []uint64{newRoot, sp.right}}
	r.id = pg.alloc()
	pg.stage(r)
	return r.id, nil
}

// btreeDelete removes key if present and returns the new root id. It never
// merges or rebalances: underfull and empty nodes are allowed (SRS v1.1 §C7),
// which keeps deletion simple and the crash surface small.
func btreeDelete(pg pager, id uint64, key []byte) (uint64, bool, error) {
	n, err := pg.node(id)
	if err != nil {
		return 0, false, err
	}
	if n.leaf {
		i := n.lowerBound(key)
		if i >= len(n.keys) || !bytes.Equal(n.keys[i], key) {
			return id, false, nil
		}
		c := n.clone()
		c.keys = append(c.keys[:i], c.keys[i+1:]...)
		c.vals = append(c.vals[:i], c.vals[i+1:]...)
		c.id = pg.alloc()
		pg.stage(c)
		return c.id, true, nil
	}
	idx := n.childIndex(key)
	childNew, found, err := btreeDelete(pg, n.children[idx], key)
	if err != nil || !found {
		return id, found, err
	}
	c := n.clone()
	c.children[idx] = childNew
	c.id = pg.alloc()
	pg.stage(c)
	return c.id, true, nil
}

// --- cursor (ordered iteration) ---

type cursorFrame struct {
	n   *node
	idx int
}

type cursor struct {
	r     reader
	stack []cursorFrame
	err   error
}

// newCursor positions at the first record >= start (start nil = first record).
func newCursor(r reader, root uint64, start []byte) *cursor {
	c := &cursor{r: r}
	id := root
	for {
		n, err := c.r.node(id)
		if err != nil {
			c.err = err
			return c
		}
		if n.leaf {
			i := 0
			if start != nil {
				i = n.lowerBound(start)
			}
			c.stack = append(c.stack, cursorFrame{n, i})
			return c
		}
		i := 0
		if start != nil {
			i = n.childIndex(start)
		}
		c.stack = append(c.stack, cursorFrame{n, i})
		id = n.children[i]
	}
}

func (c *cursor) top() *cursorFrame { return &c.stack[len(c.stack)-1] }

func (c *cursor) valid() bool {
	if c.err != nil || len(c.stack) == 0 {
		return false
	}
	f := c.top()
	return f.n.leaf && f.idx < len(f.n.keys)
}

// ascendToNextLeaf pops the current leaf and descends to the first record of
// the next leaf in key order. Returns false when the tree is exhausted.
func (c *cursor) ascendToNextLeaf() bool {
	c.stack = c.stack[:len(c.stack)-1]
	for len(c.stack) > 0 {
		f := c.top()
		f.idx++
		if f.idx < len(f.n.children) {
			id := f.n.children[f.idx]
			for {
				n, err := c.r.node(id)
				if err != nil {
					c.err = err
					return false
				}
				c.stack = append(c.stack, cursorFrame{n, 0})
				if n.leaf {
					return true
				}
				id = n.children[0]
			}
		}
		c.stack = c.stack[:len(c.stack)-1]
	}
	return false
}

// settle ensures the cursor points at a valid record or is exhausted.
func (c *cursor) settle() {
	for c.err == nil && len(c.stack) > 0 {
		if c.top().idx < len(c.top().n.keys) {
			return
		}
		if !c.ascendToNextLeaf() {
			return
		}
	}
}

func (c *cursor) step() {
	if len(c.stack) == 0 {
		return
	}
	c.top().idx++
	c.settle()
}

// Iterator is a public ordered range iterator over [start, end).
type Iterator struct {
	c       *cursor
	end     []byte
	started bool
}

// Next advances to the next record and reports whether one is available.
func (it *Iterator) Next() bool {
	if it.started {
		it.c.step()
	} else {
		it.started = true
		it.c.settle()
	}
	if !it.c.valid() {
		return false
	}
	if it.end != nil && bytes.Compare(it.c.key(), it.end) >= 0 {
		return false
	}
	return true
}

// Key returns a copy of the current key.
func (it *Iterator) Key() []byte { return clonebytes(it.c.top().n.keys[it.c.top().idx]) }

// Value returns a copy of the current value.
func (it *Iterator) Value() []byte { return clonebytes(it.c.top().n.vals[it.c.top().idx]) }

// Err reports any error encountered during iteration.
func (it *Iterator) Err() error { return it.c.err }

func (c *cursor) key() []byte { return c.top().n.keys[c.top().idx] }
