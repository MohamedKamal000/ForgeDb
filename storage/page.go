package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	BTREE_PAGE_SIZE        = 4096
	BTREE_MAX_KEY_SIZE     = 1000
	BTREE_MAX_VAL_SIZE     = 3000
	BTREE_HEADER_SIZE      = 16
	BTREE_CELL_HEADER_SIZE = 8
)

const (
	BNODE_LEAF_TYPE = 1

	BNODE_INTERNAL_TYPE = 2
)

type BPage []byte

/*
| type | nkeys | reserved |  pointers  |  offsets   | key-values | unused |
|  2B  |   2B  |    12B   | nkeys × 8B | nkeys × 2B |     ...    |        |

| key_size | val_size | reserved | key | val |
|    2B    |    2B    |    4B    | ... | ... |

byte offset => page number (aka pointer) * page size
*/

func assert(condition bool, msg string) {
	if !condition {
		panic(errors.New(msg))
	}
}

func init() {
	node1max := 4 + 12 + 1*8 + 1*2 + 8 + BTREE_MAX_KEY_SIZE + BTREE_MAX_VAL_SIZE
	assert(node1max <= BTREE_PAGE_SIZE, "init: node size does not meet page size")
}

func (node BPage) btye() uint16 {
	return binary.LittleEndian.Uint16(node[0:2])
}

func (node BPage) nkeys() uint16 {
	return binary.LittleEndian.Uint16(node[2:4])
}

func (node BPage) setHeader(btype, nkeys uint16) {
	binary.LittleEndian.PutUint16(node[0:2], btype)
	binary.LittleEndian.PutUint16(node[2:4], nkeys)
	clear(node[4:16]) // reserved
}

func (node BPage) getPtr(idx uint16) uint64 {
	assert(idx < node.nkeys(), "getPtr: pointer index out of range")
	pos := BTREE_HEADER_SIZE + idx*8
	return binary.LittleEndian.Uint64(node[pos:])
}

func (node BPage) setPtr(idx uint16, val uint64) {
	assert(idx < node.nkeys(), "setPtr: pointer index out of range")
	pos := BTREE_HEADER_SIZE + 8*idx
	binary.LittleEndian.PutUint64(node[pos:], val)
}

func (node BPage) getOffst(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}

	// idx - 1 because the last index indicates the starting free area to appen at i guess from my understanding
	pos := BTREE_HEADER_SIZE + node.nkeys()*8 + (idx-1)*2
	return binary.LittleEndian.Uint16(node[pos:])
}

func (node BPage) setOffst(idx uint16, offst uint16) {
	pos := BTREE_HEADER_SIZE + node.nkeys()*8 + (idx-1)*2
	binary.LittleEndian.PutUint16(node[pos:], offst)
}

func (node BPage) kvPos(idx uint16) uint16 {
	assert(idx <= node.nkeys(), "kvPos: pointer index out of range")
	nKeys := node.nkeys()
	off := node.getOffst(idx)
	return BTREE_HEADER_SIZE + nKeys*8 + nKeys*2 + off
}

func (node BPage) getKey(idx uint16) []byte {
	assert(idx < node.nkeys(), "getKey: pointer index out of range")
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node[pos:])
	k_start := pos + BTREE_CELL_HEADER_SIZE
	return node[k_start : k_start+klen]
}

func (node BPage) getVal(idx uint16) []byte {
	assert(idx < node.nkeys(), "getVal: pointer index out of range")
	pos := node.kvPos(idx)
	klen := binary.LittleEndian.Uint16(node[pos:])
	vlen := binary.LittleEndian.Uint16(node[pos+2:])
	val_start := pos + BTREE_CELL_HEADER_SIZE + klen
	return node[val_start : val_start+vlen]
}

func nodeAppendKV(new BPage, idx uint16, ptr uint64, key []byte, val []byte) {
	new.setPtr(idx, ptr)

	pos := new.kvPos(idx) // last index is the new place for the next kv
	klen := uint16(len(key))
	vlen := uint16(len(val))

	binary.LittleEndian.PutUint16(new[pos:], klen)
	binary.LittleEndian.PutUint16(new[pos+2:], vlen)
	clear(new[pos+4 : pos+8]) // reserved 4B

	copy(new[pos+BTREE_CELL_HEADER_SIZE:], key)
	copy(new[pos+BTREE_CELL_HEADER_SIZE+klen:], val)
	new.setOffst(idx+1, new.getOffst(idx)+BTREE_CELL_HEADER_SIZE+klen+vlen)
}

func nodeAppendRange(new, old BPage, startingNew, startingOld, n uint16) {
	for i := range n {
		des, src := startingNew+i, startingOld+i
		nodeAppendKV(new, des, old.getPtr(src), old.getKey(src), old.getVal(src))
	}
}

func (node BPage) nbytes() uint16 {
	return node.kvPos(node.nkeys())
}

func leafInsert(new, old BPage, idx uint16, key, val []byte) {
	new.setHeader(BNODE_LEAF_TYPE, old.nkeys()+1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, 0, key, val)
	nodeAppendRange(new, old, idx+1, idx, old.nkeys()-idx)
}

func leafUpdate(new, old BPage, idx uint16, key, val []byte) {
	new.setHeader(BNODE_LEAF_TYPE, old.nkeys())
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, 0, key, val)
	nodeAppendRange(new, old, idx+1, idx+1, old.nkeys()-(idx+1))
}

func nodeLookupIndex(node BPage, key []byte) uint16 {
	nkeys := node.nkeys()
	l, r := uint16(0), nkeys

	for l < r {
		mid := l + (r-l)/2
		cmp := bytes.Compare(node.getKey(mid), key)

		if cmp == 0 {
			return mid
		}

		if cmp < 0 {
			l = mid + 1
		} else {
			r = mid
		}
	}

	return l
}

func navidageProperIndex(node BPage, key []byte) uint16 {
	nkeys := node.nkeys()
	l, r := uint16(0), nkeys

	for l < r {
		mid := l + (r-l)/2
		cmp := bytes.Compare(node.getKey(mid), key)

		if cmp <= 0 {
			l = mid + 1
		} else {
			r = mid
		}
	}

	return l - 1
}

/*
For an in-memory B+tree, an oversized node can be split into 2 nodes, each with half of the keys. For a disk-based B+tree,
half of the keys may not fit into a page due to uneven key sizes.
However, we can use the half position as an initial guess, then move it left or right if the half is too large.

what he means is, the next key/val may also make the page uneven is size to the point where you would have a page
exceeding in size after split
*/

// initialSplit here fits old content page to right, but left may still bigger
// due to a bigger key that in the left page, which will result in a 3 nodes rather than two
func initialSplit(left, right, old BPage) {
	assert(old.nkeys() >= 2, "nodeSplit: number of keys less than 2")

	nleft := old.nkeys() / 2

	leftBytes := func() uint16 {
		return BTREE_HEADER_SIZE + nleft*8 + nleft*2 + old.getOffst(nleft)
	}

	for leftBytes() > BTREE_PAGE_SIZE {
		nleft--
	}

	assert(nleft >= 1, "initialSplit: nleft some how became 0 or negative")

	rightBytes := func() uint16 {
		return old.nbytes() - leftBytes() + BTREE_HEADER_SIZE // we add the header size again since we have two nodes
	}

	for rightBytes() > BTREE_PAGE_SIZE {
		nleft++
	}

	nright := old.nkeys() - nleft

	left.setHeader(old.btye(), nleft)
	right.setHeader(old.btye(), nright)
	nodeAppendRange(left, old, 0, 0, nleft)
	nodeAppendRange(right, old, 0, nleft, nright)
	assert(right.nbytes() <= BTREE_PAGE_SIZE, "initialSplit: right node still size bigger than header size")
}

func split(old BPage) (nodesNumber uint16, nodes [3]BPage) {
	nbytes := old.nbytes()
	if nbytes <= BTREE_PAGE_SIZE {
		return 1, [3]BPage{old[:BTREE_PAGE_SIZE]}
	}

	// we multiply by 2 because the left may be in the bigger than page size case and might need another split
	leftNode := BPage(make([]byte, 2*BTREE_PAGE_SIZE))
	rightNode := BPage(make([]byte, BTREE_PAGE_SIZE))
	initialSplit(leftNode, rightNode, old)

	if leftNode.nbytes() <= BTREE_PAGE_SIZE {
		leftNode = leftNode[:BTREE_PAGE_SIZE] // get it back to normal page size
		return 2, [3]BPage{leftNode, rightNode}
	}

	leftOfLeft := BPage(make([]byte, BTREE_PAGE_SIZE))
	middle := BPage(make([]byte, BTREE_PAGE_SIZE))
	initialSplit(leftOfLeft, middle, leftNode)

	assert(leftOfLeft.nbytes() <= BTREE_PAGE_SIZE, "split: left of the left node still bigger than page size")
	return 3, [3]BPage{leftOfLeft, middle, rightNode}
}

func leafDelete(new BPage, old BPage, idx uint16) {
	new.setHeader(BNODE_LEAF_TYPE, old.nkeys()-1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendRange(new, old, idx, idx+1, old.nkeys()-(idx+1))
}

func nodeMerge(new BPage, left BPage, right BPage) {
	nLeft := left.nkeys()
	nRight := right.nkeys()
	new.setHeader(left.btye(), nLeft+nRight)
	l, r := uint16(0), uint16(0)
	idx := uint16(0)

	for l < nLeft && r < nRight {
		rightKey := right.getKey(r)
		leftKey := left.getKey(l)
		cmp := bytes.Compare(leftKey, rightKey)

		if cmp > 0 {
			nodeAppendKV(new, idx, right.getPtr(r), rightKey, right.getVal(r))
			r++
		} else {
			nodeAppendKV(new, idx, left.getPtr(l), leftKey, left.getVal(l))
			l++
		}
		idx++
	}

	for l < nLeft {
		leftKey := left.getKey(l)
		nodeAppendKV(new, idx, left.getPtr(l), leftKey, left.getVal(l))
		l++
		idx++
	}

	for r < nRight {
		rightKey := right.getKey(r)
		nodeAppendKV(new, idx, right.getPtr(r), rightKey, right.getVal(r))
		r++
		idx++
	}
}
