package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"syscall"
)

const (
	DB_SIG                   = "ForgeDbSignature"
	DATABASE_DIR_HEADER_SIZE = 48
)

/*


|     the_meta_page    | pages... | root_node | pages... | (end_of_file)
| root_ptr | page_used |                ^                ^
      |          |                      |                |
      +----------|----------------------+                |
                 |                                       |
                 +---------------------------------------+

/*
meta_page format

| sig | root_ptr | page_used | freeList_page_header_ptr | freeList_page_tail_ptr |
| 16B |    8B    |     8B    |          8B				|          8B			 |

*/

type KV struct {
	Path string

	fd int

	tree BTree

	bufferpool BufferPool

	freeList FreeList

	nextOffset uint64

	inMemoryPages [][]byte // used for copy on write updates

	pendingUpdates map[uint64][]byte // used for in-place updates
}

func NewKV(filePath string) (error, *KV) {
	kv := KV{
		Path:           filePath,
		tree:           BTree{},
		bufferpool:     BufferPool{},
		freeList:       FreeList{},
		nextOffset:     0,
		inMemoryPages:  [][]byte{},
		pendingUpdates: map[uint64][]byte{},
	}

	return kv.Open(), &kv
}

func (db *KV) Open() error {
	db.freeList.new = db.pageAppend
	db.freeList.get = func(u uint64) []byte {
		if node, ok := db.pendingUpdates[u]; ok {
			return node
		}
		return db.bufferpool.pageRead(u)
	}
	db.freeList.updateNode = func(f FLNode, u uint64) {
		db.pendingUpdates[u] = f
	}
	db.tree.get = func(u uint64) []byte {
		if node, ok := db.pendingUpdates[u]; ok {
			return node
		}
		return db.bufferpool.pageRead(u)
	}
	db.tree.new = db.pageAlloc
	db.tree.del = db.pageDelete

	fd, err := createFile(db.Path)
	if err != nil {
		return err
	}
	db.fd = fd

	size, err := syscall.Seek(fd, 0, io.SeekEnd)
	if err != nil {
		return err
	}

	return db.readRoot(size)
}

func (db *KV) Close() error {
	return syscall.Close(db.fd)
}

func (db *KV) Get(key []byte) ([]byte, bool) {
	return db.tree.Get(key)
}

func (db *KV) Set(key, val []byte) error {
	err := db.tree.Insert(key, val)
	if err != nil {
		return err
	}
	return db.updateFile()
}

func (db *KV) Del(key []byte) (error, bool) {
	ok, err := db.tree.Delete(key)
	if err != nil {
		return err, false
	}

	return db.updateFile(), ok
}

func (db *KV) updateFile() error {
	err := db.bufferpool.writePages(db.fd, int(db.nextOffset), db.inMemoryPages)
	if err != nil {
		return err
	}

	if err = syscall.Fsync(db.fd); err != nil {
		return err
	}

	db.nextOffset += uint64(len(db.inMemoryPages))
	db.inMemoryPages = db.inMemoryPages[:0]

	for k, v := range db.pendingUpdates {
		offset := k * BTREE_PAGE_SIZE

		if err := db.bufferpool.writePage(db.fd, int(offset), v); err != nil {
			return err
		}

		delete(db.pendingUpdates, k)
	}

	if err = db.updateRoot(); err != nil {
		return err
	}

	return syscall.Fsync(db.fd)
}

func createFile(file string) (int, error) {
	flags := os.O_RDONLY | syscall.O_DIRECTORY
	dirp, err := syscall.Open(path.Dir(file), flags, 0o644)
	if err != nil {
		return -1, fmt.Errorf("oepn dir failed: %w", err)
	}

	flags = os.O_RDWR | os.O_CREATE
	fd, err := syscall.Openat(dirp, path.Base(file), flags, 0o644)
	if err != nil {
		return -1, fmt.Errorf("oepn file failed: %w", err)
	}

	if err = syscall.Fsync(fd); err != nil {
		_ = syscall.Close(fd)
		return -1, fmt.Errorf("failed to flush out dir info: %w", err)
	}

	return fd, nil
}

func (db *KV) saveMeta() []byte {
	data := BPage(make([]byte, BTREE_PAGE_SIZE))
	copy(data[:16], []byte(DB_SIG))
	binary.LittleEndian.PutUint64(data[16:], db.tree.root)
	binary.LittleEndian.PutUint64(data[24:], db.nextOffset)
	binary.LittleEndian.PutUint64(data[32:], db.freeList.headptr)
	binary.LittleEndian.PutUint64(data[40:], db.freeList.tailptr)
	return data[:]
}

func (db *KV) loadMeta(data []byte) error {
	Sig := make([]byte, 16)
	copy(Sig, data[:16])
	if string(Sig) != DB_SIG {
		return fmt.Errorf("metadata load error, sig does not match,got:%s", string(Sig))
	}
	db.tree.root = binary.LittleEndian.Uint64(data[16:])
	db.nextOffset = binary.LittleEndian.Uint64(data[24:])
	db.freeList.headptr = binary.LittleEndian.Uint64(data[32:])
	db.freeList.tailptr = binary.LittleEndian.Uint64(data[40:])
	return nil
}

func (db *KV) readRoot(fileSize int64) error {
	if fileSize == 0 {
		db.nextOffset = 2 // 0 for header, 1 for first free list node
		db.freeList.headptr = 1
		db.freeList.tailptr = 1
		initalFreeListNode := NewFLNode()
		initalFreeListNode.setNextNodePtr(0)
		initalFreeListNode.setNextEntryIndex(0)
		db.inMemoryPages = append(db.inMemoryPages, [][]byte{db.saveMeta(), initalFreeListNode}...)
		if err := db.bufferpool.writePages(db.fd, 0, db.inMemoryPages); err != nil {
			return err
		}
		db.inMemoryPages = db.inMemoryPages[:0]
		return syscall.Fsync(db.fd)
	}
	db.bufferpool.extendIfNeeded(db.fd, int(fileSize))
	data := db.bufferpool.chunks[0]
	return db.loadMeta(data)
}

func (db *KV) updateRoot() error {
	if _, err := syscall.Pwrite(db.fd, db.saveMeta(), 0); err != nil {
		return fmt.Errorf("write meta page: %w", err)
	}
	return nil
}

func (db *KV) pageAlloc(node []byte) uint64 {
	if reusableNode, ok := db.freeList.PopList(); ok {
		db.pendingUpdates[reusableNode] = node
		return reusableNode
	}
	return db.pageAppend(node)
}

func (db *KV) pageAppend(node []byte) uint64 {
	ptr := db.nextOffset + uint64(len(db.inMemoryPages))
	db.inMemoryPages = append(db.inMemoryPages, node)
	return ptr
}

func (db *KV) pageDelete(ptr uint64) {
	db.freeList.PushList(ptr)
}
