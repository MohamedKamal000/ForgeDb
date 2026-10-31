package storage

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"unsafe"
)

/*
Disk IO can be simulated with these callbacks.
So a disk-based B+tree can easily be used as an in-memory B+tree, while the reverse is not true.
*/
type BTree struct {
	root uint64

	get func(uint64) []byte

	new func([]byte) uint64

	del func(uint64)
}

func treeInsert(tree *BTree, node BPage, key, val []byte) BPage {
	newNode := BPage(make([]byte, 2*BTREE_PAGE_SIZE))

	switch node.btye() {
	case BNODE_LEAF_TYPE:

		idx := nodeLookupIndex(node, key)
		if idx < node.nkeys() && bytes.Equal(node.getKey(idx), key) {
			leafUpdate(newNode, node, idx, key, val)
		} else {
			leafInsert(newNode, node, idx, key, val)
		}
	case BNODE_INTERNAL_TYPE:
		idx := navidageProperIndex(node, key)
		kptr := node.getPtr(idx)
		knode := treeInsert(tree, tree.get(kptr), key, val)

		nSplits, nodes := split(knode)

		tree.del(kptr)
		nodeReplaceKidN(tree, newNode, node, idx, nodes[:nSplits]...)
	}

	return newNode
}

func nodeReplaceKidN(tree *BTree, new, old BPage, idx uint16, kids ...BPage) {
	inc := uint16(len(kids))

	// we decrement 1 because if the number of kids is 1 that mean we just replace the old node
	new.setHeader(BNODE_INTERNAL_TYPE, old.nkeys()+inc-1)
	nodeAppendRange(new, old, 0, 0, idx)
	for i, node := range kids {
		nodeAppendKV(new, idx+uint16(i), tree.new(node), node.getKey(0), nil)
	}
	nodeAppendRange(new, old, idx+inc, idx+1, old.nkeys()-(idx+1))
}

func nodeReplace2Kid(new, old BPage, idx uint16, ptr uint64, key []byte) {
	new.setHeader(old.btye(), old.nkeys()-1)
	nodeAppendRange(new, old, 0, 0, idx)
	nodeAppendKV(new, idx, ptr, key, nil)
	nodeAppendRange(new, old, idx+1, idx+2, old.nkeys()-(idx+2))
}

func checkLimit(key, val []byte) error {
	kvSize := len(key) + len(val)
	if kvSize >= BTREE_PAGE_SIZE {
		return errors.New("insereted key value bigger than page size")
	}
	return nil
}

func shouldMerge(tree *BTree, node, updated BPage, idx uint16) (int, BPage) {
	if updated.nbytes() > BTREE_PAGE_SIZE/4 {
		return 0, BPage{}
	}

	if idx > 0 {
		leftSibling := BPage(tree.get(node.getPtr(idx - 1)))
		sizeAfterMerge := leftSibling.nbytes() + updated.nbytes() - BTREE_HEADER_SIZE
		if sizeAfterMerge <= BTREE_PAGE_SIZE {
			return -1, leftSibling
		}
	}

	if idx+1 < node.nkeys() {
		rightSibling := BPage(tree.get(node.getPtr(idx + 1)))
		sizeAfterMerge := rightSibling.nbytes() + updated.nbytes() - BTREE_HEADER_SIZE
		if sizeAfterMerge <= BTREE_PAGE_SIZE {
			return 1, rightSibling
		}
	}

	return 0, BPage{}
}

func (tree *BTree) Insert(key, val []byte) error {
	err := checkLimit(key, val)
	if err != nil {
		return err
	}

	if tree.root == 0 {
		root := BPage(make(BPage, BTREE_PAGE_SIZE))
		root.setHeader(BNODE_LEAF_TYPE, 2)
		// append a dummy node to avoid an edge case at the lookup of an index
		// not sure if this is now fixes the edge case since the tutorial was using a sequential iteration for key search
		// i do use binary search istead based on node type
		nodeAppendKV(root, 0, 0, nil, nil)
		nodeAppendKV(root, 1, 0, key, val)

		tree.root = tree.new(root)
		return nil
	}

	node := treeInsert(tree, tree.get(tree.root), key, val)

	nSplits, splitNodes := split(node)

	tree.del(tree.root)
	if nSplits > 1 {
		root := BPage(make(BPage, BTREE_PAGE_SIZE))
		root.setHeader(BNODE_INTERNAL_TYPE, nSplits)

		for i, node := range splitNodes[:nSplits] {
			nodeAppendKV(root, uint16(i), tree.new(node), node.getKey(0), nil)
		}

		tree.root = tree.new(root)
	} else {
		tree.root = tree.new(splitNodes[0])
	}

	return nil
}

func treeDelete(tree *BTree, node BPage, key []byte) BPage {
	newNode := BPage(make([]byte, BTREE_PAGE_SIZE))

	switch node.btye() {
	case BNODE_LEAF_TYPE:
		idx := nodeLookupIndex(node, key)
		if idx < node.nkeys() && bytes.Equal(node.getKey(idx), key) {
			leafDelete(newNode, node, idx)
		} else {
			return BPage{}
		}
	case BNODE_INTERNAL_TYPE:
		idx := navidageProperIndex(node, key)
		return nodeDelete(tree, node, idx, key)
	}
	return newNode
}

func nodeDelete(tree *BTree, node BPage, idx uint16, key []byte) BPage {
	kptr := node.getPtr(idx)
	updated := treeDelete(tree, tree.get(kptr), key)

	if len(updated) == 0 {
		return BPage{} // node not found
	}

	tree.del(kptr)
	newNode := BPage(make([]byte, BTREE_PAGE_SIZE))
	mergeDirection, sibiling := shouldMerge(tree, node, updated, idx)

	switch {
	case mergeDirection < 0:
		merged := BPage(make([]byte, BTREE_PAGE_SIZE))
		nodeMerge(merged, sibiling, updated)
		tree.del(node.getPtr(idx - 1))
		nodeReplace2Kid(newNode, node, idx-1, tree.new(merged), merged.getKey(0))

	case mergeDirection > 0:
		merged := BPage(make([]byte, BTREE_PAGE_SIZE))
		nodeMerge(merged, sibiling, updated)
		tree.del(node.getPtr(idx + 1))
		nodeReplace2Kid(newNode, node, idx, tree.new(merged), merged.getKey(0))
	case mergeDirection == 0 && updated.nkeys() == 0:
		assert(node.nkeys() == 1 && idx == 0, "nodeDelete: parent number of keys is more than 1 while merge direction is 0 on a merge call")
		newNode.setHeader(BNODE_INTERNAL_TYPE, 0) // at that point will add the dummy key back
	case mergeDirection == 0 && updated.nkeys() > 0:
		nodeReplaceKidN(tree, newNode, node, idx, updated)
	}

	return newNode
}

func (tree *BTree) Delete(key []byte) (bool, error) {
	if tree.root == 0 {
		return false, errors.New("btree is already empty")
	}

	updated := treeDelete(tree, tree.get(tree.root), key)

	if len(updated) == 0 {
		return false, nil
	}

	tree.del(tree.root)
	if updated.nkeys() == 0 {
		updated.setHeader(BNODE_LEAF_TYPE, 1)
		nodeAppendKV(updated, 0, 0, nil, nil)
	}
	tree.root = tree.new(updated)
	return true, nil
}

func treeGet(tree *BTree, node BPage, key []byte) (BPage, int) {
	if node.btye() == BNODE_LEAF_TYPE {
		idx := nodeLookupIndex(node, key)

		if idx < node.nkeys() &&
			bytes.Equal(node.getKey(idx), key) {
			return node, int(idx)
		}

		return BPage{}, -1
	}

	idx := navidageProperIndex(node, key)
	return treeGet(tree, tree.get(node.getPtr(idx)), key)
}

func (tree *BTree) Get(key []byte) ([]byte, bool) {
	if tree.root == 0 {
		return BPage{}, false
	}

	node, idx := treeGet(tree, tree.get(tree.root), key)
	if len(node) == 0 {
		return nil, false
	}

	return node.getVal(uint16(idx)), true
}

func fullScan(tree *BTree, node BPage, writer *bufio.Writer) {
	if node.btye() == BNODE_LEAF_TYPE {
		for i := range node.nkeys() {
			fmt.Fprintf(writer, "%s:%s", node.getKey(i), node.getVal(i))
		}
		return
	}

	for i := range node.nkeys() {
		fullScan(tree, tree.get(node.getPtr(i)), writer)
	}
}

func (tree *BTree) FullScan(w *bufio.Writer) {
	fullScan(tree, tree.get(tree.root), w)
}

type InMemoryBTree struct {
	Tree  BTree
	ref   map[string]string
	pages map[uint64]BPage
}

func NewInMemoryBTree() *InMemoryBTree {
	pages := map[uint64]BPage{}
	return &InMemoryBTree{
		Tree: BTree{
			get: func(u uint64) []byte {
				node, ok := pages[u]
				assert(ok, fmt.Sprintf("page of id:%d not found", u))
				return node
			},
			new: func(b []byte) uint64 {
				assert(BPage(b).btye() <= BTREE_PAGE_SIZE, "node size is bigger than page size")
				ptr := uint64(uintptr(unsafe.Pointer(&b[0])))
				assert(pages[ptr] == nil, "inserting failed, key,value already exist")
				pages[ptr] = b
				return ptr
			},
			del: func(u uint64) {
				assert(pages[u] != nil, "deleting failed, node already deleted")
				delete(pages, u)
			},
		},
		ref:   map[string]string{},
		pages: pages,
	}
}
