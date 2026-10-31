package storage

import (
	"encoding/csv"
	"io"
	"math/rand"
	"os"
	"strings"
	"testing"
)

type keyValue struct {
	key   string
	value string
}

const testDataPath = "random_key_value_pairs.csv"

func readKeyValues(t *testing.T, path string) []keyValue {
	t.Helper()

	fd, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	defer fd.Close()

	reader := csv.NewReader(fd)
	var records []keyValue

	for {
		row, err := reader.Read()

		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read CSV: %v", err)
		}
		if len(row) != 2 {
			t.Fatalf("expected 2 CSV fields, got %d: %v", len(row), row)
		}

		records = append(records, keyValue{
			key:   strings.TrimSpace(row[0]),
			value: strings.TrimSpace(row[1]),
		})
	}

	return records
}

func TestBtreeOperations(t *testing.T) {
	btree := NewInMemoryBTree()

	keyVals := map[string]string{
		"data-one":   "value-one",
		"data-two":   "value-two",
		"data-three": "value-three",
		"data-four":  "value-four",
		"data-five":  "value-five",
		"data-six":   "value-six",
	}

	btree.ref = keyVals

	t.Run("inserting and getting data", func(t *testing.T) {
		for k, v := range keyVals {
			if err := btree.Tree.Insert([]byte(k), []byte(v)); err != nil {
				t.Fatalf("failed to insert %s: %v", k, err)
			}

			got, found := btree.Tree.Get([]byte(k))
			if !found {
				t.Fatalf("key %s not found after insertion", k)
			}
			if string(got) != v {
				t.Fatalf("key %s: got %q, want %q", k, got, v)
			}
		}
	})

	t.Run("deleting data", func(t *testing.T) {
		for k := range keyVals {
			deleted, err := btree.Tree.Delete([]byte(k))
			if err != nil {
				t.Fatalf("failed to delete %s: %v", k, err)
			}
			if !deleted {
				t.Fatalf("key %s was not deleted", k)
			}

			_, found := btree.Tree.Get([]byte(k))
			if found {
				t.Fatalf("key %s still exists after deletion", k)
			}
		}
	})

	records := readKeyValues(t, testDataPath)

	t.Run("large insertion from file", func(t *testing.T) {
		for _, record := range records {
			if err := btree.Tree.Insert(
				[]byte(record.key),
				[]byte(record.value),
			); err != nil {
				t.Fatalf(
					"failed to insert %s:%s: %v",
					record.key, record.value, err,
				)
			}

			got, found := btree.Tree.Get([]byte(record.key))
			if !found {
				t.Fatalf("key %s not found after insertion", record.key)
			}
			if string(got) != record.value {
				t.Fatalf(
					"key %s: got %q, want %q",
					record.key, got, record.value,
				)
			}
		}
	})

	t.Run("deleting large input in shuffled order", func(t *testing.T) {
		keys := make([]string, len(records))
		for i, record := range records {
			keys[i] = record.key
		}

		// Fixed seed: the deletion order is shuffled but reproducible.
		rng := rand.New(rand.NewSource(42))
		rng.Shuffle(len(keys), func(i, j int) {
			keys[i], keys[j] = keys[j], keys[i]
		})

		for _, k := range keys {
			deleted, err := btree.Tree.Delete([]byte(k))
			if err != nil {
				t.Fatalf("failed to delete %s: %v", k, err)
			}
			if !deleted {
				t.Fatalf("key %s was not found during deletion", k)
			}

			_, found := btree.Tree.Get([]byte(k))
			if found {
				t.Fatalf("key %s still exists after deletion", k)
			}
		}
	})
}
