## ForgeDb

building a pet project database while studying database internals from `Alex Pertov Database internals` book, CMU ccourse and build your own x.
you will find the notes under `notes/` dir

currently you can just call the three commands it has, it reads from a csv file you provide and either scan given the key-values, instert or delete
Note: it is not concurrent safe yet

## Plan and What have been made so far

- [x] on-disk b+tree with a fixed key value page format
- [x] on-disk free list for reusable deleted nodes
- [x] kind of buffer pool using mmap with read + shared

## what i want to do next on the short term

- [ ] refactor buffer pool to use a LRU cache and abandon mmap (a dedicated buffer pool)
- [ ] compaction
- [ ] move to a more than a key value page format with dedicated columns (variable length) ( will need basically to refactor the current page format)
- [ ] latches and concurrent b+tree operations

## long term stuff idk when i will do it

- [ ] lock and transaction managers
- [ ] Query Parser and Executor
- [ ] Table and schema metadata (catalog)
- [ ] Row identifiers (RID)
- [ ] Column types
- [ ] NULL values
- [ ] Secondary indexes
- [ ] Table creation and deletion
- [ ] Benchmarking
