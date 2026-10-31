package storage

import (
	_ "embed"
	"strconv"
	"strings"
	"testing"
)

const INITIAL_OFFSET_FOR_TESTING = 100

//go:embed random_ptrs_free_list_testing.txt
var testingValues string

func TestFreeListOperations(t *testing.T) {
	fl := NewInmemoryFreeList(INITIAL_OFFSET_FOR_TESTING, map[uint64][]byte{})

	t.Run("inserting some values", func(t *testing.T) {
		somePointers := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		for _, p := range somePointers {
			fl.PushList(p)
		}

		noMoreLeft := false
		currentIdx := 9
		for !noMoreLeft {
			ptr, ok := fl.PopList()

			if ptr != somePointers[currentIdx] {
				t.Fatalf("expected: %d, got: %d", somePointers[currentIdx], ptr)
			}

			noMoreLeft = ok != false
			currentIdx--
		}
	})

	t.Run("large file testing", func(t *testing.T) {
		testingValues = strings.Trim(testingValues, "\n")
		fatPointers := strings.Split(testingValues, ",")

		for _, p := range fatPointers {
			p_u, err := strconv.ParseUint(p, 10, 64)
			if err != nil {
				t.Fatalf("failed to parse pointer:%s, err: %v", p, err)
			}

			fl.PushList(p_u)
		}

		fatPointersMap := map[uint64]uint64{}
		for _, v := range fatPointers {
			parsed_p, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				t.Fatalf("failed to parse pointer:%s", v)
			}
			fatPointersMap[parsed_p] = parsed_p
		}

		for _, v := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
			fatPointersMap[uint64(v)] = uint64(v)
		}

		for i, v := range fatPointers {
			poped_ptr, ok := fl.PopList()

			if !ok {
				t.Fatalf("couldn't pop a pointer got false, index: %d, ptr: %s", i, v)
			}

			_, ok = fatPointersMap[poped_ptr]
			if !ok {
				t.Fatalf("popped pointer not in the map, pointer: %d, expected to be found: %s", poped_ptr, v)
			}
		}
	})
}
