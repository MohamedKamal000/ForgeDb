### page header:

```
| type | nkeys | reserved |  pointers  |  offsets   | key-values | unused |
|  2B  |   2B  |    12B   | nkeys × 8B | nkeys × 2B |     ...    |        |
```

- type => node type either leaf or internal
- nkeys => number of keys i have

Notes:

- the header is 16-bytes, will use the 12B reserved for later refactor and future work
- the pointers array is not used for leaf nodes

### cell format:

```
| key_size | val_size | reserved | key | val |
|    2B    |    2B    |    4B    | ... | ... |
```

Notes:

- i will not make overflown pages at the current stage, later.
- internal nodes the value of the size will be 0
- child pointers are paired with the keys, we don't have N + 1 pointers to avoid edge cases.
  meaning each key has a child pointer.

### constant values:

- page size => 4kb
- max key size => 1000B
- max val size => 3000B

- byte offset => page number (aka pointer) * page size
- data is written in little endian on disk
