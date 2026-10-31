package storage

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

/*
the main idea:
have a free list of free pages ids in the file to reuse
we append to it when deleting a page,
we pop from it when allocating a new node incase if we have some

page metadata we need to same in the metadata header of our database:
1- head,tail page id
2- current index of the head,tail page

problems we face:
1- what if we consumed all head ? => move it to next node
2- what if the list is empty ? => at this point the head and tail on same id and their seq equals zero, we return and allocate the next page in the db

for the tail i think there is no probelm we just append on it and allocate a new head if it reaches the cap


free list page format:

| next_pointer | current_idex_to_write_at | id entries |
|	  8B	   |		   4B			  |     ...    |


id entries => a 8B entriy holds page id, there can be only CAP = (PAGE_SIZE - 12) / 8 entries in the page
if this exceeds meaning, currIdx % cap is equal to zero, we allocate a new page

the current_index_to_write_at is between [1,CAP - 1]
if == CAP => allocate new one
if == 0, move to next node

the idx is always placed so that it is pointing to a ptr
it is checked after doing the adjustments to it either decrementing or incrementing
*/

const FLNODE_HEADER_SIZE = 12

type FLNode []byte

func NewFLNode() FLNode {
	return FLNode(make([]byte, BTREE_PAGE_SIZE))
}

func (fn FLNode) getNextNodePtr() uint64 {
	return binary.LittleEndian.Uint64(fn)
}

func (fn FLNode) setNextNodePtr(ptr uint64) {
	binary.LittleEndian.PutUint64(fn, ptr)
}

func (fn FLNode) getNextEntryIndex() uint32 {
	return binary.LittleEndian.Uint32(fn[8:])
}

func (fn FLNode) setNextEntryIndex(idx uint32) {
	binary.LittleEndian.PutUint32(fn[8:], idx)
}

func (fn FLNode) appendEntry(idx uint32, entry uint64) {
	binary.LittleEndian.PutUint64(fn[FLNODE_HEADER_SIZE+idx*8:], entry)
}

func (fn FLNode) readAt(idx uint32) uint64 {
	return binary.LittleEndian.Uint64(fn[FLNODE_HEADER_SIZE+idx*8:])
}

const ENTRIES_CAP = (BTREE_PAGE_SIZE - FLNODE_HEADER_SIZE) / 8

type FreeList struct {
	get        func(uint64) []byte
	new        func([]byte) uint64
	updateNode func(FLNode, uint64)
	headptr    uint64
	tailptr    uint64
}

func (fl *FreeList) PopList() (uint64, bool) {
	head := FLNode(fl.get(fl.headptr))
	idx := head.getNextEntryIndex()
	if idx == 0 && fl.tailptr == fl.headptr {
		return 0, false
	}

	idx--
	ptr := head.readAt(idx)
	if idx == 0 {
		nextPtr := head.getNextNodePtr()
		if nextPtr == 0 { // at this point it means we are at the last node, we popped the last element
			head.setNextEntryIndex(0)
			fl.updateNode(head, fl.headptr)
			return ptr, true
		}
		fl.headptr = nextPtr
		head = FLNode(fl.get(fl.headptr))
	} else {
		head.setNextEntryIndex(idx)
	}

	fl.updateNode(head, fl.headptr)
	return ptr, true
}

func (fl *FreeList) PushList(ptr uint64) {
	tail := FLNode(fl.get(fl.tailptr))
	idx := tail.getNextEntryIndex()
	if idx == ENTRIES_CAP {
		newFlNode := NewFLNode()
		newFlNode.setNextEntryIndex(1)
		newFlNode.setNextNodePtr(0)
		newFlNode.appendEntry(0, ptr)
		nextNodePtr := fl.new(newFlNode)
		tail.setNextNodePtr(nextNodePtr)
		fl.updateNode(tail, fl.tailptr)
		fl.tailptr = nextNodePtr
		fl.updateNode(newFlNode, nextNodePtr)
	} else {
		tail.appendEntry(idx, ptr)
		idx++
		tail.setNextEntryIndex(idx)
		fl.updateNode(tail, fl.tailptr)
	}
}

func NewInmemoryFreeList(initalOffset uint64, updatesMap map[uint64][]byte) FreeList {
	pages := map[uint64][]byte{}
	initalNode := NewFLNode()
	initalNode.setNextNodePtr(0)
	initalNode.setNextEntryIndex(0)
	pages[initalOffset] = initalNode
	return FreeList{
		get: func(u uint64) []byte {
			node, ok := pages[u]
			assert(ok, fmt.Sprintf("page of id:%d not found", u))
			return node
		},
		new: func(b []byte) uint64 {
			ptr := uint64(uintptr(unsafe.Pointer(&b[0])))
			assert(pages[ptr] == nil, "inserting failed, key,value already exist")
			pages[ptr] = b
			return ptr
		},
		updateNode: func(f FLNode, u uint64) {
			updatesMap[u] = f
		},
		headptr: initalOffset,
		tailptr: initalOffset,
	}
}
