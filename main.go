package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"forgedb/storage"
)

/*
Real-world OLTP queries fall into 1 of the 3 types:

Scan the whole data set without an index if the table is small.
Point query: Query the index by a single key.
Range query: Query a range of keys in sort order.
*/

var (
	operationFlag string
	fileName      string
)

type Record struct {
	key []byte
	val []byte
}

func NewRecord(r []string) Record {
	return Record{key: []byte(r[0]), val: []byte(r[1])}
}

func TestCall(callback func(record Record) error) error {
	fd, err := os.OpenFile(fileName, os.O_RDONLY, 0o664)
	if err != nil {
		return err
	}
	csvReader := csv.NewReader(bufio.NewReader(fd))
	for {
		r_s, err := csvReader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
		record := NewRecord(r_s)

		err = callback(record)
		if err != nil {
			return err
		}
	}
	return nil
}

func TryAdd(kv *storage.KV) {
	err := TestCall(func(record Record) error {
		return kv.Set(record.key, record.val)
	})
	if err != nil {
		log.Fatal(err)
	}
}

func TryScan(kv *storage.KV) {
	err := TestCall(func(record Record) error {
		vale, ok := kv.Get(record.key)
		if !ok {
			return fmt.Errorf("didn't find the value of key: %s", string(record.key))
		} else {
			fmt.Println(string(vale))
			return nil
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}

func TryDel(kv *storage.KV) {
	err := TestCall(func(record Record) error {
		err, ok := kv.Del(record.key)
		if !ok {
			return fmt.Errorf("failed to remove key: %s", string(record.key))
		}
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	err, kv := storage.NewKV("db.forge")
	defer kv.Close()
	if err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&operationFlag, "operation", "", "-operation [add|del|scan]")
	flag.StringVar(&fileName, "fileName", "random_key_value_pairs.csv", "-fileName <file name>")
	flag.Parse()

	switch operationFlag {
	case "add":
		TryAdd(kv)
	case "scan":
		TryScan(kv)
	case "del":
		TryDel(kv)
	default:
		fmt.Println("no valid operation")
	}
}
