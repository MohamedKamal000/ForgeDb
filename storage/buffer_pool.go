package storage

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

/*
a buffer pool that uses mmap for now, we need to create our own later
*/

type BufferPool struct {
	total  int
	chunks [][]byte
}

func (bf *BufferPool) pageRead(ptr uint64) []byte {
	start := uint64(0)

	offset := ptr * BTREE_PAGE_SIZE

	if offset+BTREE_PAGE_SIZE > uint64(bf.total) {
		panic(fmt.Sprintf(
			"invalid page: ptr=%d offset=%d fileSize=%d",
			ptr, offset, bf.total,
		))
	}

	for _, chunk := range bf.chunks {
		end := start + uint64(len(chunk))/BTREE_PAGE_SIZE
		if ptr < end {
			offst := BTREE_PAGE_SIZE * (ptr - start)
			page := make([]byte, BTREE_PAGE_SIZE)
			copy(page, chunk[offst:offst+BTREE_PAGE_SIZE]) // we need a copy since we have mmap as a read-only, our writes are done without mmap

			return page
		}
		start = end
	}

	panic("bad ptr")
}

func (bf *BufferPool) extendIfNeeded(fd int, size int) error {
	if size <= bf.total {
		return nil
	}

	alloc := max(bf.total, 64<<20) // 64 * 2 ^ 20 => roughly around 64 MB
	for bf.total+alloc < size {
		alloc *= 2
	}

	chunk, err := syscall.Mmap(fd, int64(bf.total), alloc, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap: %w", err)
	}
	bf.total += alloc
	bf.chunks = append(bf.chunks, chunk)
	return nil
}

func (bf *BufferPool) writePage(fd int, offset int, page []byte) error {
	if _, err := unix.Pwrite(fd, page, int64(offset)); err != nil {
		return fmt.Errorf("error when writing a page: %w", err)
	}
	return nil
}

func (bf *BufferPool) writePages(fd int, lastFlushedOffst int, pages [][]byte) error {
	size := (lastFlushedOffst + len(pages)) * BTREE_PAGE_SIZE

	if err := bf.extendIfNeeded(fd, size); err != nil {
		return err
	}

	offstInSize := lastFlushedOffst * BTREE_PAGE_SIZE
	if _, err := unix.Pwritev(fd, pages, int64(offstInSize)); err != nil {
		return err
	}

	return nil
}
