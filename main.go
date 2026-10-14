package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"forgedb/storage"
)

/*
	we are trying to achieve both:
	1- atomicity => we don't want to read a corrupted data (ex => half written file, empty file, corrupted data from crash)
	2- durability => data stays on disk and garunteed to stay on it, syscall fsync i guess

*/

func randomInt() int {
	return rand.Int()
}

/*
	the following is a read-write atomicity approach
	since we switch the file from tmp to actual one and avoid making the reader read half-written data


	it is not power-loss safe tho, doing this might require a log file similar to WAL in databases
*/

func SaveData(directory, path string, data []byte) error {
	tmp := fmt.Sprintf("%s-%d.tmp", path, randomInt())
	fp, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o664)
	var fd *os.File
	// making changes using rename in the directory requries a fsync call aswell, since it does changes to the metadata
	fd, err = os.Open(directory)
	if err != nil {
		return err
	}

	defer func() {
		fp.Close()
		fd.Close()
		if err != nil {
			os.Remove(tmp)
		}
	}()

	if _, err = fp.Write(data); err != nil {
		return err
	}

	if err = fp.Sync(); err != nil {
		return err
	}

	if err = os.Rename(tmp, path); err != nil {
		return err
	}

	if err != nil {
		return err
	}

	if err = fd.Sync(); err != nil {
		return err
	}

	return err
}

/*
Real-world OLTP queries fall into 1 of the 3 types:

Scan the whole data set without an index if the table is small.
Point query: Query the index by a single key.
Range query: Query a range of keys in sort order.
*/

func testBtree() {
	btree := storage.NewInMemoryBTree()
	fd, err := os.OpenFile("random_key_value_pairs.csv", os.O_RDONLY, 0o664)
	defer func() {
		err := fd.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	if err != nil {
		log.Fatal(err)
	}

	reader := bufio.NewReader(fd)
	for {
		row, err := reader.ReadString('\n')

		if err == io.EOF {
			fmt.Println("end of file")
			break
		}

		data := strings.Split(row, ",")
		k, v := data[0], data[1]
		btree.Tree.Insert([]byte(k), []byte(v))

		fmt.Printf("inserted => %s:%s", k, v)
	}

	time.Sleep(time.Second)

	fd2, err := os.OpenFile("random_key_value_pairs_rm.csv", os.O_RDONLY, 0o664)
	defer func() {
		err := fd2.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	if err != nil {
		log.Fatal(err)
	}
	anotherReader := bufio.NewReader(fd2)

	for {
		row, err := anotherReader.ReadString('\n')

		if err == io.EOF {
			fmt.Println("end of file")
			break
		}

		data := strings.Split(row, ",")
		k, _ := data[0], data[1]
		ok, err := btree.Tree.Delete([]byte(k))
		if err != nil {
			log.Fatal(err)
		}
		if !ok {
			fmt.Printf("couldn't delete: %s\n", k)
			break
		} else {
			fmt.Printf("deleted %s\n", k)
		}
	}
}

func main() {
	// var fileName string
	// var dataToWrite string
	// flag.StringVar(&fileName, "file", "default.txt", "")
	// flag.StringVar(&dataToWrite, "data", "default data", "")
	// flag.Parse()
	// currentDir, err := os.Getwd()
	// if err != nil {
	// 	panic(err)
	// }
	//
	// err = SaveData(currentDir, path.Join(currentDir, fileName), []byte(dataToWrite))
	// if err != nil {
	// 	panic(err)
	// }
	testBtree()
}
