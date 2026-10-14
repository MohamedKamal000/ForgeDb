package storage

import (
	"fmt"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestInsertKeyValues(t *testing.T) {
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
	t.Run("inserting data", func(t *testing.T) {
		for k, v := range keyVals {
			err := btree.Tree.Insert([]byte(k), []byte(v))
			tassert.NoErrorf(t, err, fmt.Sprintf("failed when inserting %s:%s", k, v))
			nodeVal, ok, _ := btree.Tree.Get([]byte(k))
			tassert.Truef(t, ok, "value %s:%s not found after inserting", k, v)
			tassert.Equalf(t, v, string(nodeVal), "value %s does not equaul to each other", string(nodeVal))
		}
	})
}
